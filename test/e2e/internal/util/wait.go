/*
Copyright 2025 Flant JSC

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

package util

import (
	"context"
	"errors"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type Watcher interface {
	Watch(ctx context.Context, opts metav1.ListOptions) (watch.Interface, error)
}

type Resource interface {
	*v1alpha2.VirtualMachineIPAddress | *v1alpha2.VirtualMachineIPAddressLease | *v1alpha2.VirtualMachine | *v1alpha2.VirtualDisk
}

type EventHandler[R Resource] func(eventType watch.EventType, r R) (bool, error)

// WaitFor waits for a specific event by listening to events related to a resource. WaitFor helps avoid race conditions in test cases when a certain condition may occur very quickly and cannot always be caught using Eventually.
//
// For example,
// the `Migrating` phase of a virtual machine during migration may appear for only about 400 ms and then disappear immediately,
// making it impossible to catch with Eventually, which checks at one-second intervals.
// In this scenario, it’s better to use WaitFor.
//
// Another example:
// if disks were created for a test case and you need to wait until they reach the `Ready` phase.
// In this case, since the disk remains in the Ready phase once it gets there, it’s preferable to use Eventually.
func WaitFor[R Resource](ctx context.Context, w Watcher, h EventHandler[R], opts metav1.ListOptions) (R, error) {
	var zero R
	wi, err := w.Watch(ctx, opts)
	if err != nil {
		return zero, err
	}

	defer wi.Stop()

	for event := range wi.ResultChan() {
		r, ok := event.Object.(R)
		if !ok {
			return zero, errors.New("conversion error")
		}

		ok, err = h(event.Type, r)
		if err != nil {
			return zero, err
		}

		if ok {
			return r, nil
		}
	}

	return zero, fmt.Errorf("the condition for matching was not successfully met: %w", ctx.Err())
}
