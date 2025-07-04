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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vdexport/internal/service"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	vdexportcondition "github.com/deckhouse/virtualization/api/core/v1alpha2/vdexport-condition"
)

const lifecycleHandlerName = "LifecycleHandler"

func NewLifecycleHandler(client client.Client, dataExportEnabled bool, sourceCreator ExportSourceCreator) *LifecycleHandler {
	return &LifecycleHandler{
		client:            client,
		dataExportEnabled: dataExportEnabled,
		sourceCreator:     sourceCreator,
	}
}

type LifecycleHandler struct {
	client            client.Client
	dataExportEnabled bool
	sourceCreator     ExportSourceCreator
}

func (h *LifecycleHandler) Name() string {
	return lifecycleHandlerName
}

func (h *LifecycleHandler) Handle(ctx context.Context, vdexport *v1alpha2.VirtualDataExport) (reconcile.Result, error) {
	if isVDExportFinished(vdexport) {
		return reconcile.Result{}, nil
	}

	source, err := h.sourceCreator(h.client, vdexport)
	if err != nil {
		return reconcile.Result{}, err
	}
	cb := conditions.NewConditionBuilder(vdexportcondition.TypeCompleted).Generation(vdexport.Generation)

	if source.Type() == service.DataExport && !h.dataExportEnabled {
		log, _ := logger.GetHandlerContext(ctx, lifecycleHandlerName)
		log.Warn("DataExport is disabled, skipping lifecycle of DataExport")
		cb.
			Status(metav1.ConditionFalse).
			Reason(vdexportcondition.ReasonPending).
			Message("DataExport is disabled. Please enable module for using VirtualDataExport with VirtualDisk.")
		conditions.SetCondition(cb, &vdexport.Status.Conditions)
		return reconcile.Result{}, nil
	}

	err = source.Prepare(ctx)
	if err != nil {
		return reconcile.Result{}, err
	}

	status, err := source.Status(ctx)
	if err != nil {
		return reconcile.Result{}, err
	}
	vdexport.Status.URL = status.URL
	cb.
		Status(status.CompletedStatus).
		Reason(status.CompletedReason).
		Message(status.CompletedMessage)
	conditions.SetCondition(cb, &vdexport.Status.Conditions)

	return reconcile.Result{}, nil
}

func isVDExportFinished(vdexport *v1alpha2.VirtualDataExport) bool {
	if vdexport == nil || !vdexport.GetDeletionTimestamp().IsZero() {
		return true
	}

	completed, _ := conditions.GetCondition(vdexportcondition.TypeCompleted, vdexport.Status.Conditions)
	return completed.Status == metav1.ConditionTrue || (completed.Status == metav1.ConditionFalse && completed.Reason == vdexportcondition.ReasonFailed.String())
}
