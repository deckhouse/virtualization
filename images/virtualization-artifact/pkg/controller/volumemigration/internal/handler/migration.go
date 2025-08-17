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

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	vmopbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vmop"
	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	commonvd "github.com/deckhouse/virtualization-controller/pkg/common/vd"
	commonvmop "github.com/deckhouse/virtualization-controller/pkg/common/vmop"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdcondition"
)

const (
	MigrationHandlerName = "MigrationHandler"
)

type MigrationHandler struct {
	client client.Client
}

func NewMigrationHandler(client client.Client) *MigrationHandler {
	return &MigrationHandler{
		client: client,
	}
}

func (h *MigrationHandler) Handle(ctx context.Context, vd *v1alpha2.VirtualDisk) (reconcile.Result, error) {
	if !commonvd.StorageClassChanged(vd) {
		return reconcile.Result{}, nil
	}

	log, ctx := logger.GetHandlerContext(ctx, MigrationHandlerName)
	log.Info("Detected VirtualDisk with changed StorageClass")

	ready, _ := conditions.GetCondition(vdcondition.ReadyType, vd.Status.Conditions)
	if !(ready.Status == metav1.ConditionTrue && conditions.IsLastUpdated(ready, vd)) {
		log.Info("VirtualDisk is not ready. Cannot be migrated now. Skip...")
		return reconcile.Result{}, nil
	}

	vm, err := h.getVirtualMachine(ctx, vd)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			log.Info("VirtualMachine not found. Cannot be migrated now. Skip...")
			return reconcile.Result{RequeueAfter: 30 * time.Second}, nil
		}
		return reconcile.Result{}, err
	}

	migratingVMOPs, finishedVMOPs, err := h.getMigrationVMOPs(ctx, vm)
	if err != nil {
		return reconcile.Result{}, err
	}

	if len(migratingVMOPs) > 0 {
		log.Info("VirtualMachine is already migrating. Skip...")
		return reconcile.Result{}, nil
	}

	backoff := h.calculateBackoff(finishedVMOPs)
	if backoff > 0 {
		log.Info("VMOP will be created after the backoff", slog.Duration("backoff", backoff))
		return reconcile.Result{RequeueAfter: backoff}, nil
	}

	vmop := newVolumeMigrationVMOP(vm.Name, vm.Namespace)
	log.Info("Create VMOP", slog.String("vmop.generate-name", vmop.GenerateName), slog.String("vmop.namespace", vmop.Namespace))
	if err := h.client.Create(ctx, vmop); err != nil {
		return reconcile.Result{}, err
	}
	log.Debug("VMOP created", slog.String("vmop.name", vmop.Name), slog.String("vmop.namespace", vmop.Namespace))

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

func (h *MigrationHandler) getMigrationVMOPs(ctx context.Context, vm *v1alpha2.VirtualMachine) ([]*v1alpha2.VirtualMachineOperation, []*v1alpha2.VirtualMachineOperation, error) {
	vmops := &v1alpha2.VirtualMachineOperationList{}
	err := h.client.List(ctx, vmops, client.InNamespace(vm.Namespace))
	if err != nil {
		return nil, nil, err
	}

	var (
		migrationVMOPs []*v1alpha2.VirtualMachineOperation
		finishedVMOPs  []*v1alpha2.VirtualMachineOperation
	)
	for _, vmop := range vmops.Items {
		if vmop.Spec.VirtualMachine != vm.Name || !commonvmop.IsMigration(&vmop) {
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

func (h *MigrationHandler) calculateBackoff(finishedVMOPs []*v1alpha2.VirtualMachineOperation) time.Duration {
	slices.SortFunc(finishedVMOPs, func(a, b *v1alpha2.VirtualMachineOperation) int {
		return cmp.Compare(b.CreationTimestamp.Unix(), a.CreationTimestamp.Unix())
	})

	failedCount := 0
	for _, vmop := range finishedVMOPs {
		if vmop.Status.Phase == v1alpha2.VMOPPhaseFailed {
			failedCount++
		} else {
			break
		}
	}

	if failedCount == 0 {
		return 0
	}
	baseDelay := 5 * time.Second
	maxDelay := 5 * time.Minute

	// exponential backoff formula = baseDelay * 2^(failedCount - 1)
	backoff := baseDelay * time.Duration(1<<(failedCount-1))
	if backoff > maxDelay {
		backoff = maxDelay
	}

	return backoff
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
