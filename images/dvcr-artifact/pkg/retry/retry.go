/*
Copyright 2024 Flant JSC

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

package retry

import (
	"context"
	"fmt"
	"time"
)

// Fn is a func to retry.
type Fn func(ctx context.Context) error

// Retry retries a given function, f, using exponential backoff.
// If the predicate is never satisfied, it will return the
// last error returned by f.
func Retry(ctx context.Context, f Fn) error {
	if f == nil {
		return fmt.Errorf("nil f passed to retry")
	}

	return ExponentialBackoff(ctx, f, defaultBackoff)
}

// Backoff with delays increasing with each step. Factor of 3 will overflow the Cap
// in around 5 steps (1s -> 3s -> 9s -> 27s -> 1m21s (trimmed to Cap of 1m)) and then wait for another 15 minutes more.
// Small delays at the start should cover networking blips and ~17 minutes overall
// timeout should be enough to survive dvcr cleanup procedure.
var defaultBackoff = Backoff{
	Duration: time.Second,
	Factor:   3.0,
	Jitter:   0.1,
	Steps:    20,
	Cap:      time.Minute,
}
