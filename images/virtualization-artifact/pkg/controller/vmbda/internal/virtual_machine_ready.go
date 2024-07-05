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
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmbdacondition"
)

type VirtualMachineReadyHandler struct {
	attachment *service.AttachmentService
}

func NewVirtualMachineReadyHandler(attachment *service.AttachmentService) *VirtualMachineReadyHandler {
	return &VirtualMachineReadyHandler{
		attachment: attachment,
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

	vm, err := h.attachment.GetVirtualMachine(ctx, vmbda.Spec.VirtualMachineName, vmbda.Namespace)
	if err != nil {
		return reconcile.Result{}, err
	}

	if vm == nil {
		condition.Status = metav1.ConditionFalse
		condition.Reason = vmbdacondition.VirtualMachineNotReady
		condition.Message = fmt.Sprintf("VirtualMachine %s not found.", vmKey.String())
		return reconcile.Result{}, nil
	}

	switch vm.Status.Phase {
	case virtv2.MachineRunning:
		// OK.
	case virtv2.MachineStopping, virtv2.MachineStopped, virtv2.MachineStarting:
		vmbda.Status.Phase = virtv2.BlockDeviceAttachmentPhasePending
		condition.Status = metav1.ConditionFalse
		condition.Reason = vmbdacondition.NotAttached
		condition.Message = fmt.Sprintf("VirtualMachine %s is %s: waiting for the VirtualMachine to be Running.", vm.Name, vm.Status.Phase)
		return reconcile.Result{}, nil
	default:
		condition.Status = metav1.ConditionFalse
		condition.Reason = vmbdacondition.VirtualMachineNotReady
		condition.Message = fmt.Sprintf("Waiting for the VirtualMachine %s to be Running.", vmKey.String())
		return reconcile.Result{}, nil
	}

	kvvm, err := h.attachment.GetKVVM(ctx, vm)
	if err != nil {
		return reconcile.Result{}, err
	}

	if kvvm == nil {
		condition.Status = metav1.ConditionFalse
		condition.Reason = vmbdacondition.VirtualMachineNotReady
		condition.Message = fmt.Sprintf("VirtualMachine %s Running, but underlying InternalVirtualizationVirtualMachine not found.", vmKey.String())
		return reconcile.Result{}, nil
	}

	kvvmi, err := h.attachment.GetKVVMI(ctx, vm)
	if err != nil {
		return reconcile.Result{}, err
	}

	if kvvmi == nil {
		condition.Status = metav1.ConditionFalse
		condition.Reason = vmbdacondition.VirtualMachineNotReady
		condition.Message = fmt.Sprintf("VirtualMachine %s Running, but underlying InternalVirtualizationVirtualMachineInstance not found.", vmKey.String())
		return reconcile.Result{}, nil
	}

	condition.Status = metav1.ConditionTrue
	condition.Reason = vmbdacondition.VirtualMachineReady
	condition.Message = ""
	return reconcile.Result{}, nil
}
