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

package service

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	"github.com/deckhouse/virtualization-controller/pkg/common/steptaker"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmsop/snapshot/internal/step"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmsopcondition"
)

func NewCreateVirtualMachineOperation(client client.Client, eventRecorder eventrecord.EventRecorderLogger, vmsop *v1alpha2.VirtualMachineSnapshotOperation) *CreateVirtualMachineOperation {
	return &CreateVirtualMachineOperation{
		vmsop:    vmsop,
		client:   client,
		recorder: eventRecorder,
	}
}

type CreateVirtualMachineOperation struct {
	vmsop    *v1alpha2.VirtualMachineSnapshotOperation
	client   client.Client
	recorder eventrecord.EventRecorderLogger
}

func (o CreateVirtualMachineOperation) Execute(ctx context.Context) (reconcile.Result, error) {
	cb := conditions.NewConditionBuilder(vmsopcondition.TypeCreateVirtualMachineCompleted)
	defer func() { conditions.SetCondition(cb.Generation(o.vmsop.Generation), &o.vmsop.Status.Conditions) }()

	cond, exist := conditions.GetCondition(vmsopcondition.TypeCreateVirtualMachineCompleted, o.vmsop.Status.Conditions)
	if exist {
		if cond.Status == metav1.ConditionUnknown {
			cb.Status(metav1.ConditionFalse).Reason(vmsopcondition.ReasonCreateVirtualMachineOperationInProgress)
		} else {
			cb.Status(cond.Status).Reason(vmsopcondition.ReasonCreateVirtualMachineCompleted(cond.Reason)).Message(cond.Message)
		}
	}

	if o.vmsop.Spec.CreateVirtualMachine == nil {
		err := fmt.Errorf("clone specification is mandatory to start creating virtual machine")
		cb.Status(metav1.ConditionFalse).Reason(vmsopcondition.ReasonCreateVirtualMachineOperationFailed).Message(service.CapitalizeFirstLetter(err.Error()))
		return reconcile.Result{}, err
	}

	vmsKey := types.NamespacedName{Namespace: o.vmsop.Namespace, Name: o.vmsop.Spec.VirtualMachineSnapshotName}
	vms, err := object.FetchObject(ctx, vmsKey, o.client, &v1alpha2.VirtualMachineSnapshot{})
	if err != nil {
		err := fmt.Errorf("failed to fetch the virtual machine snapshot %q: %w", vmsKey.Name, err)
		cb.Status(metav1.ConditionFalse).Reason(vmsopcondition.ReasonCreateVirtualMachineOperationFailed).Message(service.CapitalizeFirstLetter(err.Error()))
		return reconcile.Result{}, err
	}

	if vms == nil {
		err := fmt.Errorf("virtual machine snapshot specified is not found")
		cb.Status(metav1.ConditionFalse).Reason(vmsopcondition.ReasonCreateVirtualMachineOperationFailed).Message(service.CapitalizeFirstLetter(err.Error()))
		return reconcile.Result{}, err
	}

	return steptaker.NewStepTakers(
		step.NewProcessStep(o.client, o.recorder, cb),
	).Run(ctx, o.vmsop)
}

func (o CreateVirtualMachineOperation) IsApplicableForVMSPhase(phase v1alpha2.VirtualMachineSnapshotPhase) bool {
	return phase == v1alpha2.VirtualMachineSnapshotPhaseReady
}

func (o CreateVirtualMachineOperation) GetInProgressReason() vmsopcondition.ReasonCompleted {
	return vmsopcondition.ReasonCreateVirtualMachineInProgress
}

func (o CreateVirtualMachineOperation) IsInProgress() bool {
	cloneCondition, found := conditions.GetCondition(vmsopcondition.TypeCreateVirtualMachineCompleted, o.vmsop.Status.Conditions)
	if found && cloneCondition.Status != metav1.ConditionUnknown {
		return true
	}

	return false
}

func (o CreateVirtualMachineOperation) IsComplete() (bool, string) {
	createVMCondition, ok := conditions.GetCondition(vmsopcondition.TypeCreateVirtualMachineCompleted, o.vmsop.Status.Conditions)
	if !ok {
		return false, ""
	}

	if createVMCondition.Reason == string(vmsopcondition.ReasonCreateVirtualMachineOperationFailed) {
		return true, createVMCondition.Message
	}

	return createVMCondition.Status == metav1.ConditionTrue, ""
}
