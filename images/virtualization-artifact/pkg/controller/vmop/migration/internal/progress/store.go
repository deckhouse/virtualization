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
	"time"

	"k8s.io/apimachinery/pkg/types"
	utilcache "k8s.io/apimachinery/pkg/util/cache"
)

const (
	storeMaxSize = 1024
	storeTTL     = 30 * time.Minute
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
	cache *utilcache.LRUExpireCache
}

func NewStore() *Store {
	return &Store{cache: utilcache.NewLRUExpireCache(storeMaxSize)}
}

func (s *Store) Load(uid types.UID) (State, bool) {
	v, ok := s.cache.Get(uid)
	if !ok {
		return State{}, false
	}
	return v.(State), true
}

func (s *Store) Store(uid types.UID, state State) {
	if uid == "" {
		return
	}
	s.cache.Add(uid, state, storeTTL)
}

func (s *Store) Delete(uid types.UID) {
	if uid == "" {
		return
	}
	s.cache.Remove(uid)
}

func (s *Store) Len() int {
	return len(s.cache.Keys())
}
