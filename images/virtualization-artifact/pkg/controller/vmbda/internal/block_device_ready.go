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

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdcondition"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmbdacondition"
)

type BlockDeviceReadyHandler struct {
	client client.Client
}

func NewBlockDeviceReadyHandler(client client.Client) *BlockDeviceReadyHandler {
	return &BlockDeviceReadyHandler{
		client: client,
	}
}

func (h BlockDeviceReadyHandler) Handle(ctx context.Context, vmbda *virtv2.VirtualMachineBlockDeviceAttachment) (reconcile.Result, error) {
	condition, ok := service.GetCondition(vmbdacondition.BlockDeviceReadyType, vmbda.Status.Conditions)
	if !ok {
		condition = metav1.Condition{
			Type:   vmbdacondition.BlockDeviceReadyType,
			Status: metav1.ConditionUnknown,
		}
	}

	defer func() { service.SetCondition(condition, &vmbda.Status.Conditions) }()

	if vmbda.DeletionTimestamp != nil {
		condition.Status = metav1.ConditionUnknown
		condition.Reason = ""
		condition.Message = ""
		return reconcile.Result{}, nil
	}

	switch vmbda.Spec.BlockDeviceRef.Kind {
	case virtv2.VMBDAObjectRefKindVirtualDisk:
		vdKey := types.NamespacedName{
			Name:      vmbda.Spec.BlockDeviceRef.Name,
			Namespace: vmbda.Namespace,
		}

		var vd virtv2.VirtualDisk
		err := h.client.Get(ctx, vdKey, &vd, &client.GetOptions{})
		if err != nil {
			if errors.IsNotFound(err) {
				condition.Status = metav1.ConditionFalse
				condition.Reason = vmbdacondition.BlockDeviceNotReady
				condition.Message = fmt.Sprintf("VirtualDisk %s not found.", vdKey.String())
				return reconcile.Result{}, nil
			}

			return reconcile.Result{}, err
		}

		if vd.Generation != vd.Status.ObservedGeneration {
			condition.Status = metav1.ConditionFalse
			condition.Reason = vmbdacondition.BlockDeviceNotReady
			condition.Message = fmt.Sprintf("Waiting for the VirtualDisk %s to be observed in its latest state generation.", vdKey.String())
			return reconcile.Result{}, nil
		}

		var diskReadyCondition metav1.Condition
		diskReadyCondition, ok = service.GetCondition(vdcondition.ReadyType, vd.Status.Conditions)
		if !ok || diskReadyCondition.Status != metav1.ConditionTrue {
			condition.Status = metav1.ConditionFalse
			condition.Reason = vmbdacondition.BlockDeviceNotReady
			condition.Message = fmt.Sprintf("VirtualDisk %s is not Ready: waiting for the VirtualDisk to be Ready.", vdKey.String())
			return reconcile.Result{}, nil
		}

		condition.Status = metav1.ConditionTrue
		condition.Reason = vmbdacondition.BlockDeviceReady
		condition.Message = ""
		return reconcile.Result{}, nil
	default:
		return reconcile.Result{}, fmt.Errorf("unknown block device kind %s", vmbda.Spec.BlockDeviceRef.Kind)
	}
}
