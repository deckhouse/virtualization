package ipam

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	virtv2 "github.com/deckhouse/virtualization-controller/api/v2alpha1"
	"github.com/deckhouse/virtualization-controller/pkg/controller/common"
	"github.com/deckhouse/virtualization-controller/pkg/sdk/framework/helper"
)

type ClaimReconcilerState struct {
	Client client.Client
	Claim  *helper.Resource[*virtv2.VirtualMachineIPAddressClaim, virtv2.VirtualMachineIPAddressClaimStatus]
	Lease  *virtv2.VirtualMachineIPAddressLease

	VM *virtv2.VirtualMachine

	AllocatedIPs AllocatedIPs

	Result *reconcile.Result
}

func NewClaimReconcilerState(name types.NamespacedName, log logr.Logger, client client.Client, cache cache.Cache) *ClaimReconcilerState {
	return &ClaimReconcilerState{
		Client: client,
		Claim: helper.NewResource(
			name, log, client, cache,
			func() *virtv2.VirtualMachineIPAddressClaim {
				return &virtv2.VirtualMachineIPAddressClaim{}
			},
			func(obj *virtv2.VirtualMachineIPAddressClaim) virtv2.VirtualMachineIPAddressClaimStatus {
				return obj.Status
			},
		),
	}
}

func (state *ClaimReconcilerState) ApplySync(ctx context.Context, _ logr.Logger) error {
	if err := state.Claim.UpdateMeta(ctx); err != nil {
		return fmt.Errorf("unable to update Claim %q meta: %w", state.Claim.Name(), err)
	}
	return nil
}

func (state *ClaimReconcilerState) ApplyUpdateStatus(ctx context.Context, _ logr.Logger) error {
	return state.Claim.UpdateStatus(ctx)
}

func (state *ClaimReconcilerState) SetReconcilerResult(result *reconcile.Result) {
	state.Result = result
}

func (state *ClaimReconcilerState) GetReconcilerResult() *reconcile.Result {
	return state.Result
}

func (state *ClaimReconcilerState) Reload(ctx context.Context, req reconcile.Request, log logr.Logger, client client.Client) error {
	err := state.Claim.Fetch(ctx)
	if err != nil {
		return fmt.Errorf("unable to get %q: %w", req.NamespacedName, err)
	}

	if state.Claim.IsEmpty() {
		log.Info("Reconcile observe an absent Claim: it may be deleted", "claim.name", req.Name, "claim.namespace", req.Namespace)
		return nil
	}

	vmName := state.Claim.Current().Annotations[common.AnnBoundVirtualMachineName]
	if vmName != "" {
		vmKey := types.NamespacedName{Name: vmName, Namespace: state.Claim.Current().Namespace}
		state.VM, err = helper.FetchObject(ctx, vmKey, client, &virtv2.VirtualMachine{})
		if err != nil {
			return fmt.Errorf("unable to get VM %s: %w", vmKey, err)
		}
	}

	leaseName := state.Claim.Current().Spec.LeaseName
	if leaseName == "" {
		leaseName = ipToLeaseName(state.Claim.Current().Spec.Address)
	}
	if leaseName != "" {
		leaseKey := types.NamespacedName{Name: leaseName}
		state.Lease, err = helper.FetchObject(ctx, leaseKey, client, &virtv2.VirtualMachineIPAddressLease{})
		if err != nil {
			return fmt.Errorf("unable to get Lease %s: %w", leaseKey, err)
		}
	}

	if state.Lease == nil {
		// Improve by moving the processing of AllocatingIPs to the controller level and not requesting them at each iteration of the reconciler.
		state.AllocatedIPs, err = getAllocatedIPs(ctx, client)
		if err != nil {
			return err
		}
	}

	return nil
}

func (state *ClaimReconcilerState) ShouldReconcile(_ logr.Logger) bool {
	return !state.Claim.IsEmpty()
}

func (state *ClaimReconcilerState) isDeletion() bool {
	return state.Claim.Current().DeletionTimestamp != nil
}

type AllocatedIPs map[string]*virtv2.VirtualMachineIPAddressLease

func getAllocatedIPs(ctx context.Context, c client.Client) (AllocatedIPs, error) {
	var leases virtv2.VirtualMachineIPAddressLeaseList

	err := c.List(ctx, &leases)
	if err != nil {
		return nil, fmt.Errorf("error getting leases: %w", err)
	}

	allocatedIPs := make(AllocatedIPs, len(leases.Items))
	for _, lease := range leases.Items {
		l := lease
		allocatedIPs[leaseNameToIP(lease.Name)] = &l
	}

	return allocatedIPs, nil
}
