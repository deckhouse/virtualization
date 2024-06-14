/*
Copyright 2024 Flant JSC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

     http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package ipam

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/sdk/framework/helper"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
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

func (state *ClaimReconcilerState) Reload(ctx context.Context, req reconcile.Request, log logr.Logger, apiClient client.Client) error {
	err := state.Claim.Fetch(ctx)
	if err != nil {
		return fmt.Errorf("unable to get %q: %w", req.NamespacedName, err)
	}

	if state.Claim.IsEmpty() {
		log.Info("Reconcile observe an absent Claim: it may be deleted", "claim.name", req.Name, "claim.namespace", req.Namespace)
		return nil
	}

	if state.Claim.Current().Status.VirtualMachine != "" {
		vmKey := types.NamespacedName{Name: state.Claim.Current().Status.VirtualMachine, Namespace: state.Claim.Name().Namespace}
		state.VM, err = helper.FetchObject(ctx, vmKey, apiClient, &virtv2.VirtualMachine{})
		if err != nil {
			return fmt.Errorf("unable to get VM %s: %w", vmKey, err)
		}
	}

	if state.VM == nil {
		var vms virtv2.VirtualMachineList
		err = apiClient.List(ctx, &vms, &client.ListOptions{
			Namespace: state.Claim.Name().Namespace,
		})
		if err != nil && !k8serrors.IsNotFound(err) {
			return err
		}

		for _, vm := range vms.Items {
			if vm.Spec.VirtualMachineIPAddressClaim == state.Claim.Name().Name ||
				vm.Spec.VirtualMachineIPAddressClaim == "" && vm.Name == state.Claim.Name().Name {
				state.VM = new(virtv2.VirtualMachine)
				*state.VM = vm
				break
			}
		}
	}

	leaseName := state.Claim.Current().Spec.VirtualMachineIPAddressLease
	if leaseName == "" {
		leaseName = ipToLeaseName(state.Claim.Current().Spec.Address)
	}
	if leaseName != "" {
		leaseKey := types.NamespacedName{Name: leaseName}
		state.Lease, err = helper.FetchObject(ctx, leaseKey, apiClient, &virtv2.VirtualMachineIPAddressLease{})
		if err != nil {
			return fmt.Errorf("unable to get Lease %s: %w", leaseKey, err)
		}
	}

	if state.Lease == nil {
		// Improve by moving the processing of AllocatingIPs to the controller level and not requesting them at each iteration of the reconciler.
		state.AllocatedIPs, err = getAllocatedIPs(ctx, apiClient)
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

func getAllocatedIPs(ctx context.Context, apiClient client.Client) (AllocatedIPs, error) {
	var leases virtv2.VirtualMachineIPAddressLeaseList

	err := apiClient.List(ctx, &leases)
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
