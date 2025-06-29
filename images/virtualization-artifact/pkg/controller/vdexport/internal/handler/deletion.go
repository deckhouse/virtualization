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

	"github.com/deckhouse/virtualization-controller/pkg/controller/vdexport/internal/service"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

const deletionHandlerName = "DeletionHandler"

func NewDeletionHandler(client client.Client, dataExportEnabled bool, sourceCreator ExportSourceCreator) *DeletionHandler {
	return &DeletionHandler{
		client:            client,
		dataExportEnabled: dataExportEnabled,
		sourceCreator:     sourceCreator,
	}
}

type DeletionHandler struct {
	client            client.Client
	dataExportEnabled bool
	sourceCreator     ExportSourceCreator
}

func (h *DeletionHandler) Name() string {
	return deletionHandlerName
}

func (h *DeletionHandler) Handle(ctx context.Context, vdexport *v1alpha2.VirtualDataExport) (reconcile.Result, error) {
	if vdexport.DeletionTimestamp.IsZero() {
		controllerutil.AddFinalizer(vdexport, v1alpha2.FinalizerVDExportCleanup)
		return reconcile.Result{}, nil
	}

	source, err := h.sourceCreator(h.client, vdexport)
	if err != nil {
		return reconcile.Result{}, err
	}

	if source.Type() == service.DataExport && !h.dataExportEnabled {
		log, _ := logger.GetHandlerContext(ctx, deletionHandlerName)
		log.Warn("DataExport is disabled, skipping deletion of DataExport")
		return reconcile.Result{}, nil

	}

	err = source.CleanUp(ctx)
	if err != nil {
		return reconcile.Result{}, err
	}

	controllerutil.RemoveFinalizer(vdexport, v1alpha2.FinalizerVDExportCleanup)

	return reconcile.Result{}, nil
}
