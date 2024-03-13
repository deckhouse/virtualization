package cpu

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	virtv2 "github.com/deckhouse/virtualization-controller/api/v1alpha2"

	"github.com/deckhouse/virtualization-controller/pkg/sdk/framework/two_phase_reconciler"
)

type VMCPUReconciler struct{}

func NewVMCPUReconciler() *VMCPUReconciler {
	return &VMCPUReconciler{}
}

func (r *VMCPUReconciler) SetupController(_ context.Context, mgr manager.Manager, ctr controller.Controller) error {
	return ctr.Watch(source.Kind(mgr.GetCache(), &virtv2.VirtualMachineCPUModel{}), &handler.EnqueueRequestForObject{},
		predicate.Funcs{
			CreateFunc: func(e event.CreateEvent) bool { return true },
			DeleteFunc: func(e event.DeleteEvent) bool { return true },
			UpdateFunc: func(e event.UpdateEvent) bool { return true },
		},
	)
}

func (r *VMCPUReconciler) Sync(ctx context.Context, _ reconcile.Request, state *VMCPUReconcilerState, opts two_phase_reconciler.ReconcilerOptions) error {
	// TODO(VMCPU): check if requested type is available.
	// TODO(VMCPU): check if requested model is available.
	// kubectl get nodes virtlab-pt-1 -o json | jq '.metadata.labels | to_entries | .[] | select(.key | test("cpu-model.node.kubevirt.io"))' -c
	// TODO(VMCPU): check if requested features are available.
	// kubectl get nodes virtlab-pt-1 -o json | jq '.metadata.labels | to_entries | .[] | select(.key | test("cpu-feature.node.kubevirt.io"))' -c

	return nil
}

func (r *VMCPUReconciler) UpdateStatus(_ context.Context, _ reconcile.Request, state *VMCPUReconcilerState, _ two_phase_reconciler.ReconcilerOptions) error {
	// Do nothing if object is being deleted as any update will lead to en error.
	if state.isDeletion() {
		return nil
	}

	return nil
}
