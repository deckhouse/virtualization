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
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdcondition"
)

type AttacheeHandler struct {
	client client.Client
}

func NewAttacheeHandler(client client.Client) *AttacheeHandler {
	return &AttacheeHandler{
		client: client,
	}
}

func (h AttacheeHandler) Handle(ctx context.Context, vd *virtv2.VirtualDisk) (reconcile.Result, error) {
	log := logger.FromContext(ctx).With(logger.SlogHandler("attachee"))

	lockedCondition, ok := service.GetCondition(vdcondition.LockedType, vd.Status.Conditions)
	if !ok {
		lockedCondition = metav1.Condition{
			Type:   vdcondition.LockedType,
			Status: metav1.ConditionUnknown,
		}

		service.SetCondition(lockedCondition, &vd.Status.Conditions)
	}

	var isLocked bool

	attachedVMs, err := h.getAttachedVM(ctx, vd)
	if err != nil {
		return reconcile.Result{}, err
	}

	vd.Status.AttachedToVirtualMachines = nil

	for _, vm := range attachedVMs {
		vd.Status.AttachedToVirtualMachines = append(vd.Status.AttachedToVirtualMachines, virtv2.AttachedVirtualMachine{
			Name: vm.GetName(),
		})

		if vm.Status.Phase == virtv2.MachineRunning {
			isLocked = true
		}
	}

	if isLocked {
		lockedCondition.Status = metav1.ConditionTrue
		lockedCondition.Reason = vdcondition.Locked
		lockedCondition.Message = "VirtualDisk attached to running VirtualMachine"
	} else {
		lockedCondition.Status = metav1.ConditionFalse
		lockedCondition.Reason = ""
		lockedCondition.Message = ""
	}
	service.SetCondition(lockedCondition, &vd.Status.Conditions)

	if len(vd.Status.AttachedToVirtualMachines) > 1 {
		log.Error("virtual disk connected to multiple virtual machines", "vms", len(attachedVMs))
	}

	switch {
	case len(vd.Status.AttachedToVirtualMachines) == 0:
		log.Debug("Allow virtual disk deletion")
		controllerutil.RemoveFinalizer(vd, virtv2.FinalizerVDProtection)
	case vd.DeletionTimestamp == nil:
		log.Debug("Protect virtual disk from deletion")
		controllerutil.AddFinalizer(vd, virtv2.FinalizerVDProtection)
	default:
		log.Debug("Virtual disk deletion is delayed: it's protected by virtual machines")
	}

	return reconcile.Result{}, nil
}

func (h AttacheeHandler) getAttachedVM(ctx context.Context, vd client.Object) ([]virtv2.VirtualMachine, error) {
	var vms virtv2.VirtualMachineList
	err := h.client.List(ctx, &vms, &client.ListOptions{
		Namespace: vd.GetNamespace(),
	})
	if err != nil {
		return nil, fmt.Errorf("error getting virtual machines: %w", err)
	}

	var attachedVMs []virtv2.VirtualMachine

	for _, vm := range vms.Items {
		if h.isVDAttachedToVM(vd.GetName(), vm) {
			attachedVMs = append(attachedVMs, vm)
		}
	}

	return attachedVMs, nil
}

func (h AttacheeHandler) isVDAttachedToVM(vdName string, vm virtv2.VirtualMachine) bool {
	for _, bda := range vm.Status.BlockDeviceRefs {
		if bda.Kind == virtv2.DiskDevice && bda.Name == vdName {
			return true
		}
	}

	return false
}
