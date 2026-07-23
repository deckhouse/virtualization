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

package handler

import (
	"context"
	"fmt"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/types"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	commonvmop "github.com/deckhouse/virtualization-controller/pkg/common/vmop"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	migrationprogress "github.com/deckhouse/virtualization-controller/pkg/controller/vmop/migration/internal/progress"
	migrationservice "github.com/deckhouse/virtualization-controller/pkg/controller/vmop/migration/internal/service"
	genericservice "github.com/deckhouse/virtualization-controller/pkg/controller/vmop/service"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization-controller/pkg/livemigration"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmopcondition"
)

const lifecycleHandlerName = "LifecycleHandler"

const timeElapsedUpdateInterval = 10 * time.Second

// waitForVMReadyToMigrateTimeout fails a migration whose disks never sync instead
// of waiting for ReadyToMigrate forever. Healthy migrations reach it within ~1m.
const waitForVMReadyToMigrateTimeout = 5 * time.Minute

// prepareTargetTimeout fails a migration stuck preparing its target instead of leaving it
// InProgress forever. A target that never becomes ready — target pod OOMKilled/CrashLooping,
// unschedulable, or a disk that never attaches — otherwise keeps the migration (and the
// migration slots it holds) alive indefinitely. Healthy targets are prepared within ~1m.
const prepareTargetTimeout = 5 * time.Minute

const (
	progressMigrationPending   int32 = 0
	progressTargetScheduling   int32 = 2
	progressTargetPreparing    int32 = 3
	progressWaitingForSyncSlot int32 = 4
	progressSourceSuspended    int32 = 91
	progressTargetResumed      int32 = 92
	progressMigrationCompleted int32 = 100
)

const (
	messageMigrationPending       = "The VirtualMachineOperation for migrating the virtual machine has been queued. Waiting for the queue to be processed and for this operation to be executed."
	messageSyncingSourceAndTarget = "Source and target are being synchronized"
	messageTargetPodScheduling    = "Scheduling the migration target"
	messageTargetPodPreparing     = "Preparing the migration target"
	messageWaitingForSyncSlot     = "Target prepared; waiting for a free sync slot on the source node."
	messageWaitingForSyncSlotFmt  = "Target prepared; waiting for a free sync slot on source node %q."
	messageTargetVMResumed        = "The virtual machine has resumed on the target"
	messageSourceVMSuspended      = "The virtual machine has been suspended on the source"
)

const (
	reasonFailedAttachVolume = "FailedAttachVolume"
	reasonFailedMount        = "FailedMount"

	reasonTargetNodeIncomingMigrationLimitExceeded     = "TargetNodeIncomingMigrationLimitExceeded"
	messageTargetNodeIncomingMigrationLimitExceeded    = "Waiting for a free inbound migration slot on the target node."
	messageTargetNodeIncomingMigrationLimitExceededFmt = "Waiting for a free inbound migration slot on target node %q."

	messageClusterMigrationLimitReached         = "Waiting for a free migration slot: the cluster live migration limit is reached."
	messageOutboundNodeMigrationLimitReached    = "Waiting for a free outbound migration slot on the source node."
	messageOutboundNodeMigrationLimitReachedFmt = "Waiting for a free outbound migration slot on source node %q."
	messageMigrationLimitReached                = "Waiting for a free migration slot."
)

type Base interface {
	Init(vmop *v1alpha2.VirtualMachineOperation)
	ShouldExecuteOrSetFailedPhase(ctx context.Context, vmop *v1alpha2.VirtualMachineOperation) (bool, error)
	FetchVirtualMachineOrSetFailedPhase(ctx context.Context, vmop *v1alpha2.VirtualMachineOperation) (*v1alpha2.VirtualMachine, error)
	IsApplicableOrSetFailedPhase(checker genericservice.ApplicableChecker, vmop *v1alpha2.VirtualMachineOperation, vm *v1alpha2.VirtualMachine) bool
}
type LifecycleHandler struct {
	client            client.Client
	migration         *migrationservice.MigrationService
	base              Base
	recorder          eventrecord.EventRecorderLogger
	progressStrategy  migrationprogress.Strategy
	systemNetworkName string
}

func NewLifecycleHandler(client client.Client, migration *migrationservice.MigrationService, base Base, recorder eventrecord.EventRecorderLogger, systemNetworkName string) *LifecycleHandler {
	return &LifecycleHandler{
		client:            client,
		migration:         migration,
		base:              base,
		recorder:          recorder,
		progressStrategy:  migrationprogress.NewProgress(),
		systemNetworkName: systemNetworkName,
	}
}

