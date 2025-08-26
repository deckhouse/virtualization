/*
Copyright 2025 Flant JSC

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

package snapshot

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/common/steptaker"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmop/internal/snapshot/step"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmopcondition"
)

type VMSnapshotRestore struct {
	client   client.Client
	recorder eventrecord.EventRecorderLogger
	vmop     *v1alpha2.VirtualMachineOperation
}

func NewVMSnapshotRestore(client client.Client, recorder eventrecord.EventRecorderLogger, vmop *v1alpha2.VirtualMachineOperation) *VMSnapshotRestore {
	return &VMSnapshotRestore{
		client:   client,
		recorder: recorder,
		vmop:     vmop,
	}
}

func (r VMSnapshotRestore) Sync(ctx context.Context, vm *v1alpha2.VirtualMachine) (reconcile.Result, error) {
	cb := conditions.NewConditionBuilder(vmopcondition.TypeRestoreCompleted)
	defer func() { conditions.SetCondition(cb.Generation(r.vmop.Generation), &r.vmop.Status.Conditions) }()

	if r.vmop.Spec.Restore == nil {
		err := fmt.Errorf("restore specification is nil")
		cb.Status(metav1.ConditionFalse).Reason(vmopcondition.ReasonOperationFailed).Message(service.CapitalizeFirstLetter(err.Error()))
		return reconcile.Result{}, err
	}

	if r.vmop.Spec.Restore.VirtualMachineSnapshotName == "" {
		err := fmt.Errorf("virtual machine snapshot name is required")
		cb.Status(metav1.ConditionFalse).Reason(vmopcondition.ReasonOperationFailed).Message(service.CapitalizeFirstLetter(err.Error()))
		return reconcile.Result{}, err
	}
	if vm == nil {
		err := fmt.Errorf("virtual machine is nil")
		cb.Status(metav1.ConditionFalse).Reason(vmopcondition.ReasonOperationFailed).Message(service.CapitalizeFirstLetter(err.Error()))
		return reconcile.Result{}, err
	}

	return steptaker.NewStepTakers(
		step.NewVMSnapshotReadyStep(r.client, cb, r.vmop),
		step.NewValidateStep(r.client, r.recorder, cb, r.vmop),
		step.NewEnterMaintenanceStep(r.client, r.recorder, cb, r.vmop),
		step.NewBestEffortRestoreStep(r.client, r.recorder, cb, r.vmop),
		step.NewStrictRestoreStep(r.client, r.recorder, cb, r.vmop),
		step.NewExitMaintenanceStep(r.client, r.recorder, cb, r.vmop),
	).Run(ctx, vm)
}
