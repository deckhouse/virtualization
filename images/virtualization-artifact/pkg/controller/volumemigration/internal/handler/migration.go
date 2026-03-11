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
	"cmp"
	"context"
	"log/slog"
	"slices"
	"time"

	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	vmopbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vmop"
	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	commonvd "github.com/deckhouse/virtualization-controller/pkg/common/vd"
	commonvmop "github.com/deckhouse/virtualization-controller/pkg/common/vmop"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdcondition"
)

const (
	MigrationHandlerName = "MigrationHandler"
)

type MigrationHandler struct {
	client     client.Client
	recorder   eventrecord.EventRecorderLogger
	backoffSvc *service.BackoffService
}

func NewMigrationHandler(client client.Client, recorder eventrecord.EventRecorderLogger) *MigrationHandler {
	return &MigrationHandler{
		client:     client,
		recorder:   recorder,
		backoffSvc: service.NewBackoffService(),
	}
}

func (h *MigrationHandler) Handle(ctx context.Context, vd *v1alpha2.VirtualDisk) (reconcile.Result, error) {
	if !commonvd.StorageClassChanged(vd) {
		return reconcile.Result{}, nil
	}

	if !vd.GetDeletionTimestamp().IsZero() {
		return reconcile.Result{}, nil
	}

	log, ctx := logger.GetHandlerContext(ctx, MigrationHandlerName)
	log.Info("Detected VirtualDisk with changed StorageClass")

	ready, _ := conditions.GetCondition(vdcondition.ReadyType, vd.Status.Conditions)
	if ready.Status != metav1.ConditionTrue || !conditions.IsLastUpdated(ready, vd) {
		h.recorder.Eventf(vd, corev1.EventTypeWarning, v1alpha2.ReasonVolumeMigrationCannotBeProcessed, "VirtualDisk is not ready. Cannot be migrated now.")
		return reconcile.Result{}, nil
	}
	migrating, _ := conditions.GetCondition(vdcondition.MigratingType, vd.Status.Conditions)
	if migrating.Status == metav1.ConditionTrue {
		h.recorder.Eventf(vd, corev1.EventTypeNormal, v1alpha2.ReasonVolumeMigrationCannotBeProcessed, "VirtualDisk is migrating. Cannot be migrated now.")
		return reconcile.Result{}, nil
	}

	vm, err := h.getVirtualMachine(ctx, vd)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			h.recorder.Eventf(vd, corev1.EventTypeWarning, v1alpha2.ReasonVolumeMigrationCannotBeProcessed, "VirtualMachine not found. Cannot be migrated now.")
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}

	migratingVMOPs, finishedVMOPs, err := h.getVMOPs(ctx, vm)
	if err != nil {
		return reconcile.Result{}, err
	}

	if len(migratingVMOPs) > 0 {
		log.Info("VirtualMachine is already migrating. Skip...")
		return reconcile.Result{}, nil
	}

	delay := h.calculateBackoff(finishedVMOPs, vm.GetCreationTimestamp())
	if delay > 0 {
		h.recorder.Eventf(vd, corev1.EventTypeNormal, v1alpha2.ReasonVolumeMigrationCannotBeProcessed, "VMOP will be created after the backoff. backoff: %q", delay.String())
		return reconcile.Result{RequeueAfter: delay}, nil
	}

	vmop := newVolumeMigrationVMOP(vm.Name, vm.Namespace)
	log.Info("Create VMOP", slog.String("vmop.generate-name", vmop.GenerateName), slog.String("vmop.namespace", vmop.Namespace))
	if err := h.client.Create(ctx, vmop); err != nil {
		return reconcile.Result{}, err
	}

	h.recorder.Eventf(vd, corev1.EventTypeNormal, v1alpha2.ReasonVMOPStarted, "Volume migration is started. vmop.name: %q, vmop.namespace: %q", vmop.Name, vmop.Namespace)

	return reconcile.Result{}, nil
}

func (h *MigrationHandler) Name() string {
	return MigrationHandlerName
}

func (h *MigrationHandler) getVirtualMachine(ctx context.Context, vd *v1alpha2.VirtualDisk) (*v1alpha2.VirtualMachine, error) {
	vmName := commonvd.GetCurrentlyMountedVMName(vd)
	vm := &v1alpha2.VirtualMachine{}
	err := h.client.Get(ctx, client.ObjectKey{Name: vmName, Namespace: vd.Namespace}, vm)
	return vm, err
}

func (h *MigrationHandler) getVMOPs(ctx context.Context, vm *v1alpha2.VirtualMachine) (finishedVMOPs, migrationVMOPs []*v1alpha2.VirtualMachineOperation, err error) {
	vmops := &v1alpha2.VirtualMachineOperationList{}
	err = h.client.List(ctx, vmops, client.InNamespace(vm.Namespace))
	if err != nil {
		return nil, nil, err
	}

	for _, vmop := range vmops.Items {
		if vmop.Spec.VirtualMachine != vm.Name {
			continue
		}

		if commonvmop.IsFinished(&vmop) {
			finishedVMOPs = append(finishedVMOPs, &vmop)
			continue
		}

		if commonvmop.IsMigration(&vmop) {
			migrationVMOPs = append(migrationVMOPs, &vmop)
		}
	}

	return migrationVMOPs, finishedVMOPs, nil
}

func (h *MigrationHandler) calculateBackoff(finishedVMOPs []*v1alpha2.VirtualMachineOperation, after metav1.Time) time.Duration {
	// sort from the latest to the oldest
	slices.SortFunc(finishedVMOPs, func(a, b *v1alpha2.VirtualMachineOperation) int {
		return cmp.Compare(b.CreationTimestamp.UnixNano(), a.CreationTimestamp.UnixNano())
	})

	failedCount := 0
	for _, vmop := range finishedVMOPs {
		// we should calculate the backoff only for the last failed VMOP migrations in a row
		if commonvmop.IsMigration(vmop) && vmop.Status.Phase == v1alpha2.VMOPPhaseFailed && vmop.CreationTimestamp.After(after.Time) {
			failedCount++
			continue
		}

		break
	}

	return h.backoffSvc.CalculateBackoff(failedCount)
}

func newVolumeMigrationVMOP(vmName, namespace string) *v1alpha2.VirtualMachineOperation {
	return vmopbuilder.New(
		vmopbuilder.WithGenerateName("volume-migration-"),
		vmopbuilder.WithNamespace(namespace),
		vmopbuilder.WithAnnotation(annotations.AnnVMOPVolumeMigration, "true"),
		vmopbuilder.WithType(v1alpha2.VMOPTypeEvict),
		vmopbuilder.WithVirtualMachine(vmName),
	)
}