func (h LifecycleHandler) Handle(ctx context.Context, vmop *v1alpha2.VirtualMachineOperation) (reconcile.Result, error) {
	// Do not update conditions for object in the deletion state.
	if commonvmop.IsTerminating(vmop) {
		h.forgetProgress(vmop)
		vmop.Status.Phase = v1alpha2.VMOPPhaseTerminating
		return reconcile.Result{}, nil
	}

	// Ignore if VMOP is in final state.
	if commonvmop.IsFinished(vmop) {
		h.forgetProgress(vmop)
		return reconcile.Result{}, nil
	}

	// 1.Initialize new VMOP resource: set phase to Pending and all conditions to Unknown.
	if vmop.Status.Phase == "" {
		conditions.SetCondition(
			conditions.NewConditionBuilder(vmopcondition.TypeSignalSent).
				Generation(vmop.GetGeneration()).
				Reason(conditions.ReasonUnknown).
				Status(metav1.ConditionUnknown).
				Message(""),
			&vmop.Status.Conditions,
		)
	}

	h.base.Init(vmop)

	// Fails if Type is 'Migrate', 'NodeSelector' is specified and `TargetMigration` is not available.
	if !h.migration.IsTargetMigrationEnabled() {
		if vmop.Spec.Migrate != nil && vmop.Spec.Migrate.NodeSelector != nil {
			vmop.Status.Phase = v1alpha2.VMOPPhaseFailed
			conditions.SetCondition(
				conditions.NewConditionBuilder(vmopcondition.TypeCompleted).
					Generation(vmop.GetGeneration()).
					Reason(vmopcondition.ReasonOperationFailed).
					Status(metav1.ConditionFalse).
					Message("The `nodeSelector` field is not available in the Community Edition version."),
				&vmop.Status.Conditions)
			return reconcile.Result{}, nil
		}
	}

	completedCond := conditions.NewConditionBuilder(vmopcondition.TypeCompleted).Generation(vmop.GetGeneration())

	// Pending if quota exceeded.
	isQuotaExceededDuringMigration, err := h.isKubeVirtMigrationRejectedDueToQuota(ctx, vmop)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("failed to check if migration was rejected due to quota for VMOP: %w", err)
	}
	if isQuotaExceededDuringMigration {
		h.recorder.Event(vmop, corev1.EventTypeWarning, v1alpha2.ReasonErrVMOPPending, "Project quota exceeded")
		conditions.SetCondition(
			completedCond.
				Reason(vmopcondition.ReasonQuotaExceeded).
				Status(metav1.ConditionFalse).
				Message("Project quota exceeded"),
			&vmop.Status.Conditions)
		return reconcile.Result{}, nil
	}

	// 2. Get VirtualMachine for validation vmop.
	vm, err := h.base.FetchVirtualMachineOrSetFailedPhase(ctx, vmop)
	if vm == nil || err != nil {
		return reconcile.Result{}, fmt.Errorf("failed to fetch VirtualMachine for VMOP: %w", err)
	}

	log, ctx := logger.GetHandlerContext(ctx, lifecycleHandlerName)

	// 3. Operation already in progress. Check if the operation is completed.
	// Synchronize conditions to the VMOP.
	if commonvmop.IsOperationInProgress(vmop) {
		log.Debug("Operation in progress, check if VM is completed", "vm.phase", vm.Status.Phase, "vmop.phase", vmop.Status.Phase)
		return h.syncOperationCompleteResult(ctx, vmop)
	}

	// 4. Check migration, if exists, that means previous reconcile finished with error and SignalSent condition is not synced.
	// Do it now.
	mig, err := h.migration.GetMigration(ctx, vmop)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("failed to get migration for VMOP: %w", err)
	}
	if mig != nil {
		conditions.SetCondition(
			conditions.NewConditionBuilder(vmopcondition.TypeSignalSent).
				Generation(vmop.GetGeneration()).
				Reason(vmopcondition.ReasonSignalSentSuccess).
				Status(metav1.ConditionTrue),
			&vmop.Status.Conditions)
		return h.syncOperationCompleteResult(ctx, vmop)
	}

	// 5. VMOP is not in progress.
	// All operations must be performed in course, check it and set phase if operation cannot be executed now.
	should, err := h.base.ShouldExecuteOrSetFailedPhase(ctx, vmop)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("failed to determine if VMOP should execute: %w", err)
	}
	if !should {
		return reconcile.Result{}, nil
	}

	// 6. Check if the operation is applicable for executed.
	isApplicable := h.base.IsApplicableOrSetFailedPhase(h.migration, vmop, vm)
	if !isApplicable {
		return reconcile.Result{}, nil
	}

	// Check if SystemNetwork is configured and ready
	if h.systemNetworkName != "" {
		if msg, ok := h.checkMigrationNetwork(ctx, vm); !ok {
			vmop.Status.Phase = v1alpha2.VMOPPhaseFailed
			h.recorder.Event(vmop, corev1.EventTypeWarning, v1alpha2.ReasonErrVMOPFailed, msg)
			conditions.SetCondition(
				completedCond.
					Reason(vmopcondition.ReasonMigrationNetworkUnavailable).
					Status(metav1.ConditionFalse).
					Message(msg),
				&vmop.Status.Conditions)
			return reconcile.Result{}, nil
		}
	}

	// 6.1 Check if force flag is applicable for effective liveMigrationPolicy.
	msg, isApplicable := h.isApplicableForLiveMigrationPolicy(vmop, vm)
	if !isApplicable {
		vmop.Status.Phase = v1alpha2.VMOPPhaseFailed
		h.recorder.Event(vmop, corev1.EventTypeWarning, v1alpha2.ReasonErrVMOPFailed, msg)
		conditions.SetCondition(
			completedCond.
				Reason(vmopcondition.ReasonNotApplicableForLiveMigrationPolicy).
				Status(metav1.ConditionFalse).
				Message(msg),
			&vmop.Status.Conditions)
		return reconcile.Result{}, nil
	} else if msg != "" {
		h.recorder.Event(vmop, corev1.EventTypeNormal, v1alpha2.ReasonVMOPStarted, msg)
	}

	// 6.2 Fail if there is at least one other migration in progress.
	found, err := h.otherMigrationsAreInProgress(ctx, vmop)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("failed to check other migrations in progress for VMOP: %w", err)
	}
	if found {
		vmop.Status.Phase = v1alpha2.VMOPPhaseFailed
		h.recorder.Event(vmop, corev1.EventTypeWarning, v1alpha2.ReasonErrVMOPFailed, "Other Migrations are in progress")
		conditions.SetCondition(
			completedCond.
				Reason(vmopcondition.ReasonOtherMigrationInProgress).
				Status(metav1.ConditionFalse).
				Message("Other Migrations are in progress"),
			&vmop.Status.Conditions)
		return reconcile.Result{}, nil
	}

	// 7. Check if the vm is migratable.
	if !h.canExecute(vmop, vm) {
		// Drive the deadline in canExecute while the operation waits for the VM to
		// become ready to migrate, so a wedged migration is failed even without
		// further watch events.
		if vmop.Status.Phase == v1alpha2.VMOPPhasePending {
			return reconcile.Result{RequeueAfter: timeElapsedUpdateInterval}, nil
		}
		return reconcile.Result{}, nil
	}
	// 7.1 The Operation is valid, and can be executed.
	err = h.execute(ctx, vmop, vm)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("failed to execute VMOP: %w", err)
	}

	return reconcile.Result{}, nil
}

