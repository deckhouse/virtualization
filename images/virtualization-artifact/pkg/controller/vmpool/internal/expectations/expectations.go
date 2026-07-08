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

// Package expectations provides an in-memory, thread-safe tracker of pending
// child-object creations and deletions for the pool controller, modelled on the
// battle-tested Kubernetes ReplicaSet UIDTrackingControllerExpectations.
//
// A controller that creates anonymous children (via GenerateName) cannot rely
// on its informer cache being up to date within a single reconcile: right after
// Create/Delete the cache still shows the old set, so the next reconcile would
// recompute the same diff and act again, overshooting. Expectations close that
// gap: after acting, the controller records how many creations/deletions it
// expects to observe; it does not act again for the same key until those
// expectations are Satisfied — either observed through the informer, or expired
// by TTL as a safety valve against a lost watch event.
//
// Creations are tracked as a counter because the child UID is unknown until the
// API server assigns it. Deletions are tracked by UID so a duplicate delete
// event (or a delete of an object we did not expect) cannot wrongly satisfy an
// expectation.
package expectations

import (
	"sync"
	"time"

	"k8s.io/apimachinery/pkg/types"
)

// DefaultTTL is how long an unmet expectation is honoured before it is treated
// as satisfied. It mirrors the Kubernetes ExpectationsTimeout: long enough to
// ride out normal informer lag, short enough that a lost watch event cannot
// wedge the controller forever.
const DefaultTTL = 5 * time.Minute

// Expectations tracks, per controller key, the number of child creations and
// the set of child deletions the controller is still waiting to observe.
//
// All methods are safe for concurrent use.
type Expectations struct {
	mu    sync.Mutex
	items map[string]*item
	ttl   time.Duration
	// now is injectable so tests can control TTL expiry deterministically.
	now func() time.Time
}

type item struct {
	creations int
	deletions map[types.UID]struct{}
	timestamp time.Time
}

// New returns an Expectations tracker with the default TTL.
func New() *Expectations {
	return NewWithTTL(DefaultTTL)
}

// NewWithTTL returns an Expectations tracker with a custom TTL.
func NewWithTTL(ttl time.Duration) *Expectations {
	return &Expectations{
		items: make(map[string]*item),
		ttl:   ttl,
		now:   time.Now,
	}
}

// getOrCreate must be called with the mutex held.
func (e *Expectations) getOrCreate(key string) *item {
	it, ok := e.items[key]
	if !ok {
		it = &item{deletions: make(map[types.UID]struct{})}
		e.items[key] = it
	}
	return it
}

// ExpectCreations records that the controller has just created (or is about to
// create) n children for key and expects to observe n creation events. It
// resets the expectation's timestamp.
func (e *Expectations) ExpectCreations(key string, n int) {
	if n <= 0 {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	it := e.getOrCreate(key)
	it.creations += n
	it.timestamp = e.now()
}

// ExpectDeletions records that the controller has just deleted the children
// with the given UIDs for key and expects to observe their deletion events. It
// resets the expectation's timestamp.
func (e *Expectations) ExpectDeletions(key string, uids ...types.UID) {
	if len(uids) == 0 {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	it := e.getOrCreate(key)
	for _, uid := range uids {
		it.deletions[uid] = struct{}{}
	}
	it.timestamp = e.now()
}

// CreationObserved records that one expected creation for key has been observed
// through the informer. Surplus observations (more than expected) are ignored,
// keeping the counter from going negative.
func (e *Expectations) CreationObserved(key string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	it, ok := e.items[key]
	if !ok {
		return
	}
	if it.creations > 0 {
		it.creations--
	}
}

// DeletionObserved records that the child with the given UID has been observed
// deleted through the informer. Only UIDs the controller expected are cleared,
// so duplicate or unrelated delete events do not satisfy an expectation.
func (e *Expectations) DeletionObserved(key string, uid types.UID) {
	e.mu.Lock()
	defer e.mu.Unlock()
	it, ok := e.items[key]
	if !ok {
		return
	}
	delete(it.deletions, uid)
}

// Satisfied reports whether the controller may act on key again. It is true
// when there is no tracked expectation, when all expected creations and
// deletions have been observed, or when the expectation has outlived the TTL
// (the safety valve against a lost watch event).
func (e *Expectations) Satisfied(key string) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	it, ok := e.items[key]
	if !ok {
		return true
	}
	if it.creations <= 0 && len(it.deletions) == 0 {
		return true
	}
	return e.now().Sub(it.timestamp) >= e.ttl
}

// Forget drops all expectations for key. Call it when the controlled object is
// deleted so its entry does not leak.
func (e *Expectations) Forget(key string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	delete(e.items, key)
}
