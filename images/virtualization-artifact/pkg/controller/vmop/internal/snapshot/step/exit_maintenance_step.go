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
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmop/internal/snapshot/common"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmopcondition"
)

type ExitMaintenanceStep struct {
	client   client.Client
	recorder eventrecord.EventRecorderLogger
	cb       *conditions.ConditionBuilder
	vmop     *virtv2.VirtualMachineOperation
}

func NewExitMaintenanceStep(
	client client.Client,
	recorder eventrecord.EventRecorderLogger,
	cb *conditions.ConditionBuilder,
	vmop *virtv2.VirtualMachineOperation,
) *ExitMaintenanceStep {
	return &ExitMaintenanceStep{
		client:   client,
		recorder: recorder,
		cb:       cb,
		vmop:     vmop,
	}
}

func (s ExitMaintenanceStep) Take(ctx context.Context, vm *virtv2.VirtualMachine) (*reconcile.Result, error) {
	if s.vmop.Spec.Restore.Mode == virtv2.VMOPRestoreModeDryRun {
		return nil, nil
	}

	cb := conditions.NewConditionBuilder(vmopcondition.TypeCompleted)
	defer func() { conditions.SetCondition(cb.Generation(s.vmop.Generation), &s.vmop.Status.Conditions) }()

	restoreCondition, _ := conditions.GetCondition(vmopcondition.TypeRestoreCompleted, s.vmop.Status.Conditions)
	if restoreCondition.Status != metav1.ConditionTrue {
		return nil, nil
	}

	maintenanceCondition, found := conditions.GetCondition(vmcondition.TypeMaintenance, vm.Status.Conditions)
	if !found || maintenanceCondition.Status != metav1.ConditionTrue || maintenanceCondition.Reason != vmcondition.ReasonMaintenanceRestore.String() {
		return nil, nil
	}

	vmCopy := vm.DeepCopy()
	conditions.RemoveCondition(vmcondition.TypeMaintenance, &vmCopy.Status.Conditions)

	err := s.client.Status().Update(ctx, vmCopy)
	if err != nil {
		s.recorder.Event(
			s.vmop,
			corev1.EventTypeWarning,
			virtv2.ReasonErrVMOPFailed,
			"Failed to exit maintenance mode: "+err.Error(),
		)
		common.SetPhaseConditionToFailed(cb, &s.vmop.Status.Phase, err)
		return &reconcile.Result{}, err
	}

	s.recorder.Event(s.vmop, corev1.EventTypeNormal, "MaintenanceMode", "VM exited maintenance mode after restore completion")
	return nil, nil
}