func (h LifecycleHandler) Name() string {
	return lifecycleHandlerName
}

func (h LifecycleHandler) syncOperationCompleteResult(ctx context.Context, vmop *v1alpha2.VirtualMachineOperation) (reconcile.Result, error) {
	err := h.syncOperationComplete(ctx, vmop)
	if err != nil {
		return reconcile.Result{}, err
	}
	if vmop.Status.Phase == v1alpha2.VMOPPhaseInProgress || vmop.Status.Phase == v1alpha2.VMOPPhasePending {
		return reconcile.Result{RequeueAfter: timeElapsedUpdateInterval}, nil
	}
	return reconcile.Result{}, nil
}

func (h LifecycleHandler) syncOperationComplete(ctx context.Context, vmop *v1alpha2.VirtualMachineOperation) error {
	completedCond := conditions.NewConditionBuilder(vmopcondition.TypeCompleted).Generation(vmop.GetGeneration())

	mig, err := h.migration.GetMigration(ctx, vmop)
	if err != nil {
		return err
	}

	// 1. If migration is missing. Set Failed phase
	if mig == nil {
		kvvmi, err := object.FetchObject(ctx, types.NamespacedName{Name: vmop.Spec.VirtualMachine, Namespace: vmop.Namespace}, h.client, &virtv1.VirtualMachineInstance{})
		if err != nil {
			return err
		}

		vmop.Status.Phase = v1alpha2.VMOPPhaseFailed
		h.recorder.Event(vmop, corev1.EventTypeWarning, v1alpha2.ReasonErrVMOPFailed, "VirtualMachineOperation failed")

		completedCond.
			Status(metav1.ConditionFalse).
			Reason(vmopcondition.ReasonOperationFailed)

		if kvvmi != nil {
			migrationState := kvvmi.Status.MigrationState
			if migrationState != nil &&
				migrationState.Failed &&
				migrationState.EndTimestamp != nil &&
				genericservice.IsAfterSignalSentOrCreation(migrationState.EndTimestamp.Time, vmop) {
				completedCond.Message(fmt.Sprintf("Migration failed: %s", migrationState.FailureReason))
			}
		} else {
			completedCond.Message("Migration failed because the virtual machine is currently not running.")
		}

		conditions.SetCondition(completedCond, &vmop.Status.Conditions)
		return nil
	}

	// 2. If migration is completed. Set completed phase
	switch mig.Status.Phase {
	case virtv1.MigrationFailed:
		vmop.Status.Phase = v1alpha2.VMOPPhaseFailed
		h.recorder.Event(vmop, corev1.EventTypeWarning, v1alpha2.ReasonErrVMOPFailed, "VirtualMachineOperation failed")

		reason := h.getFailedReason(mig)
		if reason == vmopcondition.ReasonFailed {
			if prev, found := conditions.GetCondition(vmopcondition.TypeCompleted, vmop.Status.Conditions); found {
				if prev.Reason == vmopcondition.ReasonNotConverging.String() {
					reason = vmopcondition.ReasonNotConverging
				}
			}
		}
		msg := h.getFailedMessage(reason, mig)
		progress := h.calculateMigrationProgress(vmop, mig, reason)
		vmop.Status.Progress = migrationprogress.FormatPercent(progress)

		completedCond.
			Status(metav1.ConditionFalse).
			Reason(reason).
			Message(msg)
		conditions.SetCondition(completedCond, &vmop.Status.Conditions)
		return nil
	case virtv1.MigrationSucceeded:
		vmop.Status.Phase = v1alpha2.VMOPPhaseCompleted
		h.recorder.Event(vmop, corev1.EventTypeNormal, v1alpha2.ReasonVMOPSucceeded, "VirtualMachineOperation succeeded")
		vmop.Status.Progress = migrationprogress.FormatPercent(100)

		completedCond.
			Status(metav1.ConditionTrue).
			Reason(vmopcondition.ReasonMigrationCompleted)
		conditions.SetCondition(completedCond, &vmop.Status.Conditions)
		return nil
	}

	// 3. Migration in progress. Set in progress phase
	reason, msg, err := h.getInProgressReasonAndMessage(ctx, mig)
	if err != nil {
		return err
	}

	if reason == vmopcondition.ReasonSyncing {
		record := migrationprogress.BuildRecord(vmop, mig, time.Now())
		if h.progressStrategy != nil && h.progressStrategy.IsNotConverging(record) {
			reason = vmopcondition.ReasonNotConverging
			msg = "Migration is not converging: data remaining is not decreasing at maximum throttle"
		}
	}

	prevCompleted, _ := conditions.GetCondition(vmopcondition.TypeCompleted, vmop.Status.Conditions)
	if isTargetPreparationStalled(reason, prevCompleted, time.Now()) {
		if err := h.migration.DeleteMigration(ctx, vmop); err != nil {
			return fmt.Errorf("failed to delete migration stuck preparing target: %w", err)
		}
		vmop.Status.Phase = v1alpha2.VMOPPhaseFailed
		h.recorder.Event(vmop, corev1.EventTypeWarning, v1alpha2.ReasonErrVMOPFailed, "Timed out preparing the migration target")
		vmop.Status.Progress = migrationprogress.FormatPercent(h.calculateMigrationProgress(vmop, mig, reason))
		conditions.SetCondition(
			completedCond.
				Status(metav1.ConditionFalse).
				Reason(vmopcondition.ReasonOperationFailed).
				Message("Timed out preparing the migration target."),
			&vmop.Status.Conditions)
		return nil
	}

	vmop.Status.Phase = v1alpha2.VMOPPhaseInProgress
	if reason == vmopcondition.ReasonMigrationPending || reason == vmopcondition.ReasonWaitingForSyncSlot {
		vmop.Status.Phase = v1alpha2.VMOPPhasePending
	}
	progress := h.calculateMigrationProgress(vmop, mig, reason)
	vmop.Status.Progress = migrationprogress.FormatPercent(progress)

	completedCond.
		Status(metav1.ConditionFalse).
		Reason(reason).
		Message(msg)
	conditions.SetCondition(completedCond, &vmop.Status.Conditions)

	return nil
}

