package vmattachee

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
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

// AttacheeReconciler struct aimed to be included into the image or disk, which is attached to the VM
type AttacheeReconciler[T helper.Object[T, ST], ST any] struct {
	Kind virtv2.BlockDeviceType
}

func NewAttacheeReconciler[T helper.Object[T, ST], ST any](kind virtv2.BlockDeviceType) *AttacheeReconciler[T, ST] {
	return &AttacheeReconciler[T, ST]{
		Kind: kind,
	}
}

func (r *AttacheeReconciler[T, ST]) SetupController(_ context.Context, mgr manager.Manager, ctr controller.Controller) error {
	if err := ctr.Watch(
		source.Kind(mgr.GetCache(), &virtv2.VirtualMachine{}),
		handler.EnqueueRequestsFromMapFunc(r.enqueueAttacheeRequestsFromVM),
		predicate.Funcs{
			CreateFunc: func(e event.CreateEvent) bool {
				return r.hasAttachedKind(e.Object)
			},
			DeleteFunc: func(e event.DeleteEvent) bool {
				return r.hasAttachedKind(e.Object)
			},
			UpdateFunc: func(e event.UpdateEvent) bool {
				return r.hasAttachedKind(e.ObjectOld) || r.hasAttachedKind(e.ObjectNew)
			},
		},
	); err != nil {
		return fmt.Errorf("error setting watch on VirtualMachineInstance: %w", err)
	}

	return nil
}

func (r *AttacheeReconciler[T, ST]) Sync(_ context.Context, state *AttacheeState[T, ST], opts two_phase_reconciler.ReconcilerOptions) bool {
	opts.Log.V(2).Info("AttacheeReconciler Sync", "ShouldRemoveProtectionFinalizer", state.ShouldRemoveProtectionFinalizer())
	if state.ShouldRemoveProtectionFinalizer() {
		state.RemoveProtectionFinalizer()
		state.SetReconcilerResult(&reconcile.Result{Requeue: true})
		return true
	}
	return false
}

func (r *AttacheeReconciler[T, ST]) hasAttachedKind(obj client.Object) bool {
	vm, ok := obj.(*virtv2.VirtualMachine)
	if !ok {
		return false
	}

	for _, bda := range vm.Status.BlockDevicesAttached {
		switch r.Kind {
		case virtv2.ClusterImageDevice:
			if bda.Type == r.Kind && bda.ClusterVirtualMachineImage != nil {
				return true
			}
		case virtv2.ImageDevice:
			if bda.Type == r.Kind && bda.VirtualMachineImage != nil {
				return true
			}
		case virtv2.DiskDevice:
			if bda.Type == r.Kind && bda.VirtualMachineDisk != nil {
				return true
			}
		}
	}

	return false
}

func (r *AttacheeReconciler[T, ST]) enqueueAttacheeRequestsFromVM(_ context.Context, obj client.Object) []reconcile.Request {
	vm, ok := obj.(*virtv2.VirtualMachine)
	if !ok {
		return nil
	}

	var requests []reconcile.Request

	for _, bda := range vm.Status.BlockDevicesAttached {
		switch r.Kind {
		case virtv2.ClusterImageDevice:
			if bda.Type == r.Kind && bda.ClusterVirtualMachineImage != nil {
				requests = append(requests, reconcile.Request{NamespacedName: types.NamespacedName{
					Name: bda.ClusterVirtualMachineImage.Name,
				}})
			}
		case virtv2.ImageDevice:
			if bda.Type == r.Kind && bda.VirtualMachineImage != nil {
				requests = append(requests, reconcile.Request{NamespacedName: types.NamespacedName{
					Name:      bda.VirtualMachineImage.Name,
					Namespace: vm.Namespace,
				}})
			}
		case virtv2.DiskDevice:
			if bda.Type == r.Kind && bda.VirtualMachineDisk != nil {
				requests = append(requests, reconcile.Request{NamespacedName: types.NamespacedName{
					Name:      bda.VirtualMachineDisk.Name,
					Namespace: vm.Namespace,
				}})
			}
		}
	}

	return requests
}
