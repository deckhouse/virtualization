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

	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/cvi/internal/source"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/cvicondition"
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

func (h DeletionHandler) Handle(ctx context.Context, cvi *v1alpha2.ClusterVirtualImage) (reconcile.Result, error) {
	log := logger.FromContext(ctx).With(logger.SlogHandler(deletionHandlerName))

	if cvi.DeletionTimestamp != nil {
		if controllerutil.ContainsFinalizer(cvi, v1alpha2.FinalizerCVIProtection) {
			// The deletion is held by a VirtualMachine. The InUse condition set by the
			// AttacheeHandler already names the VirtualMachines and how to release the image.
			log.Debug("ClusterVirtualImage deletion is blocked by the protection finalizer")
			return reconcile.Result{}, nil
		}

		requeue, reason, err := h.sources.CleanUp(ctx, cvi)
		if err != nil {
			return reconcile.Result{}, err
		}

		if requeue {
			h.setTerminatingCondition(cvi, cvicondition.CleanupPending, reason)
			log.Info("ClusterVirtualImage cleanup is pending", slog.String("reason", reason))
			return reconcile.Result{RequeueAfter: time.Second}, nil
		}

		conditions.RemoveCondition(cvicondition.TerminatingType, &cvi.Status.Conditions)
		log.Info("Deletion observed: remove cleanup finalizer from ClusterVirtualImage")
		controllerutil.RemoveFinalizer(cvi, v1alpha2.FinalizerCVICleanup)
		return reconcile.Result{}, nil
	}

	conditions.RemoveCondition(cvicondition.TerminatingType, &cvi.Status.Conditions)
	controllerutil.AddFinalizer(cvi, v1alpha2.FinalizerCVICleanup)
	return reconcile.Result{}, nil
}

func (h DeletionHandler) setTerminatingCondition(cvi *v1alpha2.ClusterVirtualImage, reason cvicondition.TerminatingReason, message string) {
	service.SetTerminatingCondition(&cvi.Status.Conditions, cvicondition.TerminatingType, reason, cvi.Generation, message)
}