// isTargetPreparationStalled reports whether the migration spent more than
// prepareTargetTimeout in the same target-preparation reason. The clock is the Completed
// condition's LastTransitionTime, which bumps on every reason change, so time queued in an
// excluded reason (pending, slot waits) does not consume the budget.
func isTargetPreparationStalled(reason vmopcondition.ReasonCompleted, prevCompleted metav1.Condition, now time.Time) bool {
	switch reason {
	case vmopcondition.ReasonTargetScheduling,
		vmopcondition.ReasonTargetUnschedulable,
		vmopcondition.ReasonTargetPreparing,
		vmopcondition.ReasonTargetDiskError:
	default:
		return false
	}
	if prevCompleted.Reason != reason.String() || prevCompleted.LastTransitionTime.IsZero() {
		return false
	}
	return now.Sub(prevCompleted.LastTransitionTime.Time) > prepareTargetTimeout
}

func (h LifecycleHandler) isApplicableForLiveMigrationPolicy(vmop *v1alpha2.VirtualMachineOperation, vm *v1alpha2.VirtualMachine) (string, bool) {
	effectivePolicy, autoConverge, err := livemigration.CalculateEffectivePolicy(*vm, vmop)
	if err != nil {
		msg := fmt.Sprintf("Operation is invalid: %v", err)
		return msg, false
	}

	msg := fmt.Sprintf("Migration settings for operation type %s: liveMigrationPolicy %s, autoConverge %v", vmop.Spec.Type, effectivePolicy, autoConverge)
	return msg, true
}

func (h LifecycleHandler) isKubeVirtMigrationRejectedDueToQuota(ctx context.Context, vmop *v1alpha2.VirtualMachineOperation) (bool, error) {
	mig, err := h.migration.GetMigration(ctx, vmop)
	if err != nil {
		return false, err
	}
	if mig == nil {
		return false, nil
	}

	_, ok := conditions.GetKVVMIMCondition(conditions.KubevirtMigrationRejectedByResourceQuotaType, mig.Status.Conditions)
	if ok {
		return true, nil
	}

	return false, nil
}

func (h LifecycleHandler) otherMigrationsAreInProgress(ctx context.Context, vmop *v1alpha2.VirtualMachineOperation) (bool, error) {
	migList := &virtv1.VirtualMachineInstanceMigrationList{}
	err := h.client.List(ctx, migList, client.InNamespace(vmop.GetNamespace()))
	if err != nil {
		return false, err
	}
	for _, mig := range migList.Items {
		if mig.Spec.VMIName == vmop.Spec.VirtualMachine && !mig.IsFinal() && !metav1.IsControlledBy(&mig, vmop) {
			return true, nil
		}
	}
	return false, nil
}

