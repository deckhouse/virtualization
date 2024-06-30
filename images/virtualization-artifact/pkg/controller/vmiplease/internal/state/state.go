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

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/sdk/framework/helper"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type VMIPLeaseState interface {
	VirtualMachineIPAddressLease() *service.Resource[*virtv2.VirtualMachineIPAddressLease, virtv2.VirtualMachineIPAddressLeaseStatus]
	VirtualMachineIPAddress(ctx context.Context) (*virtv2.VirtualMachineIPAddressClaim, error)
}

type state struct {
	client    client.Client
	vmipLease *service.Resource[*virtv2.VirtualMachineIPAddressLease, virtv2.VirtualMachineIPAddressLeaseStatus]
	vmip      *virtv2.VirtualMachineIPAddressClaim
}

func New(c client.Client, vmipLease *service.Resource[*virtv2.VirtualMachineIPAddressLease, virtv2.VirtualMachineIPAddressLeaseStatus]) VMIPLeaseState {
	return &state{client: c, vmipLease: vmipLease}
}

func (s *state) VirtualMachineIPAddressLease() *service.Resource[*virtv2.VirtualMachineIPAddressLease, virtv2.VirtualMachineIPAddressLeaseStatus] {
	return s.vmipLease
}

func (s *state) VirtualMachineIPAddress(ctx context.Context) (*virtv2.VirtualMachineIPAddressClaim, error) {
	if s.vmip != nil {
		return s.vmip, nil
	}

	var err error

	if s.vmipLease.Current().Spec.ClaimRef != nil {
		vmipKey := types.NamespacedName{Name: s.vmipLease.Current().Spec.ClaimRef.Name, Namespace: s.vmipLease.Current().Spec.ClaimRef.Namespace}
		s.vmip, err = helper.FetchObject(ctx, vmipKey, s.client, &virtv2.VirtualMachineIPAddressClaim{})
		if err != nil {
			return nil, fmt.Errorf("unable to get VirtualMachineIP %s: %w", vmipKey, err)
		}
	}

	return s.vmip, nil
}
