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
	"log/slog"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	commonvm "github.com/deckhouse/virtualization-controller/pkg/common/vm"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vi/internal/source"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vicondition"
)

const deletionHandlerName = "DeletionHandler"

type DeletionHandler struct {
	sources *source.Sources
	client  client.Client
}

func NewDeletionHandler(sources *source.Sources, client client.Client) *DeletionHandler {
	return &DeletionHandler{
		sources: sources,
		client:  client,
	}
}

func (h DeletionHandler) Handle(ctx context.Context, vi *v1alpha2.VirtualImage) (reconcile.Result, error) {
	log := logger.FromContext(ctx).With(logger.SlogHandler(deletionHandlerName))

	if vi.DeletionTimestamp != nil {
		if controllerutil.ContainsFinalizer(vi, v1alpha2.FinalizerVIProtection) {
			attachedVMs, err := commonvm.MountedVirtualMachineNames(ctx, h.client, v1alpha2.ImageDevice, vi.GetName(), vi.GetNamespace(), false)
			if err != nil {
				return reconcile.Result{}, err
			}

			h.setDeletingCondition(
				vi,
				vicondition.DeletionBlockedByProtection,
				service.DeletionBlockedByProtectionMessage("VirtualImage", attachedVMs),
			)
			return reconcile.Result{}, nil
		}

		requeue, reason, err := h.sources.CleanUp(ctx, vi)
		if err != nil {
			return reconcile.Result{}, err
		}

		if requeue {
			h.setDeletingCondition(vi, vicondition.DeletionCleanupPending, reason)
			log.Info("VirtualImage cleanup is pending", slog.String("reason", reason))
			return reconcile.Result{RequeueAfter: time.Second}, nil
		}

		conditions.RemoveCondition(vicondition.DeletingType, &vi.Status.Conditions)
		log.Info("Deletion observed: remove cleanup finalizer from VirtualImage")
		controllerutil.RemoveFinalizer(vi, v1alpha2.FinalizerVICleanup)
		return reconcile.Result{}, nil
	}

	conditions.RemoveCondition(vicondition.DeletingType, &vi.Status.Conditions)
	controllerutil.AddFinalizer(vi, v1alpha2.FinalizerVICleanup)
	return reconcile.Result{}, nil
}

func (h DeletionHandler) setDeletingCondition(vi *v1alpha2.VirtualImage, reason vicondition.DeletingReason, message string) {
	service.SetDeletingCondition(&vi.Status.Conditions, vicondition.DeletingType, reason, vi.Generation, message)
}

func (h DeletionHandler) Name() string {
	return "DeletionHandler"
}