func (h LifecycleHandler) canExecute(vmop *v1alpha2.VirtualMachineOperation, vm *v1alpha2.VirtualMachine) bool {
	migrating, _ := conditions.GetCondition(vmcondition.TypeMigrating, vm.Status.Conditions)
	if migrating.Reason == vmcondition.ReasonReadyToMigrate.String() {
		return true
	}

	migratable, _ := conditions.GetCondition(vmcondition.TypeMigratable, vm.Status.Conditions)

	if migratable.Status == metav1.ConditionTrue {
		completed, _ := conditions.GetCondition(vmopcondition.TypeCompleted, vmop.Status.Conditions)
		waitingForReady := completed.Reason == vmopcondition.ReasonWaitingForVirtualMachineToBeReadyToMigrate.String()
		if waitingForReady && !completed.LastTransitionTime.IsZero() &&
			time.Since(completed.LastTransitionTime.Time) > waitForVMReadyToMigrateTimeout {
			vmop.Status.Phase = v1alpha2.VMOPPhaseFailed
			h.recorder.Event(vmop, corev1.EventTypeWarning, v1alpha2.ReasonErrVMOPFailed, "Timed out waiting for the VirtualMachine to become ready to migrate")
			conditions.SetCondition(
				conditions.NewConditionBuilder(vmopcondition.TypeCompleted).
					Generation(vmop.GetGeneration()).
					Reason(vmopcondition.ReasonOperationFailed).
					Status(metav1.ConditionFalse).
					Message("Timed out waiting for the VirtualMachine to become ready to migrate."),
				&vmop.Status.Conditions)
			return false
		}

		vmop.Status.Phase = v1alpha2.VMOPPhasePending
		conditions.SetCondition(
			conditions.NewConditionBuilder(vmopcondition.TypeCompleted).
				Generation(vmop.GetGeneration()).
				Reason(vmopcondition.ReasonWaitingForVirtualMachineToBeReadyToMigrate).
				Status(metav1.ConditionFalse),
			&vmop.Status.Conditions)
		return false
	}

	vmop.Status.Phase = v1alpha2.VMOPPhaseFailed
	conditions.SetCondition(
		conditions.NewConditionBuilder(vmopcondition.TypeCompleted).
			Generation(vmop.GetGeneration()).
			Reason(vmopcondition.ReasonOperationFailed).
			Status(metav1.ConditionFalse).
			Message("VirtualMachine is not migratable, cannot be processed."),
		&vmop.Status.Conditions)
	return false
}

func (h LifecycleHandler) execute(ctx context.Context, vmop *v1alpha2.VirtualMachineOperation, vm *v1alpha2.VirtualMachine) error {
	log := logger.FromContext(ctx)

	h.recordEvent(ctx, vmop, vm)

	err := h.migration.CreateMigration(ctx, vmop)
	if err != nil {
		return err
	}

	// The Operation is successfully executed.
	// Turn the phase to InProgress and set the send signal condition to true.
	{
		msg := fmt.Sprintf("Sent signal %q to VM without errors.", vmop.Spec.Type)
		log.Debug(msg)
		h.recorder.Event(vmop, corev1.EventTypeNormal, v1alpha2.ReasonVMOPInProgress, msg)
	}

	mig, err := h.migration.GetMigration(ctx, vmop)
	if mig == nil || err != nil {
		return err
	}

	reason, msg, err := h.getInProgressReasonAndMessage(ctx, mig)
	if err != nil {
		return err
	}

	vmop.Status.Phase = v1alpha2.VMOPPhaseInProgress
	if reason == vmopcondition.ReasonMigrationPending || reason == vmopcondition.ReasonWaitingForSyncSlot {
		vmop.Status.Phase = v1alpha2.VMOPPhasePending
	}
	progress := h.calculateMigrationProgress(vmop, mig, reason)
	vmop.Status.Progress = migrationprogress.FormatPercent(progress)

	conditions.SetCondition(
		conditions.NewConditionBuilder(vmopcondition.TypeCompleted).
			Generation(vmop.GetGeneration()).
			Reason(reason).
			Message(msg).
			Status(metav1.ConditionFalse),
		&vmop.Status.Conditions)
	conditions.SetCondition(
		conditions.NewConditionBuilder(vmopcondition.TypeSignalSent).
			Generation(vmop.GetGeneration()).
			Reason(vmopcondition.ReasonSignalSentSuccess).
			Status(metav1.ConditionTrue),
		&vmop.Status.Conditions)

	return nil
}

func (h LifecycleHandler) recordEvent(ctx context.Context, vmop *v1alpha2.VirtualMachineOperation, vm *v1alpha2.VirtualMachine) {
	log := logger.FromContext(ctx)

	switch vmop.Spec.Type {
	case v1alpha2.VMOPTypeEvict:
		h.recorder.WithLogging(log).Event(
			vm,
			corev1.EventTypeNormal,
			v1alpha2.ReasonVMEvicted,
			"Evict initiated with VirtualMachineOperation",
		)
	case v1alpha2.VMOPTypeMigrate:
		h.recorder.WithLogging(log).Event(
			vm,
			corev1.EventTypeNormal,
			v1alpha2.ReasonVMMigrated,
			"Migrate initiated with VirtualMachineOperation",
		)
	}
}

