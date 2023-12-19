package controller

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	virtv2 "github.com/deckhouse/virtualization-controller/api/v2alpha1"
	vmutil "github.com/deckhouse/virtualization-controller/pkg/common/vm"
	"github.com/deckhouse/virtualization-controller/pkg/controller/common"
	"github.com/deckhouse/virtualization-controller/pkg/controller/kvbuilder"
	"github.com/deckhouse/virtualization-controller/pkg/dvcr"
	"github.com/deckhouse/virtualization-controller/pkg/sdk/framework/helper"
	"github.com/deckhouse/virtualization-controller/pkg/sdk/framework/two_phase_reconciler"
)

type IPAM interface {
	IsBound(vmName string, claim *virtv2.VirtualMachineIPAddressClaim) bool
	CheckClaimAvailableForBinding(vmName string, claim *virtv2.VirtualMachineIPAddressClaim) error
	CreateIPAddressClaim(ctx context.Context, vm *virtv2.VirtualMachine, client client.Client) error
	BindIPAddressClaim(ctx context.Context, vmName string, claim *virtv2.VirtualMachineIPAddressClaim, client client.Client) error
	DeleteIPAddressClaim(ctx context.Context, claim *virtv2.VirtualMachineIPAddressClaim, client client.Client) error
}

type VMReconciler struct {
	dvcrSettings *dvcr.Settings
	ipam         IPAM
}

func (r *VMReconciler) SetupController(_ context.Context, mgr manager.Manager, ctr controller.Controller) error {
	if err := ctr.Watch(source.Kind(mgr.GetCache(), &virtv2.VirtualMachine{}), &handler.EnqueueRequestForObject{},
		predicate.Funcs{
			CreateFunc: func(e event.CreateEvent) bool { return true },
			DeleteFunc: func(e event.DeleteEvent) bool { return true },
			UpdateFunc: func(e event.UpdateEvent) bool { return true },
		},
	); err != nil {
		return fmt.Errorf("error setting watch on VM: %w", err)
	}

	if err := ctr.Watch(
		source.Kind(mgr.GetCache(), &virtv1.VirtualMachine{}),
		handler.EnqueueRequestForOwner(
			mgr.GetScheme(),
			mgr.GetRESTMapper(),
			&virtv2.VirtualMachine{},
			handler.OnlyControllerOwner(),
		),
	); err != nil {
		return fmt.Errorf("error setting watch on VirtualMachineInstance: %w", err)
	}

	return nil
}

func (r *VMReconciler) Sync(ctx context.Context, _ reconcile.Request, state *VMReconcilerState, opts two_phase_reconciler.ReconcilerOptions) error {
	if state.isDeletion() {
		return r.cleanupOnDeletion(ctx, state, opts)
	}

	// Set finalizer atomically using requeue.
	if controllerutil.AddFinalizer(state.VM.Changed(), virtv2.FinalizerVMCleanup) {
		state.SetReconcilerResult(&reconcile.Result{Requeue: true})
		return nil
	}

	// Ensure IP address claim.
	claimed, err := r.ensureIPAddressClaim(ctx, state, opts)
	if err != nil {
		return err
	}

	if !claimed {
		return nil
	}

	// Ensure live migration annotation.
	common.AddAnnotation(state.VM.Changed(), virtv1.AllowPodBridgeNetworkLiveMigrationAnnotation, "true")

	disksMessage := r.checkBlockDevicesSanity(state)
	if disksMessage != "" {
		state.SetStatusMessage(disksMessage)
		opts.Log.Error(fmt.Errorf("invalid disks: %s", disksMessage), "disks mismatch")
		return r.syncMetadata(ctx, state, opts)
	}

	actions, newKVVM, err := r.detectApplyChangeActions(state, opts)
	if err != nil {
		return err
	}

	// Delay changes propagation to KVVM until user approves them.
	if r.shouldWaitForApproval(state, opts, actions) {
		// Always update metadata for underlying kubevirt resources: set finalizers and propagate labels and annotations.
		return r.syncMetadata(ctx, state, opts)
	}

	// First set VM labels with attached devices names and requeue to go to the next step.
	if state.SetVMLabelsWithAttachedBlockDevices() {
		state.SetReconcilerResult(&reconcile.Result{Requeue: true})
		return nil
	}
	// Next set finalizers on attached devices.
	if err = state.SetFinalizersOnBlockDevices(ctx); err != nil {
		return fmt.Errorf("unable to add block devices finalizers: %w", err)
	}

	if state.BlockDevicesReady() {
		if state.KVVM == nil {
			err = r.createKVVM(ctx, state, opts)
			if err != nil {
				return err
			}
		} else {
			err = r.applyVMChangesToKVVM(ctx, state, opts, actions, newKVVM)
			if err != nil {
				return err
			}
		}
	} else {
		// Wait until block devices are ready.
		opts.Log.Info("Waiting for block devices to become available")
		opts.Recorder.Event(state.VM.Current(), corev1.EventTypeNormal, virtv2.ReasonVMWaitForBlockDevices, "Waiting for block devices to become available")
		state.SetReconcilerResult(&reconcile.Result{RequeueAfter: 2 * time.Second})
	}

	// Always update metadata for underlying kubevirt resources: set finalizers and propagate labels and annotations.
	return r.syncMetadata(ctx, state, opts)
}

