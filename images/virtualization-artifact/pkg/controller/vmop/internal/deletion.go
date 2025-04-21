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

package internal

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	commonvmop "github.com/deckhouse/virtualization-controller/pkg/common/vmop"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

const deletionHandlerName = "DeletionHandler"

// DeletionHandler manages finalizers on VirtualMachineOperation resource.
type DeletionHandler struct {
	srvCreator SrvCreator
}

func NewDeletionHandler(srvCreator SrvCreator) *DeletionHandler {
	return &DeletionHandler{
		srvCreator: srvCreator,
	}
}

func (h DeletionHandler) Handle(ctx context.Context, vmop *virtv2.VirtualMachineOperation) (reconcile.Result, error) {
	if vmop == nil {
		return reconcile.Result{}, nil
	}

	_, ctx = logger.GetHandlerContext(ctx, deletionHandlerName)

	if commonvmop.IsMigration(vmop) {
		return h.migrateHandle(ctx, vmop)
	}
	return h.defaultHandle(ctx, vmop)
}

func (h DeletionHandler) migrateHandle(ctx context.Context, vmop *virtv2.VirtualMachineOperation) (reconcile.Result, error) {
	log := logger.FromContext(ctx)

	if vmop.DeletionTimestamp.IsZero() {
		log.Debug("Add cleanup finalizer")
		controllerutil.AddFinalizer(vmop, virtv2.FinalizerVMOPCleanup)
		return reconcile.Result{}, nil
	}

	log.Info("Deletion observed: cleanup VirtualMachineOperation")
	srv, err := h.srvCreator(vmop)
	if err != nil {
		return reconcile.Result{}, err
	}

	canceled, err := srv.Cancel(ctx)
	if err != nil {
		return reconcile.Result{}, err
	}
	if !canceled {
		return reconcile.Result{}, nil
	}

	controllerutil.RemoveFinalizer(vmop, virtv2.FinalizerVMOPCleanup)
	return reconcile.Result{}, nil
}

func (h DeletionHandler) defaultHandle(ctx context.Context, vmop *virtv2.VirtualMachineOperation) (reconcile.Result, error) {
	log := logger.FromContext(ctx)

	if vmop.DeletionTimestamp.IsZero() && vmop.Status.Phase == virtv2.VMOPPhaseInProgress {
		log.Debug("Add cleanup finalizer while in the InProgress phase")
		controllerutil.AddFinalizer(vmop, virtv2.FinalizerVMOPCleanup)
		return reconcile.Result{}, nil
	}

	// Remove finalizer when VirtualMachineOperation is in deletion state or not in progress.
	if vmop.DeletionTimestamp.IsZero() {
		log.Debug("Remove cleanup finalizer from VirtualMachineOperation: not InProgress state", "phase", vmop.Status.Phase)
	} else {
		log.Info("Deletion observed: remove cleanup finalizer from VirtualMachineOperation", "phase", vmop.Status.Phase)
	}
	controllerutil.RemoveFinalizer(vmop, virtv2.FinalizerVMOPCleanup)
	return reconcile.Result{}, nil
}

func (h DeletionHandler) Name() string {
	return deletionHandlerName
}
