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

package step

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmopcondition"
)

type CompletedStep struct {
	recorder eventrecord.EventRecorderLogger
}

func NewCompletedStep(
	recorder eventrecord.EventRecorderLogger,
) *CompletedStep {
	return &CompletedStep{
		recorder: recorder,
	}
}

func (s CompletedStep) Take(ctx context.Context, vmop *v1alpha2.VirtualMachineOperation) (*reconcile.Result, error) {
	cb := conditions.NewConditionBuilder(vmopcondition.TypeCompleted).Status(metav1.ConditionTrue).Reason(vmopcondition.ReasonOperationCompleted)
	conditions.SetCondition(cb, &vmop.Status.Conditions)
	vmop.Status.Phase = v1alpha2.VMOPPhaseCompleted
	s.recorder.Event(vmop, corev1.EventTypeNormal, v1alpha2.ReasonVMOPSucceeded, "VirtualMachineOperation is successful completed")
	return &reconcile.Result{}, nil
}
