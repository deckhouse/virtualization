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

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmip/internal/util"
	"github.com/deckhouse/virtualization-controller/pkg/sdk/framework/helper"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type VMIPState interface {
	VirtualMachineIP() *service.Resource[*virtv2.VirtualMachineIPAddress, virtv2.VirtualMachineIPAddressStatus]
	VirtualMachineIPLease(ctx context.Context) (*virtv2.VirtualMachineIPAddressLease, error)
	VirtualMachine(ctx context.Context) (*virtv2.VirtualMachine, error)

	AllocatedIPs() util.AllocatedIPs
}

type state struct {
	client       client.Client
	vmip         *service.Resource[*virtv2.VirtualMachineIPAddress, virtv2.VirtualMachineIPAddressStatus]
	vmipLease    *virtv2.VirtualMachineIPAddressLease
	vm           *virtv2.VirtualMachine
	allocatedIPs util.AllocatedIPs
}

func New(c client.Client, vmip *service.Resource[*virtv2.VirtualMachineIPAddress, virtv2.VirtualMachineIPAddressStatus]) VMIPState {
	return &state{client: c, vmip: vmip}
}

func (s *state) VirtualMachineIP() *service.Resource[*virtv2.VirtualMachineIPAddress, virtv2.VirtualMachineIPAddressStatus] {
	return s.vmip
}

func (s *state) VirtualMachineIPLease(ctx context.Context) (*virtv2.VirtualMachineIPAddressLease, error) {
	if s.vmipLease != nil {
		return s.vmipLease, nil
	}

	var err error

	leaseName := s.vmip.Current().Spec.VirtualMachineIPAddressLease
	if leaseName == "" {
		leaseName = util.IpToLeaseName(s.vmip.Current().Spec.Address)
	}

	if leaseName != "" {
		leaseKey := types.NamespacedName{Name: leaseName}
		s.vmipLease, err = helper.FetchObject(ctx, leaseKey, s.client, &virtv2.VirtualMachineIPAddressLease{})
		if err != nil {
			return nil, fmt.Errorf("unable to get Lease %s: %w", leaseKey, err)
		}
	}

	if s.vmipLease == nil {
		// Improve by moving the processing of AllocatingIPs to the controller level and not requesting them at each iteration of the reconciler.
		s.allocatedIPs, err = util.GetAllocatedIPs(ctx, s.client)
		if err != nil {
			return nil, err
		}
	}

	return s.vmipLease, nil
}

func (s *state) VirtualMachine(ctx context.Context) (*virtv2.VirtualMachine, error) {
	if s.vm != nil {
		return s.vm, nil
	}

	var err error
	if s.vmip.Current().Status.VirtualMachine != "" {
		vmKey := types.NamespacedName{Name: s.vmip.Current().Status.VirtualMachine, Namespace: s.vmip.Name().Namespace}
		s.vm, err = helper.FetchObject(ctx, vmKey, s.client, &virtv2.VirtualMachine{})
		if err != nil {
			return nil, fmt.Errorf("unable to get VM %s: %w", vmKey, err)
		}
	}

	if s.vm == nil {
		var vms virtv2.VirtualMachineList
		err = s.client.List(ctx, &vms, &client.ListOptions{
			Namespace: s.vmip.Name().Namespace,
		})
		if err != nil && !k8serrors.IsNotFound(err) {
			return nil, err
		}

		for _, vm := range vms.Items {
			if vm.Spec.VirtualMachineIPAddress == s.vmip.Name().Name ||
				vm.Spec.VirtualMachineIPAddress == "" && vm.Name == s.vmip.Name().Name {
				s.vm = new(virtv2.VirtualMachine)
				*s.vm = vm
				break
			}
		}
	}

	return s.vm, nil
}

func (s *state) AllocatedIPs() util.AllocatedIPs {
	return s.allocatedIPs
}
