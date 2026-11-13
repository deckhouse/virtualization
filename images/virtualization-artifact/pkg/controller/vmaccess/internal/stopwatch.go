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
	"time"

	"k8s.io/apimachinery/pkg/types"
)

// stopwatch measures, per claim UID, how long an observation has held since it was first seen. It is in
// memory only: a restart starts every measurement again, which delays the action waiting on it by its
// tolerance at most.
type stopwatch struct {
	mu    sync.Mutex
	since map[types.UID]time.Time
}

func newStopwatch() *stopwatch {
	return &stopwatch{since: make(map[types.UID]time.Time)}
}

// elapsed returns how long the measurement for claimUID has been running, starting it at now when it is
// not.
func (w *stopwatch) elapsed(claimUID types.UID, now time.Time) time.Duration {
	w.mu.Lock()
	defer w.mu.Unlock()

	since, ok := w.since[claimUID]
	if !ok {
		w.since[claimUID] = now
		return 0
	}

	return now.Sub(since)
}

// due reports whether interval has passed since the measurement for claimUID last started, and starts it
// again at now when it has. The first call for claimUID is due.
func (w *stopwatch) due(claimUID types.UID, now time.Time, interval time.Duration) bool {
	w.mu.Lock()
	defer w.mu.Unlock()

	if since, ok := w.since[claimUID]; ok && now.Sub(since) < interval {
		return false
	}

	w.since[claimUID] = now

	return true
}

// reset stops the measurement for claimUID.
func (w *stopwatch) reset(claimUID types.UID) {
	w.mu.Lock()
	defer w.mu.Unlock()

	delete(w.since, claimUID)
}
