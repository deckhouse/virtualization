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

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	commonvmop "github.com/deckhouse/virtualization-controller/pkg/common/vmop"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmop/migration/internal/service"
	genericservice "github.com/deckhouse/virtualization-controller/pkg/controller/vmop/service"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization-controller/pkg/livemigration"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmopcondition"
)

const lifecycleHandlerName = "LifecycleHandler"

type Base interface {
	Init(vmop *v1alpha2.VirtualMachineOperation)
	ShouldExecuteOrSetFailedPhase(ctx context.Context, vmop *v1alpha2.VirtualMachineOperation) (bool, error)
	FetchVirtualMachineOrSetFailedPhase(ctx context.Context, vmop *v1alpha2.VirtualMachineOperation) (*v1alpha2.VirtualMachine, error)
	IsApplicableOrSetFailedPhase(checker genericservice.ApplicableChecker, vmop *v1alpha2.VirtualMachineOperation, vm *v1alpha2.VirtualMachine) bool
}
type LifecycleHandler struct {
	client    client.Client
	migration *service.MigrationService
	base      Base
	recorder  eventrecord.EventRecorderLogger
}

func NewLifecycleHandler(client client.Client, migration *service.MigrationService, base Base, recorder eventrecord.EventRecorderLogger) *LifecycleHandler {
	return &LifecycleHandler{
		client:    client,
		migration: migration,
		base:      base,
		recorder:  recorder,
	}
}

func (h LifecycleHandler) Handle(ctx context.Context, vmop *v1alpha2.VirtualMachineOperation) (reconcile.Result, error) {
	// Do not update conditions for object in the deletion state.
	if commonvmop.IsTerminating(vmop) {
		vmop.Status.Phase = v1alpha2.VMOPPhaseTerminating
		return reconcile.Result{}, nil
	}

	// Ignore if VMOP is in final state.
	if commonvmop.IsFinished(vmop) {
		return reconcile.Result{}, nil
	}

	// 1.Initialize new VMOP resource: set phase to Pending and all conditions to Unknown.
	h.base.Init(vmop)

	completedCond := conditions.NewConditionBuilder(vmopcondition.TypeCompleted).Generation(vmop.GetGeneration())

	// Pending if quota exceeded.
	isQuotaExceededDuringMigration, err := h.isKubeVirtMigrationRejectedDueToQuota(ctx, vmop)
	if err != nil {
		return reconcile.Result{}, err
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
		return reconcile.Result{}, err
	}

	log, ctx := logger.GetHandlerContext(ctx, lifecycleHandlerName)

	// 3. Operation already in progress. Check if the operation is completed.
	// Synchronize conditions to the VMOP.
	if isOperationInProgress(vmop) {
		log.Debug("Operation in progress, check if VM is completed", "vm.phase", vm.Status.Phase, "vmop.phase", vmop.Status.Phase)
		return reconcile.Result{}, h.syncOperationComplete(ctx, vmop)
	}

	// 4. Check migration, if exists, that means previous reconcile finished with error and SignalSent condition is not synced.
	// Do it now.
	mig, err := h.migration.GetMigration(ctx, vmop)
	if err != nil {
		return reconcile.Result{}, err
	}
	if mig != nil {
		conditions.SetCondition(
			conditions.NewConditionBuilder(vmopcondition.TypeSignalSent).
				Generation(vmop.GetGeneration()).
				Reason(vmopcondition.ReasonSignalSentSuccess).
				Status(metav1.ConditionTrue),
			&vmop.Status.Conditions)
		return reconcile.Result{}, h.syncOperationComplete(ctx, vmop)
	}

	// 5. VMOP is not in progress.
	// All operations must be performed in course, check it and set phase if operation cannot be executed now.
	should, err := h.base.ShouldExecuteOrSetFailedPhase(ctx, vmop)
	if err != nil {
		return reconcile.Result{}, err
	}
	if !should {
		return reconcile.Result{}, nil
	}

	// 6. Check if the operation is applicable for executed.
	isApplicable := h.base.IsApplicableOrSetFailedPhase(h.migration, vmop, vm)
	if !isApplicable {
		return reconcile.Result{}, nil
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
		return reconcile.Result{}, err
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

	// 7. The Operation is valid, and can be executed.
	err = h.execute(ctx, vmop, vm)

	return reconcile.Result{}, err
}

func (h LifecycleHandler) Name() string {
	return lifecycleHandlerName
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

		msg := "Migration failed"
		if mig.Status.MigrationState != nil && mig.Status.MigrationState.FailureReason != "" {
			msg += ": " + mig.Status.MigrationState.FailureReason
		}
		completedCond.
			Status(metav1.ConditionFalse).
			Reason(vmopcondition.ReasonOperationFailed).
			Message(msg)
		conditions.SetCondition(completedCond, &vmop.Status.Conditions)
		return nil
	case virtv1.MigrationSucceeded:
		vmop.Status.Phase = v1alpha2.VMOPPhaseCompleted
		h.recorder.Event(vmop, corev1.EventTypeNormal, v1alpha2.ReasonVMOPSucceeded, "VirtualMachineOperation succeeded")

		completedCond.
			Status(metav1.ConditionTrue).
			Reason(vmopcondition.ReasonOperationCompleted)
		conditions.SetCondition(completedCond, &vmop.Status.Conditions)
		return nil
	}

	// 3. Migration in progress. Set in progress phase
	vmop.Status.Phase = v1alpha2.VMOPPhaseInProgress
	reason := mapMigrationPhaseToReason[mig.Status.Phase]
	if reason == vmopcondition.ReasonMigrationPending {
		vmop.Status.Phase = v1alpha2.VMOPPhasePending
	}

	completedCond.
		Status(metav1.ConditionFalse).
		Reason(reason).
		Message("Wait until operation is completed")
	conditions.SetCondition(completedCond, &vmop.Status.Conditions)

	return nil
}

