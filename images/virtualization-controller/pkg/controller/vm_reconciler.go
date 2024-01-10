package controller

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
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
	"github.com/deckhouse/virtualization-controller/pkg/controller/kvbuilder"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmchange"
	"github.com/deckhouse/virtualization-controller/pkg/dvcr"
	"github.com/deckhouse/virtualization-controller/pkg/sdk/framework/helper"
	"github.com/deckhouse/virtualization-controller/pkg/sdk/framework/two_phase_reconciler"
)

type IPAM interface {
	IsBound(vmName string, claim *virtv2.VirtualMachineIPAddressClaim) bool
	CheckClaimAvailableForBinding(vmName string, claim *virtv2.VirtualMachineIPAddressClaim) error
	CreateIPAddressClaim(ctx context.Context, vm *virtv2.VirtualMachine, client client.Client) error
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

	disksMessage := r.checkBlockDevicesSanity(state)
	if disksMessage != "" {
		state.SetStatusMessage(disksMessage)
		opts.Log.Error(fmt.Errorf("invalid disks: %s", disksMessage), "disks mismatch")
		return r.syncMetadata(ctx, state, opts)
	}

	if !state.BlockDevicesReady() {
		// Wait until block devices are ready.
		opts.Log.Info("Waiting for block devices to become available")
		opts.Recorder.Event(state.VM.Current(), corev1.EventTypeNormal, virtv2.ReasonVMWaitForBlockDevices, "Waiting for block devices to become available")
		state.SetReconcilerResult(&reconcile.Result{RequeueAfter: 2 * time.Second})
		// Always update metadata for underlying kubevirt resources: set finalizers and propagate labels and annotations.
		return r.syncMetadata(ctx, state, opts)
	}

	changes := r.detectSpecChanges(state, opts)

	// Delay changes propagation to KVVM until user approves them.
	if r.shouldWaitForChangesApproval(state, opts, changes) {
		// Always update metadata for underlying kubevirt resources: set finalizers and propagate labels and annotations.
		return r.syncMetadata(ctx, state, opts)
	}

	// Next set finalizers on attached devices.
	if err = state.SetFinalizersOnBlockDevices(ctx); err != nil {
		return fmt.Errorf("unable to add block devices finalizers: %w", err)
	}

	if state.KVVM == nil {
		err = r.createKVVM(ctx, state, opts)
		if err != nil {
			return err
		}
	} else {
		err = r.applyVMChangesToKVVM(ctx, state, opts, changes)
		if err != nil {
			return err
		}
	}

	// Always update metadata for underlying kubevirt resources: set finalizers and propagate labels and annotations.
	return r.syncMetadata(ctx, state, opts)
}

