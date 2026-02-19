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
	"log/slog"
	"time"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	vmopbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vmop"
	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/backoff"
	commonvmop "github.com/deckhouse/virtualization-controller/pkg/common/vmop"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
)

const nameEvacuationHandler = "EvacuationHandler"

func NewEvacuationHandler(client client.Client, evacuateCanceler EvacuateCanceler) *EvacuationHandler {
	return &EvacuationHandler{
		client:           client,
		evacuateCanceler: evacuateCanceler,
	}
}

//go:generate go tool moq -rm -out mock.go . EvacuateCanceler
type EvacuateCanceler interface {
	Cancel(ctx context.Context, name, namespace string) error
}

type EvacuationHandler struct {
	client           client.Client
	evacuateCanceler EvacuateCanceler
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
		return reconcile.Result{RequeueAfter: requeueAfter}, nil
	}

	if !isVMNeedEvict(vm) || isVMMigrating(vm) {
		return reconcile.Result{}, nil
	}

	failedCount := 0
	for _, vmop := range finishedVMOPs {
		_, isEvacuation := vmop.GetAnnotations()[annotations.AnnVMOPEvacuation]
		if isEvacuation && vmop.Status.Phase == v1alpha2.VMOPPhaseFailed {
			failedCount++
		}
	}

	backoff := backoff.CalculateBackOff(failedCount)
	if backoff > 0 {
		return reconcile.Result{RequeueAfter: backoff}, nil
	}

	log.Info("Create evacuation vmop")
	vmop := newEvacuationVMOP(vm.GetName(), vm.GetNamespace())
	if err = h.client.Create(ctx, vmop); err != nil {
		return reconcile.Result{}, err
	}

	return reconcile.Result{RequeueAfter: requeueAfter}, nil
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

func isVMMigrating(vm *v1alpha2.VirtualMachine) bool {
	cond, _ := conditions.GetCondition(vmcondition.TypeMigrating, vm.Status.Conditions)
	return cond.Status == metav1.ConditionTrue
}
