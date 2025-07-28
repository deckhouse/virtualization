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
	"time"

	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/vd/internal/source"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

const deletionHandlerName = "DeletionHandler"

type DeletionHandler struct {
	sources *source.Sources
}

func NewDeletionHandler(sources *source.Sources) *DeletionHandler {
	return &DeletionHandler{
		sources: sources,
	}
}

func (h DeletionHandler) Handle(ctx context.Context, vd *virtv2.VirtualDisk) (reconcile.Result, error) {
	log := logger.FromContext(ctx).With(logger.SlogHandler(deletionHandlerName))

	if vd.DeletionTimestamp != nil && !controllerutil.ContainsFinalizer(vd, virtv2.FinalizerVDProtection) {
		requeue, err := h.sources.CleanUp(ctx, vd)
		if err != nil {
			return reconcile.Result{}, err
		}

		if requeue {
			return reconcile.Result{RequeueAfter: time.Second}, nil
		}

		log.Info("Deletion observed: remove cleanup finalizer from VirtualDisk")
		controllerutil.RemoveFinalizer(vd, virtv2.FinalizerVDCleanup)
		return reconcile.Result{}, nil
	}

	controllerutil.AddFinalizer(vd, virtv2.FinalizerVDCleanup)
	return reconcile.Result{}, nil
}
