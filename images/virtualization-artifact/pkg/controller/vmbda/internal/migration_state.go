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

package internal

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/controller-runtime/pkg/client"

	commonvmop "github.com/deckhouse/virtualization-controller/pkg/common/vmop"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/indexer"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmopcondition"
)

var migrationQueueReasons = sets.New(
	vmopcondition.ReasonQuotaExceeded.String(),
	vmopcondition.ReasonWaitingForBlockDeviceAttachment.String(),
)

func migrationIsQueued(ctx context.Context, c client.Client, namespace, vmName string) (bool, error) {
	vmops := &v1alpha2.VirtualMachineOperationList{}
	err := c.List(ctx, vmops,
		client.InNamespace(namespace),
		client.MatchingFields{indexer.IndexFieldVMOPByVM: vmName},
	)
	if err != nil {
		return false, err
	}

	found := false
	for _, vmop := range vmops.Items {
		if !commonvmop.IsMigration(&vmop) || !commonvmop.IsInProgressOrPending(&vmop) {
			continue
		}

		found = true
		if !operationIsQueued(&vmop) {
			return false, nil
		}
	}

	return found, nil
}

func migrationBlockedHotPlugMessage(vmbda *v1alpha2.VirtualMachineBlockDeviceAttachment) string {
	return fmt.Sprintf(
		"Cannot hot-plug the %s %q while the VirtualMachine %q is migrating. Attachment will continue after the migration completes.",
		vmbda.Spec.BlockDeviceRef.Kind, vmbda.Spec.BlockDeviceRef.Name, vmbda.Spec.VirtualMachineName,
	)
}

func operationIsQueued(vmop *v1alpha2.VirtualMachineOperation) bool {
	completed, ok := conditions.GetCondition(vmopcondition.TypeCompleted, vmop.Status.Conditions)
	if !ok || completed.Reason == "" {
		return vmop.Status.Phase == v1alpha2.VMOPPhasePending
	}

	return migrationQueueReasons.Has(completed.Reason)
}
