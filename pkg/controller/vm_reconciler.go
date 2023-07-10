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
	"k8s.io/apimachinery/pkg/util/uuid"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	virtv2 "github.com/deckhouse/virtualization-controller/api/v2alpha1"
	"github.com/deckhouse/virtualization-controller/pkg/sdk/framework/helper"
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

	if err := ctr.Watch(&source.Kind{Type: &virtv1.VirtualMachineInstance{}}, &handler.EnqueueRequestForOwner{
		OwnerType:    &virtv2.VirtualMachine{},
		IsController: true,
	}); err != nil {
		return fmt.Errorf("error setting watch on VirtualMachineInstance: %w", err)
	}

	return nil
}

func (r *VMReconciler) Sync(ctx context.Context, req reconcile.Request, state *VMReconcilerState, opts two_phase_reconciler.ReconcilerOptions) error {
	if !state.VM.Current().ObjectMeta.DeletionTimestamp.IsZero() {
		// FIXME(VM): implement deletion case
		return nil
	}

	if vmiName, hasKey := state.VM.Current().Annotations[AnnVMVirtualMachineInstance]; !hasKey {
		if state.VM.Changed().Annotations == nil {
			state.VM.Changed().Annotations = make(map[string]string)
		}
		state.VM.Changed().Annotations[AnnVMVirtualMachineInstance] = fmt.Sprintf("virtual-machine-instance-%s", uuid.NewUUID())
		opts.Log.Info("Generated DV name", "name", state.VM.Changed().Annotations[AnnVMVirtualMachineInstance])
	} else {
		name := types.NamespacedName{Name: vmiName, Namespace: req.Namespace}

		vmi, err := helper.FetchObject(ctx, name, opts.Client, &virtv1.VirtualMachineInstance{})
		if err != nil {
			return fmt.Errorf("unable to get VMI %q: %w", name, err)
		}

		if vmi == nil {
			// Check all VMD are loaded
			for _, bd := range state.VM.Current().Spec.BlockDevices {
				switch bd.Type {
				case virtv2.ImageDevice:
					panic("NOT IMPLEMENTED")

				case virtv2.DiskDevice:
					if vmd, hasKey := state.VMDByName[bd.VirtualMachineDisk.Name]; hasKey {
						opts.Log.Info("Check VM ready", "VMD", vmd, "Status", vmd.Status)
						if vmd.Status.Phase != virtv2.DiskReady {
							opts.Log.Info("Waiting for VMD to become ready", "VMD", bd.VirtualMachineDisk.Name)
							state.SetReconcilerResult(&reconcile.Result{RequeueAfter: 2 * time.Second})
							return nil
						}
					} else {
						// TODO(VM): Maybe set waiting for BlockDevices annotation
						opts.Log.Info("Waiting for VMD to become available", "VMD", bd.VirtualMachineDisk.Name)
						state.SetReconcilerResult(&reconcile.Result{RequeueAfter: 2 * time.Second})
						return nil
					}

				default:
					panic(fmt.Sprintf("unknown block device type %q", bd.Type))
				}
			}

			vmi = NewVMIFromVirtualMachine(name, state.VM.Current(), state.VMDByName)
			if err := opts.Client.Create(ctx, vmi); err != nil {
				return fmt.Errorf("unable to create VMI %q: %w", vmi.Name, err)
			}
			opts.Log.Info("Created new VMI", "name", vmi.Name, "vmi", vmi)
		}

		state.VMI = vmi
	}

	return nil
}

func (r *VMReconciler) UpdateStatus(ctx context.Context, req reconcile.Request, state *VMReconcilerState, opts two_phase_reconciler.ReconcilerOptions) error {
	opts.Log.Info("VMReconciler.UpdateStatus")

	_ = ctx
	_ = req
	_ = state

	return nil
}

func NewVMIFromVirtualMachine(name types.NamespacedName, vm *virtv2.VirtualMachine, vmdByName map[string]*virtv2.VirtualMachineDisk) *virtv1.VirtualMachineInstance {
	labels := map[string]string{}
	annotations := map[string]string{}

	res := &virtv1.VirtualMachineInstance{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:   name.Namespace,
			Name:        name.Name,
			Labels:      labels,
			Annotations: annotations,
		},
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
			res.Spec.Domain.Devices.Disks = append(res.Spec.Domain.Devices.Disks, disk)

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
			res.Spec.Volumes = append(res.Spec.Volumes, volume)

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

	return res
}
