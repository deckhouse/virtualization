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
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
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

	// The kvvmi migration record is the source of truth, so assign it
	// unconditionally. Keeping the previous value when the kvvmi no longer reports
	// a migration (e.g. the VMI was recreated after a migration that never got an
	// EndTimestamp) would leave a stale in-progress state, which keeps
	// liveMigrationInProgress true forever and blocks every future migration of
	// the VM.
	vm.Status.MigrationState = h.wrapMigrationState(kvvmi, vm.Status.MigrationState)

	err = h.syncMigratable(ctx, s, vm, kvvm, kvvmi)
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

func (h *MigratingHandler) wrapMigrationState(kvvmi *virtv1.VirtualMachineInstance, prev *v1alpha2.VirtualMachineMigrationState) *v1alpha2.VirtualMachineMigrationState {
	if kvvmi == nil {
		return nil
	}

	migrationState := kvvmi.Status.MigrationState

	if migrationState == nil {
		return nil
	}

	return &v1alpha2.VirtualMachineMigrationState{
		StartTimestamp: migrationState.StartTimestamp,
		EndTimestamp:   migrationState.EndTimestamp,
		Target: v1alpha2.VirtualMachineLocation{
			Node: migrationState.TargetNode,
			Pod:  migrationState.TargetPod,
		},
		Source: v1alpha2.VirtualMachineLocation{
			Node: migrationState.SourceNode,
		},
		Result:          h.getMigrationResult(migrationState),
		VolumeMigration: h.isVolumeMigration(kvvmi, migrationState, prev),
	}
}

// isVolumeMigration reports whether the disks move to another storage along with the memory.
// KubeVirt fills kvvmi.status.migratedVolumes before the migration starts and drops it once the
// migration is over or cancelled, while the migration state stays in the status of the
// VirtualMachine afterwards. The answer is therefore remembered for as long as the same migration
// is reported, otherwise a finished volume migration would look like an ordinary one. The start
// time identifies the migration: the next migration of the same machine cannot start in the same
// second, because the current one has to finish first.
func (h *MigratingHandler) isVolumeMigration(
	kvvmi *virtv1.VirtualMachineInstance,
	state *virtv1.VirtualMachineInstanceMigrationState,
	prev *v1alpha2.VirtualMachineMigrationState,
) bool {
	if len(kvvmi.Status.MigratedVolumes) > 0 {
		return true
	}

	return prev != nil && prev.VolumeMigration && prev.StartTimestamp.Equal(state.StartTimestamp)
}

func (h *MigratingHandler) getMigrationResult(state *virtv1.VirtualMachineInstanceMigrationState) v1alpha2.MigrationResult {
	if state == nil {
		return ""
	}
	switch {
	case state.Completed && !state.Failed:
		return v1alpha2.MigrationResultSucceeded
	case state.Failed:
		return v1alpha2.MigrationResultFailed
	default:
		return ""
	}
}

func (h *MigratingHandler) syncMigrating(ctx context.Context, s state.VirtualMachineState, vm *v1alpha2.VirtualMachine, kvvmi *virtv1.VirtualMachineInstance) error {
	// 0. If KVVMI is nil, migration cannot be in progress. Remove Migrating condition, but keep if migration failed.
	if kvvmi == nil {
		conditions.RemoveCondition(vmcondition.TypeMigrating, &vm.Status.Conditions)
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

		case vmopcondition.ReasonTargetScheduling.String():
			cb.Message("Migration is in progress: target pod is being scheduled.")

		case vmopcondition.ReasonQuotaExceeded.String():
			cb.Message(fmt.Sprintf("Migration is pending: %s.", completed.Message))

		case vmopcondition.ReasonMigrationPrepareTarget.String(), vmopcondition.ReasonTargetPreparing.String():
			cb.Message("Migration is in progress: preparing the migration target.")

		case vmopcondition.ReasonMigrationTargetReady.String(), vmopcondition.ReasonSyncing.String(), vmopcondition.ReasonSourceSuspended.String(), vmopcondition.ReasonTargetResumed.String():
			cb.Message("Migration is in progress: source and target are being synchronized.")

		case vmopcondition.ReasonWaitingForVirtualMachineToBeReadyToMigrate.String():
			// 3.1 Check if virtual disks can be migrated or ready to migrate
			if err := h.syncWaitingForVMToBeReadyMigrate(ctx, s, cb); err != nil {
				return err
			}

		case vmopcondition.ReasonMigrationRunning.String():
			cb.Status(metav1.ConditionTrue).Reason(vmcondition.ReasonMigratingInProgress)

		case vmopcondition.ReasonOperationCompleted.String(), vmopcondition.ReasonMigrationCompleted.String():
			conditions.RemoveCondition(vmcondition.TypeMigrating, &vm.Status.Conditions)
			return nil

		default:

			switch vmop.Status.Phase {
			case "":
				conditions.RemoveCondition(vmcondition.TypeMigrating, &vm.Status.Conditions)
				return nil

			case v1alpha2.VMOPPhasePending:
				cb.Reason(vmcondition.ReasonMigratingPending).Message(
					fmt.Sprintf("Wait until operation is completed; VirtualMachineOperation: %s.", vmop.Name),
				)

			case v1alpha2.VMOPPhaseInProgress:
				cb.Reason(vmcondition.ReasonMigratingInProgress).Message(
					fmt.Sprintf("Wait until operation is completed; VirtualMachineOperation: %s.", vmop.Name),
				)

			case v1alpha2.VMOPPhaseCompleted, v1alpha2.VMOPPhaseFailed, v1alpha2.VMOPPhaseTerminating, v1alpha2.VMOPPhaseSuperseded:
				conditions.RemoveCondition(vmcondition.TypeMigrating, &vm.Status.Conditions)
				return nil
			}
		}

		conditions.SetCondition(cb, &vm.Status.Conditions)
		return nil
	}

	// 4. Remove Migrating condition if migration is finished or migration was not be requested.
	conditions.RemoveCondition(vmcondition.TypeMigrating, &vm.Status.Conditions)
	return nil
}

