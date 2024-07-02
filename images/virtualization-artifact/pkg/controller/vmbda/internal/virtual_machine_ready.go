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
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmbdacondition"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
)

type VirtualMachineReadyHandler struct {
	client client.Client
}

func NewVirtualMachineReadyHandler(client client.Client) *VirtualMachineReadyHandler {
	return &VirtualMachineReadyHandler{
		client: client,
	}
}

func (h VirtualMachineReadyHandler) Handle(ctx context.Context, vmbda *virtv2.VirtualMachineBlockDeviceAttachment) (reconcile.Result, error) {
	condition, ok := service.GetCondition(vmbdacondition.VirtualMachineReadyType, vmbda.Status.Conditions)
	if !ok {
		condition = metav1.Condition{
			Type:   vmbdacondition.VirtualMachineReadyType,
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

	vmKey := types.NamespacedName{
		Name:      vmbda.Spec.VirtualMachineName,
		Namespace: vmbda.Namespace,
	}

	var vm virtv2.VirtualMachine
	err := h.client.Get(ctx, vmKey, &vm, &client.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			condition.Status = metav1.ConditionFalse
			condition.Reason = vmbdacondition.VirtualMachineNotReady
			condition.Message = fmt.Sprintf("VirtualMachine %s not found.", vmKey.String())
			return reconcile.Result{}, nil
		}

		return reconcile.Result{}, err
	}

	var runningCondition metav1.Condition
	runningCondition, ok = service.GetCondition(vmcondition.TypeRunning.String(), vm.Status.Conditions)
	if !ok || vm.Generation != runningCondition.ObservedGeneration {
		condition.Status = metav1.ConditionFalse
		condition.Reason = vmbdacondition.VirtualMachineNotReady
		condition.Message = fmt.Sprintf("Waiting for the VirtualMachine %s condition %s to be observed in its latest state generation.", vmcondition.TypeRunning, vmKey.String())
		return reconcile.Result{}, nil
	}

	if runningCondition.Status != metav1.ConditionTrue {
		condition.Status = metav1.ConditionFalse
		condition.Reason = vmbdacondition.VirtualMachineNotReady
		condition.Message = fmt.Sprintf("Waiting for the VirtualMachine %s to be Running.", vmKey.String())
		return reconcile.Result{}, nil
	}

	// TODO: what if vm stopped or starting? Or kvvmi missed?

	condition.Status = metav1.ConditionTrue
	condition.Reason = vmbdacondition.VirtualMachineReady
	condition.Message = ""
	return reconcile.Result{}, nil
}
