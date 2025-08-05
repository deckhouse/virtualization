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

package internal

import (
	"context"
	"log/slog"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	commonvmop "github.com/deckhouse/virtualization-controller/pkg/common/vmop"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vm/internal/state"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmopcondition"
)

const nameMigratingHandler = "MigratingHandler"

type MigratingHandler struct{}

func NewMigratingHandler() *MigratingHandler {
	return &MigratingHandler{}
}

func (h *MigratingHandler) Handle(ctx context.Context, s state.VirtualMachineState) (reconcile.Result, error) {
	vm := s.VirtualMachine().Changed()

	if isDeletion(vm) {
		return reconcile.Result{}, nil
	}

	if vm == nil {
		return reconcile.Result{}, nil
	}

	kvvm, err := s.KVVM(ctx)
	if err != nil {
		return reconcile.Result{}, err
	}

	kvvmi, err := s.KVVMI(ctx)
	if err != nil {
		return reconcile.Result{}, err
	}

	vmops, err := s.VMOPs(ctx)
	if err != nil {
		return reconcile.Result{}, err
	}

	log := logger.FromContext(ctx).With(logger.SlogHandler(nameLifeCycleHandler))
	vm.Status.MigrationState = h.wrapMigrationState(kvvmi)

	h.syncMigrating(vm, kvvmi, vmops, log)
	h.syncMigratable(vm, kvvm)
	return reconcile.Result{}, nil
}

func (h *MigratingHandler) Name() string {
	return nameMigratingHandler
}

func (h *MigratingHandler) wrapMigrationState(kvvmi *virtv1.VirtualMachineInstance) *virtv2.VirtualMachineMigrationState {
	if kvvmi == nil {
		return nil
	}

	migrationState := kvvmi.Status.MigrationState

	if migrationState == nil {
		return nil
	}

	return &virtv2.VirtualMachineMigrationState{
		StartTimestamp: migrationState.StartTimestamp,
		EndTimestamp:   migrationState.EndTimestamp,
		Target: virtv2.VirtualMachineLocation{
			Node: migrationState.TargetNode,
			Pod:  migrationState.TargetPod,
		},
		Source: virtv2.VirtualMachineLocation{
			Node: migrationState.SourceNode,
		},
		Result: h.getMigrationResult(migrationState),
	}
}

func (h *MigratingHandler) getMigrationResult(state *virtv1.VirtualMachineInstanceMigrationState) virtv2.MigrationResult {
	if state == nil {
		return ""
	}
	switch {
	case state.Completed && !state.Failed:
		return virtv2.MigrationResultSucceeded
	case state.Failed:
		return virtv2.MigrationResultFailed
	default:
		return ""
	}
}

func (h *MigratingHandler) syncMigrating(vm *virtv2.VirtualMachine, kvvmi *virtv1.VirtualMachineInstance, vmops []*virtv2.VirtualMachineOperation, log *slog.Logger) {
	cb := conditions.NewConditionBuilder(vmcondition.TypeMigrating).Generation(vm.GetGeneration())
	defer func() {
		if cb.Condition().Status == metav1.ConditionTrue ||
			cb.Condition().Reason == vmcondition.ReasonLastMigrationFinishedWithError.String() ||
			cb.Condition().Message != "" {
			conditions.SetCondition(cb, &vm.Status.Conditions)
		} else {
			conditions.RemoveCondition(vmcondition.TypeMigrating, &vm.Status.Conditions)
		}
	}()

	var vmop *virtv2.VirtualMachineOperation
	{
		var inProgressVmops []*virtv2.VirtualMachineOperation
		for _, op := range vmops {
			if commonvmop.IsMigration(op) && (op.Status.Phase == virtv2.VMOPPhaseInProgress || op.Status.Phase == virtv2.VMOPPhasePending) {
				inProgressVmops = append(inProgressVmops, op)
			}
		}

		switch length := len(inProgressVmops); length {
		case 0:
		case 1:
			vmop = inProgressVmops[0]
		default:
			log.Error("Found vmops in progress phase. This is unexpected. Please report a bug.", slog.Int("VMOPCount", length))
		}
	}

	switch {
	case liveMigrationInProgress(vm.Status.MigrationState):
		cb.Status(metav1.ConditionTrue).Reason(vmcondition.ReasonVmIsMigrating)
		conditions.SetCondition(cb, &vm.Status.Conditions)

	case vmop != nil:
		cb.Status(metav1.ConditionFalse).Reason(vmcondition.ReasonVmIsNotMigrating)
		completed, _ := conditions.GetCondition(vmopcondition.TypeCompleted, vmop.Status.Conditions)
		switch completed.Reason {
		case vmopcondition.ReasonMigrationPending.String():
			cb.Message("Migration is awaiting start.")
		case vmopcondition.ReasonMigrationPrepareTarget.String():
			cb.Message("Migration is awaiting target preparation.")
		case vmopcondition.ReasonMigrationTargetReady.String():
			cb.Message("Migration is awaiting execution.")
		case vmopcondition.ReasonMigrationRunning.String():
			cb.Status(metav1.ConditionTrue).Reason(vmcondition.ReasonVmIsMigrating)
		}
		conditions.SetCondition(cb, &vm.Status.Conditions)

	case kvvmi != nil && liveMigrationFailed(vm.Status.MigrationState):
		msg := kvvmi.Status.MigrationState.FailureReason
		cb.Status(metav1.ConditionFalse).
			Reason(vmcondition.ReasonLastMigrationFinishedWithError).
			Message(msg)
		conditions.SetCondition(cb, &vm.Status.Conditions)
	}
}

func (h *MigratingHandler) syncMigratable(vm *virtv2.VirtualMachine, kvvm *virtv1.VirtualMachine) {
	cb := conditions.NewConditionBuilder(vmcondition.TypeMigratable).Generation(vm.GetGeneration())

	if kvvm != nil {
		liveMigratable := service.GetKVVMCondition(string(virtv1.VirtualMachineInstanceIsMigratable), kvvm.Status.Conditions)
		if liveMigratable != nil && liveMigratable.Status == corev1.ConditionFalse && liveMigratable.Reason == virtv1.VirtualMachineInstanceReasonDisksNotMigratable {
			cb.Status(metav1.ConditionFalse).
				Reason(vmcondition.ReasonNotMigratable).
				Message("Live migration requires that all PVCs must be shared (using ReadWriteMany access mode)")
			conditions.SetCondition(cb, &vm.Status.Conditions)
			return
		}
	}
	cb.Status(metav1.ConditionTrue).Reason(vmcondition.ReasonMigratable)
	conditions.SetCondition(cb, &vm.Status.Conditions)
}

func liveMigrationInProgress(migrationState *virtv2.VirtualMachineMigrationState) bool {
	return migrationState != nil && migrationState.StartTimestamp != nil && migrationState.EndTimestamp == nil
}

func liveMigrationFailed(migrationState *virtv2.VirtualMachineMigrationState) bool {
	return migrationState != nil && migrationState.EndTimestamp != nil && migrationState.Result == virtv2.MigrationResultFailed
}
