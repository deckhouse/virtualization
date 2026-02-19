/*
Copyright 2024 Flant JSC

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

	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/vmop/migration/internal/service"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

const deletionHandlerName = "DeletionHandler"

type DeletionHandler struct {
	migration *service.MigrationService
}

func NewDeletionHandler(migration *service.MigrationService) *DeletionHandler {
	return &DeletionHandler{
		migration: migration,
	}
}

func (h DeletionHandler) Handle(ctx context.Context, vmop *v1alpha2.VirtualMachineOperation) (reconcile.Result, error) {
	if vmop == nil {
		return reconcile.Result{}, nil
	}

	log, _ := logger.GetHandlerContext(ctx, deletionHandlerName)

	if vmop.DeletionTimestamp.IsZero() {
		log.Debug("Add cleanup finalizer")
		controllerutil.AddFinalizer(vmop, v1alpha2.FinalizerVMOPCleanup)
		return reconcile.Result{}, nil
	}

	log.Info("Deletion observed: cleanup VirtualMachineOperation")

	// Delete migration if exists.
	err := h.migration.DeleteMigration(ctx, vmop)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("failed to delete migration: %w", err)
	}

	// Check if migration removed.
	mig, err := h.migration.GetMigration(ctx, vmop)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("failed to get migration after deletion': %w", err)
	}

	if mig != nil {
		return reconcile.Result{}, nil
	}

	controllerutil.RemoveFinalizer(vmop, v1alpha2.FinalizerVMOPCleanup)
	return reconcile.Result{}, nil
}

func (h DeletionHandler) Name() string {
	return deletionHandlerName
}
