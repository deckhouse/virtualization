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

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
)

const deletionHandlerName = "DeletionHandler"

// DeletionHandler manages finalizers on VirtualMachineOperation resource.
type DeletionHandler struct {
	client client.Client
}

func NewDeletionHandler(client client.Client) *DeletionHandler {
	return &DeletionHandler{client: client}
}

func (h DeletionHandler) Handle(ctx context.Context, vmop *v1alpha2.VirtualMachineOperation) (reconcile.Result, error) {
	log := logger.FromContext(ctx)

	// Add finalizer for operations in progress
	if vmop.DeletionTimestamp.IsZero() {
		if vmop.Status.Phase == v1alpha2.VMOPPhaseInProgress {
			log.Debug("Add cleanup finalizer while in the InProgress phase")
			controllerutil.AddFinalizer(vmop, v1alpha2.FinalizerVMOPCleanup)
		}

		return reconcile.Result{}, nil
	}

	vmKey := types.NamespacedName{Namespace: vmop.Namespace, Name: vmop.Spec.VirtualMachine}
	vm, err := object.FetchObject(ctx, vmKey, h.client, &v1alpha2.VirtualMachine{})
	if err != nil {
		log.Debug("Failed to fetch VirtualMachine", logger.SlogErr(err))
		return reconcile.Result{}, err
	}

	if vm == nil {
		controllerutil.RemoveFinalizer(vmop, v1alpha2.FinalizerVMOPCleanup)
		return reconcile.Result{}, nil
	}

	// Clean up maintenance mode if VM is in maintenance for restore operation
	maintenanceCondition, found := conditions.GetCondition(vmcondition.TypeMaintenance, vm.Status.Conditions)
	if found && maintenanceCondition.Status == metav1.ConditionTrue && maintenanceCondition.Reason == vmcondition.ReasonMaintenanceRestore.String() {
		conditions.SetCondition(
			conditions.NewConditionBuilder(vmcondition.TypeMaintenance).
				Generation(vm.GetGeneration()).
				Reason(vmcondition.ReasonMaintenanceRestore).
				Status(metav1.ConditionFalse).
				Message("VM exited maintenance mode due to vmop deletion."),
			&vm.Status.Conditions,
		)

		err = h.client.Status().Update(ctx, vm)
		if err != nil {
			if apierrors.IsConflict(err) {
				return reconcile.Result{}, nil
			}

			log.Error("Failed to exit maintenance mode during deletion", logger.SlogErr(err))
			return reconcile.Result{}, err
		}

		log.Info("VM exited maintenance mode due to vmop deletion")
	}

	// Remove finalizer when VirtualMachineOperation is in deletion state or not in progress.
	if vmop.DeletionTimestamp.IsZero() {
		log.Debug("Remove cleanup finalizer from VirtualMachineOperation: not InProgress state", "phase", vmop.Status.Phase)
	} else {
		log.Info("Deletion observed: remove cleanup finalizer from VirtualMachineOperation", "phase", vmop.Status.Phase)
	}
	controllerutil.RemoveFinalizer(vmop, v1alpha2.FinalizerVMOPCleanup)

	return reconcile.Result{}, nil
}

func (h DeletionHandler) Name() string {
	return deletionHandlerName
}
