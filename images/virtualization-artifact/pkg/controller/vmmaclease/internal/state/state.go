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

	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type VMMACLeaseState interface {
	VirtualMachineMACAddressLease() *virtv2.VirtualMachineMACAddressLease
	VirtualMachineMACAddress(ctx context.Context) (*virtv2.VirtualMachineMACAddress, error)
	SetDeletion(value bool)
	ShouldDeletion() bool
}

type state struct {
	client     client.Client
	lease      *virtv2.VirtualMachineMACAddressLease
	mac        *virtv2.VirtualMachineMACAddress
	isDeletion bool
}

func New(c client.Client, lease *virtv2.VirtualMachineMACAddressLease) VMMACLeaseState {
	return &state{client: c, lease: lease}
}

func (s *state) VirtualMachineMACAddressLease() *virtv2.VirtualMachineMACAddressLease {
	return s.lease
}

func (s *state) VirtualMachineMACAddress(ctx context.Context) (*virtv2.VirtualMachineMACAddress, error) {
	if s.mac != nil {
		return s.mac, nil
	}

	var err error

	if s.lease.Spec.VirtualMachineMACAddressRef != nil {
		macKey := types.NamespacedName{Name: s.lease.Spec.VirtualMachineMACAddressRef.Name, Namespace: s.lease.Spec.VirtualMachineMACAddressRef.Namespace}
		s.mac, err = object.FetchObject(ctx, macKey, s.client, &virtv2.VirtualMachineMACAddress{})
		if err != nil {
			return nil, fmt.Errorf("unable to get VirtualMachineMAC %s: %w", macKey, err)
		}
	}

	return s.mac, nil
}

func (s *state) SetDeletion(value bool) {
	s.isDeletion = value
}

func (s *state) ShouldDeletion() bool {
	return s.isDeletion
}
