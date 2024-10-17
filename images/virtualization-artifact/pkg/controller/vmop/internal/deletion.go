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

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/vmop/internal/state"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

const deletionHandlerName = "DeletionHandler"

// DeletionHandler manages finalizers on VirtualMachineOperation resource.
type DeletionHandler struct {
	client client.Client
}

func NewDeletionHandler(client client.Client) *DeletionHandler {
	return &DeletionHandler{
		client: client,
	}
}

func (h DeletionHandler) Handle(ctx context.Context, s state.VMOperationState) (reconcile.Result, error) {
	log := logger.FromContext(ctx).With(logger.SlogHandler(deletionHandlerName))

	if s.VirtualMachineOperation() == nil {
		return reconcile.Result{}, nil
	}

	changed := s.VirtualMachineOperation().Changed()

	// The only case when we need finalizer on VirtualMachineOperation is when operation is in progress.
	if changed.DeletionTimestamp == nil && changed.Status.Phase == virtv2.VMOPPhaseInProgress {
		log.Debug("Add cleanup finalizer while in the InProgress phase")
		controllerutil.AddFinalizer(changed, virtv2.FinalizerVMOPCleanup)
		return reconcile.Result{}, nil
	}

	// Remove finalizer when VirtualMachineOperation is in deletion state or not in progress.
	if changed.DeletionTimestamp != nil {
		log.Info("Deletion observed: remove cleanup finalizer from VirtualMachineOperation", "phase", changed.Status.Phase)
	} else {
		log.Debug("Remove cleanup finalizer from VirtualMachineOperation: not InProgress state", "phase", changed.Status.Phase)
	}
	controllerutil.RemoveFinalizer(changed, virtv2.FinalizerVMOPCleanup)
	return reconcile.Result{}, nil
}

func (h DeletionHandler) Name() string {
	return deletionHandlerName
}