func (r *VMReconciler) UpdateStatus(_ context.Context, _ reconcile.Request, state *VMReconcilerState, opts two_phase_reconciler.ReconcilerOptions) error {
	// Do nothing if object is being deleted as any update will lead to en error.
	if state.isDeletion() {
		return nil
	}

	opts.Log.Info("VMReconciler.UpdateStatus")

	state.VM.Changed().Status.Message = ""

	// Ensure IP address claim.
	if !r.ipam.IsBound(state.VM.Name().Name, state.IPAddressClaim) {
		state.VM.Changed().Status.Phase = virtv2.MachinePending

		switch {
		case state.IPAddressClaim != nil:
			err := r.ipam.CheckClaimAvailableForBinding(state.VM.Name().Name, state.IPAddressClaim)
			if err != nil {
				state.VM.Changed().Status.Message = err.Error()
			}
		case state.VM.Current().Spec.VirtualMachineIPAddressClaimName != "":
			state.VM.Changed().Status.Message = "Claim not found: waiting for the Claim"
		default:
			state.VM.Changed().Status.Message = "Claim not found: it may be in the process of being created"
		}

		return nil
	}

	state.VM.Changed().Status.IPAddressClaim = state.IPAddressClaim.Name
	state.VM.Changed().Status.IPAddress = state.IPAddressClaim.Spec.Address

	// Change previous state to new
	switch state.VM.Current().Status.Phase {
	case "":
		state.VM.Changed().Status.Phase = virtv2.MachinePending
		state.SetReconcilerResult(&reconcile.Result{Requeue: true})
	case virtv2.MachinePending:
		if state.KVVMI != nil {
			switch state.KVVMI.Status.Phase {
			case virtv1.Running:
				state.VM.Changed().Status.Phase = virtv2.MachineScheduling
				state.SetReconcilerResult(&reconcile.Result{Requeue: true})
			case virtv1.Scheduled, virtv1.Scheduling:
				state.VM.Changed().Status.Phase = virtv2.MachineScheduling
			}
		}
	case virtv2.MachineScheduling, virtv2.MachineTerminating:
		if state.KVVMI != nil && state.KVVMI.Status.Phase == virtv1.Running {
			state.VM.Changed().Status.Phase = virtv2.MachineRunning
		}
	case virtv2.MachineRunning:
		// VM restart is in progress.
		if state.KVVM != nil && state.KVVMI == nil {
			state.VM.Changed().Status.Phase = virtv2.MachineTerminating
		}
	case virtv2.MachineStopped:
	case virtv2.MachineFailed:
		state.VM.Changed().Status.Phase = virtv2.MachinePending
	}

	// Set fields after phase changed
	switch state.VM.Changed().Status.Phase {
	case virtv2.MachinePending:
	case virtv2.MachineScheduling:
		if errs := state.GetKVVMErrors(); len(errs) > 0 {
			state.VM.Changed().Status.Phase = virtv2.MachineFailed
			for _, err := range errs {
				opts.Log.Error(err, "KVVM failure", "kvvm", state.KVVM.Name)
			}
		}

	case virtv2.MachineRunning:
		if state.KVVMI != nil {
			state.VM.Changed().Status.GuestOSInfo = state.KVVMI.Status.GuestOSInfo
			state.VM.Changed().Status.NodeName = state.KVVMI.Status.NodeName

			for _, i := range state.KVVMI.Status.Interfaces {
				if i.Name == "default" {
					state.VM.Changed().Status.IPAddress = i.IP
					break
				}
			}

			for _, bd := range state.VM.Current().Spec.BlockDevices {
				if state.FindAttachedBlockDevice(bd) == nil {
					if abd := state.CreateAttachedBlockDevice(bd); abd != nil {
						state.VM.Changed().Status.BlockDevicesAttached = append(
							state.VM.Changed().Status.BlockDevicesAttached,
							*abd,
						)
					}
				}
			}
		}
	case virtv2.MachineTerminating:
		// Wait until restart completes.
		state.SetReconcilerResult(&reconcile.Result{RequeueAfter: 2 * time.Second})
	case virtv2.MachineStopped:
	case virtv2.MachineFailed:
	default:
		opts.Log.Error(fmt.Errorf("unexpected VM status phase %q, fallback to Pending", state.VM.Changed().Status.Phase), "")
		state.VM.Changed().Status.Phase = virtv2.MachinePending
	}

	state.VM.Changed().Status.Message = state.StatusMessage
	state.VM.Changed().Status.ChangeID = state.ChangeID
	return nil
}

