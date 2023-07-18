package controller

import (
	"context"
	"fmt"
	"os"
	"time"

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
	"github.com/deckhouse/virtualization-controller/pkg/controller/kvbuilder"
	"github.com/deckhouse/virtualization-controller/pkg/sdk/framework/two_phase_reconciler"
)

type VMReconciler struct{}

func (r *VMReconciler) SetupController(_ context.Context, _ manager.Manager, ctr controller.Controller) error {
	if err := ctr.Watch(&source.Kind{Type: &virtv2.VirtualMachine{}}, &handler.EnqueueRequestForObject{},
		predicate.Funcs{
			CreateFunc: func(e event.CreateEvent) bool { return true },
			DeleteFunc: func(e event.DeleteEvent) bool { return true },
			UpdateFunc: func(e event.UpdateEvent) bool { return true },
		},
	); err != nil {
		return fmt.Errorf("error setting watch on VM: %w", err)
	}

	if err := ctr.Watch(&source.Kind{Type: &virtv1.VirtualMachine{}}, &handler.EnqueueRequestForOwner{
		OwnerType:    &virtv2.VirtualMachine{},
		IsController: true,
	}); err != nil {
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

	// Set finalizer atomically
	if controllerutil.AddFinalizer(state.VM.Changed(), virtv2.FinalizerVMCleanup) {
		state.SetReconcilerResult(&reconcile.Result{Requeue: true})
		return nil
	}

	kvvmName := state.VM.Name()

	if state.KVVM == nil {
		// Check all images and disks are ready to use
		for _, bd := range state.VM.Current().Spec.BlockDevices {
			switch bd.Type {
			case virtv2.ImageDevice:
				panic("NOT IMPLEMENTED")

			case virtv2.DiskDevice:
				if vmd, hasKey := state.VMDByName[bd.VirtualMachineDisk.Name]; hasKey {
					opts.Log.Info("Check VMD ready", "VMD", vmd, "Status", vmd.Status)
					if vmd.Status.Phase != virtv2.DiskReady {
						opts.Log.Info("Waiting for VMD to become ready", "VMD Name", bd.VirtualMachineDisk.Name)
						state.SetReconcilerResult(&reconcile.Result{RequeueAfter: 2 * time.Second})
						return nil
					}
				} else {
					opts.Log.Info("Waiting for VMD to become available", "VMD", bd.VirtualMachineDisk.Name)
					state.SetReconcilerResult(&reconcile.Result{RequeueAfter: 2 * time.Second})
					return nil
				}

			default:
				panic(fmt.Sprintf("unknown block device type %q", bd.Type))
			}
		}

		kvvmBuilder := kvbuilder.NewEmptyKVVM(kvvmName, kvbuilder.KVVMOptions{
			EnableParavirtualization:  state.VM.Current().Spec.EnableParavirtualization,
			OsType:                    state.VM.Current().Spec.OsType,
			ForceBridgeNetworkBinding: os.Getenv("FORCE_BRIDGE_NETWORK_BINDING") == "1",
		})
		kvbuilder.ApplyVirtualMachineSpec(kvvmBuilder, state.VM.Current(), state.VMDByName)
		kvvm := kvvmBuilder.GetResource()

		if err := opts.Client.Create(ctx, kvvm); err != nil {
			return fmt.Errorf("unable to create KubeVirt VM %q: %w", kvvmName, err)
		}
		state.KVVM = kvvm

		opts.Log.Info("Created new KubeVirt VM", "name", kvvmName, "kvvm", state.KVVM)
	} else {
		// FIXME(VM): This will be extended for effective-spec logic implementation

		kvvmBuilder := kvbuilder.NewKVVM(state.KVVM, kvbuilder.KVVMOptions{
			EnableParavirtualization:  state.VM.Current().Spec.EnableParavirtualization,
			OsType:                    state.VM.Current().Spec.OsType,
			ForceBridgeNetworkBinding: os.Getenv("FORCE_BRIDGE_NETWORK_BINDING") == "1",
		})
		kvvmBuilder.SetRunPolicy(state.VM.Current().Spec.RunPolicy)
		kvvm := kvvmBuilder.GetResource()

		if err := opts.Client.Update(ctx, kvvm); err != nil {
			return fmt.Errorf("unable to update KubeVirt VM %q: %w", kvvmName, err)
		}
		state.KVVM = kvvm

		opts.Log.Info("Updated KubeVirt VM RunPolicy", "name", kvvmName, "kvvm", state.KVVM)
	}

	// Add KubeVirt VM finalizer
	if state.KVVM != nil {
		// Ensure KubeVirt VM finalizer is set in case VM was created manually (take ownership of already existing object)
		if controllerutil.AddFinalizer(state.KVVM, virtv2.FinalizerKVVMProtection) {
			if err := opts.Client.Update(ctx, state.KVVM); err != nil {
				return fmt.Errorf("error setting finalizer on a KubeVirt VM %q: %w", state.KVVM.Name, err)
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
	case virtv2.MachineTerminating:
	case virtv2.MachineStopped:
	case virtv2.MachineFailed:
	}

	// Set fields after phase changed
	switch state.VM.Changed().Status.Phase {
	case virtv2.MachinePending:
	case virtv2.MachineScheduling:
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
