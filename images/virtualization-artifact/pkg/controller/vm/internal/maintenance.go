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
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/reconciler"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vm/internal/state"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
)

const nameMaintenanceHandler = "MaintenanceHandler"

type MaintenanceHandler struct {
	client client.Client
}

func NewMaintenanceHandler(client client.Client) *MaintenanceHandler {
	return &MaintenanceHandler{
		client: client,
	}
}

func (h *MaintenanceHandler) Handle(ctx context.Context, s state.VirtualMachineState) (reconcile.Result, error) {
	log := logger.FromContext(ctx)

	if s.VirtualMachine().IsEmpty() {
		return reconcile.Result{}, nil
	}
	changed := s.VirtualMachine().Changed()

	maintenance, _ := conditions.GetCondition(vmcondition.TypeMaintenance, changed.Status.Conditions)

	if maintenance.Status == metav1.ConditionFalse {
		conditions.RemoveCondition(vmcondition.TypeMaintenance, &changed.Status.Conditions)
		return reconcile.Result{}, nil
	}

	if maintenance.Status != metav1.ConditionTrue {
		return reconcile.Result{}, nil
	}

	log.Info("Reconcile observe a VirtualMachine in maintenance mode")

	kvvmi, err := s.KVVMI(ctx)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("get KVVMI: %w", err)
	}

	// If VM is not stopped yet, wait for it to stop (SyncPowerStateHandler will handle stopping)
	if kvvmi != nil {
		log.Info("VM is still running, waiting for shutdown in maintenance mode")
		return reconcile.Result{}, nil
	}

	// Hide all other conditions when in maintenance mode
	if changed.Status.Conditions != nil {
		var newConditions []metav1.Condition
		for _, cond := range changed.Status.Conditions {
			if vmcondition.Type(cond.Type) == vmcondition.TypeMaintenance {
				newConditions = append(newConditions, cond)
			}
		}
		changed.Status.Conditions = newConditions
	}

	log.Info("VM is stopped, cleaning up resources if any for maintenance mode")

	kvvm, err := s.KVVM(ctx)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("%w: get KVVM: %w", reconciler.ErrStopHandlerChain, err)
	}
	if kvvm != nil {
		log.Info("Deleting KVVM for maintenance mode")
		err = object.CleanupObject(ctx, h.client, kvvm)
		if err != nil {
			return reconcile.Result{}, fmt.Errorf("%w: delete KVVM: %w", reconciler.ErrStopHandlerChain, err)
		}
	}

	pods, err := s.Pods(ctx)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("%w: get pods: %w", reconciler.ErrStopHandlerChain, err)
	}
	if pods != nil && len(pods.Items) > 0 {
		log.Info("Deleting pods for maintenance mode")
		for i := range pods.Items {
			err = object.CleanupObject(ctx, h.client, &pods.Items[i])
			if err != nil {
				return reconcile.Result{}, fmt.Errorf("%w: delete pod: %w", reconciler.ErrStopHandlerChain, err)
			}
		}
	}

	return reconcile.Result{}, reconciler.ErrStopHandlerChain
}

func (h *MaintenanceHandler) Name() string {
	return nameMaintenanceHandler
}