func (r *VMReconciler) ensureIPAddressClaim(ctx context.Context, state *VMReconcilerState, opts two_phase_reconciler.ReconcilerOptions) (bool, error) {
	// 1. OK: already bound.
	if r.ipam.IsBound(state.VM.Name().Name, state.IPAddressClaim) {
		return true, nil
	}

	// 2. Claim not found: create if possible or wait for the claim.
	if state.IPAddressClaim == nil {
		if state.VM.Current().Spec.VirtualMachineIPAddressClaimName != "" {
			opts.Log.Info("Claim not found: waiting for the Claim", "claimName", state.VM.Current().Spec.VirtualMachineIPAddressClaimName)
			state.SetReconcilerResult(&reconcile.Result{RequeueAfter: 2 * time.Second})

			return false, nil
		}

		opts.Log.Info("Claim not found: create the new one", "claimName", state.VM.Name().Name)
		state.SetReconcilerResult(&reconcile.Result{Requeue: true})

		return false, r.ipam.CreateIPAddressClaim(ctx, state.VM.Current(), opts.Client)
	}

	// 3. Check if possible to bind virtual machine with the found claim.
	err := r.ipam.CheckClaimAvailableForBinding(state.VM.Name().Name, state.IPAddressClaim)
	if err != nil {
		opts.Log.Info("Claim is not available to be bound", "err", err, "claimName", state.VM.Current().Spec.VirtualMachineIPAddressClaimName)
		opts.Recorder.Event(state.VM.Current(), corev1.EventTypeWarning, virtv2.ReasonClaimNotAvailable, err.Error())

		return false, nil
	}

	// 4. Claim exists and available for binding with virtual machine: set binding.
	opts.Log.Info("Bind VM with Claim and requeue request", "claimName", state.VM.Current().Spec.VirtualMachineIPAddressClaimName)
	state.SetReconcilerResult(&reconcile.Result{Requeue: true})

	return false, r.ipam.BindIPAddressClaim(ctx, state.VM.Name().Name, state.IPAddressClaim, opts.Client)
}