func getMessageByMigrationFailedReason(mig *virtv1.VirtualMachineInstanceMigration) string {
	cond, found := conditions.GetKVVMIMCondition(virtv1.VirtualMachineInstanceMigrationFailed, mig.Status.Conditions)

	if cond.Status == corev1.ConditionTrue && found {
		switch cond.Reason {
		case virtv1.VirtualMachineInstanceMigrationFailedReasonVMIDoesNotExist, virtv1.VirtualMachineInstanceMigrationFailedReasonVMIIsShutdown:
			return "VirtualMachine is stopped"
		default:
			return humanizeMigrationFailedMessage(cond.Message)
		}
	}

	return ""
}

func (h LifecycleHandler) getFailedReason(mig *virtv1.VirtualMachineInstanceMigration) vmopcondition.ReasonCompleted {
	if mig == nil {
		return vmopcondition.ReasonFailed
	}

	if mig.Status.MigrationState != nil {
		state := mig.Status.MigrationState
		if state.AbortRequested || state.AbortStatus == virtv1.MigrationAbortSucceeded {
			return vmopcondition.ReasonAborted
		}
		if strings.Contains(strings.ToLower(state.FailureReason), "converg") || strings.Contains(strings.ToLower(state.FailureReason), "progress") {
			return vmopcondition.ReasonNotConverging
		}
	}

	if cond, found := conditions.GetKVVMIMCondition(virtv1.VirtualMachineInstanceMigrationFailed, mig.Status.Conditions); found {
		reason := strings.ToLower(cond.Reason + " " + cond.Message)
		if strings.Contains(reason, "schedul") || strings.Contains(reason, "unschedul") {
			return vmopcondition.ReasonTargetUnschedulable
		}
		if strings.Contains(reason, "csi") || strings.Contains(reason, "attach") || strings.Contains(reason, "volume") || strings.Contains(reason, "disk") {
			return vmopcondition.ReasonTargetDiskError
		}
	}

	return vmopcondition.ReasonFailed
}

func (h LifecycleHandler) getFailedMessage(reason vmopcondition.ReasonCompleted, mig *virtv1.VirtualMachineInstanceMigration) string {
	base := "Migration failed"
	switch reason {
	case vmopcondition.ReasonAborted:
		base = "Migration aborted"
	case vmopcondition.ReasonNotConverging:
		base = "Migration did not converge"
	case vmopcondition.ReasonTargetUnschedulable:
		base = "Migration failed: target pod is unschedulable"
	case vmopcondition.ReasonTargetDiskError:
		base = "Migration failed: target disk attach error"
	}

	if mig != nil && mig.Status.MigrationState != nil && mig.Status.MigrationState.FailureReason != "" {
		return fmt.Sprintf("%s: %s", base, mig.Status.MigrationState.FailureReason)
	}
	if msg := getMessageByMigrationFailedReason(mig); msg != "" {
		return fmt.Sprintf("%s: %s", base, msg)
	}
	return base
}

func (h LifecycleHandler) getInProgressReasonAndMessage(
	ctx context.Context,
	mig *virtv1.VirtualMachineInstanceMigration,
) (vmopcondition.ReasonCompleted, string, error) {
	reason := vmopcondition.ReasonSyncing
	message := messageSyncingSourceAndTarget

	switch mig.Status.Phase {
	case virtv1.MigrationPhaseUnset, virtv1.MigrationPending:
		reason = vmopcondition.ReasonMigrationPending
		message = messageMigrationPending
		if _, found := conditions.GetKVVMIMCondition(virtv1.VirtualMachineInstanceMigrationConditionType(reasonTargetNodeIncomingMigrationLimitExceeded), mig.Status.Conditions); found {
			kvvmi, err := h.getMigrationVMI(ctx, mig)
			if err != nil {
				return reason, message, err
			}
			message = inboundSlotWaitMessage(kvvmi)
		} else if cond, found := conditions.GetKVVMIMCondition(virtv1.VirtualMachineInstanceMigrationConcurrencyLimitReached, mig.Status.Conditions); found && cond.Status == corev1.ConditionTrue {
			kvvmi, err := h.getMigrationVMI(ctx, mig)
			if err != nil {
				return reason, message, err
			}
			message = concurrencyLimitWaitMessage(cond.Reason, kvvmi)
		}
	case virtv1.MigrationScheduling:
		reason = vmopcondition.ReasonTargetScheduling
		message = messageTargetPodScheduling
	case virtv1.MigrationScheduled, virtv1.MigrationPreparingTarget:
		reason = vmopcondition.ReasonTargetPreparing
		message = messageTargetPodPreparing
	case virtv1.MigrationTargetReady, virtv1.MigrationWaitingForSync, virtv1.MigrationSynchronizing, virtv1.MigrationRunning:
		reason = vmopcondition.ReasonSyncing
		message = messageSyncingSourceAndTarget
	}

	pod, err := h.getTargetPod(ctx, mig)
	if err != nil {
		return "", "", err
	}
	if unschedulableMsg, isUnschedulable := getPodPendingUnschedulableMessage(pod); isUnschedulable {
		return vmopcondition.ReasonTargetUnschedulable, unschedulableMsg, nil
	}
	if diskErrMsg, hasDiskErr := h.getTargetPodDiskError(ctx, pod); hasDiskErr {
		return vmopcondition.ReasonTargetDiskError, fmt.Sprintf("Target pod has disk attach error: %s", diskErrMsg), nil
	}

	// Surface slot waits only after ruling out a broken target, and report them as a
	// queue wait: the inbound wait happens in the Scheduled phase and must not look like
	// target preparation, or the stall timeout would fail a healthy queued migration.
	kvvmi, err := h.getMigrationVMI(ctx, mig)
	if err != nil {
		return "", "", err
	}
	if kvvmi != nil && livemigration.IsInboundMigrationSlotWaiting(kvvmi) {
		return vmopcondition.ReasonMigrationPending, inboundSlotWaitMessage(kvvmi), nil
	}
	if kvvmi != nil && livemigration.IsSyncMigrationSlotWaiting(kvvmi) {
		return vmopcondition.ReasonWaitingForSyncSlot, syncSlotWaitMessage(kvvmi), nil
	}

	if mig.Status.MigrationState != nil {
		state := mig.Status.MigrationState
		if state.TargetNodeDomainReadyTimestamp != nil {
			reason = vmopcondition.ReasonTargetResumed
			message = messageTargetVMResumed
		}
		if state.Completed {
			reason = vmopcondition.ReasonSourceSuspended
			message = messageSourceVMSuspended
		}
		if details := formatMigrationDetails(mig, time.Now()); details != "" {
			message = fmt.Sprintf("%s. %s", message, details)
		}
	}

	return reason, message, nil
}