func (h *MigratingHandler) syncWaitingForVMToBeReadyMigrate(ctx context.Context, s state.VirtualMachineState, cb *conditions.ConditionBuilder) error {
	synced, err := h.migratingVolumesService.VolumesSynced(ctx, s)
	if err != nil {
		return err
	}

	if !synced {
		cb.Message("The virtual machine disks are not synchronized to the migration target yet.")
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

func (h *MigratingHandler) getVMOPCandidate(ctx context.Context, s state.VirtualMachineState) (*v1alpha2.VirtualMachineOperation, error) {
	vmops, err := s.VMOPs(ctx)
	if err != nil {
		return nil, err
	}

	if len(vmops) == 0 {
		return nil, nil
	}

	// sort vmops from the oldest to the newest
	slices.SortFunc(vmops, func(a, b *v1alpha2.VirtualMachineOperation) int {
		return cmp.Compare(a.GetCreationTimestamp().UnixNano(), b.GetCreationTimestamp().UnixNano())
	})

	migrations := slices.DeleteFunc(vmops, func(vmop *v1alpha2.VirtualMachineOperation) bool {
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

func (h *MigratingHandler) syncMigratable(ctx context.Context, s state.VirtualMachineState, vm *v1alpha2.VirtualMachine, kvvm *virtv1.VirtualMachine, kvvmi *virtv1.VirtualMachineInstance) error {
	cb := conditions.NewConditionBuilder(vmcondition.TypeMigratable).Generation(vm.GetGeneration())

	// Migratability describes a running virtual machine and is calculated from the state of its
	// instance. There is no instance while the machine is stopped, and the conditions of the
	// internal virtual machine keep the values of the last run, so reporting them would present
	// yesterday's answer as the current one: the disks may have been moved to a shared storage
	// class and the device may have been detached since then.
	if kvvm == nil || kvvmi == nil {
		conditions.RemoveCondition(vmcondition.TypeMigratable, &vm.Status.Conditions)
		return nil
	}

	// A machine whose provisioning secret is gone cannot be migrated however migratable its
	// devices and disks are: every virt-launcher pod mounts that secret and renders the
	// provisioning image from it anew, so kubelet stalls the target pod in ContainerCreating long
	// before KubeVirt gets a say. Reported before the reasons that describe the instance itself,
	// because no change of the instance can make this one go away.
	if provisioningSecretIsMissing(vm) {
		cb.Status(metav1.ConditionFalse).
			Reason(vmcondition.ReasonProvisioningSecretMissing).
			Message(messageProvisioningSecretMissing)
		conditions.SetCondition(cb, &vm.Status.Conditions)
		return nil
	}

	// A machine that has nowhere to go is not migratable, no matter how well it is fit for the
	// migration itself and no matter how its disks would travel along. Every positive answer goes
	// through this check; the reasons that describe the machine itself are more specific and are
	// reported instead of it.
	//
	// A cluster that has fitting nodes which cannot take the machine right now is a different
	// answer: the machine stays migratable, because the state of a cordoned or a rebooting node
	// clears up on its own, while the change of a machine which is reported as non-migratable does
	// not — hot-plugging its CPU and memory would turn into a restart it does not need.
	reportMigratable := func(reason vmcondition.MigratableReason) {
		switch migrationTargetReason(kvvm) {
		case "":
			cb.Status(metav1.ConditionTrue).Reason(reason).Message("")
		case virtv1.VirtualMachineInstanceReasonMigrationTargetUnavailable:
			cb.Status(metav1.ConditionTrue).
				Reason(vmcondition.ReasonWaitingForMigrationTarget).
				Message(messageMigrationTargetUnavailable)
		default:
			// Any other reason of a missing target is reported as a machine that cannot be
			// migrated: an unknown answer must not read as a temporary one.
			cb.Status(metav1.ConditionFalse).
				Reason(vmcondition.ReasonNoMigrationTarget).
				Message(messageNoMigrationTarget)
		}
	}

	liveMigratable := service.GetKVVMCondition(string(virtv1.VirtualMachineInstanceIsMigratable), kvvm.Status.Conditions)
	switch {
	case liveMigratable == nil:
	case liveMigratable.Reason == virtv1.VirtualMachineInstanceReasonDisksNotMigratable:
		if featuregates.Default().Enabled(featuregates.VolumeMigration) {
			reportMigratable(vmcondition.ReasonDisksShouldBeMigrating)
		} else {
			cb.Status(metav1.ConditionFalse).
				Reason(vmcondition.ReasonDisksNotMigratable).
				Message("Live migration requires all disks to use ReadWriteMany (shared) storage. Make sure the StorageClass supports the ReadWriteMany access mode.")
		}
		conditions.SetCondition(cb, &vm.Status.Conditions)
		return nil
	case liveMigratable.Reason == virtv1.VirtualMachineInstanceReasonHostDeviceNotMigratable:
		cb.Status(metav1.ConditionFalse).
			Reason(vmcondition.ReasonHostDevicesNotMigratable).
			Message("Live migration is blocked because the VirtualMachine has a device that cannot be migrated. Remove it to enable live migration.")

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
				reportMigratable(vmcondition.ReasonDisksShouldBeMigrating)
			} else {
				cb.Status(metav1.ConditionFalse).
					Reason(vmcondition.ReasonDisksNotMigratable).
					Message("Live migration requires all disks to use ReadWriteMany (shared) storage. Make sure the StorageClass supports the ReadWriteMany access mode.")
			}
			conditions.SetCondition(cb, &vm.Status.Conditions)
			return nil
		}
	}

	reportMigratable(vmcondition.ReasonMigratable)
	conditions.SetCondition(cb, &vm.Status.Conditions)

	return nil
}

// The fork tells two cases apart — no node matches the placement rules at all, and the matching
// nodes are rejected by the affinity rules — but reports both under one reason, so the message
// has to hold for either of them. Naming only the placement rules would be wrong for the second.
const messageNoMigrationTarget = "Live migration is not possible: no other node in the cluster can accept this VirtualMachine. Check its placement and affinity rules and those of its VirtualMachineClass."

const messageMigrationTargetUnavailable = "Live migration is possible, but there is no node to migrate to at the moment: the nodes matching the placement rules of this VirtualMachine are excluded from scheduling or do not run the virtualization."

const messageProvisioningSecretMissing = "Live migration is not possible: the provisioning secret of this VirtualMachine no longer exists, and every virt-launcher pod mounts it, so the target pod would never start. Restore the secret, or remove spec.provisioning and restart the VirtualMachine."

// provisioningSecretIsMissing reports whether the ProvisioningReady condition blames a secret that
// does not exist. The other ways provisioning can be invalid do not block a migration: the secret
// is still there to be mounted, however wrong its contents are.
func provisioningSecretIsMissing(vm *v1alpha2.VirtualMachine) bool {
	provisioning, _ := conditions.GetCondition(vmcondition.TypeProvisioningReady, vm.Status.Conditions)

	return provisioning.Status == metav1.ConditionFalse &&
		provisioning.Reason == vmcondition.ReasonProvisioningSecretNotFound.String()
}

// migrationTargetReason returns the reason of the MigrationTargetAvailable condition of the
// internal virtual machine while the cluster has no node to migrate to, and an empty string while
// it has one. The condition is evaluated by virt-controller, which is the only component watching
// every node of the cluster, and reaches the internal virtual machine along with the rest of the
// instance conditions.
func migrationTargetReason(kvvm *virtv1.VirtualMachine) string {
	targetAvailable := service.GetKVVMCondition(string(virtv1.VirtualMachineInstanceMigrationTargetAvailable), kvvm.Status.Conditions)
	if targetAvailable == nil || targetAvailable.Status != corev1.ConditionFalse {
		return ""
	}
	return targetAvailable.Reason
}

func liveMigrationInProgress(migrationState *v1alpha2.VirtualMachineMigrationState) bool {
	return migrationState != nil && migrationState.StartTimestamp != nil && migrationState.EndTimestamp == nil
}
