/*
Copyright 2026 Flant JSC

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

package defaulter

import (
	"context"

	"github.com/deckhouse/virtualization-controller/pkg/common/network"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type NetworksDefaulter struct{}

func NewNetworksDefaulter() *NetworksDefaulter {
	return &NetworksDefaulter{}
}

func (d *NetworksDefaulter) Default(_ context.Context, vm *v1alpha2.VirtualMachine) error {
	networks := vm.Spec.Networks
	allocator := network.NewInterfaceIDAllocator()

	ensureMainNetworkID(networks)

	for _, net := range networks {
		allocator.Reserve(net.ID)
	}

	assignMissingIDs(networks, allocator)
	return nil
}

func ensureMainNetworkID(networks []v1alpha2.NetworksSpec) {
	for i := range networks {
		if networks[i].Type == v1alpha2.NetworksTypeMain && networks[i].ID == 0 {
			networks[i].ID = network.ReservedMainID
			return
		}
	}
}

func assignMissingIDs(networks []v1alpha2.NetworksSpec, allocator *network.InterfaceIDAllocator) {
	for i := range networks {
		if networks[i].ID == 0 {
			networks[i].ID = allocator.NextAvailable()
		}
	}
}
