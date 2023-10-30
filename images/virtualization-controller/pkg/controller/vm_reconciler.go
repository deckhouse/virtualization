package controller

import (
	"context"
	"fmt"
	"os"
	"time"

	corev1 "k8s.io/api/core/v1"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	virtv2 "github.com/deckhouse/virtualization-controller/api/v2alpha1"
	cc "github.com/deckhouse/virtualization-controller/pkg/controller/common"
	"github.com/deckhouse/virtualization-controller/pkg/controller/kvbuilder"
	"github.com/deckhouse/virtualization-controller/pkg/sdk/framework/two_phase_reconciler"
)

type VMReconciler struct {
	dvcrSettings *cc.DVCRSettings
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
	if !state.VM.Current().ObjectMeta.DeletionTimestamp.IsZero() {
		// The object is being deleted
		if controllerutil.ContainsFinalizer(state.VM.Current(), virtv2.FinalizerVMCleanup) {
			// Our finalizer is present, so lets cleanup DV, PVC & PV dependencies
			if state.KVVM != nil {
				if controllerutil.RemoveFinalizer(state.KVVM, virtv2.FinalizerKVVMProtection) {
					if err := opts.Client.Update(ctx, state.KVVM); err != nil {
						return fmt.Errorf("unable to remove KubeVirt VM %q finalizer %q: %w", state.KVVM.Name, virtv2.FinalizerKVVMProtection, err)
					}
				}
			}
			controllerutil.RemoveFinalizer(state.VM.Changed(), virtv2.FinalizerVMCleanup)
		}

		// Stop reconciliation as the item is being deleted
		return nil
	}

	// Set finalizer atomically using requeue.
	if controllerutil.AddFinalizer(state.VM.Changed(), virtv2.FinalizerVMCleanup) {
		state.SetReconcilerResult(&reconcile.Result{Requeue: true})
		return nil
	}

	// First set VM labels with attached devices names and requeue to go to the next step.
	if state.SetVMLabelsWithAttachedBlockDevices() {
		state.SetReconcilerResult(&reconcile.Result{Requeue: true})
		return nil
	}
	// Next set finalizers on attached devices.
	if err := state.SetFinalizersOnBlockDevices(ctx); err != nil {
		return fmt.Errorf("unable to add block devices finalizers: %w", err)
	}

	if state.BlockDevicesReady() {
		kvvmName := state.VM.Name()

		if state.KVVM == nil {
			// No underlying VM found, create fresh kubevirt VirtualMachine resource from d8 VirtualMachine spec.
			kvvmBuilder := kvbuilder.NewEmptyKVVM(kvvmName, kvbuilder.KVVMOptions{
				EnableParavirtualization:  state.VM.Current().Spec.EnableParavirtualization,
				OsType:                    state.VM.Current().Spec.OsType,
				ForceBridgeNetworkBinding: os.Getenv("FORCE_BRIDGE_NETWORK_BINDING") == "1",
				DisableHypervSyNIC:        os.Getenv("DISABLE_HYPERV_SYNIC") == "1",
			})
			kvbuilder.ApplyVirtualMachineSpec(kvvmBuilder, state.VM.Current(), state.VMDByName, state.CVMIByName, r.dvcrSettings)
			kvvm := kvvmBuilder.GetResource()

			if err := opts.Client.Create(ctx, kvvm); err != nil {
				return fmt.Errorf("unable to create KubeVirt VM %q: %w", kvvmName, err)
			}
			state.KVVM = kvvm

			opts.Log.Info("Created new KubeVirt VM", "name", kvvmName, "kvvm", state.KVVM)
		} else {
			// Save current KVVM.
			currKVVM := &kvbuilder.KVVM{}
			currKVVM.Resource = state.KVVM.DeepCopy()

			// Update underlying kubevirt VirtualMachine resource from updated d8 VirtualMachine spec.
			// FIXME(VM): This will be changed for effective-spec logic implementation
			kvvmBuilder := kvbuilder.NewKVVM(state.KVVM, kvbuilder.KVVMOptions{
				EnableParavirtualization:  state.VM.Current().Spec.EnableParavirtualization,
				OsType:                    state.VM.Current().Spec.OsType,
				ForceBridgeNetworkBinding: os.Getenv("FORCE_BRIDGE_NETWORK_BINDING") == "1",
				DisableHypervSyNIC:        os.Getenv("DISABLE_HYPERV_SYNIC") == "1",
			})

			kvbuilder.ApplyVirtualMachineSpec(kvvmBuilder, state.VM.Current(), state.VMDByName, state.CVMIByName, r.dvcrSettings)
			kvvm := kvvmBuilder.GetResource()

			// Compare current version of the underlying KVVM
			// with newly generated from VM spec. Perform action if needed to apply
			// changes to underlying virtual machine instance.

			action, err := kvbuilder.CompareKVVM(currKVVM, kvvmBuilder)
			if err != nil {
				opts.Log.Error(err, "Detect action to apply changes failed", "vm.name", state.VM.Current().GetName())
				return fmt.Errorf("detect action to apply changes on vm/%s failed: %w", state.VM.Current().GetName(), err)
			}

			switch action.ActionType() {
			case kvbuilder.ActionRestart:
				// Restart is a dangerous action, check if Manual approve is required.
				// TODO(approve Manual) sync virtv2.VirtualMachine and its CRD, add Disruptions fields.
				opts.Log.Info("Change underlying KVVM requires restart", "vm.name", state.VM.Current().GetName(), "action", action)
				if err := r.ApplyChangesWithRestart(ctx, action, state, kvvm, opts); err != nil {
					return fmt.Errorf("unable to apply restart KVVM instance action: %w", err)
				}
			case kvbuilder.ActionSubresourceSignal:
				// TODO implement
			case kvbuilder.ActionApplyImmediate:
				opts.Log.Info("Apply changes without restart", "vm.name", state.VM.Current().GetName(), "action", action)
				if err := opts.Client.Update(ctx, kvvm); err != nil {
					return fmt.Errorf("unable to update KubeVirt VM %q: %w", kvvmName, err)
				}
				state.KVVM = kvvm
			case kvbuilder.ActionNone:
				opts.Log.V(2).Info("No changes to underlying KVVM", "vm.name", state.VM.Current().GetName())
			}
		}
	} else {
		// Wait until block devices are ready.
		opts.Log.Info("Waiting for block devices to become available")
		opts.Recorder.Event(state.VM.Current(), corev1.EventTypeNormal, "WaitForBlockDevices", "Waiting for block devices to become available")
		state.SetReconcilerResult(&reconcile.Result{RequeueAfter: 2 * time.Second})
	}

	// Always update metadata for underlying kubevirt resources: set finalizers and propagate labels and annotations.

	// Ensure kubevirt VM has finalizer in case d8 VM was created manually (use case: take ownership of already existing object).
	if state.KVVM != nil {
		// Propagate user specified labels and annotations from the d8 VM to kubevirt VM.
		shouldUpdate := PropagateVMMetadata(state.VM.Current(), state.KVVM)

		shouldUpdate = shouldUpdate || controllerutil.AddFinalizer(state.KVVM, virtv2.FinalizerKVVMProtection)

		if shouldUpdate {
			if err := opts.Client.Update(ctx, state.KVVM); err != nil {
				return fmt.Errorf("error setting finalizer on a KubeVirt VM %q: %w", state.KVVM.Name, err)
			}
		}
	}

	// Propagate user specified labels and annotations from the d8 VM to the kubevirt VirtualMachineInstance.
	if state.KVVMI != nil {
		if PropagateVMMetadata(state.VM.Current(), state.KVVMI) {
			if err := opts.Client.Update(ctx, state.KVVMI); err != nil {
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
			if PropagateVMMetadata(state.VM.Current(), &pod) {
				if err := opts.Client.Update(ctx, &pod); err != nil {
					return fmt.Errorf("unable to update KubeVirt Pod %q: %w", pod.GetName(), err)
				}
			}
		}
	}

	return nil
}

func (r *VMReconciler) UpdateStatus(_ context.Context, _ reconcile.Request, state *VMReconcilerState, opts two_phase_reconciler.ReconcilerOptions) error {
	opts.Log.Info("VMReconciler.UpdateStatus")

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
	case virtv2.MachineScheduling:
		if state.KVVMI != nil {
			if state.KVVMI.Status.Phase == virtv1.Running {
				state.VM.Changed().Status.Phase = virtv2.MachineRunning
			}
		}
	case virtv2.MachineRunning:
		if state.KVVM != nil && state.KVVMI == nil {
			state.VM.Changed().Status.Phase = virtv2.MachineTerminating
		}
	case virtv2.MachineTerminating:
	case virtv2.MachineStopped:
	case virtv2.MachineFailed:
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
	case virtv2.MachineStopped:
	case virtv2.MachineFailed:
	default:
		panic(fmt.Sprintf("unexpected phase %q", state.VM.Changed().Status.Phase))
	}

	return nil
}
