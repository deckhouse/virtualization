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

package util

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework/failfast"
	vmopobserver "github.com/deckhouse/virtualization/test/e2e/internal/observer/vmop"
)

// UntilVMOPMigrationSucceeded waits for the migration VMOP to complete using a VMOP observer
// with the BeCompleted predicate. A VMOP that turns Failed or Superseded fails the test
// immediately instead of waiting out the remaining timeout; the known-issue skips are checked
// before failing.
func UntilVMOPMigrationSucceeded(ctx context.Context, vmop *v1alpha2.VirtualMachineOperation, timeout time.Duration) {
	GinkgoHelper()

	if err := untilVMOPCompleted(ctx, vmop, timeout); err != nil {
		//nolint:contextcheck // the skip checks intentionally use a fresh context, see skipIfKnownMigrationIssue
		failVMOPMigration(vmop, err)
	}
}

// untilVMOPCompleted waits for the VMOP to complete and returns the failure
// instead of failing the spec, so callers can retry known transient outcomes.
//
// A migration can wedge with the VMOP forever Pending/InProgress while the
// real error sits on another object (a KVVM sync denied by an admission
// webhook, a terminally failed migration attempt on the internal VMI, a
// reverted volume migration, an unschedulable target pod). Those wedges are
// watched by the framework's fail-fast rules, which abort the spec directly.
func untilVMOPCompleted(ctx context.Context, vmop *v1alpha2.VirtualMachineOperation, timeout time.Duration) error {
	obs := vmopobserver.StartObserver(ctx, vmop)
	defer obs.Stop()

	// The observer only sees events emitted after its watch started, so evaluate the current
	// state explicitly: a VMOP that settled earlier would never produce another event.
	current, err := framework.GetClients().VirtClient().VirtualMachineOperations(vmop.Namespace).Get(ctx, vmop.Name, metav1.GetOptions{})
	if err == nil {
		ok, predicateErr := vmopobserver.BeCompleted()(current)
		if predicateErr != nil {
			return predicateErr
		}
		if ok {
			return nil
		}
	}

	return obs.WaitFor(vmopobserver.BeCompleted(), timeout)
}

func failVMOPMigration(vmop *v1alpha2.VirtualMachineOperation, err error) {
	GinkgoHelper()

	skipIfKnownMigrationIssue(vmop)

	// The VMI migration state inspected above may already be gone (or belong to
	// a newer attempt) by the time the VMOP settles, so also match the known
	// kubevirt flakes against the VMOP failure message itself.
	if err != nil && (IsKnownKubeVirtTargetPodShutdownFailureReason(err.Error()) ||
		IsKnownKubeVirtClientSocketClosedFailureReason(err.Error()) ||
		IsKnownKubeVirtHotplugDiskNotPermittedFailureReason(err.Error()) ||
		failfast.IsKnownDRBDDualPrimaryDeniedFailureReason(err.Error())) {
		Skip(fmt.Sprintf("skip due to known kubevirt migration issue for vmop %s/%s: %s",
			vmop.Namespace, vmop.Name, err))
	}

	Fail(fmt.Sprintf("migration is not completed: %s", err))
}

func skipIfKnownMigrationIssue(vmop *v1alpha2.VirtualMachineOperation) {
	GinkgoHelper()

	// The context is intentionally fresh: the caller's context may already be expired on the
	// timeout path, while the skip checks must still be able to inspect the cluster.
	ctx := context.Background()

	vm, err := framework.GetClients().VirtClient().VirtualMachines(vmop.Namespace).Get(ctx, vmop.Spec.VirtualMachine, metav1.GetOptions{})
	if err != nil {
		return
	}
	// TODO: remove temporary migration skip logic when both known issues are fixed:
	// kubevirt "client socket is closed" and Volume(s)UpdateError.
	SkipIfKnownMigrationFailureWithContext(ctx, vm)
}
