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

package state

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/controller/common"
	"github.com/deckhouse/virtualization-controller/pkg/controller/indexer"
	"github.com/deckhouse/virtualization-controller/pkg/controller/ipam"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmip/internal/util"
	"github.com/deckhouse/virtualization-controller/pkg/sdk/framework/helper"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmiplcondition"
)

type VMIPState interface {
	VirtualMachineIP() *virtv2.VirtualMachineIPAddress
	VirtualMachineIPLease(ctx context.Context) (*virtv2.VirtualMachineIPAddressLease, error)
	VirtualMachine(ctx context.Context) (*virtv2.VirtualMachine, error)

	AllocatedIPs() common.AllocatedIPs
}

type state struct {
	client       client.Client
	vmip         *virtv2.VirtualMachineIPAddress
	lease        *virtv2.VirtualMachineIPAddressLease
	vm           *virtv2.VirtualMachine
	allocatedIPs common.AllocatedIPs
}

func New(c client.Client, vmip *virtv2.VirtualMachineIPAddress) VMIPState {
	return &state{client: c, vmip: vmip}
}

func (s *state) VirtualMachineIP() *virtv2.VirtualMachineIPAddress {
	return s.vmip
}

func (s *state) VirtualMachineIPLease(ctx context.Context) (*virtv2.VirtualMachineIPAddressLease, error) {
	if s.lease != nil {
		return s.lease, nil
	}

	var err error

	leaseName := common.IpToLeaseName(s.vmip.Status.Address)

	if leaseName != "" {
		leaseKey := types.NamespacedName{Name: leaseName}
		s.lease, err = helper.FetchObject(ctx, leaseKey, s.client, &virtv2.VirtualMachineIPAddressLease{})
		if err != nil {
			return nil, fmt.Errorf("unable to get Lease %s: %w", leaseKey, err)
		}
	}

	if s.lease == nil {
		var leases virtv2.VirtualMachineIPAddressLeaseList
		err = s.client.List(ctx, &leases,
			client.InNamespace(s.vmip.Namespace),
			&client.MatchingFields{
				indexer.IndexFieldVMIPLeaseByVMIP: s.vmip.Name,
			})
		if err != nil {
			return nil, err
		}

		for i, lease := range leases.Items {
			boundCondition, exist := service.GetCondition(vmiplcondition.BoundType, lease.Status.Conditions)
			if exist && boundCondition.Status == metav1.ConditionTrue {
				s.lease = &leases.Items[i]
				break
			}
		}
	}

	if s.lease == nil {
		s.allocatedIPs, err = util.GetAllocatedIPs(ctx, s.client, s.vmip.Spec.Type)
		if err != nil {
			return nil, err
		}
	}

	return s.lease, nil
}

func (s *state) VirtualMachine(ctx context.Context) (*virtv2.VirtualMachine, error) {
	if s.vm != nil {
		return s.vm, nil
	}

	var err error
	if s.vmip.Status.VirtualMachine != "" {
		vmKey := types.NamespacedName{Name: s.vmip.Status.VirtualMachine, Namespace: s.vmip.Namespace}
		s.vm, err = helper.FetchObject(ctx, vmKey, s.client, &virtv2.VirtualMachine{})
		if err != nil {
			return nil, fmt.Errorf("unable to get VM %s: %w", vmKey, err)
		}
	}

	if s.vm == nil {
		var vms virtv2.VirtualMachineList
		err = s.client.List(ctx, &vms, &client.ListOptions{
			Namespace: s.vmip.Namespace,
		})
		if err != nil {
			return nil, err
		}

		for i, vm := range vms.Items {
			if vm.Spec.VirtualMachineIPAddress == s.vmip.Name ||
				vm.Spec.VirtualMachineIPAddress == "" && vm.Name == ipam.GetVirtualMachineName(s.vmip) {
				s.vm = &vms.Items[i]
				break
			}
		}
	}

	return s.vm, nil
}

func (s *state) AllocatedIPs() common.AllocatedIPs {
	return s.allocatedIPs
}
