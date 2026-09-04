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
	"errors"
	"fmt"
	"log/slog"
	"time"

	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	vmopbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vmop"
	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/backoff"
	commonvmop "github.com/deckhouse/virtualization-controller/pkg/common/vmop"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmopcondition"
)

const nameEvacuationHandler = "EvacuationHandler"

func NewEvacuationHandler(
	client client.Client,
	evacuateCanceler EvacuateCanceler,
	recorder eventrecord.EventRecorderLogger,
) *EvacuationHandler {
	return &EvacuationHandler{
		client:           client,
		evacuateCanceler: evacuateCanceler,
		recorder:         recorder,
	}
}

//go:generate go tool moq -rm -out mock.go . EvacuateCanceler
type EvacuateCanceler interface {
	Cancel(ctx context.Context, name, namespace string) error
}

type EvacuationHandler struct {
	client           client.Client
	evacuateCanceler EvacuateCanceler
	recorder         eventrecord.EventRecorderLogger
}

func (h *EvacuationHandler) Handle(ctx context.Context, vm *v1alpha2.VirtualMachine) (reconcile.Result, error) {
	if vm == nil {
		return reconcile.Result{}, nil
	}

	migrationVMOPs, finishedVMOPs, err := h.getVMOPsByVM(ctx, vm)
	if err != nil {
		return reconcile.Result{}, err
	}

	log := logger.FromContext(ctx).With(logger.SlogHandler(nameEvacuationHandler))

	var requeueAfter time.Duration
	if err = h.removeFinalizerFromVMOPs(ctx, finishedVMOPs); err != nil {
		requeueAfter = 100 * time.Millisecond
		if k8serrors.IsConflict(err) {
			log.Debug("Conflict occurred during handler execution", logger.SlogErr(err))
		} else {
			log.Error("Remove finalizer failed", logger.SlogErr(err))
		}
	}

	if len(migrationVMOPs) > 0 {
		if err = h.cancelEvacuationForTerminatingVMOPs(ctx, migrationVMOPs, log); err != nil {
			return reconcile.Result{}, err
		}

		// A machine that has to be restarted will not finish this evacuation: it only sits out the
		// target preparation timeout, and the node stays occupied for those minutes. Give up on the
		// evacuation now, so the restart happens when it was promised.
		giveUp, err := h.restartCannotWait(ctx, vm)
		if err != nil {
			return reconcile.Result{}, err
		}
		if giveUp {
			if err = h.dropEvacuations(ctx, migrationVMOPs, log); err != nil {
				return reconcile.Result{}, err
			}
			return reconcile.Result{RequeueAfter: 100 * time.Millisecond}, nil
		}

		return reconcile.Result{RequeueAfter: requeueAfter}, nil
	}

	if !isVMNeedEvict(vm) || isVMMigrating(vm) {
		return reconcile.Result{}, nil
	}

	switch evictionReason(vm) {
	case vmcondition.ReasonNodeUnderMaintenance.String():
		// The node is only being prepared for maintenance: nothing has to leave it yet.
		return reconcile.Result{}, nil
	case vmcondition.ReasonEvictionBlocked.String():
		// The machine cannot be live migrated at all and no restart is allowed, so an evacuation
		// would only die on the target preparation timeout: the node waits for a person instead of
		// collecting failed operations. A machine that merely lacks a free target keeps its
		// migratability and never reaches this reason.
		return reconcile.Result{}, nil
	case vmcondition.ReasonRestartRequired.String():
		// An administrator who allowed the restart on the node is already waiting for it: there is
		// nothing left to wait for, the machine is restarted as soon as it cannot leave. The node is
		// read again instead of trusting the reason alone, so a permission taken back a moment ago
		// does not produce a restart nobody allows any more.
		approved, err := h.restartApprovedOnNode(ctx, vm)
		if err != nil {
			return reconcile.Result{}, err
		}
		if approved {
			return h.restartToReleaseNode(ctx, vm, log)
		}

		return reconcile.Result{}, nil
	}

	retryAfter, err := h.evacuate(ctx, vm, finishedVMOPs, log)
	if err != nil {
		return reconcile.Result{}, err
	}
	if retryAfter > 0 {
		return reconcile.Result{RequeueAfter: retryAfter}, nil
	}

	return reconcile.Result{RequeueAfter: requeueAfter}, nil
}

