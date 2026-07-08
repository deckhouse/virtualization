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

package backoff

import "time"

// Progressive returns a requeue interval that grows with how long a resource
// has already spent in a state: it stays at base until elapsed exceeds base,
// then tracks elapsed (so each requeue lands at roughly double the time-in-state),
// capped at max.
//
// It is stateless: pass elapsed = time.Since(condition.LastTransitionTime) and
// the backoff survives controller restarts without any in-memory bookkeeping.
func Progressive(elapsed, base, max time.Duration) time.Duration {
	switch {
	case elapsed < base:
		return base
	case elapsed > max:
		return max
	default:
		return elapsed
	}
}
