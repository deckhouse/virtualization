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

package service

import (
	"reflect"
	"sync"
	"time"

	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	defaultBaseDelay = 2 * time.Second
	defaultFactor    = 2.0
	defaultMaxDelay  = 5 * time.Minute
)

// BackoffService tracks per-object-type failure counts and calculates exponential backoff durations.
// It is safe for concurrent use. All methods are nil-safe: if the object is nil,
// the service returns the base delay instead of an error.
type BackoffService struct {
	mu sync.RWMutex
	// stores maps object-type key -> (object UID string -> failed attempts count).
	stores    map[string]map[string]int
	baseDelay time.Duration
	factor    float64
	maxDelay  time.Duration
}

type BackoffOption func(*BackoffService)

func WithBaseDelay(d time.Duration) BackoffOption {
	return func(s *BackoffService) { s.baseDelay = d }
}

func WithFactor(f float64) BackoffOption {
	return func(s *BackoffService) { s.factor = f }
}

func WithMaxDelay(d time.Duration) BackoffOption {
	return func(s *BackoffService) { s.maxDelay = d }
}

func NewBackoffService(opts ...BackoffOption) *BackoffService {
	s := &BackoffService{
		stores:    make(map[string]map[string]int),
		baseDelay: defaultBaseDelay,
		factor:    defaultFactor,
		maxDelay:  defaultMaxDelay,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// RegisterFailure increments the failure counter for the given object and returns the new count.
// If obj is nil, returns 1 (treated as a single failure).
func (s *BackoffService) RegisterFailure(obj client.Object) int {
	typeKey, objKey := objectKeys(obj)
	if typeKey == "" {
		return 1
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	store := s.stores[typeKey]
	if store == nil {
		store = make(map[string]int)
		s.stores[typeKey] = store
	}
	store[objKey]++
	return store[objKey]
}

// ResetFailures resets the failure counter for the given object.
// If obj is nil, this is a no-op.
func (s *BackoffService) ResetFailures(obj client.Object) {
	typeKey, objKey := objectKeys(obj)
	if typeKey == "" {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if store, ok := s.stores[typeKey]; ok {
		delete(store, objKey)
	}
}

// GetFailures returns the current failure count for the given object.
// If obj is nil, returns 0.
func (s *BackoffService) GetFailures(obj client.Object) int {
	typeKey, objKey := objectKeys(obj)
	if typeKey == "" {
		return 0
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	if store, ok := s.stores[typeKey]; ok {
		return store[objKey]
	}
	return 0
}

// Backoff calculates the backoff duration for the given object based on its failure count.
// If obj is nil, returns the base delay.
func (s *BackoffService) Backoff(obj client.Object) time.Duration {
	return s.calculateBackoff(s.GetFailures(obj))
}

// RegisterFailureAndBackoff increments the failure counter and returns the backoff duration.
// If obj is nil, returns the base delay.
func (s *BackoffService) RegisterFailureAndBackoff(obj client.Object) time.Duration {
	return s.calculateBackoff(s.RegisterFailure(obj))
}

// CalculateBackoff computes the backoff duration for the given failure count without modifying any state.
func (s *BackoffService) CalculateBackoff(failedCount int) time.Duration {
	return s.calculateBackoff(failedCount)
}

func (s *BackoffService) calculateBackoff(failedCount int) time.Duration {
	if failedCount == 0 {
		return 0
	}

	b := wait.Backoff{
		Duration: s.baseDelay,
		Factor:   s.factor,
		Jitter:   0,
		Cap:      s.maxDelay,
		Steps:    failedCount,
	}

	var d time.Duration
	for range failedCount {
		d = b.Step()
	}
	return d
}

func objectKeys(obj client.Object) (typeKey, objKey string) {
	if obj == nil {
		return "", ""
	}
	t := reflect.TypeOf(obj)
	if t == nil {
		return "", ""
	}
	uid := string(obj.GetUID())
	if uid == "" {
		uid = obj.GetNamespace() + "/" + obj.GetName()
	}
	return t.String(), uid
}