// evacuate asks the machine to leave its node by live migration, backing off after failed attempts.
// The backoff is counted from the moment the last attempt failed and the remainder is returned, so
// the handler comes back on its own: a failed evacuation produces no further events, and without
// that the machine would keep a condition promising retries while nothing retries.
func (h *EvacuationHandler) evacuate(ctx context.Context, vm *v1alpha2.VirtualMachine, finishedVMOPs []*v1alpha2.VirtualMachineOperation, log *slog.Logger) (time.Duration, error) {
	failedCount := 0
	var lastFailure time.Time
	for _, vmop := range finishedVMOPs {
		_, isEvacuation := vmop.GetAnnotations()[annotations.AnnVMOPEvacuation]
		if !isEvacuation || vmop.Status.Phase != v1alpha2.VMOPPhaseFailed {
			continue
		}

		failedCount++
		if failedAt := evacuationFailedAt(vmop); failedAt.After(lastFailure) {
			lastFailure = failedAt
		}
	}

	if wait := time.Until(lastFailure.Add(backoff.CalculateBackOff(failedCount))); wait > 0 {
		return wait, nil
	}

	log.Info("Create evacuation vmop")

	return 0, h.client.Create(ctx, newEvacuationVMOP(vm.GetName(), vm.GetNamespace()))
}

// evacuationFailedAt tells when the attempt failed. The Completed condition holds the moment the
// operation stopped; a missing timestamp falls back to the creation of the operation, which is never
// later than the failure and therefore only shortens the wait.
func evacuationFailedAt(vmop *v1alpha2.VirtualMachineOperation) time.Time {
	cond, found := conditions.GetCondition(vmopcondition.TypeCompleted, vmop.Status.Conditions)
	if found && !cond.LastTransitionTime.IsZero() {
		return cond.LastTransitionTime.Time
	}

	return vmop.GetCreationTimestamp().Time
}

func (h *EvacuationHandler) Name() string {
	return nameEvacuationHandler
}

func (h *EvacuationHandler) getVMOPsByVM(ctx context.Context, vm *v1alpha2.VirtualMachine) ([]*v1alpha2.VirtualMachineOperation, []*v1alpha2.VirtualMachineOperation, error) {
	vmops := v1alpha2.VirtualMachineOperationList{}
	err := h.client.List(ctx, &vmops, client.InNamespace(vm.GetNamespace()))
	if err != nil {
		return nil, nil, err
	}

	var (
		migrationVMOPs []*v1alpha2.VirtualMachineOperation
		finishedVMOPs  []*v1alpha2.VirtualMachineOperation
	)

	for _, vmop := range vmops.Items {
		if vmop.Spec.VirtualMachine != vm.GetName() || !commonvmop.IsMigration(&vmop) {
			continue
		}
		if commonvmop.IsFinished(&vmop) {
			finishedVMOPs = append(finishedVMOPs, &vmop)
		} else {
			migrationVMOPs = append(migrationVMOPs, &vmop)
		}
	}

	return migrationVMOPs, finishedVMOPs, nil
}

func (h *EvacuationHandler) removeFinalizerFromVMOPs(ctx context.Context, vmops []*v1alpha2.VirtualMachineOperation) error {
	var errs error
	for _, vmop := range vmops {
		if err := h.removeFinalizer(ctx, vmop); err != nil {
			errs = errors.Join(errs, err)
		}
	}
	return errs
}

func (h *EvacuationHandler) removeFinalizer(ctx context.Context, vmop *v1alpha2.VirtualMachineOperation) error {
	if controllerutil.RemoveFinalizer(vmop, v1alpha2.FinalizerVMOPProtectionByEvacuationController) {
		return h.client.Update(ctx, vmop)
	}
	return nil
}

