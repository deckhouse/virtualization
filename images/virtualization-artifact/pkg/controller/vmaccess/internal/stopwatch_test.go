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
	"testing"
	"time"
)

func TestStopwatchDueOncePerInterval(t *testing.T) {
	w := newStopwatch()
	start := time.Unix(0, 0)

	if !w.due("a", start, time.Minute) {
		t.Fatal("the first call is not due")
	}

	if w.due("a", start, time.Minute) {
		t.Fatal("a second call at the same moment is due")
	}

	if w.due("a", start.Add(time.Minute-time.Second), time.Minute) {
		t.Fatal("a call within the interval is due")
	}

	if !w.due("a", start.Add(time.Minute), time.Minute) {
		t.Fatal("a call once the interval has passed is not due")
	}

	if !w.due("b", start, time.Minute) {
		t.Fatal("another claim is held back by the first one")
	}

	w.reset("a")

	if !w.due("a", start.Add(time.Minute), time.Minute) {
		t.Fatal("a call after a reset is not due")
	}
}
