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

package observer

import (
	"context"
	"errors"
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
)

// WaitForFirst blocks until the watch delivers an object satisfying match and
// returns it. It is the discovery counterpart of [New]: New observes a single
// (name, namespace) pair, while WaitForFirst finds an object whose name the
// caller cannot know in advance (e.g. a VMOP created by a controller with a
// generated name).
//
// The watch is started with an unset resourceVersion, so the current state
// arrives first as synthetic Added events — an object that already satisfies
// match is found without an extra list. Deleted events are ignored.
func WaitForFirst[T Object](
	ctx context.Context,
	w Watcher,
	timeout time.Duration,
	match func(T) bool,
) (T, error) {
	var zero T
	if w == nil {
		return zero, errors.New("observer: watcher is nil")
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	wi, err := w.Watch(ctx, metav1.ListOptions{})
	if err != nil {
		return zero, fmt.Errorf("observer: start watch: %w", err)
	}
	defer wi.Stop()

	for {
		select {
		case <-ctx.Done():
			return zero, fmt.Errorf("observer: WaitForFirst timed out after %s: %w", timeout, ctx.Err())
		case event, ok := <-wi.ResultChan():
			if !ok {
				return zero, errors.New("observer: watch closed before a matching object was observed")
			}
			if event.Type == watch.Deleted {
				continue
			}
			obj, ok := event.Object.(T)
			if !ok {
				continue
			}
			if match(obj) {
				return obj, nil
			}
		}
	}
}
