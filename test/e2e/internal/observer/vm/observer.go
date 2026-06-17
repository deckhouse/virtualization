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

// Package vm provides a VirtualMachine-specialized [observer.Observer] together
// with a curated set of predicates ready to be used with its Never, Always and
// WaitFor primitives.
package vm

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	"github.com/deckhouse/virtualization/test/e2e/internal/observer"
)

// Observer is a convenience type alias for the generic Observer specialized
// for VirtualMachines.
type Observer = observer.Observer[*v1alpha2.VirtualMachine]

// Predicate is a convenience type alias for the generic Predicate specialized
// for VirtualMachines.
type Predicate = observer.Predicate[*v1alpha2.VirtualMachine]

// StartObserver starts a VirtualMachine Observer for the given machine and
// registers a DeferCleanup that:
//
//  1. stops the underlying watch, releasing the watcher resources;
//  2. asserts that no Never/Always invariant registered on the observer was
//     violated during the test.
//
// Unlike the VirtualDisk/VirtualImage observers, VirtualMachines created in the
// e2e suite use generateName, so their name is only known after creation. The
// caller is therefore expected to create the VirtualMachine first and start the
// observer afterwards; the initial watch event still carries the current state.
func StartObserver(ctx context.Context, f *framework.Framework, vm *v1alpha2.VirtualMachine) Observer {
	GinkgoHelper()

	obs, err := observer.New[*v1alpha2.VirtualMachine](
		ctx,
		f.VirtClient().VirtualMachines(vm.Namespace),
		vm.Name,
		vm.Namespace,
	)
	Expect(err).NotTo(HaveOccurred(), "failed to start observer for VirtualMachine %s/%s", vm.Namespace, vm.Name)

	DeferCleanup(func() {
		obs.Stop()
		Expect(obs.Err()).NotTo(HaveOccurred(),
			"VirtualMachine %s/%s observer reported an invariant violation",
			vm.Namespace, vm.Name)
	})

	return obs
}