func (r *VMReconciler) ShouldDeleteChildResources(state *VMReconcilerState) bool {
	return state.KVVM != nil ||
		(state.IPAddressClaim != nil && r.isIPAddressClaimImplicit(state.IPAddressClaim))
}

func (r *VMReconciler) cleanupOnDeletion(ctx context.Context, state *VMReconcilerState, opts two_phase_reconciler.ReconcilerOptions) error {
	// The object is being deleted
	opts.Log.V(1).Info("Delete VM, remove protective finalizers")
	if err := r.removeFinalizerChildResources(ctx, state, opts); err != nil {
		return err
	}
	if r.ShouldDeleteChildResources(state) {
		// IP address is implicitly linked to the virtual machine: it needs to be deleted when deleting the virtual machine.
		if state.IPAddressClaim != nil && state.IPAddressClaim.DeletionTimestamp == nil && r.isIPAddressClaimImplicit(state.IPAddressClaim) {
			err := r.ipam.DeleteIPAddressClaim(ctx, state.IPAddressClaim, opts.Client)
			if err != nil && !k8serrors.IsNotFound(err) {
				return err
			}
		}
		if state.KVVM != nil {
			if err := helper.DeleteObject(ctx, opts.Client, state.KVVM); err != nil {
				return err
			}
		}
		requeueAfter := 30 * time.Second
		if p := state.VM.Current().Spec.TerminationGracePeriodSeconds; p != nil {
			newRequeueAfter := time.Duration(*p) * time.Second
			if requeueAfter > newRequeueAfter {
				requeueAfter = newRequeueAfter
			}
		}
		state.SetReconcilerResult(&reconcile.Result{RequeueAfter: requeueAfter})
		return nil
	}
	controllerutil.RemoveFinalizer(state.VM.Changed(), virtv2.FinalizerVMCleanup)
	// Stop reconciliation as the item is being deleted
	return nil
}

// checkBlockDevicesSanity compares spec.blockDevices and status.blockDevicesAttached.
// It returns false if the same disk contains in both arrays.
// It is a precaution to not apply changes in spec.blockDevices if disk is already
// hotplugged using the VMBDA resource. The reverse check is done by the vmbda-controller.
func (r *VMReconciler) checkBlockDevicesSanity(state *VMReconcilerState) string {
	disks := make([]string, 0)
	hotplugged := make(map[string]struct{})

	for _, bda := range state.VM.Current().Status.BlockDevicesAttached {
		if bda.Hotpluggable && bda.VirtualMachineDisk != nil {
			hotplugged[bda.VirtualMachineDisk.Name] = struct{}{}
		}
	}

	for _, bd := range state.VM.Current().Spec.BlockDevices {
		disk := bd.VirtualMachineDisk
		if disk != nil {
			if _, ok := hotplugged[disk.Name]; ok {
				disks = append(disks, disk.Name)
			}
		}
	}

	if len(disks) == 0 {
		return ""
	}

	return fmt.Sprintf("spec.blockDevices contain hotplugged disks: %s. Unplug or remove them from spec to continue.", strings.Join(disks, ", "))
}

// createKVVM constructs and creates new KubeVirt VirtualMachine based on d8 VirtualMachine spec.
func (r *VMReconciler) createKVVM(ctx context.Context, state *VMReconcilerState, opts two_phase_reconciler.ReconcilerOptions) error {
	kvvmName := state.VM.Name()

	// No underlying VM found, create fresh kubevirt VirtualMachine resource from d8 VirtualMachine spec.
	kvvmBuilder := kvbuilder.NewEmptyKVVM(kvvmName, kvbuilder.KVVMOptions{
		EnableParavirtualization:  state.VM.Current().Spec.EnableParavirtualization,
		OsType:                    state.VM.Current().Spec.OsType,
		ForceBridgeNetworkBinding: os.Getenv("FORCE_BRIDGE_NETWORK_BINDING") == "1",
		DisableHypervSyNIC:        os.Getenv("DISABLE_HYPERV_SYNIC") == "1",
	})
	kvbuilder.ApplyVirtualMachineSpec(kvvmBuilder, state.VM.Current(), state.VMDByName, state.VMIByName, state.CVMIByName, r.dvcrSettings, state.IPAddressClaim.Spec.Address)

	kvvm := kvvmBuilder.GetResource()

	if err := opts.Client.Create(ctx, kvvm); err != nil {
		return fmt.Errorf("unable to create KubeVirt VM %q: %w", kvvmName, err)
	}

	state.KVVM = kvvm

	opts.Log.Info("Created new KubeVirt VM", "name", kvvm.Name, "kvvm", state.KVVM)

	return nil
}

