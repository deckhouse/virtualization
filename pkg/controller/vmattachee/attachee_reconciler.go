package vmattachee

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	virtv2alpha1 "github.com/deckhouse/virtualization-controller/api/v2alpha1"
	"github.com/deckhouse/virtualization-controller/pkg/controller/common"
	"github.com/deckhouse/virtualization-controller/pkg/sdk/framework/helper"
	"github.com/deckhouse/virtualization-controller/pkg/sdk/framework/two_phase_reconciler"
)

// Attachee struct aimed to be included into the image or disk, which is attached to the VM
type AttacheeReconciler[T helper.Object[T, ST], ST any] struct {
	GroupVersionKind schema.GroupVersionKind
}

func NewAttacheeReconciler[T helper.Object[T, ST], ST any](gvk schema.GroupVersionKind) *AttacheeReconciler[T, ST] {
	return &AttacheeReconciler[T, ST]{
		GroupVersionKind: gvk,
	}
}

func (r *AttacheeReconciler[T, ST]) SetupController(_ context.Context, mgr manager.Manager, ctr controller.Controller) error {
	matchAttacheeKindFunc := func(k, _ string) bool {
		_, isAttachee := ExtractAttachedResourceName(r.GroupVersionKind.Kind, k)
		return isAttachee
	}

	if err := ctr.Watch(
		source.Kind(mgr.GetCache(), &virtv2alpha1.VirtualMachine{}),
		handler.EnqueueRequestsFromMapFunc(r.enqueueAttacheeRequestsFromVM),
		predicate.Funcs{
			CreateFunc: func(e event.CreateEvent) bool {
				return common.HasLabel(e.Object.GetLabels(), matchAttacheeKindFunc)
			},
			DeleteFunc: func(e event.DeleteEvent) bool {
				return common.HasLabel(e.Object.GetLabels(), matchAttacheeKindFunc)
			},
			UpdateFunc: func(e event.UpdateEvent) bool {
				return common.HasLabel(e.ObjectOld.GetLabels(), matchAttacheeKindFunc) ||
					common.HasLabel(e.ObjectNew.GetLabels(), matchAttacheeKindFunc)
			},
		},
	); err != nil {
		return fmt.Errorf("error setting watch on VirtualMachineInstance: %w", err)
	}

	return nil
}

func (r *AttacheeReconciler[T, ST]) enqueueAttacheeRequestsFromVM(_ context.Context, obj client.Object) (res []reconcile.Request) {
	for k := range obj.GetLabels() {
		name, isAttachee := ExtractAttachedResourceName(r.GroupVersionKind.Kind, k)
		if isAttachee {
			res = append(res, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: name},
			})
		}
	}
	return
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