func (r *VMReconciler) UpdateStatus(_ context.Context, _ reconcile.Request, state *VMReconcilerState, opts two_phase_reconciler.ReconcilerOptions) error {
	opts.Log.V(2).Info("VMReconciler.UpdateStatus")
	if state.isDeletion() {
		state.VM.Changed().Status.Phase = virtv2.MachineTerminating
		return nil
	}

	state.VM.Changed().Status.Message = ""
	if state.VM.Current().Status.Phase == "" {
		state.VM.Current().Status.Phase = virtv2.MachinePending
	}

	// Ensure IP address claim.
	if !r.ipam.IsBound(state.VM.Name().Name, state.IPAddressClaim) {
		state.VM.Changed().Status.Phase = virtv2.MachinePending
		state.VM.Changed().Status.Message = "Waiting for IPAddressClaim to become available"
		return nil
	}

	state.VM.Changed().Status.IPAddressClaim = state.IPAddressClaim.Name
	state.VM.Changed().Status.IPAddress = state.IPAddressClaim.Spec.Address

	if !state.BlockDevicesReady() {
		state.VM.Changed().Status.Phase = virtv2.MachinePending
		state.VM.Changed().Status.Message = "Waiting for block devices to become available"
		return nil
	}

	switch {
	case state.vmIsPending():
		state.VM.Changed().Status.Phase = virtv2.MachinePending
	case state.vmIsStopping():
		state.VM.Changed().Status.Phase = virtv2.MachineStopping
	case state.vmIsStopped():
		state.VM.Changed().Status.Phase = virtv2.MachineStopped
	case state.vmIsScheduling():
		state.VM.Changed().Status.Phase = virtv2.MachineScheduling
	case state.vmIsStarting():
		state.VM.Changed().Status.Phase = virtv2.MachineStarting
	case state.vmIsRunning():
		state.VM.Changed().Status.Phase = virtv2.MachineRunning
		state.VM.Changed().Status.GuestOSInfo = state.KVVMI.Status.GuestOSInfo
		state.VM.Changed().Status.NodeName = state.KVVMI.Status.NodeName
		for _, i := range state.KVVMI.Status.Interfaces {
			if i.Name == "default" {
				if state.IPAddressClaim.Spec.Address != i.IP {
					err := fmt.Errorf("allocated ip address (%s) is not equeal to assigned (%s)", state.IPAddressClaim.Spec.Address, i.IP)
					opts.Log.Error(err, "Unexpected kubevirt virtual machine ip address for default network interface, please report a bug", "kvvm", state.KVVM.Name)
				}
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
	case state.vmIsMigrating():
		state.VM.Changed().Status.Phase = virtv2.MachineMigrating
	case state.vmIsPaused():
		state.VM.Changed().Status.Phase = virtv2.MachinePause
	case state.vmIsFailed():
		state.VM.Changed().Status.Phase = virtv2.MachineFailed
		opts.Log.Error(errors.New(string(state.KVVM.Status.PrintableStatus)), "KVVM failure", "kvvm", state.KVVM.Name)
	default:
		opts.Log.Error(fmt.Errorf("unexpected VM status phase %q, fallback to Pending", state.VM.Changed().Status.Phase), "")
		state.VM.Changed().Status.Phase = virtv2.MachinePending
	}

	state.VM.Changed().Status.Message = state.StatusMessage
	state.VM.Changed().Status.ChangeID = state.ChangeID
	state.VM.Changed().Status.PendingChanges = state.PendingChanges
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
			opts.Log.Info(fmt.Sprintf("The requested ip address claim (%s) for the virtual machine not found: waiting for the Claim", state.VM.Current().Spec.VirtualMachineIPAddressClaimName))
			state.SetStatusMessage(fmt.Sprintf("The requested ip address claim (%s) for the virtual machine not found: waiting for the Claim", state.VM.Current().Spec.VirtualMachineIPAddressClaimName))
			state.SetReconcilerResult(&reconcile.Result{RequeueAfter: 2 * time.Second})

			return false, nil
		}

		opts.Log.Info("Claim not found: create the new one", "claimName", state.VM.Name().Name)
		state.SetStatusMessage("Claim not found: it may be in the process of being created")
		state.SetReconcilerResult(&reconcile.Result{Requeue: true})

		return false, r.ipam.CreateIPAddressClaim(ctx, state.VM.Current(), opts.Client)
	}

	// 3. Check if possible to bind virtual machine with the found claim.
	err := r.ipam.CheckClaimAvailableForBinding(state.VM.Name().Name, state.IPAddressClaim)
	if err != nil {
		opts.Log.Info("Claim is not available to be bound", "err", err, "claimName", state.VM.Current().Spec.VirtualMachineIPAddressClaimName)
		state.SetStatusMessage(err.Error())
		opts.Recorder.Event(state.VM.Current(), corev1.EventTypeWarning, virtv2.ReasonClaimNotAvailable, err.Error())

		return false, nil
	}

	// 4. Claim exists and available for binding with virtual machine: waiting for the claim.
	opts.Log.Info("Waiting for the Claim to be bound to VM", "claimName", state.VM.Current().Spec.VirtualMachineIPAddressClaimName)
	state.SetStatusMessage("Claim not bound: waiting for the Claim")
	state.SetReconcilerResult(&reconcile.Result{RequeueAfter: 2 * time.Second})

	return false, nil
}

func (r *VMReconciler) ShouldDeleteChildResources(state *VMReconcilerState) bool {
	return state.KVVM != nil
}

