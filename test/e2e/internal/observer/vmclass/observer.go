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

// Package vmclass provides a VirtualMachineClass-specialized
// [observer.Observer] together with predicates ready to be used with its
// Never, Always and WaitFor primitives.
package vmclass

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	"github.com/deckhouse/virtualization/test/e2e/internal/observer"
)

// Observer is a convenience type alias for the generic Observer specialized
// for VirtualMachineClasses.
type Observer = observer.Observer[*v1alpha2.VirtualMachineClass]

// Predicate is a convenience type alias for the generic Predicate specialized
// for VirtualMachineClasses.
type Predicate = observer.Predicate[*v1alpha2.VirtualMachineClass]

// StartObserver starts a VirtualMachineClass Observer (a cluster-scoped
// resource) for the given class and registers a DeferCleanup that stops the
// underlying watch. Start it before creating the class so the very first
// phase transitions are captured.
func StartObserver(ctx context.Context, f *framework.Framework, class *v1alpha2.VirtualMachineClass) Observer {
	GinkgoHelper()

	obs, err := observer.New[*v1alpha2.VirtualMachineClass](
		ctx,
		f.VirtClient().VirtualMachineClasses(),
		class.Name,
		"",
	)
	Expect(err).NotTo(HaveOccurred(), "failed to start observer for VirtualMachineClass %s", class.Name)

	DeferCleanup(obs.Stop)

	return obs
}

// BeReady reports the VirtualMachineClass has reached the Ready phase.
// Intended for use with [Observer.WaitFor].
func BeReady() Predicate {
	return func(c *v1alpha2.VirtualMachineClass) (bool, error) {
		return c.Status.Phase == v1alpha2.ClassPhaseReady, nil
	}
}
