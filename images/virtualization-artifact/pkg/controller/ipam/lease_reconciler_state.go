package ipam

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	virtv2 "github.com/deckhouse/virtualization-controller/api/core/v1alpha2"
	"github.com/deckhouse/virtualization-controller/pkg/sdk/framework/helper"
)

type LeaseReconcilerState struct {
	Client client.Client
	Lease  *helper.Resource[*virtv2.VirtualMachineIPAddressLease, virtv2.VirtualMachineIPAddressLeaseStatus]
	Claim  *virtv2.VirtualMachineIPAddressClaim

	Result *reconcile.Result
}

func NewLeaseReconcilerState(name types.NamespacedName, log logr.Logger, client client.Client, cache cache.Cache) *LeaseReconcilerState {
	return &LeaseReconcilerState{
		Client: client,
		Lease: helper.NewResource(
			name, log, client, cache,
			func() *virtv2.VirtualMachineIPAddressLease {
				return &virtv2.VirtualMachineIPAddressLease{}
			},
			func(obj *virtv2.VirtualMachineIPAddressLease) virtv2.VirtualMachineIPAddressLeaseStatus {
				return obj.Status
			},
		),
	}
}

func (state *LeaseReconcilerState) ApplySync(ctx context.Context, _ logr.Logger) error {
	if err := state.Lease.UpdateMeta(ctx); err != nil {
		return fmt.Errorf("unable to update Lease %q meta: %w", state.Lease.Name(), err)
	}
	return nil
}

func (state *LeaseReconcilerState) ApplyUpdateStatus(ctx context.Context, _ logr.Logger) error {
	return state.Lease.UpdateStatus(ctx)
}

func (state *LeaseReconcilerState) SetReconcilerResult(result *reconcile.Result) {
	state.Result = result
}

func (state *LeaseReconcilerState) GetReconcilerResult() *reconcile.Result {
	return state.Result
}

func (state *LeaseReconcilerState) Reload(ctx context.Context, req reconcile.Request, log logr.Logger, client client.Client) error {
	err := state.Lease.Fetch(ctx)
	if err != nil {
		return fmt.Errorf("unable to get %q: %w", req.NamespacedName, err)
	}

	if state.Lease.IsEmpty() {
		log.Info("Reconcile observe an absent Lease: it may be deleted", "lease.name", req.Name, "lease.namespace", req.Namespace)
		return nil
	}

	if state.Lease.Current().Spec.ClaimRef != nil {
		claimKey := types.NamespacedName{Name: state.Lease.Current().Spec.ClaimRef.Name, Namespace: state.Lease.Current().Spec.ClaimRef.Namespace}
		state.Claim, err = helper.FetchObject(ctx, claimKey, client, &virtv2.VirtualMachineIPAddressClaim{})
		if err != nil {
			return fmt.Errorf("unable to get Claim %s: %w", claimKey, err)
		}
	}

	return nil
}

func (state *LeaseReconcilerState) ShouldReconcile(_ logr.Logger) bool {
	return !state.Lease.IsEmpty()
}

func (state *LeaseReconcilerState) isDeletion() bool {
	return state.Lease.Current().DeletionTimestamp != nil
}
