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
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

const (
	MinID = 1
	MaxID = 16*1024 - 1 // 16383
)

// EnsureNetworkInterfaceIDs sets missing IDs in VM networks: every network without an ID
// takes the lowest free ID, walking the list in order and starting from MinID. The Main
// network is not special here: it gets 1 only when it is listed first and 1 is free, so an
// additional network that already holds 1 does not block adding Main later.
//
// Returns true when at least one ID was assigned.
func EnsureNetworkInterfaceIDs(networks []v1alpha2.NetworksSpec) bool {
	if len(networks) == 0 {
		return false
	}

	changed := false
	used := make(map[int]struct{}, len(networks))

	for i := range networks {
		if networks[i].ID != nil && *networks[i].ID > 0 {
			used[*networks[i].ID] = struct{}{}
		}
	}

	nextID := MinID
	for i := range networks {
		if networks[i].ID != nil {
			continue
		}

		id, ok := allocateNextID(used, nextID)
		if !ok {
			return changed
		}

		networks[i].ID = &id
		nextID = id + 1
		changed = true
	}

	return changed
}

func allocateNextID(used map[int]struct{}, nextID int) (int, bool) {
	for id := nextID; id <= MaxID; id++ {
		if _, exists := used[id]; exists {
			continue
		}

		used[id] = struct{}{}
		return id, true
	}

	return 0, false
}
