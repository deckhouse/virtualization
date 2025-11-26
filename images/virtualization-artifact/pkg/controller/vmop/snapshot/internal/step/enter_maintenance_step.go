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
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmop/snapshot/internal/common"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmopcondition"
)

type EnterMaintenanceStep struct {
	client   client.Client
	recorder eventrecord.EventRecorderLogger
	cb       *conditions.ConditionBuilder
}

func NewEnterMaintenanceStep(
	client client.Client,
	recorder eventrecord.EventRecorderLogger,
	cb *conditions.ConditionBuilder,
) *EnterMaintenanceStep {
	return &EnterMaintenanceStep{
		client:   client,
		recorder: recorder,
		cb:       cb,
	}
}

func (s EnterMaintenanceStep) Take(ctx context.Context, vmop *v1alpha2.VirtualMachineOperation) (*reconcile.Result, error) {
	if vmop.Spec.Restore.Mode == v1alpha2.SnapshotOperationModeDryRun {
		return nil, nil
	}

	vmKey := types.NamespacedName{Namespace: vmop.Namespace, Name: vmop.Spec.VirtualMachine}
	vm, err := object.FetchObject(ctx, vmKey, s.client, &v1alpha2.VirtualMachine{})
	if err != nil {
		return nil, fmt.Errorf("failed to fetch the virtual machine %q: %w", vmKey.Name, err)
	}

	if s.cb.Condition().Status == metav1.ConditionTrue {
		return nil, nil
	}

	maintenanceCondition, found := conditions.GetCondition(vmcondition.TypeMaintenance, vm.Status.Conditions)
	if found && maintenanceCondition.Status == metav1.ConditionTrue && maintenanceCondition.Reason == vmcondition.ReasonMaintenanceRestore.String() {
		if vm.Status.Phase != v1alpha2.MachineStopped && vm.Status.Phase != v1alpha2.MachinePending {
			return &reconcile.Result{}, nil
		}

		conditions.SetCondition(
			conditions.NewConditionBuilder(vmopcondition.TypeMaintenanceMode).
				Generation(vmop.GetGeneration()).
				Reason(vmopcondition.ReasonMaintenanceModeEnabled).
				Status(metav1.ConditionTrue).
				Message("VMOP has enabled maintenance mode on VM for restore operation."),
			&vmop.Status.Conditions,
		)

		return nil, nil
	}

	conditions.SetCondition(
		conditions.NewConditionBuilder(vmcondition.TypeMaintenance).
			Generation(vm.GetGeneration()).
			Reason(vmcondition.ReasonMaintenanceRestore).
			Status(metav1.ConditionTrue).
			Message("VM is in maintenance mode for restore operation."),
		&vm.Status.Conditions,
	)

	err = s.client.Status().Update(ctx, vm)
	if err != nil {
		if apierrors.IsConflict(err) {
			return &reconcile.Result{}, nil
		}

		s.recorder.Event(vmop, corev1.EventTypeWarning, v1alpha2.ReasonErrVMOPFailed, "Failed to enter maintenance mode: "+err.Error())
		common.SetPhaseConditionToFailed(s.cb, &vmop.Status.Phase, err)
		return &reconcile.Result{}, err
	}

	conditions.SetCondition(
		conditions.NewConditionBuilder(vmopcondition.TypeMaintenanceMode).
			Generation(vmop.GetGeneration()).
			Reason(vmopcondition.ReasonMaintenanceModeEnabled).
			Status(metav1.ConditionTrue).
			Message("VMOP has enabled maintenance mode on VM for restore operation."),
		&vmop.Status.Conditions,
	)

	s.recorder.Event(vmop, corev1.EventTypeNormal, "MaintenanceMode", "VM entered maintenance mode for restore operation")

	if vm.Status.Phase != v1alpha2.MachineStopped {
		return &reconcile.Result{}, nil
	}

	return nil, nil
}