func (h LifecycleHandler) isApplicableForLiveMigrationPolicy(vmop *v1alpha2.VirtualMachineOperation, vm *v1alpha2.VirtualMachine) (string, bool) {
	// No problems if force flag is not specified.
	if vmop.Spec.Force == nil {
		return "", true
	}

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

func (h LifecycleHandler) execute(ctx context.Context, vmop *v1alpha2.VirtualMachineOperation, vm *v1alpha2.VirtualMachine) error {
	log := logger.FromContext(ctx)

	h.recordEvent(ctx, vmop, vm)

	err := h.migration.CreateMigration(ctx, vmop)
	if err != nil {
		return err
	}

	// The Operation is successfully executed.
	// Turn the phase to InProgress and set the send signal condition to true.
	msg := fmt.Sprintf("Sent signal %q to VM without errors.", vmop.Spec.Type)
	log.Debug(msg)
	h.recorder.Event(vmop, corev1.EventTypeNormal, v1alpha2.ReasonVMOPInProgress, msg)

	mig, err := h.migration.GetMigration(ctx, vmop)
	if mig == nil || err != nil {
		return err
	}

	vmop.Status.Phase = v1alpha2.VMOPPhaseInProgress
	reason := mapMigrationPhaseToReason[mig.Status.Phase]
	if reason == vmopcondition.ReasonMigrationPending {
		vmop.Status.Phase = v1alpha2.VMOPPhasePending
	}

	conditions.SetCondition(
		conditions.NewConditionBuilder(vmopcondition.TypeCompleted).
			Generation(vmop.GetGeneration()).
			Reason(reason).
			Message("Wait for operation to complete").
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

func isOperationInProgress(vmop *v1alpha2.VirtualMachineOperation) bool {
	sent, _ := conditions.GetCondition(vmopcondition.TypeSignalSent, vmop.Status.Conditions)
	completed, _ := conditions.GetCondition(vmopcondition.TypeCompleted, vmop.Status.Conditions)
	return sent.Status == metav1.ConditionTrue && completed.Status != metav1.ConditionTrue
}

var mapMigrationPhaseToReason = map[virtv1.VirtualMachineInstanceMigrationPhase]vmopcondition.ReasonCompleted{
	virtv1.MigrationPhaseUnset:  vmopcondition.ReasonMigrationPending,
	virtv1.MigrationPending:     vmopcondition.ReasonMigrationPending,
	virtv1.MigrationScheduling:  vmopcondition.ReasonMigrationPrepareTarget,
	virtv1.MigrationScheduled:   vmopcondition.ReasonMigrationPrepareTarget,
	virtv1.MigrationTargetReady: vmopcondition.ReasonMigrationTargetReady,
	virtv1.MigrationRunning:     vmopcondition.ReasonMigrationRunning,
	virtv1.MigrationSucceeded:   vmopcondition.ReasonOperationCompleted,
	virtv1.MigrationFailed:      vmopcondition.ReasonOperationFailed,
}
