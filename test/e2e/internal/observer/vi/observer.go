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
func StartObserver(ctx context.Context, f *framework.Framework, vi *v1alpha2.VirtualImage) Observer {
	GinkgoHelper()

	obs, err := observer.New[*v1alpha2.VirtualImage](
		ctx,
		f.VirtClient().VirtualImages(vi.Namespace),
		vi.Name,
		vi.Namespace,
	)
	Expect(err).NotTo(HaveOccurred(), "failed to start observer for VirtualImage %s/%s", vi.Namespace, vi.Name)

	DeferCleanup(func() {
		obs.Stop()
		Expect(obs.Err()).NotTo(HaveOccurred(),
			"VirtualImage %s/%s observer reported an invariant violation",
			vi.Namespace, vi.Name)
	})

	return obs
}
