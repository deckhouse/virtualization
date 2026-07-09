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

// Package vmop provides a VirtualMachineOperation-specialized
// [observer.Observer] together with predicates ready to be used with its
// Never, Always and WaitFor primitives.
package vmop

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmopcondition"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	"github.com/deckhouse/virtualization/test/e2e/internal/observer"
)

// Observer is a convenience type alias for the generic Observer specialized
// for VirtualMachineOperations.
type Observer = observer.Observer[*v1alpha2.VirtualMachineOperation]

// Predicate is a convenience type alias for the generic Predicate specialized
// for VirtualMachineOperations.
type Predicate = observer.Predicate[*v1alpha2.VirtualMachineOperation]

// StartObserver starts a VirtualMachineOperation Observer for the given VMOP
// and registers a DeferCleanup that stops the underlying watch. The watch only
// delivers events observed after the call, so start it before (or right after)
// creating the VMOP; for a VMOP that may already be settled, evaluate the
// current state separately.
func StartObserver(ctx context.Context, vmop *v1alpha2.VirtualMachineOperation) Observer {
	GinkgoHelper()

	obs, err := observer.New[*v1alpha2.VirtualMachineOperation](
		ctx,
		framework.GetClients().VirtClient().VirtualMachineOperations(vmop.Namespace),
		vmop.Name,
		vmop.Namespace,
	)
	Expect(err).NotTo(HaveOccurred(), "failed to start observer for VirtualMachineOperation %s/%s", vmop.Namespace, vmop.Name)

	DeferCleanup(obs.Stop)

	return obs
}

// BeCompleted is satisfied when the VMOP reaches the Completed phase. A VMOP
// that turns Failed or Superseded can never complete anymore, so the predicate
// reports it as a definite error and WaitFor aborts immediately instead of
// waiting out the remaining timeout.
func BeCompleted() Predicate {
	return func(vmop *v1alpha2.VirtualMachineOperation) (bool, error) {
		switch vmop.Status.Phase {
		case v1alpha2.VMOPPhaseCompleted:
			return true, nil
		case v1alpha2.VMOPPhaseFailed, v1alpha2.VMOPPhaseSuperseded:
			completed, _ := conditions.GetCondition(vmopcondition.TypeCompleted, vmop.Status.Conditions)
			return false, fmt.Errorf("vmop %s/%s is %s: reason: %s, message: %s",
				vmop.Namespace, vmop.Name, vmop.Status.Phase, completed.Reason, completed.Message)
		default:
			return false, nil
		}
	}
}
