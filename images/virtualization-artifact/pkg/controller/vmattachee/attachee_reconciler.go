package vmattachee

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/deckhouse/virtualization-controller/pkg/sdk/framework/helper"
	"github.com/deckhouse/virtualization-controller/pkg/sdk/framework/two_phase_reconciler"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type Setupper interface {
	FilterAttachedVM(vm *virtv2.VirtualMachine) bool
	EnqueueFromAttachedVM(vm *virtv2.VirtualMachine) []reconcile.Request
}

// AttacheeReconciler struct aimed to be included into the image or disk, which is attached to the VM
type AttacheeReconciler[T helper.Object[T, ST], ST any] struct{}

func NewAttacheeReconciler[T helper.Object[T, ST], ST any]() *AttacheeReconciler[T, ST] {
	return &AttacheeReconciler[T, ST]{}
}

func (r *AttacheeReconciler[T, ST]) SetupController(
	mgr manager.Manager,
	ctr controller.Controller,
	s Setupper,
) error {
	return ctr.Watch(
		source.Kind(mgr.GetCache(), &virtv2.VirtualMachine{}),
		handler.EnqueueRequestsFromMapFunc(r.enqueueRequests(s)),
		predicate.Funcs{
			CreateFunc: func(e event.CreateEvent) bool {
				return r.filterAttachedObj(s, e.Object)
			},
			DeleteFunc: func(e event.DeleteEvent) bool {
				return r.filterAttachedObj(s, e.Object)
			},
			UpdateFunc: func(e event.UpdateEvent) bool {
				return r.filterAttachedObj(s, e.ObjectOld) || r.filterAttachedObj(s, e.ObjectNew)
			},
		},
	)
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

func (r *AttacheeReconciler[T, ST]) enqueueRequests(s Setupper) handler.MapFunc {
	return func(_ context.Context, obj client.Object) []reconcile.Request {
		vm, ok := obj.(*virtv2.VirtualMachine)
		if !ok {
			return nil
		}

		return s.EnqueueFromAttachedVM(vm)
	}
}

func (r *AttacheeReconciler[T, ST]) filterAttachedObj(s Setupper, obj client.Object) bool {
	vm, ok := obj.(*virtv2.VirtualMachine)
	if !ok {
		return false
	}

	return s.FilterAttachedVM(vm)
}
