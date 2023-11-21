package ipam

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
	"github.com/deckhouse/virtualization-controller/pkg/sdk/framework/two_phase_reconciler"
)

type LeaseReconciler struct{}

func NewLeaseReconciler() *LeaseReconciler {
	return &LeaseReconciler{}
}

func (r *LeaseReconciler) SetupController(_ context.Context, mgr manager.Manager, ctr controller.Controller) error {
	if err := ctr.Watch(
		source.Kind(mgr.GetCache(), &virtv2.VirtualMachineIPAddressClaim{}),
		handler.EnqueueRequestsFromMapFunc(r.enqueueRequestsFromClaims),
	); err != nil {
		return fmt.Errorf("error setting watch on claims: %w", err)
	}

	return ctr.Watch(source.Kind(mgr.GetCache(), &virtv2.VirtualMachineIPAddressLease{}), &handler.EnqueueRequestForObject{},
		predicate.Funcs{
			CreateFunc: func(e event.CreateEvent) bool { return true },
			DeleteFunc: func(e event.DeleteEvent) bool { return true },
			UpdateFunc: func(e event.UpdateEvent) bool { return true },
		},
	)
}

func (r *LeaseReconciler) enqueueRequestsFromClaims(_ context.Context, obj client.Object) []reconcile.Request {
	claim, ok := obj.(*virtv2.VirtualMachineIPAddressClaim)
	if !ok {
		return nil
	}

	if claim.Spec.LeaseName == "" {
		return nil
	}

	return []reconcile.Request{
		{
			NamespacedName: types.NamespacedName{
				Name: claim.Spec.LeaseName,
			},
		},
	}
}

func (r *LeaseReconciler) Sync(ctx context.Context, _ reconcile.Request, state *LeaseReconcilerState, opts two_phase_reconciler.ReconcilerOptions) error {
	if state.Claim == nil && state.Lease.Current().Spec.ClaimRef != nil {
		state.Lease.Changed().Spec.ClaimRef = nil

		return opts.Client.Update(ctx, state.Lease.Changed())
	}

	return nil
}

func (r *LeaseReconciler) UpdateStatus(_ context.Context, _ reconcile.Request, state *LeaseReconcilerState, _ two_phase_reconciler.ReconcilerOptions) error {
	// Do nothing if object is being deleted as any update will lead to en error.
	if state.isDeletion() {
		return nil
	}

	leaseStatus := state.Lease.Current().Status.DeepCopy()

	switch {
	case state.Claim != nil:
		leaseStatus.Phase = virtv2.VirtualMachineIPAddressLeasePhaseBound
	default:
		leaseStatus.Phase = virtv2.VirtualMachineIPAddressLeasePhaseReleased
	}

	state.Lease.Changed().Status = *leaseStatus

	return nil
}
