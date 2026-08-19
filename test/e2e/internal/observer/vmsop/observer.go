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

// Package vmsop provides a VirtualMachineSnapshotOperation-specialized
// [observer.Observer] together with predicates ready to be used with its
// Never, Always and WaitFor primitives.
package vmsop

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmsopcondition"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	"github.com/deckhouse/virtualization/test/e2e/internal/observer"
)

// Observer is a convenience type alias for the generic Observer specialized
// for VirtualMachineSnapshotOperations.
type Observer = observer.Observer[*v1alpha2.VirtualMachineSnapshotOperation]

// Predicate is a convenience type alias for the generic Predicate specialized
// for VirtualMachineSnapshotOperations.
type Predicate = observer.Predicate[*v1alpha2.VirtualMachineSnapshotOperation]

// StartObserver starts a VirtualMachineSnapshotOperation Observer for the
// given VMSOP and registers a DeferCleanup that stops the underlying watch.
// The watch only delivers events observed after the call, so start it before
// (or right after) creating the VMSOP.
func StartObserver(ctx context.Context, f *framework.Framework, vmsop *v1alpha2.VirtualMachineSnapshotOperation) Observer {
	GinkgoHelper()

	obs, err := observer.New[*v1alpha2.VirtualMachineSnapshotOperation](
		ctx,
		f.VirtClient().VirtualMachineSnapshotOperations(vmsop.Namespace),
		vmsop.Name,
		vmsop.Namespace,
	)
	Expect(err).NotTo(HaveOccurred(), "failed to start observer for VirtualMachineSnapshotOperation %s/%s", vmsop.Namespace, vmsop.Name)

	DeferCleanup(obs.Stop)

	return obs
}

// BeCompleted is satisfied when the VMSOP reaches the Completed phase. A VMSOP
// that turns Failed can never complete anymore, so the predicate reports it as
// a definite error and WaitFor aborts immediately instead of waiting out the
// remaining timeout.
func BeCompleted() Predicate {
	return func(op *v1alpha2.VirtualMachineSnapshotOperation) (bool, error) {
		switch op.Status.Phase {
		case v1alpha2.VMSOPPhaseCompleted:
			return true, nil
		case v1alpha2.VMSOPPhaseFailed:
			return false, fmt.Errorf("vmsop %s/%s is %s: %s", op.Namespace, op.Name, op.Status.Phase, completedDetail(op))
		default:
			return false, nil
		}
	}
}

// completedDetail extracts the reason and message of the Completed condition,
// so a Failed phase surfaces its cause instead of a bare phase name.
func completedDetail(op *v1alpha2.VirtualMachineSnapshotOperation) string {
	for _, c := range op.Status.Conditions {
		if c.Type == vmsopcondition.TypeCompleted.String() {
			return fmt.Sprintf("%s: %s", c.Reason, c.Message)
		}
	}
	return "no Completed condition with details found"
}
