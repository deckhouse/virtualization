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

	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
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

func (h VirtualMachineReadyHandler) Handle(ctx context.Context, vmbda *v1alpha2.VirtualMachineBlockDeviceAttachment) (reconcile.Result, error) {
	cb := conditions.NewConditionBuilder(vmbdacondition.VirtualMachineReadyType)
	defer func() { conditions.SetCondition(cb.Generation(vmbda.Generation), &vmbda.Status.Conditions) }()

	if !conditions.HasCondition(cb.GetType(), vmbda.Status.Conditions) {
		cb.Status(metav1.ConditionUnknown).Reason(conditions.ReasonUnknown)
	}

	if vmbda.DeletionTimestamp != nil {
		cb.Status(metav1.ConditionUnknown).Reason(conditions.ReasonUnknown)
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
		cb.
			Status(metav1.ConditionFalse).
			Reason(vmbdacondition.VirtualMachineNotReady).
			Message(fmt.Sprintf("VirtualMachine %q not found.", vmKey.String()))
		return reconcile.Result{}, nil
	}

	switch vm.Status.Phase {
	case v1alpha2.MachineRunning, v1alpha2.MachineMigrating:
		// OK.
	case v1alpha2.MachineStopping, v1alpha2.MachineStopped, v1alpha2.MachineStarting:
		vmbda.Status.Phase = v1alpha2.BlockDeviceAttachmentPhasePending
		cb.
			Status(metav1.ConditionFalse).
			Reason(vmbdacondition.NotAttached).
			Message(fmt.Sprintf("VirtualMachine %q is %s: waiting for the VirtualMachine to be Running.", vm.Name, vm.Status.Phase))
		return reconcile.Result{}, nil
	default:
		cb.
			Status(metav1.ConditionFalse).
			Reason(vmbdacondition.VirtualMachineNotReady).
			Message(fmt.Sprintf("Waiting for the VirtualMachine %q to be Running.", vmKey.String()))
		return reconcile.Result{}, nil
	}

	kvvm, err := h.attachment.GetKVVM(ctx, vm)
	if err != nil {
		return reconcile.Result{}, err
	}

	if kvvm == nil {
		cb.
			Status(metav1.ConditionFalse).
			Reason(vmbdacondition.VirtualMachineNotReady).
			Message(fmt.Sprintf("VirtualMachine %q Running, but underlying InternalVirtualizationVirtualMachine not found.", vmKey.String()))
		return reconcile.Result{}, nil
	}

	kvvmi, err := h.attachment.GetKVVMI(ctx, vm)
	if err != nil {
		return reconcile.Result{}, err
	}

	if kvvmi == nil {
		cb.
			Status(metav1.ConditionFalse).
			Reason(vmbdacondition.VirtualMachineNotReady).
			Message(fmt.Sprintf("VirtualMachine %q Running, but underlying InternalVirtualizationVirtualMachineInstance not found.", vmKey.String()))
		return reconcile.Result{}, nil
	}

	cb.Status(metav1.ConditionTrue).Reason(vmbdacondition.VirtualMachineReady)

	return reconcile.Result{}, nil
}