// detectApplyChangeActions compares KVVM generated from current VM spec with in cluster KVVM
// to calculate changes and action needed to apply these changes.
func (r *VMReconciler) detectApplyChangeActions(state *VMReconcilerState, opts two_phase_reconciler.ReconcilerOptions) (*kvbuilder.ChangeApplyActions, *virtv1.VirtualMachine, error) {
	// Not applicable if KVVM is absent.
	if state.KVVM == nil {
		return nil, nil, nil
	}

	// Save current KVVM.
	currKVVM := &kvbuilder.KVVM{}
	currKVVM.Resource = state.KVVM.DeepCopy()
	// Update underlying kubevirt VirtualMachine resource from updated d8 VirtualMachine spec.
	// FIXME(VM): This will be changed for effective-spec logic implementation
	kvvmBuilder := kvbuilder.NewKVVM(state.KVVM.DeepCopy(), kvbuilder.KVVMOptions{
		EnableParavirtualization:  state.VM.Current().Spec.EnableParavirtualization,
		OsType:                    state.VM.Current().Spec.OsType,
		ForceBridgeNetworkBinding: os.Getenv("FORCE_BRIDGE_NETWORK_BINDING") == "1",
		DisableHypervSyNIC:        os.Getenv("DISABLE_HYPERV_SYNIC") == "1",
	})
	kvbuilder.ApplyVirtualMachineSpec(kvvmBuilder, state.VM.Current(), state.VMDByName, state.VMIByName, state.CVMIByName, r.dvcrSettings, state.IPAddressClaim.Spec.Address)

	// Compare current version of the underlying KVVM
	// with newly generated from VM spec. Perform action if needed to apply
	// changes to underlying virtual machine instance.

	actions, err := kvbuilder.CompareKVVM(currKVVM, kvvmBuilder)
	if err != nil {
		opts.Log.Error(err, "Detect action to apply changes failed", "vm.name", state.VM.Current().GetName())
		return nil, nil, fmt.Errorf("detect action to apply changes on vm/%s failed: %w", state.VM.Current().GetName(), err)
	}

	return actions, kvvmBuilder.GetResource(), nil
}

