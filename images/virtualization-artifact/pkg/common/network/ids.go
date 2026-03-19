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

package network

import (
	"k8s.io/utils/ptr"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

const (
	ReservedMainID = 1
	StartGenericID = 2
)

// EnsureNetworkInterfaceIDs sets missing IDs in VM networks according to module rules:
// - Main network gets id=1 when omitted.
// - Other networks get the next available IDs starting from 2.
//
// Returns true when at least one ID was assigned.
func EnsureNetworkInterfaceIDs(networks []v1alpha2.NetworksSpec) bool {
	if len(networks) == 0 {
		return false
	}

	changed := false
	allocator := NewInterfaceIDAllocator()

	for i := range networks {
		if networks[i].Type == v1alpha2.NetworksTypeMain && networks[i].ID == nil {
			v := ReservedMainID
			networks[i].ID = &v
			changed = true
			break
		}
	}

	for _, net := range networks {
		allocator.Reserve(ptr.Deref(net.ID, 0))
	}

	for i := range networks {
		if networks[i].ID == nil {
			id := allocator.NextAvailable()
			networks[i].ID = &id
			changed = true
		}
	}

	return changed
}

type InterfaceIDAllocator struct {
	used   map[int]bool
	cursor int
}

func NewInterfaceIDAllocator() *InterfaceIDAllocator {
	return &InterfaceIDAllocator{
		used:   make(map[int]bool),
		cursor: StartGenericID,
	}
}

func (a *InterfaceIDAllocator) Reserve(id int) {
	if id > 0 {
		a.used[id] = true
	}
}

func (a *InterfaceIDAllocator) NextAvailable() int {
	for {
		if a.cursor == ReservedMainID {
			a.cursor++
			continue
		}

		if !a.used[a.cursor] {
			id := a.cursor
			a.used[id] = true
			a.cursor++
			return id
		}
		a.cursor++
	}
}
