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

type ExitMaintenanceStep struct {
	client   client.Client
	recorder eventrecord.EventRecorderLogger
	cb       *conditions.ConditionBuilder
}

func NewExitMaintenanceStep(
	client client.Client,
	recorder eventrecord.EventRecorderLogger,
	cb *conditions.ConditionBuilder,
) *ExitMaintenanceStep {
	return &ExitMaintenanceStep{
		client:   client,
		recorder: recorder,
		cb:       cb,
	}
}

func (s ExitMaintenanceStep) Take(ctx context.Context, vmop *v1alpha2.VirtualMachineOperation) (*reconcile.Result, error) {
	if vmop.Spec.Restore.Mode == v1alpha2.SnapshotOperationModeDryRun {
		return &reconcile.Result{}, nil
	}

	for _, status := range vmop.Status.Resources {
		if status.Status == v1alpha2.SnapshotResourceStatusInProgress {
			return &reconcile.Result{}, nil
		}
	}

	vmKey := types.NamespacedName{Namespace: vmop.Namespace, Name: vmop.Spec.VirtualMachine}
	vm, err := object.FetchObject(ctx, vmKey, s.client, &v1alpha2.VirtualMachine{})
	if err != nil {
		return nil, fmt.Errorf("failed to fetch the virtual machine %q: %w", vmKey.Name, err)
	}

	maintenanceVMOPCondition, found := conditions.GetCondition(vmopcondition.TypeMaintenanceMode, vmop.Status.Conditions)
	if !found || maintenanceVMOPCondition.Status == metav1.ConditionFalse {
		return &reconcile.Result{}, nil
	}

	restoreCondition, found := conditions.GetCondition(vmopcondition.TypeRestoreCompleted, vmop.Status.Conditions)
	if !found || restoreCondition.Status == metav1.ConditionFalse {
		return &reconcile.Result{}, nil
	}

	// If a VM has a maintenance condition, set it to false.
	maintenanceVMCondition, maintenanceVMConditionFound := conditions.GetCondition(vmcondition.TypeMaintenance, vm.Status.Conditions)
	if maintenanceVMConditionFound && maintenanceVMCondition.Status == metav1.ConditionTrue {
		conditions.SetCondition(
			conditions.NewConditionBuilder(vmcondition.TypeMaintenance).
				Generation(vm.GetGeneration()).
				Reason(vmcondition.ReasonMaintenanceRestore).
				Status(metav1.ConditionFalse).
				Message("VM exited maintenance mode after restore completion."),
			&vm.Status.Conditions,
		)

		err = s.client.Status().Update(ctx, vm)
		if err != nil {
			if apierrors.IsConflict(err) {
				return &reconcile.Result{}, nil
			}

			s.recorder.Event(
				vmop,
				corev1.EventTypeWarning,
				v1alpha2.ReasonErrVMOPFailed,
				"Failed to exit maintenance mode: "+err.Error(),
			)
			common.SetPhaseConditionToFailed(s.cb, &vmop.Status.Phase, err)
			return &reconcile.Result{}, err
		}
	}

	// If the maintenance condition was not present on the VM,
	// or it was already set to false,
	// or it was correctly set to false in the previous step,
	// set the maintenance condition to false on the VMOP.
	if !maintenanceVMConditionFound || maintenanceVMCondition.Status != metav1.ConditionTrue {
		conditions.SetCondition(
			conditions.NewConditionBuilder(vmopcondition.TypeMaintenanceMode).
				Generation(vmop.GetGeneration()).
				Reason(vmopcondition.ReasonMaintenanceModeDisabled).
				Status(metav1.ConditionFalse).
				Message("VMOP has disabled maintenance mode on VM."),
			&vmop.Status.Conditions,
		)
	}

	return &reconcile.Result{}, nil
}
