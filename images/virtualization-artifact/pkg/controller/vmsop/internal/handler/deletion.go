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

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/logger"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

const deletionHandlerName = "DeletionHandler"

// DeletionHandler manages finalizers on VirtualMachineSnapshotOperation resource.
type DeletionHandler struct {
	client client.Client
}

func NewDeletionHandler(client client.Client) *DeletionHandler {
	return &DeletionHandler{client: client}
}

func (h DeletionHandler) Handle(ctx context.Context, vmsop *v1alpha2.VirtualMachineSnapshotOperation) (reconcile.Result, error) {
	log := logger.FromContext(ctx)

	// Add finalizer for operations in progress
	if vmsop.DeletionTimestamp.IsZero() {
		if vmsop.Status.Phase == v1alpha2.VMSOPPhaseInProgress {
			log.Debug("Add cleanup finalizer while in the InProgress phase")
			controllerutil.AddFinalizer(vmsop, v1alpha2.FinalizerVMSOPCleanup)
		}

		return reconcile.Result{}, nil
	}

	// Remove finalizer when VirtualMachineSnapshotOperation is in deletion state or not in progress.
	if vmsop.DeletionTimestamp.IsZero() {
		log.Debug("Remove cleanup finalizer from VirtualMachineSnapshotOperation: not InProgress state", "phase", vmsop.Status.Phase)
	} else {
		log.Info("Deletion observed: remove cleanup finalizer from VirtualMachineSnapshotOperation", "phase", vmsop.Status.Phase)
	}
	controllerutil.RemoveFinalizer(vmsop, v1alpha2.FinalizerVMSOPCleanup)

	return reconcile.Result{}, nil
}

func (h DeletionHandler) Name() string {
	return deletionHandlerName
}
