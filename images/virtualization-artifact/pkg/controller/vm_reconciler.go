/*
Copyright 2024 Flant JSC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

     http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
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

	vmutil "github.com/deckhouse/virtualization-controller/pkg/common/vm"
	"github.com/deckhouse/virtualization-controller/pkg/controller/kvbuilder"
	"github.com/deckhouse/virtualization-controller/pkg/controller/powerstate"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmchange"
	"github.com/deckhouse/virtualization-controller/pkg/dvcr"
	"github.com/deckhouse/virtualization-controller/pkg/sdk/framework/helper"
	"github.com/deckhouse/virtualization-controller/pkg/sdk/framework/two_phase_reconciler"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
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
		predicate.Funcs{
			CreateFunc: func(e event.CreateEvent) bool { return true },
			DeleteFunc: func(e event.DeleteEvent) bool { return true },
			UpdateFunc: func(e event.UpdateEvent) bool {
				oldVM := e.ObjectOld.(*virtv1.VirtualMachine)
				newVM := e.ObjectNew.(*virtv1.VirtualMachine)
				return oldVM.Status.PrintableStatus != newVM.Status.PrintableStatus ||
					oldVM.Status.Ready != newVM.Status.Ready
			},
		},
	); err != nil {
		return fmt.Errorf("error setting watch on VirtualMachine: %w", err)
	}

	// Subscribe on Kubevirt VirtualMachineInstances to update our VM status.
	if err := ctr.Watch(
		source.Kind(mgr.GetCache(), &virtv1.VirtualMachineInstance{}),
		handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, vmi client.Object) []reconcile.Request {
			return []reconcile.Request{
				{
					NamespacedName: types.NamespacedName{
						Name:      vmi.GetName(),
						Namespace: vmi.GetNamespace(),
					},
				},
			}
		}),
		predicate.Funcs{
			CreateFunc: func(e event.CreateEvent) bool { return true },
			DeleteFunc: func(e event.DeleteEvent) bool { return true },
			UpdateFunc: func(e event.UpdateEvent) bool {
				oldVM := e.ObjectOld.(*virtv1.VirtualMachineInstance)
				newVM := e.ObjectNew.(*virtv1.VirtualMachineInstance)
				return !reflect.DeepEqual(oldVM.Status, newVM.Status)
			},
		},
	); err != nil {
		return fmt.Errorf("error setting watch on VirtualMachine: %w", err)
	}

	// Watch for Pods created on behalf of VMs. Handle only changes in status.phase.
	// Pod tracking is required to detect when Pod becomes Completed after guest initiated reset or shutdown.
	if err := ctr.Watch(
		source.Kind(mgr.GetCache(), &corev1.Pod{}),
		handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, pod client.Object) []reconcile.Request {
			vmName, hasLabel := pod.GetLabels()["vm.kubevirt.io/name"]
			if !hasLabel {
				return nil
			}

			return []reconcile.Request{
				{
					NamespacedName: types.NamespacedName{
						Name:      vmName,
						Namespace: pod.GetNamespace(),
					},
				},
			}
		}),
		predicate.Funcs{
			CreateFunc: func(e event.CreateEvent) bool { return true },
			DeleteFunc: func(e event.DeleteEvent) bool { return true },
			UpdateFunc: func(e event.UpdateEvent) bool {
				oldPod := e.ObjectOld.(*corev1.Pod)
				newPod := e.ObjectNew.(*corev1.Pod)
				return oldPod.Status.Phase != newPod.Status.Phase
			},
		},
	); err != nil {
		return fmt.Errorf("error setting watch on Pod: %w", err)
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

	// Wait for IP Claim and block devices.
	depsReady, depsErr := r.syncKVVMDependencies(ctx, state, opts)
	if depsErr != nil {
		opts.Log.Error(depsErr, "sync kvvm dependencies")
	}

	// Sync KVVM if dependencies are ready.
	var syncErr error
	if depsReady {
		// Sync KVVM changes and a power state.
		syncErr = r.syncKVVM(ctx, state, opts)
		if syncErr != nil {
			opts.Log.Error(syncErr, "sync kvvm")
			opts.Recorder.Event(state.VM.Current(), corev1.EventTypeWarning, virtv2.ReasonErrVmNotSynced, syncErr.Error())
		}
	}

	// Always update metadata for all underlying resources: set finalizers and propagate labels and annotations.
	metaErr := r.syncMetadata(ctx, state, opts)
	if metaErr != nil {
		opts.Log.Error(metaErr, "sync metadata")
	}

	// Return the first occurred error, others are logged already.
	switch {
	case depsErr != nil:
		return depsErr
	case syncErr != nil:
		return syncErr
	case metaErr != nil:
		return metaErr
	}

	return nil
}

// syncDependencies ensures IP Claim and block devices are ready and updates KVVM according to changes in VM spec.
func (r *VMReconciler) syncKVVMDependencies(ctx context.Context, state *VMReconcilerState, opts two_phase_reconciler.ReconcilerOptions) (ready bool, err error) {
	ok := r.ensureCPUModel(state, opts)
	if !ok {
		return false, nil
	}

	if controllerutil.AddFinalizer(state.CPUModel, virtv2.FinalizerVMCPUProtection) {
		if err = state.Client.Update(ctx, state.CPUModel); err != nil {
			return false, fmt.Errorf("error setting finalizer on the VMCPU %q: %w", state.CPUModel.Name, err)
		}
	}

	// Ensure IP address claim.
	claimed, err := r.ensureIPAddressClaim(ctx, state, opts)
	if err != nil {
		return false, err
	}

	if !claimed {
		return false, nil
	}

	disksMessage := r.checkBlockDevicesSanity(state)
	if disksMessage != "" {
		state.SetStatusMessage(disksMessage)
		// TODO convert to condition.
		opts.Log.Error(fmt.Errorf("invalid disks: %s", disksMessage), "disks mismatch")
		return false, nil
	}

	if !state.BlockDevicesReady() {
		// Wait until block devices are ready.
		opts.Log.Info("Waiting for block devices to become available")
		opts.Recorder.Event(state.VM.Current(), corev1.EventTypeNormal, virtv2.ReasonVMWaitForBlockDevices, "Waiting for block devices to become available")
		state.SetReconcilerResult(&reconcile.Result{RequeueAfter: 2 * time.Second})
		return false, nil
	}

	// Next set finalizers on attached devices.
	if err = state.SetFinalizersOnBlockDevices(ctx); err != nil {
		return false, fmt.Errorf("unable to add block devices finalizers: %w", err)
	}

	return true, nil
}

func (r *VMReconciler) syncKVVM(ctx context.Context, state *VMReconcilerState, opts two_phase_reconciler.ReconcilerOptions) error {
	if state.KVVM == nil {
		return r.createKVVM(ctx, state, opts)
	}

	lastAppliedSpec := r.loadLastAppliedSpec(state, opts)

	changes := r.detectSpecChanges(state, opts, lastAppliedSpec)

	var syncErr error
	if r.canApplyChanges(state, opts, changes) {
		// No need to wait, apply changes to KVVM immediately.
		syncErr = r.applyVMChangesToKVVM(ctx, state, opts, changes)
		// Changes are applied, consider current spec as last applied.
		lastAppliedSpec = &state.VM.Current().Spec
	} else {
		// Delay changes propagation to KVVM until user restarts VM.
		syncErr = state.SetChangesInfo(changes)
		if syncErr != nil {
			syncErr = fmt.Errorf("prepare changes info for approval: %w", syncErr)
			opts.Log.Error(syncErr, "Error should not occurs when preparing changesInfo, there is a possible bug in code")
		}
	}

	// Ensure power state according to the runPolicy.
	powerErr := r.syncPowerState(ctx, state, opts, lastAppliedSpec)
	if powerErr != nil {
		opts.Log.Error(powerErr, "sync power state")
	}

	switch {
	case syncErr != nil:
		return syncErr
	case powerErr != nil:
		return powerErr
	}

	return nil
}

func (r *VMReconciler) UpdateStatus(_ context.Context, _ reconcile.Request, state *VMReconcilerState, opts two_phase_reconciler.ReconcilerOptions) error {
	if state.isDeletion() {
		state.VM.Changed().Status.Phase = virtv2.MachineTerminating
		return nil
	}

	state.VM.Changed().Status.Message = ""
	if state.VM.Current().Status.Phase == "" {
		state.VM.Current().Status.Phase = virtv2.MachinePending
	}

	// Ensure IP address claim.
	if !r.ensureCPUModel(state, opts) {
		state.VM.Changed().Status.Phase = virtv2.MachinePending
		state.VM.Changed().Status.Message = "Waiting for CPUModel to become available"
		return nil
	}

	// Ensure IP address claim.
	if !r.ipam.IsBound(state.VM.Name().Name, state.IPAddressClaim) {
		state.VM.Changed().Status.Phase = virtv2.MachinePending
		state.VM.Changed().Status.Message = "Waiting for IPAddressClaim to become available"
		return nil
	}

	state.VM.Changed().Status.VirtualMachineIPAddressClaim = state.IPAddressClaim.Name
	state.VM.Changed().Status.IPAddress = state.IPAddressClaim.Spec.Address

	if !state.BlockDevicesReady() {
		state.VM.Changed().Status.Phase = virtv2.MachinePending
		state.VM.Changed().Status.Message = "Waiting for block devices to become available"
		return nil
	}

	switch {
	case state.KVVM == nil:
		state.VM.Changed().Status.Phase = virtv2.MachinePending
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
		state.VM.Changed().Status.Node = state.KVVMI.Status.NodeName
		for _, iface := range state.KVVMI.Status.Interfaces {
			if iface.Name == kvbuilder.NetworkInterfaceName {
				hasClaimedIP := false
				for _, ip := range iface.IPs {
					if ip == state.IPAddressClaim.Spec.Address {
						hasClaimedIP = true
					}
				}
				if !hasClaimedIP {
					msg := fmt.Sprintf("Claimed IP address (%s) is not among addresses assigned to '%s' network interface (%s)", state.IPAddressClaim.Spec.Address, kvbuilder.NetworkInterfaceName, strings.Join(iface.IPs, ", "))
					opts.Recorder.Event(state.VM.Current(), corev1.EventTypeWarning, virtv2.ReasonClaimNotAssigned, msg)
				}
				break
			}
		}
		for _, ref := range state.VM.Current().Spec.BlockDeviceRefs {
			if state.FindAttachedBlockDevice(ref) == nil {
				if abd := state.CreateAttachedBlockDevice(ref); abd != nil {
					state.VM.Changed().Status.BlockDeviceRefs = append(
						state.VM.Changed().Status.BlockDeviceRefs,
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
		// Unexpected state, fallback to Pending phase.
		state.VM.Changed().Status.Phase = virtv2.MachinePending
		opts.Log.Error(fmt.Errorf("unexpected KVVM state: status %q, fallback VM phase to %q", state.KVVM.Status.PrintableStatus, state.VM.Changed().Status.Phase), "")
	}

	state.VM.Changed().Status.Message = state.StatusMessage
	state.VM.Changed().Status.RestartAwaitingChanges = state.RestartAwaitingChanges
	return nil
}

func (r *VMReconciler) ensureCPUModel(state *VMReconcilerState, opts two_phase_reconciler.ReconcilerOptions) bool {
	if state.CPUModel != nil {
		return true
	}

	state.SetStatusMessage("CPU model not available: waiting for the CPU model")
	opts.Recorder.Event(state.VM.Current(), corev1.EventTypeWarning, virtv2.ReasonCPUModelNotFound, "CPU model not available: waiting for the CPU model")
	state.SetReconcilerResult(&reconcile.Result{RequeueAfter: 2 * time.Second})

	return false
}

func (r *VMReconciler) ensureIPAddressClaim(ctx context.Context, state *VMReconcilerState, opts two_phase_reconciler.ReconcilerOptions) (bool, error) {
	// 1. OK: already bound.
	if r.ipam.IsBound(state.VM.Name().Name, state.IPAddressClaim) {
		return true, nil
	}

	// 2. Claim not found: create if possible or wait for the claim.
	if state.IPAddressClaim == nil {
		if state.VM.Current().Spec.VirtualMachineIPAddressClaim != "" {
			opts.Log.Info(fmt.Sprintf("The requested ip address claim (%s) for the virtual machine not found: waiting for the Claim", state.VM.Current().Spec.VirtualMachineIPAddressClaim))
			state.SetStatusMessage(fmt.Sprintf("The requested ip address claim (%s) for the virtual machine not found: waiting for the Claim", state.VM.Current().Spec.VirtualMachineIPAddressClaim))
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
		opts.Log.Info("Claim is not available to be bound", "err", err, "claimName", state.VM.Current().Spec.VirtualMachineIPAddressClaim)
		state.SetStatusMessage(err.Error())
		opts.Recorder.Event(state.VM.Current(), corev1.EventTypeWarning, virtv2.ReasonClaimNotAvailable, err.Error())

		return false, nil
	}

	// 4. Claim exists and available for binding with virtual machine: waiting for the claim.
	opts.Log.Info("Waiting for the Claim to be bound to VM", "claimName", state.VM.Current().Spec.VirtualMachineIPAddressClaim)
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

	for _, bda := range state.VM.Current().Status.BlockDeviceRefs {
		if bda.Hotpluggable {
			hotplugged[bda.Name] = struct{}{}
		}
	}

	for _, bd := range state.VM.Current().Spec.BlockDeviceRefs {
		if bd.Kind == virtv2.DiskDevice {
			if _, ok := hotplugged[bd.Name]; ok {
				disks = append(disks, bd.Name)
			}
		}
	}

	if len(disks) == 0 {
		return ""
	}

	return fmt.Sprintf("spec.blockDeviceRefs contain hotplugged disks: %s. Unplug or remove them from spec to continue.", strings.Join(disks, ", "))
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
	err := kvbuilder.ApplyVirtualMachineSpec(kvvmBuilder, state.VM.Current(), state.VMDByName, state.VMIByName, state.CVMIByName, r.dvcrSettings, state.CPUModel.Spec, state.IPAddressClaim.Spec.Address)
	if err != nil {
		return nil, err
	}
	kvvm := kvvmBuilder.GetResource()

	err = kvbuilder.SetLastAppliedSpec(kvvm, state.VM.Current())
	if err != nil {
		return nil, fmt.Errorf("set last applied spec on KubeVirt VM '%s': %w", state.KVVM.GetName(), err)
	}

	return kvvm, nil
}

// createKVVM constructs and creates new KubeVirt VirtualMachine based on d8 VirtualMachine spec.
func (r *VMReconciler) createKVVM(ctx context.Context, state *VMReconcilerState, opts two_phase_reconciler.ReconcilerOptions) error {
	kvvm, err := r.makeKVVMFromVMSpec(state)
	if err != nil {
		return fmt.Errorf("prepare to create KubeVirt VM '%s': %w", state.VM.Name().Name, err)
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

// updateKVVMLastAppliedSpec updates last-applied-spec annotation on KubeVirt VirtualMachine.
func (r *VMReconciler) updateKVVMLastAppliedSpec(ctx context.Context, state *VMReconcilerState, opts two_phase_reconciler.ReconcilerOptions) error {
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

func (r *VMReconciler) loadLastAppliedSpec(state *VMReconcilerState, opts two_phase_reconciler.ReconcilerOptions) *virtv2.VirtualMachineSpec {
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

	return lastSpec
}

// detectSpecChanges compares KVVM generated from current VM spec with in cluster KVVM
// to calculate changes and action needed to apply these changes.
func (r *VMReconciler) detectSpecChanges(state *VMReconcilerState, opts two_phase_reconciler.ReconcilerOptions, lastSpec *virtv2.VirtualMachineSpec) *vmchange.SpecChanges {
	// Not applicable if KVVM is absent.
	if state.KVVM == nil || lastSpec == nil {
		return nil
	}

	// Compare VM spec applied to the underlying KVVM
	// with the current VM spec (maybe edited by the user).
	specChanges := vmchange.CompareSpecs(lastSpec, &state.VM.Current().Spec)

	opts.Log.V(2).Info(fmt.Sprintf("detected changes: empty %v, disruptive %v, actionType %v", specChanges.IsEmpty(), specChanges.IsDisruptive(), specChanges.ActionType()))
	opts.Log.V(2).Info(fmt.Sprintf("detected changes JSON: %s", specChanges.ToJSON()))

	return &specChanges
}

// canApplyChanges returns true if changes can be applied right now.
//
// Wait if changes are disruptive, and approval mode is manual, and VM is still running.
func (r *VMReconciler) canApplyChanges(state *VMReconcilerState, _ two_phase_reconciler.ReconcilerOptions, changes *vmchange.SpecChanges) bool {
	if vmutil.ApprovalMode(state.VM.Current()) == virtv2.Automatic {
		return true
	}
	if !changes.IsDisruptive() {
		return true
	}
	// Apply disruptive changes if VM is absent or not running.
	if state.KVVMI == nil {
		return true
	}

	if state.vmIsFailed() || state.vmIsPending() {
		return true
	}

	// VM is stopped if instance is not created or Pod is in the Complete state.
	podStopped := true
	if state.VMPod != nil {
		phase := state.VMPod.Status.Phase
		podStopped = phase != corev1.PodPending && phase != corev1.PodRunning
	}

	return state.vmIsStopped() && (!state.vmIsCreated() || podStopped)
}

// applyVMChangesToKVVM applies updates to underlying KVVM based on actions type.
func (r *VMReconciler) applyVMChangesToKVVM(ctx context.Context, state *VMReconcilerState, opts two_phase_reconciler.ReconcilerOptions, changes *vmchange.SpecChanges) error {
	if changes.IsEmpty() {
		return nil
	}

	action := changes.ActionType()

	if state.KVVMI == nil && action == vmchange.ActionRestart {
		action = vmchange.ActionApplyImmediate
	}

	switch action {
	case vmchange.ActionRestart:
		opts.Log.Info("Restart VM to apply changes", "vm.name", state.VM.Current().GetName())

		opts.Recorder.Event(state.VM.Current(), corev1.EventTypeNormal, virtv2.ReasonVMChangesApplied, "Apply disruptive changes")
		opts.Recorder.Event(state.VM.Current(), corev1.EventTypeNormal, virtv2.ReasonVMRestarted, "")

		// Update KVVM spec according the current VM spec.
		if err := r.updateKVVM(ctx, state, opts); err != nil {
			return fmt.Errorf("unable to update KVVM using new VM spec: %w", err)
		}
		// Ask kubevirt to re-create KVVMI to apply new spec from KVVM.
		if err := r.restartKVVM(ctx, state, opts); err != nil {
			return fmt.Errorf("unable restart KVVM instance in order to apply changes: %w", err)
		}

	case vmchange.ActionApplyImmediate:
		message := "Apply changes without restart"
		if changes.IsDisruptive() {
			message = "Apply disruptive changes without restart"
		}
		opts.Log.Info(message, "vm.name", state.VM.Current().GetName(), "action", changes)
		opts.Recorder.Event(state.VM.Current(), corev1.EventTypeNormal, virtv2.ReasonVMChangesApplied, message)

		if err := r.updateKVVM(ctx, state, opts); err != nil {
			return fmt.Errorf("unable to update KVVM using new VM spec: %w", err)
		}

	case vmchange.ActionNone:
		opts.Log.V(2).Info("No changes to underlying KVVM, update last-applied-spec annotation", "vm.name", state.VM.Current().GetName())

		if err := r.updateKVVMLastAppliedSpec(ctx, state, opts); err != nil {
			return fmt.Errorf("unable to update last-applied-spec on KVVM: %w", err)
		}
	}

	// Cleanup: remove changes from the VM status after applying changes.
	state.ResetChangesInfo()
	return nil
}

// restartKVVM deletes KVVMI to restart VM.
func (r *VMReconciler) restartKVVM(ctx context.Context, state *VMReconcilerState, opts two_phase_reconciler.ReconcilerOptions) error {
	err := powerstate.RestartVM(ctx, opts.Client, state.KVVM, state.KVVMI, false)
	if err != nil {
		return fmt.Errorf("unable to restart current KubeVirt VMI %q: %w", state.KVVMI.Name, err)
	}

	state.KVPods = nil

	return nil
}

// syncPowerState enforces runPolicy on the underlying KVVM.
func (r *VMReconciler) syncPowerState(ctx context.Context, state *VMReconcilerState, opts two_phase_reconciler.ReconcilerOptions, effectiveSpec *virtv2.VirtualMachineSpec) error {
	if state.KVVM == nil {
		return nil
	}

	vmRunPolicy := effectiveSpec.RunPolicy

	var err error
	switch vmRunPolicy {
	case virtv2.AlwaysOffPolicy:
		if state.KVVMI != nil {
			// Ensure KVVMI is absent.
			err = opts.Client.Delete(ctx, state.KVVMI)
			if err != nil && !k8serrors.IsNotFound(err) {
				return fmt.Errorf("force AlwaysOff: delete KVVMI: %w", err)
			}
		}
		err = state.EnsureRunStrategy(ctx, virtv1.RunStrategyHalted)
	case virtv2.AlwaysOnPolicy:
		// Power state change reason is not significant for AlwaysOn:
		// kubevirt restarts VM via re-creation of KVVMI.
		err = state.EnsureRunStrategy(ctx, virtv1.RunStrategyAlways)
	case virtv2.AlwaysOnUnlessStoppedManually:
		if state.KVVMI != nil && state.KVVMI.DeletionTimestamp == nil {
			if state.KVVMI.Status.Phase == virtv1.Succeeded {
				if state.VMPodCompleted {
					// Request to start new KVVMI if guest was restarted.
					// Cleanup KVVMI is enough if VM was stopped from inside.
					if state.VMShutdownReason == powerstate.GuestResetReason {
						opts.Log.Info("Restart for guest initiated reset")
						err = powerstate.SafeRestartVM(ctx, opts.Client, state.KVVM, state.KVVMI)
						if err != nil {
							return fmt.Errorf("restart VM on guest-reset: %w", err)
						}
					} else {
						opts.Log.Info("Cleanup Succeeded KVVMI")
						err = opts.Client.Delete(ctx, state.KVVMI)
						if err != nil && !k8serrors.IsNotFound(err) {
							return fmt.Errorf("delete Succeeded KVVMI: %w", err)
						}
					}
				}
			}
			if state.KVVMI.Status.Phase == virtv1.Failed {
				opts.Log.Info("Restart on Failed KVVMI", "obj", state.KVVMI.GetName())
				err = powerstate.SafeRestartVM(ctx, opts.Client, state.KVVM, state.KVVMI)
				if err != nil {
					return fmt.Errorf("restart VM on failed: %w", err)
				}
			}
		}

		err = state.EnsureRunStrategy(ctx, virtv1.RunStrategyManual)
	case virtv2.ManualPolicy:
		// Manual policy requires to handle only guest-reset event.
		// All types of shutdown are a final state.
		if state.KVVMI != nil && state.KVVMI.DeletionTimestamp == nil {
			if state.KVVMI.Status.Phase == virtv1.Succeeded && state.VMPodCompleted {
				// Request to start new KVVMI (with updated settings).
				if state.VMShutdownReason == powerstate.GuestResetReason {
					err = powerstate.SafeRestartVM(ctx, opts.Client, state.KVVM, state.KVVMI)
					if err != nil {
						return fmt.Errorf("restart VM on guest-reset: %w", err)
					}
				} else {
					// Cleanup old version of KVVMI.
					opts.Log.Info("Cleanup Succeeded KVVMI")
					err = opts.Client.Delete(ctx, state.KVVMI)
					if err != nil && !k8serrors.IsNotFound(err) {
						return fmt.Errorf("delete Succeeded KVVMI: %w", err)
					}
				}
			}
		}

		err = state.EnsureRunStrategy(ctx, virtv1.RunStrategyManual)
	}

	if err != nil {
		return fmt.Errorf("enforce runPolicy %s: %w", vmRunPolicy, err)
	}

	return nil
}

// syncMetadata propagates labels and annotations from VM to underlying objects and sets a finalizer on the KVVM.
func (r *VMReconciler) syncMetadata(ctx context.Context, state *VMReconcilerState, opts two_phase_reconciler.ReconcilerOptions) error {
	if state.KVVM == nil {
		return nil
	}

	// Propagate user specified labels and annotations from the d8 VM to the kubevirt VirtualMachineInstance.
	if state.KVVMI != nil {
		metaUpdated, err := PropagateVMMetadata(state.VM.Current(), state.KVVM, state.KVVMI)
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
			metaUpdated, err := PropagateVMMetadata(state.VM.Current(), state.KVVM, &pod)
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

	// Propagate user specified labels and annotations from the d8 VM to kubevirt VM.
	metaUpdated, err := PropagateVMMetadata(state.VM.Current(), state.KVVM, state.KVVM)
	if err != nil {
		return err
	}

	// Ensure kubevirt VM has finalizer in case d8 VM was created manually (use case: take ownership of already existing object).
	finalizerUpdated := controllerutil.AddFinalizer(state.KVVM, virtv2.FinalizerKVVMProtection)

	labelsChanged, err := SetLastPropagatedLabels(state.KVVM, state.VM.Current())
	if err != nil {
		return fmt.Errorf("failed to set last propagated labels: %w", err)
	}

	annosChanged, err := SetLastPropagatedAnnotations(state.KVVM, state.VM.Current())
	if err != nil {
		return fmt.Errorf("failed to set last propagated annotations: %w", err)
	}

	if labelsChanged || annosChanged || metaUpdated || finalizerUpdated {
		if err = opts.Client.Update(ctx, state.KVVM); err != nil {
			return fmt.Errorf("error setting finalizer on a KubeVirt VM %q: %w", state.KVVM.Name, err)
		}
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
