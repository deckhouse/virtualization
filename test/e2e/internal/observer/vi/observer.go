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

// Package vi provides a VirtualImage-specialized [observer.Observer] together
// with a curated set of predicates ready to be used with its Never, Always
// and WaitFor primitives.
package vi

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	"github.com/deckhouse/virtualization/test/e2e/internal/observer"
)

// Observer is a convenience type alias for the generic Observer specialized
// for VirtualImages.
type Observer = observer.Observer[*v1alpha2.VirtualImage]

// Predicate is a convenience type alias for the generic Predicate specialized
// for VirtualImages.
type Predicate = observer.Predicate[*v1alpha2.VirtualImage]

// StartObserver starts a VirtualImage Observer for the given image and
// registers a DeferCleanup that:
//
//  1. stops the underlying watch, releasing the watcher resources;
//  2. asserts that no Never/Always invariant registered on the observer was
//     violated during the test.
//
// The watch is started before the caller creates the VirtualImage, ensuring
// that the very first phase transitions are captured and that any live
// invariants registered on the returned observer see every emitted event.
//
// In addition to the deferred assertion, a watcher goroutine surfaces the
// very first Never/Always violation through Ginkgo's Fail the moment it
// fires, so the test fails at the precise instant of the breach instead of
// blocking on a subsequent unrelated WaitFor and only reporting the
// violation in DeferCleanup.
func StartObserver(ctx context.Context, f *framework.Framework, vi *v1alpha2.VirtualImage) Observer {
	GinkgoHelper()

	obs, err := observer.New[*v1alpha2.VirtualImage](
		ctx,
		f.VirtClient().VirtualImages(vi.Namespace),
		vi.Name,
		vi.Namespace,
	)
	Expect(err).NotTo(HaveOccurred(), "failed to start observer for VirtualImage %s/%s", vi.Namespace, vi.Name)

	go failFastOnInvariant(obs, fmt.Sprintf("VirtualImage %s/%s", vi.Namespace, vi.Name))

	DeferCleanup(func() {
		obs.Stop()
		Expect(obs.Err()).NotTo(HaveOccurred(),
			"VirtualImage %s/%s observer reported an invariant violation",
			vi.Namespace, vi.Name)
	})

	return obs
}

// failFastOnInvariant blocks until obs either reports an invariant
// violation or stops cleanly, and surfaces the first violation as a
// Ginkgo failure right away. It is meant to be launched in its own
// goroutine; defer GinkgoRecover() lets Fail's panic be captured by
// Ginkgo even though we are off the spec's main goroutine.
func failFastOnInvariant(obs Observer, label string) {
	defer GinkgoRecover()
	select {
	case <-obs.InvariantViolated():
	case <-obs.Stopped():
	}
	if err := obs.Err(); err != nil {
		Fail(fmt.Sprintf("%s observer reported an invariant violation: %s", label, err))
	}
}
