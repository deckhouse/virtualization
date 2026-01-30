/*
Copyright 2024 Flant JSC

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

package vmop

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmopcondition"
)

type dataMetric struct {
	Name           string
	Namespace      string
	UID            string
	Type           string
	VirtualMachine string
	Phase          v1alpha2.VMOPPhase
	CreatedAt      *int64 // Unix timestamp when operation was created
	StartedAt      *int64 // Unix timestamp when operation transitioned to InProgress
	FinishedAt     *int64 // Unix timestamp when operation finished (Completed/Failed)
}

// DO NOT mutate VirtualMachineOperation!
func newDataMetric(vmop *v1alpha2.VirtualMachineOperation) *dataMetric {
	if vmop == nil {
		return nil
	}

	createdAt := vmop.CreationTimestamp.Unix()

	var startedAt *int64
	signalSendCond, found := conditions.GetCondition(vmopcondition.TypeSignalSent, vmop.Status.Conditions)
	if found && signalSendCond.Status == metav1.ConditionTrue {
		ts := signalSendCond.LastTransitionTime.Unix()
		startedAt = &ts
	}

	var finishedAt *int64
	if vmop.Status.Phase == v1alpha2.VMOPPhaseCompleted || vmop.Status.Phase == v1alpha2.VMOPPhaseFailed {
		completedCond, found := conditions.GetCondition(vmopcondition.TypeCompleted, vmop.Status.Conditions)
		if found {
			if (completedCond.Status == metav1.ConditionTrue && completedCond.Reason == string(vmopcondition.ReasonOperationCompleted)) ||
				(completedCond.Status == metav1.ConditionFalse && completedCond.Reason == string(vmopcondition.ReasonOperationFailed)) {
				ts := completedCond.LastTransitionTime.Unix()
				finishedAt = &ts
			}
		}
	}

	return &dataMetric{
		Name:           vmop.Name,
		Namespace:      vmop.Namespace,
		UID:            string(vmop.UID),
		Phase:          vmop.Status.Phase,
		Type:           string(vmop.Spec.Type),
		VirtualMachine: vmop.Spec.VirtualMachine,
		CreatedAt:      &createdAt,
		StartedAt:      startedAt,
		FinishedAt:     finishedAt,
	}
}
