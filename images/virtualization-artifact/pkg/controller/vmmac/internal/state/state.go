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

	"github.com/deckhouse/virtualization-controller/pkg/common/mac"
	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/indexer"
	"github.com/deckhouse/virtualization-controller/pkg/controller/netmanager"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmmac/internal/util"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmiplcondition"
)

type VMMACState interface {
	VirtualMachineMAC() *virtv2.VirtualMachineMACAddress
	VirtualMachineMACLease(ctx context.Context) (*virtv2.VirtualMachineMACAddressLease, error)
	VirtualMachine(ctx context.Context) (*virtv2.VirtualMachine, error)

	AllocatedMACs() mac.AllocatedMACs
}

type state struct {
	client        client.Client
	mac           *virtv2.VirtualMachineMACAddress
	lease         *virtv2.VirtualMachineMACAddressLease
	vm            *virtv2.VirtualMachine
	allocatedMACs mac.AllocatedMACs
}

func New(c client.Client, vmmac *virtv2.VirtualMachineMACAddress) VMMACState {
	return &state{client: c, mac: vmmac}
}

func (s *state) VirtualMachineMAC() *virtv2.VirtualMachineMACAddress {
	return s.mac
}

func (s *state) VirtualMachineMACLease(ctx context.Context) (*virtv2.VirtualMachineMACAddressLease, error) {
	if s.lease != nil {
		return s.lease, nil
	}

	var err error

	leaseName := mac.AddressToLeaseName(s.mac.Status.Address)

	if leaseName != "" {
		leaseKey := types.NamespacedName{Name: leaseName}
		s.lease, err = object.FetchObject(ctx, leaseKey, s.client, &virtv2.VirtualMachineMACAddressLease{})
		if err != nil {
			return nil, fmt.Errorf("unable to get mac lease %s: %w", leaseKey, err)
		}
	}

	if s.lease == nil {
		var leases virtv2.VirtualMachineMACAddressLeaseList
		err = s.client.List(ctx, &leases,
			client.InNamespace(s.mac.Namespace),
			&client.MatchingFields{
				indexer.IndexFieldVMMACLeaseByVMMAC: s.mac.Name,
			})
		if err != nil {
			return nil, err
		}

		for i, lease := range leases.Items {
			boundCondition, exist := conditions.GetCondition(vmiplcondition.BoundType, lease.Status.Conditions)
			if exist && boundCondition.Status == metav1.ConditionTrue {
				s.lease = &leases.Items[i]
				break
			}
		}
	}

	if s.lease == nil {
		s.allocatedMACs, err = util.GetAllocatedMACs(ctx, s.client)
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
	if s.mac.Status.VirtualMachine != "" {
		vmKey := types.NamespacedName{Name: s.mac.Status.VirtualMachine, Namespace: s.mac.Namespace}
		vm, err := object.FetchObject(ctx, vmKey, s.client, &virtv2.VirtualMachine{})
		if err != nil {
			return nil, fmt.Errorf("unable to get VM %s: %w", vmKey, err)
		}

		if vm == nil {
			return s.vm, nil
		}

		if vm.Status.VirtualMachineIPAddress == s.mac.Name && vm.Status.MACAddress == s.mac.Status.Address {
			s.vm = vm
		}
	}

	if s.vm == nil {
		var vms virtv2.VirtualMachineList
		err = s.client.List(ctx, &vms, &client.ListOptions{
			Namespace: s.mac.Namespace,
		})
		if err != nil {
			return nil, err
		}

		for i, vm := range vms.Items {
			if vm.Spec.VirtualMachineIPAddress == s.mac.Name || vm.Spec.VirtualMachineIPAddress == "" && vm.Name == netmanager.GetVirtualMachineNameFromVMMAC(s.mac) {
				s.vm = &vms.Items[i]
				break
			}
		}
	}

	return s.vm, nil
}

func (s *state) AllocatedMACs() mac.AllocatedMACs {
	return s.allocatedMACs
}
