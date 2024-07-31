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
	virtv1 "kubevirt.io/api/core/v1"
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
	logger.Info("Sync")
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

	var kvvm *virtv1.VirtualMachine
	if vm != nil {
		kvvm, err = h.attacher.GetKVVM(ctx, vm)
		if err != nil {
			return reconcile.Result{}, err
		}
	}

	if vmbda.DeletionTimestamp != nil {
		switch vmbda.Status.Phase {
		case virtv2.BlockDeviceAttachmentPhasePending,
			virtv2.BlockDeviceAttachmentPhaseInProgress,
			virtv2.BlockDeviceAttachmentPhaseAttached:
			err = h.attacher.UnplugDisk(ctx, vd, kvvm)
			if err != nil {
				return reconcile.Result{}, err
			}
		}

		vmbda.Status.Phase = virtv2.BlockDeviceAttachmentPhaseTerminating
		condition.Status = metav1.ConditionUnknown
		condition.Reason = ""
		condition.Message = ""

		return reconcile.Result{}, nil
	}

	// Final phase: avoid processing VMDBA that had a conflict with another VMDBA previously.
	if vmbda.Status.Phase == virtv2.BlockDeviceAttachmentPhaseFailed {
		return reconcile.Result{}, nil
	}

	isConflicted, conflictWithName, err := h.attacher.IsConflictedAttachment(ctx, vmbda)
	if err != nil {
		return reconcile.Result{}, err
	}

	if isConflicted {
		if vmbda.Status.Phase != "" {
			logger.Error("Hot plug has been started for Conflicted VMBDA, please report a bug")
		}

		vmbda.Status.Phase = virtv2.BlockDeviceAttachmentPhaseFailed
		condition.Status = metav1.ConditionFalse
		condition.Reason = vmbdacondition.Conflict
		condition.Message = fmt.Sprintf(
			"Another VirtualMachineBlockDeviceAttachment %s/%s already exists "+
				"with the same virtual machine %s and block device %s for hot-plugging.",
			vmbda.Namespace, conflictWithName, vmbda.Spec.VirtualMachineName, vmbda.Spec.BlockDeviceRef.Name,
		)

		return reconcile.Result{}, nil
	}

	if vmbda.Status.Phase == "" {
		vmbda.Status.Phase = virtv2.BlockDeviceAttachmentPhasePending
	}

	blockDeviceReady, ok := service.GetCondition(vmbdacondition.BlockDeviceReadyType, vmbda.Status.Conditions)
	if !ok || blockDeviceReady.Status != metav1.ConditionTrue {
		vmbda.Status.Phase = virtv2.BlockDeviceAttachmentPhasePending
		condition.Status = metav1.ConditionFalse
		condition.Reason = vmbdacondition.NotAttached
		condition.Message = "Waiting for block device to be ready."
		return reconcile.Result{}, nil
	}

	virtualMachineReady, ok := service.GetCondition(vmbdacondition.VirtualMachineReadyType, vmbda.Status.Conditions)
	if !ok || virtualMachineReady.Status != metav1.ConditionTrue {
		vmbda.Status.Phase = virtv2.BlockDeviceAttachmentPhasePending
		condition.Status = metav1.ConditionFalse
		condition.Reason = vmbdacondition.NotAttached
		condition.Message = "Waiting for virtual machine to be ready."
		return reconcile.Result{}, nil
	}

	if vd == nil {
		vmbda.Status.Phase = virtv2.BlockDeviceAttachmentPhasePending
		condition.Status = metav1.ConditionFalse
		condition.Reason = vmbdacondition.NotAttached
		condition.Message = fmt.Sprintf("VirtualDisk %s not found.", vmbda.Spec.BlockDeviceRef.Name)
		return reconcile.Result{}, nil
	}

	if vm == nil {
		vmbda.Status.Phase = virtv2.BlockDeviceAttachmentPhasePending
		condition.Status = metav1.ConditionFalse
		condition.Reason = vmbdacondition.NotAttached
		condition.Message = fmt.Sprintf("VirtualMachine %s not found.", vmbda.Spec.VirtualMachineName)
		return reconcile.Result{}, nil
	}

	if kvvm == nil {
		vmbda.Status.Phase = virtv2.BlockDeviceAttachmentPhasePending
		condition.Status = metav1.ConditionFalse
		condition.Reason = vmbdacondition.NotAttached
		condition.Message = fmt.Sprintf("InternalVirtualizationVirtualMachine %s not found.", vm.Name)
		return reconcile.Result{}, nil
	}

	kvvmi, err := h.attacher.GetKVVMI(ctx, vm)
	if err != nil {
		return reconcile.Result{}, err
	}

	if kvvmi == nil {
		vmbda.Status.Phase = virtv2.BlockDeviceAttachmentPhasePending
		condition.Status = metav1.ConditionFalse
		condition.Reason = vmbdacondition.NotAttached
		condition.Message = fmt.Sprintf("InternalVirtualizationVirtualMachineInstance %s not found.", vm.Name)
		return reconcile.Result{}, nil
	}

	logger = logger.With("vmName", vm.Name, "vdName", vd.Name)
	logger.Info("Check if hot plug is completed and disk is attached")

	isHotPlugged, err := h.attacher.IsHotPlugged(vd, vm, kvvmi)
	if err != nil {
		if errors.Is(err, service.ErrVolumeStatusNotReady) {
			vmbda.Status.Phase = virtv2.BlockDeviceAttachmentPhaseInProgress
			condition.Status = metav1.ConditionFalse
			condition.Reason = vmbdacondition.AttachmentRequestSent
			condition.Message = service.CapitalizeFirstLetter(err.Error() + ".")
			return reconcile.Result{}, nil
		}

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

	isHotPlugRequestSent, err := h.attacher.IsHotPlugRequestSent(vd, kvvm)
	if err != nil {
		return reconcile.Result{}, err
	}

	if isHotPlugRequestSent {
		logger.Info("Attachment request sent: attachment is in progress.")

		vmbda.Status.Phase = virtv2.BlockDeviceAttachmentPhaseInProgress
		condition.Status = metav1.ConditionFalse
		condition.Reason = vmbdacondition.AttachmentRequestSent
		condition.Message = "Attachment request sent: attachment is in progress."
		return reconcile.Result{}, nil
	}

	logger.Info("Send attachment request")

	err = h.attacher.HotPlugDisk(ctx, vd, vm, kvvm)
	switch {
	case err == nil:
		vmbda.Status.Phase = virtv2.BlockDeviceAttachmentPhaseInProgress
		condition.Status = metav1.ConditionFalse
		condition.Reason = vmbdacondition.AttachmentRequestSent
		condition.Message = "Attachment request has sent: attachment is in progress."
		return reconcile.Result{}, nil
	case errors.Is(err, service.ErrVirtualDiskIsAlreadyAttached),
		errors.Is(err, service.ErrVirtualMachineWaitsForRestartApproval):
		vmbda.Status.Phase = virtv2.BlockDeviceAttachmentPhasePending
		condition.Status = metav1.ConditionFalse
		condition.Reason = vmbdacondition.NotAttached
		condition.Message = service.CapitalizeFirstLetter(err.Error())
		return reconcile.Result{}, nil
	default:
		return reconcile.Result{}, err
	}
}
