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
	"errors"
	"fmt"
	"log/slog"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmbdacondition"
)

type LifeCycleHandler struct {
	attacher *service.AttachmentService
	logger   *slog.Logger
}

func NewLifeCycleHandler(logger *slog.Logger, attacher *service.AttachmentService) *LifeCycleHandler {
	return &LifeCycleHandler{
		logger:   logger,
		attacher: attacher,
	}
}

func (h LifeCycleHandler) Handle(ctx context.Context, vmbda *virtv2.VirtualMachineBlockDeviceAttachment) (reconcile.Result, error) {
	logger := h.logger.With("name", vmbda.Name, "ns", vmbda.Namespace)
	// TODO protect vd.

	condition, ok := service.GetCondition(vmbdacondition.AttachedType, vmbda.Status.Conditions)
	if !ok {
		condition = metav1.Condition{
			Type:   vmbdacondition.AttachedType,
			Status: metav1.ConditionUnknown,
		}
	}

	defer func() { service.SetCondition(condition, &vmbda.Status.Conditions) }()

	vd, err := h.attacher.GetVirtualDisk(ctx, vmbda.Spec.BlockDeviceRef.Name, vmbda.Namespace)
	if err != nil {
		return reconcile.Result{}, err
	}

	vm, err := h.attacher.GetVirtualMachine(ctx, vmbda.Spec.VirtualMachineName, vmbda.Namespace)
	if err != nil {
		return reconcile.Result{}, err
	}

	kvvmi, err := h.attacher.GetVirtualMachineInstance(ctx, vmbda.Spec.VirtualMachineName, vmbda.Namespace)
	if err != nil {
		return reconcile.Result{}, err
	}

	if vmbda.DeletionTimestamp != nil {
		vmbda.Status.Phase = virtv2.BlockDeviceAttachmentPhaseTerminating

		err = h.attacher.UnplugDisk(ctx, vd, vm)
		if err != nil {
			return reconcile.Result{}, err
		}

		return reconcile.Result{}, nil
	}

	if vmbda.Status.Phase == "" {
		vmbda.Status.Phase = virtv2.BlockDeviceAttachmentPhasePending
	}

	blockDeviceReady, ok := service.GetCondition(vmbdacondition.BlockDeviceReadyType, vmbda.Status.Conditions)
	if !ok || blockDeviceReady.Status != metav1.ConditionTrue || vd == nil {
		condition.Status = metav1.ConditionFalse
		condition.Reason = vmbdacondition.NotAttached
		condition.Message = "Waiting for block device to be ready."
		return reconcile.Result{}, nil
	}

	virtualMachineReady, ok := service.GetCondition(vmbdacondition.VirtualMachineReadyType, vmbda.Status.Conditions)
	if !ok || virtualMachineReady.Status != metav1.ConditionTrue || vm == nil {
		condition.Status = metav1.ConditionFalse
		condition.Reason = vmbdacondition.NotAttached
		condition.Message = "Waiting for virtual machine to be ready."
		return reconcile.Result{}, nil
	}

	if kvvmi == nil {
		condition.Status = metav1.ConditionFalse
		condition.Reason = vmbdacondition.NotAttached
		condition.Message = "Waiting for virtual machine instance to be ready."
		return reconcile.Result{}, nil
	}

	logger = logger.With("vmName", vm.Name, "vdName", vd.Name)

	logger.Info("Check if hot plug is completed and disk is attached")

	isHotPlugged, err := h.attacher.IsHotPlugged(ctx, vd, vm)
	if err != nil {
		return reconcile.Result{}, err
	}

	if isHotPlugged {
		logger.Info("Hot plug is completed and disk is attached")

		vmbda.Status.Phase = virtv2.BlockDeviceAttachmentPhaseAttached
		condition.Status = metav1.ConditionTrue
		condition.Reason = vmbdacondition.Attached
		condition.Message = ""
		return reconcile.Result{}, nil
	}

	switch vmbda.Status.Phase {
	case virtv2.BlockDeviceAttachmentPhasePending:
		logger.Info("Send hot plug request")

		err = h.attacher.HotPlugDisk(ctx, vd, vm)
		switch {
		case err == nil:
			vmbda.Status.Phase = virtv2.BlockDeviceAttachmentPhaseInProgress
			condition.Status = metav1.ConditionFalse
			condition.Reason = vmbdacondition.AttachmentRequestSent
			condition.Message = "Attachment is in progress."
		case errors.Is(err, service.ErrVirtualDiskIsAlreadyAttached),
			errors.Is(err, service.ErrVirtualMachineWaitsForRestartApproval):
			vmbda.Status.Phase = virtv2.BlockDeviceAttachmentPhasePending
			condition.Status = metav1.ConditionFalse
			condition.Reason = vmbdacondition.NotAttached
			condition.Message = service.CapitalizeFirstLetter(err.Error())
		default:
			return reconcile.Result{}, err
		}
	case virtv2.BlockDeviceAttachmentPhaseAttached:
		logger.Info("Disk is attached")

		vmbda.Status.Phase = virtv2.BlockDeviceAttachmentPhaseAttached
		condition.Status = metav1.ConditionTrue
		condition.Reason = vmbdacondition.Attached
		condition.Message = ""
	default:
		return reconcile.Result{}, fmt.Errorf("unexpected phase to set vmbda status: %s", vmbda.Status.Phase)
	}

	return reconcile.Result{}, nil
}
