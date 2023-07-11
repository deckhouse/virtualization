package controller

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
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
	"github.com/deckhouse/virtualization-controller/pkg/sdk/framework/two_phase_reconciler"
	"github.com/deckhouse/virtualization-controller/pkg/util"
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
			if state.KVVMI != nil {
				if controllerutil.RemoveFinalizer(state.KVVMI, virtv2.FinalizerKVVMIProtection) {
					if err := opts.Client.Update(ctx, state.KVVMI); err != nil {
						return fmt.Errorf("unable to remove KubeVirt VMI %q finalizer %q: %w", state.KVVMI.Name, virtv2.FinalizerKVVMIProtection, err)
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

	if state.KVVM == nil {
		kvvmName := state.VM.Name()

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

		kvvm := NewKVVMFromVirtualMachine(kvvmName, state.VM.Current(), state.VMDByName)
		if err := opts.Client.Create(ctx, kvvm); err != nil {
			return fmt.Errorf("unable to create KubeVirt VM %q: %w", kvvmName, err)
		}
		state.KVVM = kvvm

		opts.Log.Info("Created new KubeVirt VM", "name", kvvmName, "kvvm", state.KVVM)
	}

	// Add KubeVirt VM and VMI finalizers
	if state.KVVM != nil {
		// Ensure KubeVirt VM finalizer is set in case VM was created manually (take ownership of already existing object)
		if controllerutil.AddFinalizer(state.KVVM, virtv2.FinalizerKVVMProtection) {
			if err := opts.Client.Update(ctx, state.KVVM); err != nil {
				return fmt.Errorf("error setting finalizer on a KubeVirt VM %q: %w", state.KVVM.Name, err)
			}
		}
	}
	if state.KVVMI != nil {
		if controllerutil.AddFinalizer(state.KVVMI, virtv2.FinalizerKVVMIProtection) {
			if err := opts.Client.Update(ctx, state.KVVMI); err != nil {
				return fmt.Errorf("error setting finalizer on a KubeVirt VMI %q: %w", state.KVVMI.Name, err)
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
		if state.KVVMI.Status.Phase == virtv1.Running {
			state.VM.Changed().Status.Phase = virtv2.MachineRunning
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
	case virtv2.MachineTerminating:
	case virtv2.MachineStopped:
	case virtv2.MachineFailed:
	default:
		panic(fmt.Sprintf("unexpected phase %q", state.VM.Changed().Status.Phase))
	}

	return nil
}

func NewKVVMFromVirtualMachine(name types.NamespacedName, vm *virtv2.VirtualMachine, vmdByName map[string]*virtv2.VirtualMachineDisk) *virtv1.VirtualMachine {
	labels := map[string]string{}
	annotations := map[string]string{}

	res := &virtv1.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:   name.Namespace,
			Name:        name.Name,
			Labels:      labels,
			Annotations: annotations,
		},
		Spec: virtv1.VirtualMachineSpec{
			// TODO(VM): Implement RunPolicy instead
			Running: util.GetPointer(true),
			// RunStrategy: util.GetPointer(virtv1.RunStrategyAlways),
			Template: &virtv1.VirtualMachineInstanceTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{},
				Spec: virtv1.VirtualMachineInstanceSpec{
					Domain: virtv1.DomainSpec{
						Resources: virtv1.ResourceRequirements{
							Requests: corev1.ResourceList{
								// FIXME: support coreFraction: req = vm.Spec.CPU.Cores * coreFraction
								corev1.ResourceCPU:    resource.MustParse(fmt.Sprintf("%d", vm.Spec.CPU.Cores)),
								corev1.ResourceMemory: resource.MustParse(vm.Spec.Memory.Size),
							},
							Limits: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse(fmt.Sprintf("%d", vm.Spec.CPU.Cores)),
								corev1.ResourceMemory: resource.MustParse(vm.Spec.Memory.Size),
							},
						},
						CPU: &virtv1.CPU{
							Model: "Nehalem",
						},
					},
				},
			},
		},
	}

	for _, bd := range vm.Spec.BlockDevices {
		switch bd.Type {
		case virtv2.ImageDevice:
			panic("NOT IMPLEMENTED")

		case virtv2.DiskDevice:
			vmd, hasVmd := vmdByName[bd.VirtualMachineDisk.Name]
			if !hasVmd {
				panic(fmt.Sprintf("not found loaded VMD %q which is used in the VM configuration", bd.VirtualMachineDisk.Name))
			}
			if vmd.Status.Phase != virtv2.DiskReady {
				panic(fmt.Sprintf("unexpected VMD %q status phase %q: expected ready phase", vmd.Name, vmd.Status.Phase))
			}

			disk := virtv1.Disk{
				Name: bd.VirtualMachineDisk.Name,
				DiskDevice: virtv1.DiskDevice{
					Disk: &virtv1.DiskTarget{
						Bus: virtv1.DiskBusVirtio, // FIXME(VM): take into account OSType & enableParavirtualization
					},
				},
			}
			res.Spec.Template.Spec.Domain.Devices.Disks = append(res.Spec.Template.Spec.Domain.Devices.Disks, disk)

			volume := virtv1.Volume{
				Name: bd.VirtualMachineDisk.Name,
				VolumeSource: virtv1.VolumeSource{
					PersistentVolumeClaim: &virtv1.PersistentVolumeClaimVolumeSource{
						PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
							ClaimName: vmd.Status.PersistentVolumeClaimName,
						},
					},
				},
			}
			res.Spec.Template.Spec.Volumes = append(res.Spec.Template.Spec.Volumes, volume)

		default:
			panic(fmt.Sprintf("unknown block device type %q", bd.Type))
		}
	}

	res.OwnerReferences = []metav1.OwnerReference{
		*metav1.NewControllerRef(vm, schema.GroupVersionKind{
			Group:   virtv2.SchemeGroupVersion.Group,
			Version: virtv2.SchemeGroupVersion.Version,
			Kind:    "VirtualMachine",
		}),
	}

	controllerutil.AddFinalizer(res, virtv2.FinalizerKVVMProtection)

	return res
}