// restartCannotWait reports whether the machine is due for a restart right now: an administrator
// allowed it on the node and is already waiting for the node, so the running evacuation has nothing
// left to win.
func (h *EvacuationHandler) restartCannotWait(ctx context.Context, vm *v1alpha2.VirtualMachine) (bool, error) {
	if evictionReason(vm) != vmcondition.ReasonRestartRequired.String() {
		return false, nil
	}

	return h.restartApprovedOnNode(ctx, vm)
}

// dropEvacuations deletes the evacuation operations of the machine. The deletion alone is enough:
// the finalizer keeps the object until the next reconcile cancels the migration behind it and
// releases it, which is the same path a manually deleted evacuation takes.
func (h *EvacuationHandler) dropEvacuations(ctx context.Context, vmops []*v1alpha2.VirtualMachineOperation, log *slog.Logger) error {
	var errs error
	for _, vmop := range vmops {
		if _, isEvacuation := vmop.GetAnnotations()[annotations.AnnVMOPEvacuation]; !isEvacuation {
			continue
		}
		if !vmop.GetDeletionTimestamp().IsZero() {
			continue
		}

		log.Info("Give up on the evacuation to restart the machine",
			slog.String("VMOPName", vmop.GetName()),
			slog.String("VMOPNamespace", vmop.GetNamespace()),
		)
		if err := h.client.Delete(ctx, vmop); err != nil && !k8serrors.IsNotFound(err) {
			errs = errors.Join(errs, err)
		}
	}
	return errs
}

func (h *EvacuationHandler) cancelEvacuationForTerminatingVMOPs(ctx context.Context, vmops []*v1alpha2.VirtualMachineOperation, log *slog.Logger) error {
	var errs error
	for _, vmop := range vmops {
		_, isEvacuation := vmop.GetAnnotations()[annotations.AnnVMOPEvacuation]
		if isEvacuation && !vmop.GetDeletionTimestamp().IsZero() {
			log.Info("VMOP terminating, cancel evacuation",
				slog.String("VMOPName", vmop.GetName()),
				slog.String("VMOPNamespace", vmop.GetNamespace()),
			)
			if err := h.evacuateCanceler.Cancel(ctx, vmop.Spec.VirtualMachine, vmop.GetNamespace()); err != nil {
				errs = errors.Join(errs, err)
				continue
			}
			if err := h.removeFinalizer(ctx, vmop); err != nil {
				errs = errors.Join(errs, err)
			}
		}
	}
	return errs
}

// newNodeMaintenanceRestartVMOP names the operation after the reason it exists: the owner sees in
// the list of operations that the restart came from the platform releasing a node, not from a person.
func newNodeMaintenanceRestartVMOP(vmName, namespace string) *v1alpha2.VirtualMachineOperation {
	return vmopbuilder.New(
		vmopbuilder.WithGenerateName("node-maintenance-restart-"),
		vmopbuilder.WithNamespace(namespace),
		vmopbuilder.WithAnnotation(annotations.AnnVMOPNodeMaintenance, "true"),
		vmopbuilder.WithType(v1alpha2.VMOPTypeRestart),
		vmopbuilder.WithVirtualMachine(vmName),
	)
}

func newEvacuationVMOP(vmName, namespace string) *v1alpha2.VirtualMachineOperation {
	return vmopbuilder.New(
		vmopbuilder.WithGenerateName("evacuation-"),
		vmopbuilder.WithNamespace(namespace),
		vmopbuilder.WithAnnotation(annotations.AnnVMOPEvacuation, "true"),
		vmopbuilder.WithFinalizer(v1alpha2.FinalizerVMOPProtectionByEvacuationController),
		vmopbuilder.WithType(v1alpha2.VMOPTypeEvict),
		vmopbuilder.WithVirtualMachine(vmName),
	)
}

func isVMNeedEvict(vm *v1alpha2.VirtualMachine) bool {
	cond, _ := conditions.GetCondition(vmcondition.TypeEvictionRequired, vm.Status.Conditions)
	return cond.Status == metav1.ConditionTrue
}

