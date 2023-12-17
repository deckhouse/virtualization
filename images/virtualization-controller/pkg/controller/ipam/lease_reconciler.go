package ipam

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
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
)

type LeaseReconciler struct{}

func NewLeaseReconciler() *LeaseReconciler {
	return &LeaseReconciler{}
}

func (r *LeaseReconciler) SetupController(_ context.Context, mgr manager.Manager, ctr controller.Controller) error {
	if err := ctr.Watch(
		source.Kind(mgr.GetCache(), &virtv2.VirtualMachineIPAddressClaim{}),
		handler.EnqueueRequestsFromMapFunc(r.enqueueRequestsFromClaims),
		predicate.Funcs{
			CreateFunc: func(e event.CreateEvent) bool { return false },
			DeleteFunc: func(e event.DeleteEvent) bool { return true },
			UpdateFunc: func(e event.UpdateEvent) bool { return false },
		},
	); err != nil {
		return fmt.Errorf("error setting watch on claims: %w", err)
	}

	return ctr.Watch(source.Kind(mgr.GetCache(), &virtv2.VirtualMachineIPAddressLease{}), &handler.EnqueueRequestForObject{})
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
	if state.Claim != nil {
		// Set finalizer atomically using requeue.
		controllerutil.AddFinalizer(state.Lease.Changed(), virtv2.FinalizerIPAddressLeaseCleanup)
		return nil
	}

	controllerutil.RemoveFinalizer(state.Lease.Changed(), virtv2.FinalizerIPAddressLeaseCleanup)

	switch state.Lease.Current().Spec.ReclaimPolicy {
	case virtv2.VirtualMachineIPAddressReclaimPolicyDelete, "":
		opts.Log.Info("Claim not found: remove this Lease")

		return opts.Client.Delete(ctx, state.Lease.Current())
	case virtv2.VirtualMachineIPAddressReclaimPolicyRetain:
		if state.Lease.Current().Spec.ClaimRef != nil {
			opts.Log.Info("Claim not found: remove this ref from the spec and retain Lease")

			state.Lease.Changed().Spec.ClaimRef = nil

			return opts.Client.Update(ctx, state.Lease.Changed())
		}
	default:
		return fmt.Errorf("unexpected reclaimPolicy: %s", state.Lease.Current().Spec.ReclaimPolicy)
	}

	return nil
}

func (r *LeaseReconciler) UpdateStatus(_ context.Context, _ reconcile.Request, state *LeaseReconcilerState, _ two_phase_reconciler.ReconcilerOptions) error {
	// Do nothing if object is being deleted as any update will lead to en error.
	if state.isDeletion() {
		return nil
	}

	switch {
	case state.Claim != nil:
		state.Lease.Changed().Status.Phase = virtv2.VirtualMachineIPAddressLeasePhaseBound
	case state.Lease.Current().Spec.ReclaimPolicy == virtv2.VirtualMachineIPAddressReclaimPolicyRetain:
		state.Lease.Changed().Status.Phase = virtv2.VirtualMachineIPAddressLeasePhaseReleased
	default:
		// No need to do anything: it should be already in the process of being deleted.
	}

	return nil
}
