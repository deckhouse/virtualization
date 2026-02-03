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

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	"github.com/deckhouse/virtualization-controller/pkg/common/steptaker"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmop/snapshot/internal/step"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
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
	log := logger.FromContext(ctx)

	if o.vmop.Spec.Clone == nil {
		err := fmt.Errorf("clone specification is mandatory to start cloning")
		return reconcile.Result{}, err
	}

	vmKey := types.NamespacedName{Namespace: o.vmop.Namespace, Name: o.vmop.Spec.VirtualMachine}
	vm, err := object.FetchObject(ctx, vmKey, o.client, &v1alpha2.VirtualMachine{})
	if err != nil {
		err := fmt.Errorf("failed to fetch the virtual machine %q: %w", vmKey.Name, err)
		return reconcile.Result{}, err
	}

	if vm == nil {
		err := fmt.Errorf("specified virtual machine was not found")
		return reconcile.Result{}, err
	}

	o.vmop.Status.Phase = v1alpha2.VMOPPhaseInProgress

	result, err := steptaker.NewStepTakers(
		step.NewCreateSnapshotStep(o.client, o.recorder),
		step.NewVMSnapshotReadyStep(o.client),
		step.NewProcessCloneStep(o.client, o.recorder),
		step.NewWaitingDisksStep(o.client, o.recorder),
		step.NewCleanupSnapshotStep(o.client, o.recorder),
		step.NewCompletedStep(o.recorder),
	).Run(ctx, o.vmop)
	if err != nil {
		failMsg := fmt.Sprintf("%s is failed", o.vmop.Spec.Type)
		log.Debug(failMsg, logger.SlogErr(err))
		failMsg = fmt.Errorf("%s: %w", failMsg, err).Error()
		o.recorder.Event(o.vmop, corev1.EventTypeWarning, v1alpha2.ReasonErrVMOPFailed, failMsg)

		o.vmop.Status.Phase = v1alpha2.VMOPPhaseFailed
		conditions.SetCondition(
			conditions.NewConditionBuilder(vmopcondition.TypeCompleted).
				Reason(vmopcondition.ReasonOperationFailed).
				Message(failMsg).Status(metav1.ConditionFalse),
			&o.vmop.Status.Conditions,
		)
	}

	return result, err
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
