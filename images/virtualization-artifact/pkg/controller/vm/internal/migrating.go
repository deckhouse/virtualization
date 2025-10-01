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
	"cmp"
	"context"
	"fmt"
	"slices"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	commonvmop "github.com/deckhouse/virtualization-controller/pkg/common/vmop"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vm/internal/state"
	"github.com/deckhouse/virtualization-controller/pkg/featuregates"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmopcondition"
)

const nameMigratingHandler = "MigratingHandler"

type migratingVolumesService interface {
	VolumesSynced(ctx context.Context, s state.VirtualMachineState) (bool, error)
	GetVirtualDiskNamesWithUnreadyTarget(ctx context.Context, s state.VirtualMachineState) ([]string, error)
}
type MigratingHandler struct {
	migratingVolumesService migratingVolumesService
}

func NewMigratingHandler(migratingVolumesService migratingVolumesService) *MigratingHandler {
	return &MigratingHandler{
		migratingVolumesService: migratingVolumesService,
	}
}

func (h *MigratingHandler) Handle(ctx context.Context, s state.VirtualMachineState) (reconcile.Result, error) {
	_, ctx = logger.GetHandlerContext(ctx, nameMigratingHandler)

	vm := s.VirtualMachine().Changed()

	if isDeletion(vm) {
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

	err = h.syncMigratable(ctx, s, vm, kvvm)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("failed to sync migratable condition: %w", err)
	}

	err = h.syncMigrating(ctx, s, vm, kvvmi)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("failed to sync migrating condition: %w", err)
	}

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
			// 3.1 Check if virtual disks can be migrated or ready to migrate
			if err := h.syncWaitingForVMToBeReadyMigrate(ctx, s, cb); err != nil {
				return err
			}

		case vmopcondition.ReasonMigrationRunning.String():
			cb.Status(metav1.ConditionTrue).Reason(vmcondition.ReasonMigratingInProgress)

		case vmopcondition.ReasonOperationFailed.String():
			cb.Reason(vmcondition.ReasonLastMigrationFinishedWithError).Message("")

		case vmopcondition.ReasonNotApplicableForVMPhase.String():
			cb.Reason(vmcondition.ReasonLastMigrationFinishedWithError).Message("Migration is not applicable for the current virtual machine phase")

		case vmopcondition.ReasonNotApplicableForLiveMigrationPolicy.String():
			cb.Reason(vmcondition.ReasonLastMigrationFinishedWithError).Message("Migration is not applicable for the live migration policy")

		case vmopcondition.ReasonNotApplicableForRunPolicy.String():
			cb.Reason(vmcondition.ReasonLastMigrationFinishedWithError).Message("Migration is not applicable for the run policy")

		case vmopcondition.ReasonOtherMigrationInProgress.String():
			cb.Reason(vmcondition.ReasonLastMigrationFinishedWithError).Message("Another migration is in progress")

		case vmopcondition.ReasonOperationCompleted.String():
			conditions.RemoveCondition(vmcondition.TypeMigrating, &vm.Status.Conditions)
			return nil

		default:

			switch vmop.Status.Phase {
			case "":
				conditions.RemoveCondition(vmcondition.TypeMigrating, &vm.Status.Conditions)
				return nil

			case virtv2.VMOPPhasePending:
				cb.Reason(vmcondition.ReasonMigratingPending).Message("Wait until operation is completed")

			case virtv2.VMOPPhaseInProgress:
				cb.Reason(vmcondition.ReasonMigratingInProgress).Message("Wait until operation is completed")

			case virtv2.VMOPPhaseCompleted:
				conditions.RemoveCondition(vmcondition.TypeMigrating, &vm.Status.Conditions)
				return nil

			case virtv2.VMOPPhaseFailed:
				cb.Reason(vmcondition.ReasonLastMigrationFinishedWithError).Message("Operation failed")

			case virtv2.VMOPPhaseTerminating:
				cb.Reason(vmcondition.ReasonLastMigrationFinishedWithError).Message("Operation terminated")
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

func (h *MigratingHandler) syncWaitingForVMToBeReadyMigrate(ctx context.Context, s state.VirtualMachineState, cb *conditions.ConditionBuilder) error {
	synced, err := h.migratingVolumesService.VolumesSynced(ctx, s)
	if err != nil {
		return err
	}

	if !synced {
		cb.Message("Target persistent volume claims are not synced yet.")
		return nil
	}

	notReadyToMigrateDisks, err := h.migratingVolumesService.GetVirtualDiskNamesWithUnreadyTarget(ctx, s)
	if err != nil {
		return err
	}

	if len(notReadyToMigrateDisks) > 0 {
		cb.Message(fmt.Sprintf("Migration is awaiting virtual disks to be ready to migrate [%s].", strings.Join(notReadyToMigrateDisks, ", ")))
		return nil
	}

	cb.Reason(vmcondition.ReasonReadyToMigrate).Message("")

	return nil
}

func (h *MigratingHandler) getVMOPCandidate(ctx context.Context, s state.VirtualMachineState) (*virtv2.VirtualMachineOperation, error) {
	vmops, err := s.VMOPs(ctx)
	if err != nil {
		return nil, err
	}

	if len(vmops) == 0 {
		return nil, nil
	}

	// sort vmops from the oldest to the newest
	slices.SortFunc(vmops, func(a, b *virtv2.VirtualMachineOperation) int {
		return cmp.Compare(a.GetCreationTimestamp().UnixNano(), b.GetCreationTimestamp().UnixNano())
	})

	migrations := slices.DeleteFunc(vmops, func(vmop *virtv2.VirtualMachineOperation) bool {
		return !commonvmop.IsMigration(vmop)
	})

	for _, migration := range migrations {
		if commonvmop.IsInProgressOrPending(migration) {
			return migration, nil
		}
	}

	if len(migrations) > 0 {
		return migrations[len(migrations)-1], nil
	}

	return nil, nil
}

func (h *MigratingHandler) syncMigratable(ctx context.Context, s state.VirtualMachineState, vm *virtv2.VirtualMachine, kvvm *virtv1.VirtualMachine) error {
	cb := conditions.NewConditionBuilder(vmcondition.TypeMigratable).Generation(vm.GetGeneration())

	if kvvm != nil {
		liveMigratable := service.GetKVVMCondition(string(virtv1.VirtualMachineInstanceIsMigratable), kvvm.Status.Conditions)
		switch {
		case liveMigratable == nil:
		case liveMigratable.Reason == virtv1.VirtualMachineInstanceReasonDisksNotMigratable:
			if featuregates.Default().Enabled(featuregates.VolumeMigration) {
				cb.Status(metav1.ConditionTrue).
					Reason(vmcondition.ReasonDisksShouldBeMigrating).
					Message("")
			} else {
				cb.Status(metav1.ConditionFalse).
					Reason(vmcondition.ReasonDisksNotMigratable).
					Message("Live migration requires that all PVCs must be shared (using ReadWriteMany access mode)")
			}
			conditions.SetCondition(cb, &vm.Status.Conditions)
			return nil
		case liveMigratable.Status == corev1.ConditionFalse:
			cb.Status(metav1.ConditionFalse).
				Reason(vmcondition.ReasonNonMigratable).
				Message(liveMigratable.Message)
			conditions.SetCondition(cb, &vm.Status.Conditions)
			return nil
		}

		if kvvm.Spec.UpdateVolumesStrategy != nil && *kvvm.Spec.UpdateVolumesStrategy == virtv1.UpdateVolumesStrategyMigration {
			readWriteOnceVirtualDisks, err := s.ReadWriteOnceVirtualDisks(ctx)
			if err != nil {
				return err
			}
			if len(readWriteOnceVirtualDisks) > 0 {
				if featuregates.Default().Enabled(featuregates.VolumeMigration) {
					cb.Status(metav1.ConditionTrue).
						Reason(vmcondition.ReasonDisksShouldBeMigrating).
						Message("")
				} else {
					cb.Status(metav1.ConditionFalse).
						Reason(vmcondition.ReasonDisksNotMigratable).
						Message("Live migration requires that all PVCs must be shared (using ReadWriteMany access mode)")
				}
				conditions.SetCondition(cb, &vm.Status.Conditions)
				return nil
			}
		}

		cb.Status(metav1.ConditionTrue).Reason(vmcondition.ReasonMigratable)
		conditions.SetCondition(cb, &vm.Status.Conditions)
		return nil
	}

	cb.Status(metav1.ConditionFalse).Reason(vmcondition.ReasonNonMigratable).Message("")
	conditions.SetCondition(cb, &vm.Status.Conditions)

	return nil
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
