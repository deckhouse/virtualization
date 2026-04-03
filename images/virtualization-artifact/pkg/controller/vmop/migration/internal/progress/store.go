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

package progress

import (
	"sync"
	"time"

	"k8s.io/apimachinery/pkg/types"
)

type State struct {
	Progress         int32
	Iterative        bool
	IterativeSince   time.Time
	LastUpdatedAt    time.Time
	LastMetricAt     time.Time
	LastIteration    uint32
	LastProcessedMiB float64
	LastRemainingMiB float64
}

type Store struct {
	mu     sync.RWMutex
	states map[types.UID]State
}

func NewStore() *Store {
	return &Store{states: make(map[types.UID]State)}
}

func (s *Store) Load(uid types.UID) (State, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	state, ok := s.states[uid]
	return state, ok
}

func (s *Store) Store(uid types.UID, state State) {
	if uid == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.states[uid] = state
}

func (s *Store) Delete(uid types.UID) {
	if uid == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.states, uid)
}

func (s *Store) Len() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.states)
}