func evictionReason(vm *v1alpha2.VirtualMachine) string {
	cond, _ := conditions.GetCondition(vmcondition.TypeEvictionRequired, vm.Status.Conditions)
	return cond.Reason
}

// restartToReleaseNode restarts a machine that cannot leave its node by live migration and whose
// restart an administrator allowed on the node.
func (h *EvacuationHandler) restartToReleaseNode(ctx context.Context, vm *v1alpha2.VirtualMachine, log *slog.Logger) (reconcile.Result, error) {
	handled, err := h.restartAlreadyHandled(ctx, vm)
	if err != nil {
		return reconcile.Result{}, err
	}
	if handled {
		return reconcile.Result{}, nil
	}

	log.Info("Restart the virtual machine to release the node")

	vmop := newNodeMaintenanceRestartVMOP(vm.GetName(), vm.GetNamespace())
	if err = h.client.Create(ctx, vmop); err != nil {
		return reconcile.Result{}, err
	}

	h.recorder.Event(vm, corev1.EventTypeNormal, v1alpha2.ReasonVMNodeMaintenanceRestarted,
		"The virtual machine is being restarted to release the node.")

	return reconcile.Result{}, nil
}

// restartApprovedOnNode reports whether the administrator allowed restarting machines on the node
// this machine runs on. The condition handler of the virtual machine reads the same annotation; here
// it is read again because the status of the machine may lag behind a permission just taken back.
func (h *EvacuationHandler) restartApprovedOnNode(ctx context.Context, vm *v1alpha2.VirtualMachine) (bool, error) {
	if vm.Status.Node == "" {
		return false, nil
	}

	node := &corev1.Node{}
	err := h.client.Get(ctx, types.NamespacedName{Name: vm.Status.Node}, node)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("failed to get node %s: %w", vm.Status.Node, err)
	}

	if !node.Spec.Unschedulable {
		return false, nil
	}

	_, found := node.GetAnnotations()[annotations.AnnNodeVMRestartApproved]
	return found, nil
}

// restartAlreadyHandled reports whether the platform has to keep its hands off the machine: another
// operation is still running on it, or the restart for this maintenance has already been attempted.
// One attempt per maintenance is the whole contract: a restart that failed is not repeated in a
// loop, because an identical second attempt fixes nothing while the node stays occupied either way,
// and the aggregate alert asks a person to look instead.
func (h *EvacuationHandler) restartAlreadyHandled(ctx context.Context, vm *v1alpha2.VirtualMachine) (bool, error) {
	vmops := v1alpha2.VirtualMachineOperationList{}
	if err := h.client.List(ctx, &vmops, client.InNamespace(vm.GetNamespace())); err != nil {
		return false, err
	}

	maintenanceStartedAt := evictionRequiredSince(vm)

	for _, vmop := range vmops.Items {
		if vmop.Spec.VirtualMachine != vm.GetName() {
			continue
		}
		if !commonvmop.IsFinished(&vmop) {
			return true, nil
		}
		if _, found := vmop.GetAnnotations()[annotations.AnnVMOPNodeMaintenance]; !found {
			continue
		}
		if !vmop.GetCreationTimestamp().Time.Before(maintenanceStartedAt) {
			return true, nil
		}
	}

	return false, nil
}

// evictionRequiredSince tells when the machine started reporting the outcome it reports now: the
// condition moves its timestamp on every change of the reason, so the maintenance that promised the
// restart is told apart from the ones before it.
func evictionRequiredSince(vm *v1alpha2.VirtualMachine) time.Time {
	cond, _ := conditions.GetCondition(vmcondition.TypeEvictionRequired, vm.Status.Conditions)
	return cond.LastTransitionTime.Time
}

func isVMMigrating(vm *v1alpha2.VirtualMachine) bool {
	cond, _ := conditions.GetCondition(vmcondition.TypeMigrating, vm.Status.Conditions)
	return cond.Status == metav1.ConditionTrue
}
