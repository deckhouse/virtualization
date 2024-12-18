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
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	cdiv1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/supplements"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdcondition"
)

const (
	DVBoundConditionType     string = "Bound"
	DVRunningConditionType   string = "Running"
	DVReadyConditionType     string = "Ready"
	DVErrExceededQuotaReason string = "ErrExceededQuota"
)

type QuotaExceededHandler struct {
	diskService *service.DiskService
}

func NewQuotaExceededHandler(client client.Client) *QuotaExceededHandler {
	return &QuotaExceededHandler{
		diskService: service.NewDiskService(client, nil, nil),
	}
}

func (q *QuotaExceededHandler) Handle(ctx context.Context, vd *virtv2.VirtualDisk) (reconcile.Result, error) {
	if vd.DeletionTimestamp != nil {
		return reconcile.Result{}, nil
	}

	cb := conditions.NewConditionBuilder(vdcondition.QuotaNotExceededType).Generation(vd.Generation)
	defer func() {
		conditions.SetCondition(cb, &vd.Status.Conditions)
	}()

	readyCondition, ok := conditions.GetCondition(vdcondition.ReadyType, vd.Status.Conditions)
	if !ok || readyCondition.Status == metav1.ConditionTrue {
		return reconcile.Result{}, nil
	}

	supgen := supplements.NewGenerator(annotations.VDShortName, vd.Name, vd.Namespace, vd.UID)
	dv, err := q.diskService.GetDataVolume(ctx, supgen)
	if err != nil {
		return reconcile.Result{}, err
	}

	if dv == nil {
		return reconcile.Result{}, nil
	}

	boundCondition, ok := getDVCondition(dv, DVBoundConditionType)
	if !ok {
		return reconcile.Result{}, nil
	}

	readyDVCondition, ok := getDVCondition(dv, DVReadyConditionType)
	if !ok {
		return reconcile.Result{}, nil
	}

	runningCondition, ok := getDVCondition(dv, DVRunningConditionType)
	if !ok {
		return reconcile.Result{}, nil
	}

	if runningCondition.Status == corev1.ConditionTrue {
		cb.
			Reason(vdcondition.QuotaNotExceeded).
			Message("")
	} else {
		switch {
		case boundCondition.Reason == DVErrExceededQuotaReason:
			cb.
				Status(metav1.ConditionFalse).
				Reason(vdcondition.QuotaExceeded).
				Message(service.CapitalizeFirstLetter(boundCondition.Message))
		case readyDVCondition.Reason == DVErrExceededQuotaReason:
			cb.
				Status(metav1.ConditionFalse).
				Reason(vdcondition.QuotaExceeded).
				Message(service.CapitalizeFirstLetter(readyDVCondition.Message))
		case runningCondition.Reason == DVErrExceededQuotaReason:
			cb.
				Status(metav1.ConditionFalse).
				Reason(vdcondition.QuotaExceeded).
				Message(service.CapitalizeFirstLetter(runningCondition.Message))
		case strings.Contains(boundCondition.Message, "Pending"):
			cb.
				Reason(vdcondition.PVCIsPending).
				Message("PVC in pending state, if this continues for a long time, check the status of PVC and DataVolume manually.")
		}
	}

	return reconcile.Result{}, nil
}

func getDVCondition(dv *cdiv1.DataVolume, conditionType string) (*cdiv1.DataVolumeCondition, bool) {
	for _, cond := range dv.Status.Conditions {
		if string(cond.Type) == conditionType {
			return &cond, true
		}
	}

	return nil, false
}
