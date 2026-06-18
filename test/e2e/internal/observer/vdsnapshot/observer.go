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

// Package vdsnapshot provides a VirtualDiskSnapshot-specialized
// [observer.Observer] together with predicates ready to be used with its
// Never, Always and WaitFor primitives.
package vdsnapshot

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	"github.com/deckhouse/virtualization/test/e2e/internal/observer"
)

// Observer is a convenience type alias for the generic Observer specialized
// for VirtualDiskSnapshots.
type Observer = observer.Observer[*v1alpha2.VirtualDiskSnapshot]

// Predicate is a convenience type alias for the generic Predicate specialized
// for VirtualDiskSnapshots.
type Predicate = observer.Predicate[*v1alpha2.VirtualDiskSnapshot]

// StartObserver starts a VirtualDiskSnapshot Observer for the given snapshot
// and registers a DeferCleanup that stops the underlying watch and asserts no
// registered invariant was violated. Start it before creating the snapshot so
// the very first phase transitions are captured.
func StartObserver(ctx context.Context, f *framework.Framework, snapshot *v1alpha2.VirtualDiskSnapshot) Observer {
	GinkgoHelper()

	obs, err := observer.New[*v1alpha2.VirtualDiskSnapshot](
		ctx,
		f.VirtClient().VirtualDiskSnapshots(snapshot.Namespace),
		snapshot.Name,
		snapshot.Namespace,
	)
	Expect(err).NotTo(HaveOccurred(), "failed to start observer for VirtualDiskSnapshot %s/%s", snapshot.Namespace, snapshot.Name)

	DeferCleanup(func() {
		obs.Stop()
		Expect(obs.Err()).NotTo(HaveOccurred(),
			"VirtualDiskSnapshot %s/%s observer reported an invariant violation",
			snapshot.Namespace, snapshot.Name)
	})

	return obs
}