func (r *VMReconciler) cleanupOnDeletion(ctx context.Context, state *VMReconcilerState, opts two_phase_reconciler.ReconcilerOptions) error {
	// The object is being deleted
	opts.Log.V(1).Info("Delete VM, remove protective finalizers")
	if err := r.removeFinalizerChildResources(ctx, state, opts); err != nil {
		return err
	}
	if r.ShouldDeleteChildResources(state) {
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

func (r *VMReconciler) makeKVVMFromVMSpec(state *VMReconcilerState) (*virtv1.VirtualMachine, error) {
	kvvmName := state.VM.Name()

	kvvmOpts := kvbuilder.KVVMOptions{
		EnableParavirtualization:  state.VM.Current().Spec.EnableParavirtualization,
		OsType:                    state.VM.Current().Spec.OsType,
		ForceBridgeNetworkBinding: os.Getenv("FORCE_BRIDGE_NETWORK_BINDING") == "1",
		DisableHypervSyNIC:        os.Getenv("DISABLE_HYPERV_SYNIC") == "1",
	}

	var kvvmBuilder *kvbuilder.KVVM
	if state.KVVM == nil {
		kvvmBuilder = kvbuilder.NewEmptyKVVM(kvvmName, kvvmOpts)
	} else {
		kvvmBuilder = kvbuilder.NewKVVM(state.KVVM.DeepCopy(), kvvmOpts)
	}

	// Create kubevirt VirtualMachine resource from d8 VirtualMachine spec.
	kvbuilder.ApplyVirtualMachineSpec(kvvmBuilder, state.VM.Current(), state.VMDByName, state.VMIByName, state.CVMIByName, r.dvcrSettings, state.IPAddressClaim.Spec.Address)

	kvvm := kvvmBuilder.GetResource()

	err := kvbuilder.SetLastAppliedSpec(kvvm, state.VM.Current())
	if err != nil {
		return nil, fmt.Errorf("set last applied spec on KubeVirt VM '%s': %w", state.KVVM.GetName(), err)
	}

	return kvvm, nil
}

// createKVVM constructs and creates new KubeVirt VirtualMachine based on d8 VirtualMachine spec.
func (r *VMReconciler) createKVVM(ctx context.Context, state *VMReconcilerState, opts two_phase_reconciler.ReconcilerOptions) error {
	kvvm, err := r.makeKVVMFromVMSpec(state)
	if err != nil {
		return fmt.Errorf("prepare to create KubeVirt VM '%s': %w", kvvm.GetName(), err)
	}

	if err := opts.Client.Create(ctx, kvvm); err != nil {
		return fmt.Errorf("unable to create KubeVirt VM '%s': %w", kvvm.GetName(), err)
	}

	state.KVVM = kvvm

	opts.Log.Info("Created new KubeVirt VM", "name", kvvm.Name)
	opts.Log.V(4).Info("Created new KubeVirt VM", "name", kvvm.Name, "kvvm", state.KVVM)

	return nil
}

// updateKVVM constructs and creates new KubeVirt VirtualMachine based on d8 VirtualMachine spec.
func (r *VMReconciler) updateKVVM(ctx context.Context, state *VMReconcilerState, opts two_phase_reconciler.ReconcilerOptions) error {
	kvvm, err := r.makeKVVMFromVMSpec(state)
	if err != nil {
		return fmt.Errorf("prepare to update KubeVirt VM '%s': %w", kvvm.GetName(), err)
	}

	if err := opts.Client.Update(ctx, kvvm); err != nil {
		return fmt.Errorf("unable to create KubeVirt VM '%s': %w", kvvm.GetName(), err)
	}

	state.KVVM = kvvm

	opts.Log.Info("Update KubeVirt VM done", "name", kvvm.Name)
	opts.Log.V(4).Info("Update KubeVirt VM done", "name", kvvm.Name, "kvvm", state.KVVM)

	return nil
}

// updateKVVMLastUsedSpec updates last-applied-spec annotation on KubeVirt VirtualMachine.
func (r *VMReconciler) updateKVVMLastUsedSpec(ctx context.Context, state *VMReconcilerState, opts two_phase_reconciler.ReconcilerOptions) error {
	if state.KVVM == nil {
		return nil
	}

	err := kvbuilder.SetLastAppliedSpec(state.KVVM, state.VM.Current())
	if err != nil {
		return fmt.Errorf("set last applied spec on KubeVirt VM '%s': %w", state.KVVM.GetName(), err)
	}

	if err := opts.Client.Update(ctx, state.KVVM); err != nil {
		return fmt.Errorf("unable to update KubeVirt VM '%s': %w", state.KVVM.GetName(), err)
	}

	opts.Log.Info("Update last applied spec on KubeVirt VM done", "name", state.KVVM.Name)

	return nil
}

// detectSpecChanges compares KVVM generated from current VM spec with in cluster KVVM
// to calculate changes and action needed to apply these changes.
func (r *VMReconciler) detectSpecChanges(state *VMReconcilerState, opts two_phase_reconciler.ReconcilerOptions) *vmchange.SpecChanges {
	// Not applicable if KVVM is absent.
	if state.KVVM == nil {
		return nil
	}

	lastSpec, err := kvbuilder.LoadLastAppliedSpec(state.KVVM)
	// TODO Add smarter handler for empty/invalid annotation.
	if lastSpec == nil && err == nil {
		opts.Recorder.Event(state.VM.Current(), corev1.EventTypeWarning, virtv2.ReasonVMLastAppliedSpecInvalid, "Could not find last applied spec. Possible old VM or partial backup restore. Restart or recreate VM to adopt it.")
		lastSpec = &virtv2.VirtualMachineSpec{}
	}
	if err != nil {
		msg := fmt.Sprintf("Could not restore last applied spec: %v. Possible old VM or partial backup restore. Restart or recreate VM to adopt it.", err)
		opts.Recorder.Event(state.VM.Current(), corev1.EventTypeWarning, virtv2.ReasonVMLastAppliedSpecInvalid, msg)
		// In Automatic mode changes are applied immediately, so last-applied-spec annotation will be restored.
		if vmutil.ApprovalMode(state.VM.Current()) == virtv2.Automatic {
			lastSpec = &virtv2.VirtualMachineSpec{}
		}
		if vmutil.ApprovalMode(state.VM.Current()) == virtv2.Manual {
			// Manual mode requires meaningful content in status.pendingChanges.
			// There are different paths:
			//   1. Return err and do nothing, user should restore annotation or recreate VM.
			//   2. Use empty VirtualMachineSpec and show full replace in status.pendingChanges.
			//      This may lead to unexpected restart.
			//   3. Restore some fields from KVVM spec to prevent unexpected restarts and reduce
			//      content in status.pendingChanges.
			//
			// At this time, variant 2 is chosen.
			// TODO(future): Implement variant 3: restore some fields from KVVM.
			lastSpec = &virtv2.VirtualMachineSpec{}
		}
	}

	// Compare VM spec applied to the underlying KVVM
	// with the current VM spec (maybe edited by the user).
	specChanges := vmchange.CompareSpecs(lastSpec, &state.VM.Current().Spec)

	return &specChanges
}

// shouldWaitForChangesApproval returns true if disruptive update was not approved yet.
func (r *VMReconciler) shouldWaitForChangesApproval(state *VMReconcilerState, opts two_phase_reconciler.ReconcilerOptions, changes *vmchange.SpecChanges) bool {
	// Should not wait in Automatic mode.
	if vmutil.ApprovalMode(state.VM.Current()) == virtv2.Automatic {
		return false
	}

	// Should not wait if no changes detected or if changes are non-disruptive.
	if changes.IsEmpty() || !changes.IsDisruptive() {
		state.SetChangeID("")
		return false
	}
	// Wait for Manual approval.
	// Always set status message when in approval wait mode.
	statusMessage := ""
	if changes.ActionType() == vmchange.ActionRestart {
		statusMessage = "VM restart required to apply changes. Check status.changeID and add spec.approvedChangeID to restart VM."
	} else {
		// Non restart changes, e.g. subresource signaling.
		statusMessage = "Approval required to apply changes. Check status.changeID and add spec.approvedChangeID to change VM."
	}
	state.SetStatusMessage(statusMessage)

	changeID := changes.ChangeID()
	currChangeID := state.VM.Current().Status.ChangeID
	// Save or update Change ID into annotation and wait for approval.
	if currChangeID == "" || currChangeID != changeID {
		state.SetChangeID(changeID)
		state.SetPendingChanges(changes.GetPendingChanges())

		opts.Log.V(2).Info("Change ID updated", "changes", changes)
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
func (r *VMReconciler) applyVMChangesToKVVM(ctx context.Context, state *VMReconcilerState, opts two_phase_reconciler.ReconcilerOptions, changes *vmchange.SpecChanges) error {
	if changes.IsEmpty() {
		return nil
	}

	switch changes.ActionType() {
	case vmchange.ActionRestart:
		opts.Log.Info("Restart VM to apply changes", "vm.name", state.VM.Current().GetName())

		message := fmt.Sprintf("Apply changes with ID %s", changes.ChangeID())
		opts.Recorder.Event(state.VM.Current(), corev1.EventTypeNormal, virtv2.ReasonVMChangesApplied, message)
		opts.Recorder.Event(state.VM.Current(), corev1.EventTypeNormal, virtv2.ReasonVMRestarted, "")

		if err := r.updateKVVM(ctx, state, opts); err != nil {
			return fmt.Errorf("unable to update KVVM using new VM spec: %w", err)
		}

		if err := r.restartKVVM(ctx, state, opts); err != nil {
			return fmt.Errorf("unable restart KVVM instance in order to apply changes: %w", err)
		}

	case vmchange.ActionSubresourceSignal:
		// TODO(future): Implement APIService and its client.
		opts.Log.Info("Apply changes using subresource signal", "vm.name", state.VM.Current().GetName(), "action", changes)
		opts.Log.Error(fmt.Errorf("unexpected action: subresource signal, do nothing"), "vm.name", state.VM.Current().GetName(), "action", changes)
	case vmchange.ActionApplyImmediate:
		opts.Log.Info("Apply changes without restart", "vm.name", state.VM.Current().GetName(), "action", changes)
		message := fmt.Sprintf("Apply changes with ID %s without restart", changes.ChangeID())
		opts.Recorder.Event(state.VM.Current(), corev1.EventTypeNormal, virtv2.ReasonVMChangesApplied, message)

		if err := r.updateKVVM(ctx, state, opts); err != nil {
			return fmt.Errorf("unable to update KVVM using new VM spec: %w", err)
		}

	case vmchange.ActionNone:
		opts.Log.V(2).Info("No changes to underlying KVVM, update last-applied-spec", "vm.name", state.VM.Current().GetName())

		if err := r.updateKVVMLastUsedSpec(ctx, state, opts); err != nil {
			return fmt.Errorf("unable to update last-applied-spec on KVVM: %w", err)
		}
	}

	// Cleanup: remove change ID and pending changes after applying changes.
	state.SetChangeID("")
	state.SetPendingChanges(nil)
	return nil
}

// restartKVVM deletes KVVMI to restart VM.
func (r *VMReconciler) restartKVVM(ctx context.Context, state *VMReconcilerState, opts two_phase_reconciler.ReconcilerOptions) error {
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
	if state.KVVM == nil {
		return nil
	}

	// Propagate user specified labels and annotations from the d8 VM to kubevirt VM.
	metaUpdated, err := PropagateVMMetadata(state.VM.Current(), state.KVVM, state.KVVM)
	if err != nil {
		return err
	}

	// Ensure kubevirt VM has finalizer in case d8 VM was created manually (use case: take ownership of already existing object).
	finalizerUpdated := controllerutil.AddFinalizer(state.KVVM, virtv2.FinalizerKVVMProtection)

	if metaUpdated || finalizerUpdated {
		if err = opts.Client.Update(ctx, state.KVVM); err != nil {
			return fmt.Errorf("error setting finalizer on a KubeVirt VM %q: %w", state.KVVM.Name, err)
		}
	}

	// Propagate user specified labels and annotations from the d8 VM to the kubevirt VirtualMachineInstance.
	if state.KVVMI != nil {
		metaUpdated, err = PropagateVMMetadata(state.VM.Current(), state.KVVM, state.KVVMI)
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
			metaUpdated, err = PropagateVMMetadata(state.VM.Current(), state.KVVM, &pod)
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

	err = SetLastPropagatedLabels(state.KVVM, state.VM.Current())
	if err != nil {
		return fmt.Errorf("failed to set last propagated labels: %w", err)
	}

	err = SetLastPropagatedAnnotations(state.KVVM, state.VM.Current())
	if err != nil {
		return fmt.Errorf("failed to set last propagated annotations: %w", err)
	}

	return nil
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
