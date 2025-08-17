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
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	commonvmop "github.com/deckhouse/virtualization-controller/pkg/common/vmop"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vm/internal/state"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmopcondition"
)

const nameMigratingHandler = "MigratingHandler"

type MigratingHandler struct {
	vmopProtection *service.ProtectionService

	finalizers []func(context.Context) error
}

func NewMigratingHandler(client client.Client) *MigratingHandler {
	return &MigratingHandler{
		vmopProtection: service.NewProtectionService(client, virtv2.FinalizerVMOPProtectionByVMController),
	}
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

	vm.Status.MigrationState = h.wrapMigrationState(kvvmi)

	h.syncMigratable(vm, kvvm)

	return reconcile.Result{}, h.syncMigrating(ctx, s, vm, kvvmi)
}

func (h *MigratingHandler) Name() string {
	return nameMigratingHandler
}

func (h *MigratingHandler) Finalize(ctx context.Context) error {
	for _, finalizer := range h.finalizers {
		if err := finalizer(ctx); err != nil {
			return err
		}
	}
	return nil
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

func (h *MigratingHandler) syncMigrating(ctx context.Context, s state.VirtualMachineState, vm *virtv2.VirtualMachine, kvvmi *virtv1.VirtualMachineInstance) error {
	// 0. If KVVMI is nil, migration cannot be in progress. Remove Migrating condition, but keep if migration failed.
	if kvvmi == nil {
		migrating, _ := conditions.GetCondition(vmcondition.TypeMigrating, vm.Status.Conditions)
		if migrating.Reason != vmcondition.ReasonLastMigrationFinishedWithError.String() {
			conditions.RemoveCondition(vmcondition.TypeMigrating, &vm.Status.Conditions)
		}
		return nil
	}

	cb := conditions.NewConditionBuilder(vmcondition.TypeMigrating).Generation(vm.GetGeneration())

	// 1. Check if live migration is in progress
	if liveMigrationInProgress(vm.Status.MigrationState) {
		cb.Status(metav1.ConditionTrue).Reason(vmcondition.ReasonMigratingInProgress)
		conditions.SetCondition(cb, &vm.Status.Conditions)
		return nil
	}

	// 2. Check if migration requested
	vmop, err := h.getVMOPCandidate(ctx, s)
	if err != nil {
		return err
	}

	if vmop != nil {
		// 3. Sync migration status from VMOP
		cb.Status(metav1.ConditionFalse).Reason(vmcondition.ReasonMigratingPending)

		completed, _ := conditions.GetCondition(vmopcondition.TypeCompleted, vmop.Status.Conditions)
		switch completed.Reason {
		case vmopcondition.ReasonMigrationPending.String():
			cb.Message("Migration is awaiting start.")

		case vmopcondition.ReasonQuotaExceeded.String():
			cb.Message(fmt.Sprintf("Migration is pending: %s", completed.Message))

		case vmopcondition.ReasonMigrationPrepareTarget.String():
			cb.Message("Migration is awaiting target preparation.")

		case vmopcondition.ReasonMigrationTargetReady.String():
			cb.Message("Migration is awaiting execution.")

		case vmopcondition.ReasonWaitingForVirtualMachineToBeReadyToMigrate.String():
			if err := h.vmopProtection.AddProtection(ctx, vmop); err != nil {
				return err
			}

			// 6.1 Check if virtual disks can be migrated or ready to migrate
			if err := h.syncWaitingForVMToBeReadyMigrate(ctx, s, cb, kvvmi); err != nil {
				return err
			}

		case vmopcondition.ReasonMigrationRunning.String():
			cb.Status(metav1.ConditionTrue).Reason(vmcondition.ReasonMigratingInProgress)

		case vmopcondition.ReasonOperationFailed.String():
			h.finalizers = append(h.finalizers, func(ctx context.Context) error {
				return h.vmopProtection.RemoveProtection(ctx, vmop)
			})

			cb.Reason(vmcondition.ReasonLastMigrationFinishedWithError).Message("")

		case vmopcondition.ReasonOperationCompleted.String():
			h.finalizers = append(h.finalizers, func(ctx context.Context) error {
				return h.vmopProtection.RemoveProtection(ctx, vmop)
			})

			conditions.RemoveCondition(vmcondition.TypeMigrating, &vm.Status.Conditions)
			return nil

		default:
			if commonvmop.IsTerminating(vmop) {
				h.finalizers = append(h.finalizers, func(ctx context.Context) error {
					return h.vmopProtection.RemoveProtection(ctx, vmop)
				})

				cb.Reason(vmcondition.ReasonLastMigrationFinishedWithError).Message("VMOP was being terminated before migration was completed.")
			}
		}

		conditions.SetCondition(cb, &vm.Status.Conditions)
		return nil
	}

	// 4. Set error if migration failed.
	if liveMigrationFailed(vm.Status.MigrationState) {
		msg := kvvmi.Status.MigrationState.FailureReason
		cb.Status(metav1.ConditionFalse).
			Reason(vmcondition.ReasonLastMigrationFinishedWithError).
			Message(msg)
		conditions.SetCondition(cb, &vm.Status.Conditions)
		return nil
	}

	if liveMigrationSucceeded(vm.Status.MigrationState) {
		conditions.RemoveCondition(vmcondition.TypeMigrating, &vm.Status.Conditions)
		return nil
	}

	// 5. Remove Migrating condition if migration is finished successfully. Or migration was not be requested.
	migrating, _ := conditions.GetCondition(vmcondition.TypeMigrating, vm.Status.Conditions)
	if migrating.Reason != vmcondition.ReasonLastMigrationFinishedWithError.String() {
		conditions.RemoveCondition(vmcondition.TypeMigrating, &vm.Status.Conditions)
	}
	return nil
}

func (h *MigratingHandler) syncWaitingForVMToBeReadyMigrate(ctx context.Context, s state.VirtualMachineState, cb *conditions.ConditionBuilder, kvvmi *virtv1.VirtualMachineInstance) error {
	// Check if virtual disks can be migrated or ready to migrate
	nonMigratableVirtualDisks, err := s.NonMigratableVirtualDisks(ctx)
	if err != nil {
		return err
	}

	var notReadyToMigrateDisks []string
	targetPVCNames := make(map[string]struct{})
	for _, vd := range nonMigratableVirtualDisks {
		// if target pvc is set, it means that the disk is ready to be migrated,
		// but if end timestamp is set, it means that TargetPVC is old, and we should wait new one.
		if vd.Status.MigrationState.TargetPVC == "" || !vd.Status.MigrationState.EndTimestamp.IsZero() {
			notReadyToMigrateDisks = append(notReadyToMigrateDisks, vd.Name)
		}
		targetPVCNames[vd.Status.MigrationState.TargetPVC] = struct{}{}
	}

	if len(notReadyToMigrateDisks) > 0 {
		cb.Message(fmt.Sprintf("Migration is awaiting virtual disks to be ready to migrate [%s].", strings.Join(notReadyToMigrateDisks, ", ")))
	} else {
		var targetSyncCount int
		for _, vol := range kvvmi.Spec.Volumes {
			if vol.PersistentVolumeClaim != nil {
				if _, ok := targetPVCNames[vol.PersistentVolumeClaim.ClaimName]; ok {
					targetSyncCount++
				}
			}
		}

		if len(targetPVCNames) == targetSyncCount {
			cb.Reason(vmcondition.ReasonReadyToMigrate).Message("")
		} else {
			cb.Message("Target persistent volume claims are not synced yet.")
		}
	}

	return nil
}

func (h *MigratingHandler) getVMOPCandidate(ctx context.Context, s state.VirtualMachineState) (*virtv2.VirtualMachineOperation, error) {
	vmops, err := s.VMOPs(ctx)
	if err != nil {
		return nil, err
	}

	var candidate *virtv2.VirtualMachineOperation
	if len(vmops) > 0 {
		candidate = vmops[0]

		for _, vmop := range vmops {
			if !commonvmop.IsMigration(vmop) {
				continue
			}
			if vmop.GetCreationTimestamp().Time.After(candidate.GetCreationTimestamp().Time) {
				candidate = vmop
			}
		}
	}

	return candidate, nil
}

func (h *MigratingHandler) syncMigratable(vm *virtv2.VirtualMachine, kvvm *virtv1.VirtualMachine) {
	cb := conditions.NewConditionBuilder(vmcondition.TypeMigratable).Generation(vm.GetGeneration())

	if kvvm != nil {
		liveMigratable := service.GetKVVMCondition(string(virtv1.VirtualMachineInstanceIsMigratable), kvvm.Status.Conditions)
		switch {
		case liveMigratable == nil:
		case liveMigratable.Reason == virtv1.VirtualMachineInstanceReasonDisksNotMigratable:
			cb.Status(metav1.ConditionFalse).
				Reason(vmcondition.ReasonDisksNotMigratable).
				Message("Live migration requires that all PVCs must be shared (using ReadWriteMany access mode)")
			conditions.SetCondition(cb, &vm.Status.Conditions)
			return
		case liveMigratable.Status == corev1.ConditionFalse:
			cb.Status(metav1.ConditionFalse).
				Reason(vmcondition.ReasonNonMigratable).
				Message(liveMigratable.Message)
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

func liveMigrationSucceeded(migrationState *virtv2.VirtualMachineMigrationState) bool {
	return migrationState != nil && migrationState.EndTimestamp != nil && migrationState.Result == virtv2.MigrationResultSucceeded
}
