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

package step

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	vmrestorecondition "github.com/deckhouse/virtualization/api/core/v1alpha2/vm-restore-condition"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmopcondition"
)

type StartVMStep struct {
	client   client.Client
	recorder eventrecord.EventRecorderLogger
	cb       *conditions.ConditionBuilder
	vmop     *virtv2.VirtualMachineOperation
}

func NewStartVMStep(
	client client.Client,
	recorder eventrecord.EventRecorderLogger,
	cb *conditions.ConditionBuilder,
	vmop *virtv2.VirtualMachineOperation,
) *StartVMStep {
	return &StartVMStep{
		client:   client,
		recorder: recorder,
		cb:       cb,
		vmop:     vmop,
	}
}

func (s StartVMStep) Take(ctx context.Context, vm *virtv2.VirtualMachine) (*reconcile.Result, error) {
	cb := conditions.NewConditionBuilder(vmopcondition.TypeCompleted)
	defer func() { conditions.SetCondition(cb.Generation(s.vmop.Generation), &s.vmop.Status.Conditions) }()

	if s.vmop.Status.Phase == virtv2.VMOPPhaseInProgress {
		err := startVirtualMachine(ctx, s.client, s.vmop)
		if err != nil {
			s.recorder.Event(
				s.vmop,
				corev1.EventTypeWarning,
				virtv2.ReasonVMStartFailed,
				err.Error(),
			)
		}

		s.vmop.Status.Phase = virtv2.VMOPPhaseCompleted
		cb.Status(metav1.ConditionTrue).Reason(vmrestorecondition.VirtualMachineRestoreReady)

		return &reconcile.Result{}, nil
	}

	return &reconcile.Result{}, nil
}