func formatMigrationDetails(mig *virtv1.VirtualMachineInstanceMigration, now time.Time) string {
	if mig == nil || mig.Status.MigrationState == nil || mig.Status.MigrationState.TransferStatus == nil {
		return ""
	}

	state := mig.Status.MigrationState
	status := state.TransferStatus
	parts := make([]string, 0, 7)
	if state.StartTimestamp != nil {
		elapsed := now.Sub(state.StartTimestamp.Time).Truncate(timeElapsedUpdateInterval)
		if elapsed < 0 {
			elapsed = 0
		}
		parts = append(parts, fmt.Sprintf("TimeElapsed:%ds", int64(elapsed.Seconds())))
	}
	if status.DataProcessedBytes != nil {
		parts = append(parts, fmt.Sprintf("DataProcessed:%dMiB", bytesToMiB(*status.DataProcessedBytes)))
	}
	if status.DataRemainingBytes != nil {
		parts = append(parts, fmt.Sprintf("DataRemaining:%dMiB", bytesToMiB(*status.DataRemainingBytes)))
	}
	if status.DataTotalBytes != nil {
		parts = append(parts, fmt.Sprintf("DataTotal:%dMiB", bytesToMiB(*status.DataTotalBytes)))
	}
	if status.Iteration != nil {
		parts = append(parts, fmt.Sprintf("Iteration:%d", *status.Iteration))
	}
	parts = append(parts, fmt.Sprintf("AutoConvergeThrottleSet:%t", status.AutoConvergeThrottle != nil))
	if status.AutoConvergeThrottle != nil {
		parts = append(parts, fmt.Sprintf("AutoConvergeThrottle:%d", *status.AutoConvergeThrottle))
	}
	if len(parts) == 0 {
		return ""
	}

	return strings.Join(parts, " ")
}

func bytesToMiB(value uint64) uint64 {
	return value / 1024 / 1024
}

func (h LifecycleHandler) calculateMigrationProgress(
	vmop *v1alpha2.VirtualMachineOperation,
	mig *virtv1.VirtualMachineInstanceMigration,
	reason vmopcondition.ReasonCompleted,
) int32 {
	switch reason {
	case vmopcondition.ReasonMigrationPending:
		return progressMigrationPending
	case vmopcondition.ReasonTargetScheduling:
		return progressTargetScheduling
	case vmopcondition.ReasonTargetUnschedulable:
		return progressTargetScheduling
	case vmopcondition.ReasonTargetPreparing:
		return progressTargetPreparing
	case vmopcondition.ReasonTargetDiskError:
		return progressTargetPreparing
	case vmopcondition.ReasonWaitingForSyncSlot:
		return progressWaitingForSyncSlot
	case vmopcondition.ReasonSyncing, vmopcondition.ReasonNotConverging:
		record := migrationprogress.BuildRecord(vmop, mig, time.Now())
		return h.progressStrategy.SyncProgress(record)
	case vmopcondition.ReasonSourceSuspended:
		h.forgetProgress(vmop)
		return progressSourceSuspended
	case vmopcondition.ReasonTargetResumed:
		h.forgetProgress(vmop)
		return progressTargetResumed
	case vmopcondition.ReasonMigrationCompleted:
		h.forgetProgress(vmop)
		return progressMigrationCompleted
	default:
		h.forgetProgress(vmop)
		if vmop != nil {
			return migrationprogress.ParsePercent(vmop.Status.Progress)
		}
		return 0
	}
}

func (h LifecycleHandler) getTargetPodDiskError(ctx context.Context, pod *corev1.Pod) (string, bool) {
	if pod == nil || !isContainerCreating(pod) || pod.DeletionTimestamp != nil {
		return "", false
	}

	eventList := &corev1.EventList{}
	err := h.client.List(ctx, eventList, &client.ListOptions{
		Namespace: pod.Namespace,
		FieldSelector: fields.SelectorFromSet(fields.Set{
			"involvedObject.name": pod.Name,
			"involvedObject.kind": "Pod",
		}),
	})
	if err != nil {
		return "", false
	}
	for _, e := range eventList.Items {
		if e.Type == corev1.EventTypeWarning && (e.Reason == reasonFailedAttachVolume || e.Reason == reasonFailedMount) {
			return fmt.Sprintf("%s: %s", e.Reason, e.Message), true
		}
	}
	return "", false
}

