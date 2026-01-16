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
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmop/snapshot/internal/step"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmopcondition"
)

func NewCloneOperation(client client.Client, eventRecorder eventrecord.EventRecorderLogger, vmop *v1alpha2.VirtualMachineOperation) *CloneOperation {
	return &CloneOperation{
		vmop:     vmop,
		client:   client,
		recorder: eventRecorder,
	}
}

type CloneOperation struct {
	vmop     *v1alpha2.VirtualMachineOperation
	client   client.Client
	recorder eventrecord.EventRecorderLogger
}

func (o CloneOperation) Execute(ctx context.Context) (reconcile.Result, error) {
	cb := conditions.NewConditionBuilder(vmopcondition.TypeCloneCompleted)
	defer func() { conditions.SetCondition(cb.Generation(o.vmop.Generation), &o.vmop.Status.Conditions) }()

	cond, exist := conditions.GetCondition(vmopcondition.TypeCloneCompleted, o.vmop.Status.Conditions)
	if exist {
		if cond.Status == metav1.ConditionUnknown {
			cb.Status(metav1.ConditionFalse).Reason(vmopcondition.ReasonCloneOperationInProgress)
		} else {
			cb.Status(cond.Status).Reason(vmopcondition.ReasonRestoreCompleted(cond.Reason)).Message(cond.Message)
		}
	}

	if o.vmop.Spec.Clone == nil {
		err := fmt.Errorf("clone specification is mandatory to start cloning")
		cb.Status(metav1.ConditionFalse).Reason(vmopcondition.ReasonCloneOperationFailed).Message(service.CapitalizeFirstLetter(err.Error()))
		return reconcile.Result{}, err
	}

	vmKey := types.NamespacedName{Namespace: o.vmop.Namespace, Name: o.vmop.Spec.VirtualMachine}
	vm, err := object.FetchObject(ctx, vmKey, o.client, &v1alpha2.VirtualMachine{})
	if err != nil {
		err := fmt.Errorf("failed to fetch the virtual machine %q: %w", vmKey.Name, err)
		cb.Status(metav1.ConditionFalse).Reason(vmopcondition.ReasonCloneOperationFailed).Message(service.CapitalizeFirstLetter(err.Error()))
		return reconcile.Result{}, err
	}

	if vm == nil {
		err := fmt.Errorf("virtual machine specified is not found")
		cb.Status(metav1.ConditionFalse).Reason(vmopcondition.ReasonCloneOperationFailed).Message(service.CapitalizeFirstLetter(err.Error()))
		return reconcile.Result{}, err
	}

	return steptaker.NewStepTakers(
		step.NewCleanupSnapshotStep(o.client, o.recorder, cb),
		step.NewCreateSnapshotStep(o.client, o.recorder, cb),
		step.NewVMSnapshotReadyStep(o.client, cb),
		step.NewProcessCloneStep(o.client, o.recorder, cb),
	).Run(ctx, o.vmop)
}

func (o CloneOperation) IsApplicableForVMPhase(phase v1alpha2.MachinePhase) bool {
	return phase == v1alpha2.MachineStopped || phase == v1alpha2.MachineRunning
}

func (o CloneOperation) IsApplicableForRunPolicy(runPolicy v1alpha2.RunPolicy) bool {
	return true
}

func (o CloneOperation) GetInProgressReason() vmopcondition.ReasonCompleted {
	return vmopcondition.ReasonCloneInProgress
}

func (o CloneOperation) IsInProgress() bool {
	snapshotCondition, found := conditions.GetCondition(vmopcondition.TypeSnapshotReady, o.vmop.Status.Conditions)
	if found && snapshotCondition.Status != metav1.ConditionUnknown {
		return true
	}

	cloneCondition, found := conditions.GetCondition(vmopcondition.TypeCloneCompleted, o.vmop.Status.Conditions)
	if found && cloneCondition.Status != metav1.ConditionUnknown {
		return true
	}

	return false
}

func (o CloneOperation) IsComplete() (bool, string) {
	cloneCondition, ok := conditions.GetCondition(vmopcondition.TypeCloneCompleted, o.vmop.Status.Conditions)
	if !ok {
		return false, ""
	}

	snapshotCondition, ok := conditions.GetCondition(vmopcondition.TypeSnapshotReady, o.vmop.Status.Conditions)
	if !ok {
		return false, ""
	}

	if cloneCondition.Reason == string(vmopcondition.ReasonCloneOperationFailed) && snapshotCondition.Reason == string(vmopcondition.ReasonSnapshotCleanedUp) {
		return true, cloneCondition.Message
	}

	return cloneCondition.Status == metav1.ConditionTrue && snapshotCondition.Reason == string(vmopcondition.ReasonSnapshotCleanedUp), ""
}
