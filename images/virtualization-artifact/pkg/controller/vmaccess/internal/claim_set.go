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

package internal

import (
	"sync"

	"k8s.io/apimachinery/pkg/types"
)

// claimSet holds a set of claim UIDs, safe for concurrent passes.
type claimSet struct {
	mu   sync.Mutex
	uids map[types.UID]struct{}
}

func newClaimSet() *claimSet {
	return &claimSet{uids: make(map[types.UID]struct{})}
}

func (s *claimSet) has(claimUID types.UID) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, ok := s.uids[claimUID]

	return ok
}

func (s *claimSet) add(claimUID types.UID) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.uids[claimUID] = struct{}{}
}

func (s *claimSet) remove(claimUID types.UID) {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.uids, claimUID)
}