func (h LifecycleHandler) forgetProgress(vmop *v1alpha2.VirtualMachineOperation) {
	if h.progressStrategy == nil || vmop == nil {
		return
	}
	h.progressStrategy.Forget(vmop.UID)
}

func humanizeMigrationFailedMessage(message string) string {
	if strings.Contains(message, "unschedulable target pod") && strings.Contains(message, "timeout period expiration") {
		return "No available nodes were found to place the target VM within the timeout period"
	}

	return message
}

// getMigrationVMI returns the internal virtual machine instance the migration
// belongs to, or nil when it does not exist.
func (h LifecycleHandler) getMigrationVMI(ctx context.Context, mig *virtv1.VirtualMachineInstanceMigration) (*virtv1.VirtualMachineInstance, error) {
	if mig == nil || mig.Spec.VMIName == "" {
		return nil, nil
	}

	var kvvmi virtv1.VirtualMachineInstance
	err := h.client.Get(ctx, types.NamespacedName{Namespace: mig.Namespace, Name: mig.Spec.VMIName}, &kvvmi)
	if err != nil {
		return nil, client.IgnoreNotFound(err)
	}

	return &kvvmi, nil
}

func inboundSlotWaitMessage(kvvmi *virtv1.VirtualMachineInstance) string {
	if kvvmi != nil {
		if node := livemigration.InboundMigrationWaitingTargetNode(kvvmi); node != "" {
			return fmt.Sprintf(messageTargetNodeIncomingMigrationLimitExceededFmt, node)
		}
	}
	return messageTargetNodeIncomingMigrationLimitExceeded
}

func syncSlotWaitMessage(kvvmi *virtv1.VirtualMachineInstance) string {
	if kvvmi != nil {
		if node := livemigration.SyncMigrationWaitingSourceNode(kvvmi); node != "" {
			return fmt.Sprintf(messageWaitingForSyncSlotFmt, node)
		}
	}
	return messageWaitingForSyncSlot
}

func concurrencyLimitWaitMessage(condReason string, kvvmi *virtv1.VirtualMachineInstance) string {
	switch condReason {
	case virtv1.VirtualMachineInstanceMigrationConcurrencyLimitReachedReasonCluster:
		return messageClusterMigrationLimitReached
	case virtv1.VirtualMachineInstanceMigrationConcurrencyLimitReachedReasonOutboundNode:
		if kvvmi != nil && kvvmi.Status.NodeName != "" {
			return fmt.Sprintf(messageOutboundNodeMigrationLimitReachedFmt, kvvmi.Status.NodeName)
		}
		return messageOutboundNodeMigrationLimitReached
	default:
		return messageMigrationLimitReached
	}
}

func (h LifecycleHandler) getTargetPod(ctx context.Context, mig *virtv1.VirtualMachineInstanceMigration) (*corev1.Pod, error) {
	selector, err := metav1.LabelSelectorAsSelector(&metav1.LabelSelector{
		MatchLabels: map[string]string{
			virtv1.AppLabel:          "virt-launcher",
			virtv1.MigrationJobLabel: string(mig.UID),
		},
	})
	if err != nil {
		return nil, err
	}

	pods := &corev1.PodList{}
	err = h.client.List(ctx, pods, client.InNamespace(mig.Namespace), client.MatchingLabelsSelector{Selector: selector})
	if err != nil {
		return nil, err
	}

	if len(pods.Items) > 0 {
		return &pods.Items[0], nil
	}

	return nil, nil
}

func isContainerCreating(pod *corev1.Pod) bool {
	if pod == nil || pod.Status.Phase != corev1.PodPending {
		return false
	}
	for _, cs := range pod.Status.ContainerStatuses {
		if cs.State.Waiting != nil && cs.State.Waiting.Reason == "ContainerCreating" {
			return true
		}
	}
	return false
}

func (h *LifecycleHandler) checkMigrationNetwork(ctx context.Context, vm *v1alpha2.VirtualMachine) (string, bool) {
	nodeName := vm.Status.Node
	if nodeName == "" {
		return "Virtual machine is not scheduled to any node", false
	}
	var node corev1.Node
	if err := h.client.Get(ctx, client.ObjectKey{Name: nodeName}, &node); err != nil {
		return fmt.Sprintf("failed to get source node %q: %v", nodeName, err), false
	}
	if node.Annotations[annotations.AnnMigrationIface] == "" {
		return fmt.Sprintf(
			"source node %q has no dedicated migration interface for network %q",
			nodeName, h.systemNetworkName,
		), false
	}
	return "", true
}

func getPodPendingUnschedulableMessage(pod *corev1.Pod) (string, bool) {
	if pod == nil {
		return "", false
	}
	if pod.Status.Phase != corev1.PodPending || pod.DeletionTimestamp != nil {
		return "", false
	}

	for _, condition := range pod.Status.Conditions {
		if condition.Type == corev1.PodScheduled && condition.Status == corev1.ConditionFalse && condition.Reason == corev1.PodReasonUnschedulable {
			return fmt.Sprintf("Target pod %q is unschedulable", pod.Namespace+"/"+pod.Name), true
		}
	}
	return "", false
}
