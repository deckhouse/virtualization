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

// Package vmbda provides a VirtualMachineBlockDeviceAttachment-specialized
// [observer.Observer] together with predicates ready to be used with its
// Never, Always and WaitFor primitives.
package vmbda

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
// for VirtualMachineBlockDeviceAttachments.
type Observer = observer.Observer[*v1alpha2.VirtualMachineBlockDeviceAttachment]

// Predicate is a convenience type alias for the generic Predicate specialized
// for VirtualMachineBlockDeviceAttachments.
type Predicate = observer.Predicate[*v1alpha2.VirtualMachineBlockDeviceAttachment]

// StartObserver starts a VirtualMachineBlockDeviceAttachment Observer for the
// given attachment and registers a DeferCleanup that stops the underlying
// watch and asserts that no Never/Always invariant was violated during the
// test. A watcher goroutine surfaces the first invariant violation through
// Ginkgo's Fail the moment it fires.
func StartObserver(ctx context.Context, f *framework.Framework, vmbda *v1alpha2.VirtualMachineBlockDeviceAttachment) Observer {
	GinkgoHelper()

	obs, err := observer.New[*v1alpha2.VirtualMachineBlockDeviceAttachment](
		ctx,
		f.VirtClient().VirtualMachineBlockDeviceAttachments(vmbda.Namespace),
		vmbda.Name,
		vmbda.Namespace,
	)
	Expect(err).NotTo(HaveOccurred(), "failed to start observer for VirtualMachineBlockDeviceAttachment %s/%s", vmbda.Namespace, vmbda.Name)

	go failFastOnInvariant(obs, fmt.Sprintf("VirtualMachineBlockDeviceAttachment %s/%s", vmbda.Namespace, vmbda.Name))

	DeferCleanup(func() {
		obs.Stop()
		Expect(obs.Err()).NotTo(HaveOccurred(),
			"VirtualMachineBlockDeviceAttachment %s/%s observer reported an invariant violation",
			vmbda.Namespace, vmbda.Name)
	})

	return obs
}

// failFastOnInvariant blocks until obs either reports an invariant violation
// or stops cleanly, and surfaces the first violation as a Ginkgo failure right
// away. It is meant to be launched in its own goroutine.
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