// shouldWaitForApproval returns true if disruptive update was not approved yet.
func (r *VMReconciler) shouldWaitForApproval(state *VMReconcilerState, opts two_phase_reconciler.ReconcilerOptions, actions *kvbuilder.ChangeApplyActions) bool {
	// Should not wait in Automatic mode.
	if vmutil.ApprovalMode(state.VM.Changed()) == virtv2.Automatic {
		return false
	}

	// Should not wait if no changes detected or if changes are non-disruptive.
	if actions.IsEmpty() || !actions.IsDisruptive() {
		state.SetChangeID("")
		return false
	}
	// Wait for Manual approval.
	// Always set status message when in approval wait mode.
	statusMessage := ""
	if actions.ActionType() == kvbuilder.ActionRestart {
		statusMessage = "VM restart required to apply changes. Check status.changeID and add spec.approvedChangeID to restart VM."
	} else {
		// Non restart changes, e.g. subresource signaling.
		statusMessage = "Approval required to apply changes. Check status.changeID and add spec.approvedChangeID to change VM."
	}
	state.SetStatusMessage(statusMessage)

	changeID := actions.ChangeID()
	currChangeID := state.VM.Current().Status.ChangeID
	// Save or update Change ID into annotation and wait for approval.
	if currChangeID == "" || currChangeID != changeID {
		state.SetChangeID(changeID)

		opts.Log.V(2).Info("Change ID updated", "actions", actions)
		state.SetReconcilerResult(&reconcile.Result{Requeue: true})
		return true
	}

	// Change ID is matched to changes, check approval.
	approveChangeID := state.VM.Current().Spec.ApprovedChangeID
	if approveChangeID == "" {
		// Change not approved yet, do nothing, wait for the next update.
		return true
	}
	// Change IDs are not equal: approved Change ID was expired. Record event and wait for the next update.
	if currChangeID != approveChangeID {
		opts.Recorder.Event(state.VM.Current(), corev1.EventTypeWarning, virtv2.ReasonVMChangeIDExpired, "Approved Change ID is expired, check VM spec and update approve annotation with the latest Change ID.")
		opts.Log.Info("Got Change ID approve with expired Change ID", "vm.name", state.VM.Name(), "curr-id", currChangeID, "approved-id", approveChangeID)
		return true
	}

	// Changes approved: change IDs become equal. Stop waiting.
	opts.Recorder.Event(state.VM.Current(), corev1.EventTypeNormal, virtv2.ReasonVMChangeIDApproveAccepted, "Approved Change ID accepted, apply changes.")
	// Reset status message.
	state.SetStatusMessage("")
	return false
}

// applyVMChangesToKVVM applies updates to underlying KVVM based on actions type.
func (r *VMReconciler) applyVMChangesToKVVM(ctx context.Context, state *VMReconcilerState, opts two_phase_reconciler.ReconcilerOptions, actions *kvbuilder.ChangeApplyActions, newKVVM *virtv1.VirtualMachine) error {
	kvvmName := state.VM.Name()

	switch actions.ActionType() {
	case kvbuilder.ActionRestart:
		opts.Log.Info("Restart VM to apply changes", "vm.name", state.VM.Current().GetName(), "action", actions)

		message := fmt.Sprintf("Apply changes with restart: %s", strings.Join(actions.GetChangesTitles(), ", "))
		opts.Recorder.Event(state.VM.Current(), corev1.EventTypeNormal, virtv2.ReasonVMChangesApplied, message)
		opts.Recorder.Event(state.VM.Current(), corev1.EventTypeNormal, virtv2.ReasonVMRestarted, "")

		if err := r.applyChangesWithRestart(ctx, state, opts, newKVVM); err != nil {
			return fmt.Errorf("unable to apply restart KVVM instance action: %w", err)
		}
	case kvbuilder.ActionSubresourceSignal:
		// TODO implement
	case kvbuilder.ActionApplyImmediate:
		opts.Log.Info("Apply changes without restart", "vm.name", state.VM.Current().GetName(), "action", actions)
		message := fmt.Sprintf("Apply changes without restart: %s", strings.Join(actions.GetChangesTitles(), ", "))
		opts.Recorder.Event(state.VM.Current(), corev1.EventTypeNormal, virtv2.ReasonVMChangesApplied, message)
		if err := opts.Client.Update(ctx, newKVVM); err != nil {
			return fmt.Errorf("unable to update KubeVirt VM %q: %w", kvvmName, err)
		}
		state.KVVM = newKVVM
	case kvbuilder.ActionNone:
		opts.Log.V(2).Info("No changes to underlying KVVM", "vm.name", state.VM.Current().GetName())
	}

	// Cleanup: remove Change ID related status after applying changes.
	state.SetChangeID("")
	return nil
}

