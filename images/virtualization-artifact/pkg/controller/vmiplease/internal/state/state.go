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

	"github.com/deckhouse/virtualization-controller/pkg/sdk/framework/helper"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type VMIPLeaseState interface {
	VirtualMachineIPAddressLease() *virtv2.VirtualMachineIPAddressLease
	VirtualMachineIPAddress(ctx context.Context) (*virtv2.VirtualMachineIPAddress, error)
	SetDeletion(value bool)
	ShouldDeletion() bool
}

type state struct {
	client     client.Client
	lease      *virtv2.VirtualMachineIPAddressLease
	vmip       *virtv2.VirtualMachineIPAddress
	isDeletion bool
}

func New(c client.Client, lease *virtv2.VirtualMachineIPAddressLease) VMIPLeaseState {
	return &state{client: c, lease: lease}
}

func (s *state) VirtualMachineIPAddressLease() *virtv2.VirtualMachineIPAddressLease {
	return s.lease
}

func (s *state) VirtualMachineIPAddress(ctx context.Context) (*virtv2.VirtualMachineIPAddress, error) {
	if s.vmip != nil {
		return s.vmip, nil
	}

	var err error

	if s.lease.Spec.VirtualMachineIPAddressRef != nil {
		vmipKey := types.NamespacedName{Name: s.lease.Spec.VirtualMachineIPAddressRef.Name, Namespace: s.lease.Spec.VirtualMachineIPAddressRef.Namespace}
		s.vmip, err = helper.FetchObject(ctx, vmipKey, s.client, &virtv2.VirtualMachineIPAddress{})
		if err != nil {
			return nil, fmt.Errorf("unable to get VirtualMachineIP %s: %w", vmipKey, err)
		}
	}

	return s.vmip, nil
}

func (s *state) SetDeletion(value bool) {
	s.isDeletion = value
}

func (s *state) ShouldDeletion() bool {
	return s.isDeletion
}
