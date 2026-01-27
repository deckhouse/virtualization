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
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmop/snapshot/internal/step"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmopcondition"
)

func NewRestoreOperation(client client.Client, eventRecorder eventrecord.EventRecorderLogger, vmop *v1alpha2.VirtualMachineOperation) *RestoreOperation {
	return &RestoreOperation{
		vmop:     vmop,
		client:   client,
		recorder: eventRecorder,
	}
}

type RestoreOperation struct {
	vmop     *v1alpha2.VirtualMachineOperation
	client   client.Client
	recorder eventrecord.EventRecorderLogger
}

func (o RestoreOperation) Execute(ctx context.Context) (reconcile.Result, error) {
	if o.vmop.Spec.Restore == nil {
		err := fmt.Errorf("restore specification is nil")
		return reconcile.Result{}, err
	}

	if o.vmop.Spec.Restore.VirtualMachineSnapshotName == "" {
		err := fmt.Errorf("virtual machine snapshot name is required")
		return reconcile.Result{}, err
	}

	vmKey := types.NamespacedName{Namespace: o.vmop.Namespace, Name: o.vmop.Spec.VirtualMachine}
	vm, err := object.FetchObject(ctx, vmKey, o.client, &v1alpha2.VirtualMachine{})
	if err != nil {
		err := fmt.Errorf("failed to fetch the virtual machine %q: %w", vmKey.Name, err)
		return reconcile.Result{}, err
	}

	if vm == nil {
		err := fmt.Errorf("virtual machine is nil")
		return reconcile.Result{}, err
	}

	return steptaker.NewStepTakers(
		step.NewVMSnapshotReadyStep(o.client),
		step.NewEnterMaintenanceStep(o.client, o.recorder),
		step.NewProcessRestoreStep(o.client, o.recorder),
		step.NewExitMaintenanceStep(o.client, o.recorder),
	).Run(ctx, o.vmop)
}

func (o RestoreOperation) IsApplicableForVMPhase(phase v1alpha2.MachinePhase) bool {
	return phase == v1alpha2.MachineStopped || phase == v1alpha2.MachineRunning || phase == v1alpha2.MachinePending
}

func (o RestoreOperation) IsApplicableForRunPolicy(runPolicy v1alpha2.RunPolicy) bool {
	return true
}

func (o RestoreOperation) GetInProgressReason() vmopcondition.ReasonCompleted {
	return vmopcondition.ReasonRestoreInProgress
}

func (o RestoreOperation) IsInProgress() bool {
	maintenanceModeCondition, found := conditions.GetCondition(vmopcondition.TypeMaintenanceMode, o.vmop.Status.Conditions)
	if found && maintenanceModeCondition.Status != metav1.ConditionUnknown {
		return true
	}

	if o.vmop.Status.Resources != nil {
		for _, status := range o.vmop.Status.Resources {
			if status.Status == v1alpha2.SnapshotResourceStatusInProgress {
				return true
			}
		}
	}

	return false
}

func (o RestoreOperation) IsCompleted() (bool, string) {
	if o.vmop.Status.Resources == nil {
		return false, ""
	}

	for _, status := range o.vmop.Status.Resources {
		if status.Status != v1alpha2.SnapshotResourceStatusCompleted {
			return false, ""
		}
	}

	if o.vmop.Spec.Restore.Mode == v1alpha2.SnapshotOperationModeDryRun {
		return true, ""
	}

	mc, ok := conditions.GetCondition(vmopcondition.TypeMaintenanceMode, o.vmop.Status.Conditions)
	return ok && mc.Status == metav1.ConditionFalse, ""
}