// applyChangesWithRestart updates underlying KVVM and deletes KVVMI to restart VM.
func (r *VMReconciler) applyChangesWithRestart(ctx context.Context, state *VMReconcilerState, opts two_phase_reconciler.ReconcilerOptions, newKVVM *virtv1.VirtualMachine) error {
	if err := opts.Client.Update(ctx, newKVVM); err != nil {
		return fmt.Errorf("unable to update KubeVirt VM %q: %w", newKVVM.Name, err)
	}
	state.KVVM = newKVVM

	if err := opts.Client.Delete(ctx, state.KVVMI); err != nil {
		return fmt.Errorf("unable to remove current KubeVirt VMI %q: %w", state.KVVMI.Name, err)
	}
	state.KVVMI = nil
	// Also reset kubevirt Pods to prevent mismatch version errors on metadata update.
	state.KVPods = nil

	return nil
}

// syncMetadata propagates labels and annotations from VM to underlying objects and sets a finalizer on the KVVM.
func (r *VMReconciler) syncMetadata(ctx context.Context, state *VMReconcilerState, opts two_phase_reconciler.ReconcilerOptions) error {
	// Ensure kubevirt VM has finalizer in case d8 VM was created manually (use case: take ownership of already existing object).
	if state.KVVM != nil {
		// Propagate user specified labels and annotations from the d8 VM to kubevirt VM.
		metaUpdated, err := PropagateVMMetadata(state.VM.Current(), state.KVVM)
		if err != nil {
			return err
		}

		finalizerUpdated := controllerutil.AddFinalizer(state.KVVM, virtv2.FinalizerKVVMProtection)

		if metaUpdated || finalizerUpdated {
			if err = opts.Client.Update(ctx, state.KVVM); err != nil {
				return fmt.Errorf("error setting finalizer on a KubeVirt VM %q: %w", state.KVVM.Name, err)
			}
		}
	}

	// Propagate user specified labels and annotations from the d8 VM to the kubevirt VirtualMachineInstance.
	if state.KVVMI != nil {
		metaUpdated, err := PropagateVMMetadata(state.VM.Current(), state.KVVMI)
		if err != nil {
			return err
		}

		if metaUpdated {
			if err = opts.Client.Update(ctx, state.KVVMI); err != nil {
				return fmt.Errorf("unable to update KubeVirt VMI %q: %w", state.KVVMI.GetName(), err)
			}
		}
	}

	// Propagate user specified labels and annotations from the d8 VM to the kubevirt virtual machine Pods.
	if state.KVPods != nil {
		for _, pod := range state.KVPods.Items {
			// Update only Running pods.
			if pod.Status.Phase != corev1.PodRunning {
				continue
			}
			metaUpdated, err := PropagateVMMetadata(state.VM.Current(), &pod)
			if err != nil {
				return err
			}

			if metaUpdated {
				if err = opts.Client.Update(ctx, &pod); err != nil {
					return fmt.Errorf("unable to update KubeVirt Pod %q: %w", pod.GetName(), err)
				}
			}
		}
	}

	err := SetLastPropagatedLabels(state.VM.Changed())
	if err != nil {
		return err
	}

	err = SetLastPropagatedAnnotations(state.VM.Changed())
	if err != nil {
		return err
	}

	return nil
}

func (r *VMReconciler) isIPAddressClaimImplicit(claim *virtv2.VirtualMachineIPAddressClaim) bool {
	return claim.Labels[common.LabelImplicitIPAddressClaim] == common.LabelImplicitIPAddressClaimValue
}

// removeFinalizerChildResources removes protective finalizers on KVVM, Ip
func (r *VMReconciler) removeFinalizerChildResources(ctx context.Context, state *VMReconcilerState, opts two_phase_reconciler.ReconcilerOptions) error {
	if state.KVVM != nil && controllerutil.RemoveFinalizer(state.KVVM, virtv2.FinalizerKVVMProtection) {
		if err := opts.Client.Update(ctx, state.KVVM); err != nil {
			return fmt.Errorf("unable to remove KubeVirt VM %q finalizer %q: %w", state.KVVM.Name, virtv2.FinalizerKVVMProtection, err)
		}
	}
	return nil
}
