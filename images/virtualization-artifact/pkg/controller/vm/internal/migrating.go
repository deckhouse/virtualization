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

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vm/internal/state"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
)

const nameMigratingHandler = "MigratingHandler"

type MigratingHandler struct {
}

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

	kvvmi, err := s.KVVMI(ctx)
	if err != nil {
		return reconcile.Result{}, err
	}

	if kvvmi == nil || kvvmi.Status.MigrationState == nil {
		vm.Status.MigrationState = nil
	} else {
		vm.Status.MigrationState = h.wrapMigrationState(kvvmi.Status.MigrationState)
	}

	cbMigrating := conditions.NewConditionBuilder(vmcondition.TypeMigrating).Generation(vm.GetGeneration())
	defer func() {
		if cbMigrating.Condition().Status == metav1.ConditionTrue || cbMigrating.Condition().Reason == vmcondition.ReasonLastMigrationFinishedWithError.String() {
			conditions.SetCondition(cbMigrating, &vm.Status.Conditions)
		} else {
			conditions.RemoveCondition(vmcondition.TypeMigrating, &vm.Status.Conditions)
		}
	}()

	switch {
	case vm.Status.MigrationState != nil &&
		vm.Status.MigrationState.StartTimestamp != nil &&
		vm.Status.MigrationState.EndTimestamp == nil:

		cbMigrating.Status(metav1.ConditionTrue).Reason(vmcondition.ReasonVmIsMigrating)
		conditions.SetCondition(cbMigrating, &vm.Status.Conditions)

	case kvvmi != nil && kvvmi.Status.MigrationState != nil &&
		kvvmi.Status.MigrationState.EndTimestamp != nil &&
		kvvmi.Status.MigrationState.Failed:

		msg := kvvmi.Status.MigrationState.FailureReason
		cbMigrating.Status(metav1.ConditionFalse).
			Reason(vmcondition.ReasonLastMigrationFinishedWithError).
			Message(msg)
		conditions.SetCondition(cbMigrating, &vm.Status.Conditions)
	}

	cbMigratable := conditions.NewConditionBuilder(vmcondition.TypeMigratable).Generation(vm.GetGeneration())

	kvvm, err := s.KVVM(ctx)
	if err != nil {
		return reconcile.Result{}, err
	}

	if kvvm != nil {
		liveMigratable := service.GetKVVMCondition(string(virtv1.VirtualMachineInstanceIsMigratable), kvvm.Status.Conditions)
		if liveMigratable != nil && liveMigratable.Status == corev1.ConditionFalse && liveMigratable.Reason == virtv1.VirtualMachineInstanceReasonDisksNotMigratable {
			cbMigratable.Status(metav1.ConditionFalse).
				Reason(vmcondition.ReasonNotMigratable).
				Message("Live migration requires that all PVCs must be shared (using ReadWriteMany access mode)")
			conditions.SetCondition(cbMigratable, &vm.Status.Conditions)
			return reconcile.Result{}, nil
		}
	}
	cbMigratable.Status(metav1.ConditionTrue).Reason(vmcondition.ReasonMigratable)
	conditions.SetCondition(cbMigratable, &vm.Status.Conditions)

	return reconcile.Result{}, nil
}

func (h *MigratingHandler) Name() string {
	return nameMigratingHandler
}

func (h *MigratingHandler) wrapMigrationState(state *virtv1.VirtualMachineInstanceMigrationState) *virtv2.VirtualMachineMigrationState {
	if state == nil {
		return nil
	}
	return &virtv2.VirtualMachineMigrationState{
		StartTimestamp: state.StartTimestamp,
		EndTimestamp:   state.EndTimestamp,
		Target: virtv2.VirtualMachineLocation{
			Node: state.TargetNode,
			Pod:  state.TargetPod,
		},
		Source: virtv2.VirtualMachineLocation{
			Node: state.SourceNode,
		},
		Result: h.getResult(state),
	}
}

func (h *MigratingHandler) getResult(state *virtv1.VirtualMachineInstanceMigrationState) virtv2.MigrationResult {
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
