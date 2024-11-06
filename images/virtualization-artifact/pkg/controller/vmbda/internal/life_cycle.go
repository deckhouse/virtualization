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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmbdacondition"
)

type LifeCycleHandler struct {
	attacher *service.AttachmentService
}

func NewLifeCycleHandler(attacher *service.AttachmentService) *LifeCycleHandler {
	return &LifeCycleHandler{
		attacher: attacher,
	}
}

func (h LifeCycleHandler) Handle(ctx context.Context, vmbda *virtv2.VirtualMachineBlockDeviceAttachment) (reconcile.Result, error) {
	log := logger.FromContext(ctx).With(logger.SlogHandler("lifecycle"))

	// TODO protect vd.

	cb := conditions.NewConditionBuilder(vmbdacondition.AttachedType)
	defer func() { conditions.SetCondition(cb.Generation(vmbda.Generation), &vmbda.Status.Conditions) }()

	if !conditions.HasCondition(cb.GetType(), vmbda.Status.Conditions) {
		cb.Status(metav1.ConditionUnknown).Reason(conditions.ReasonUnknown)
	}

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
			if h.attacher.CanUnplug(vd, kvvm) {
				err = h.attacher.UnplugDisk(ctx, vd, kvvm)
				if err != nil {
					return reconcile.Result{}, err
				}
			}
		}

		vmbda.Status.Phase = virtv2.BlockDeviceAttachmentPhaseTerminating
		cb.Status(metav1.ConditionUnknown).Reason(conditions.ReasonUnknown)

		return reconcile.Result{}, nil
	}

	isConflicted, conflictWithName, err := h.attacher.IsConflictedAttachment(ctx, vmbda)
	if err != nil {
		return reconcile.Result{}, err
	}

	if isConflicted {
		if vmbda.Status.Phase != "" {
			log.Error("Hot plug has been started for Conflicted VMBDA, please report a bug")
		}

		vmbda.Status.Phase = virtv2.BlockDeviceAttachmentPhaseFailed
		cb.
			Status(metav1.ConditionFalse).
			Reason(vmbdacondition.Conflict).
			Message(fmt.Sprintf(
				"Another VirtualMachineBlockDeviceAttachment %s/%s already exists "+
					"with the same block device %q for hot-plugging.",
				vmbda.Namespace, conflictWithName, vmbda.Spec.BlockDeviceRef.Name,
			))

		return reconcile.Result{}, nil
	}

	if vmbda.Status.Phase == "" {
		vmbda.Status.Phase = virtv2.BlockDeviceAttachmentPhasePending
	}

	blockDeviceReady, _ := conditions.GetCondition(vmbdacondition.BlockDeviceReadyType, vmbda.Status.Conditions)
	if blockDeviceReady.Status != metav1.ConditionTrue {
		vmbda.Status.Phase = virtv2.BlockDeviceAttachmentPhasePending
		cb.
			Status(metav1.ConditionFalse).
			Reason(vmbdacondition.NotAttached).
			Message("Waiting for block device to be ready.")
		return reconcile.Result{}, nil
	}

	virtualMachineReady, _ := conditions.GetCondition(vmbdacondition.VirtualMachineReadyType, vmbda.Status.Conditions)
	if virtualMachineReady.Status != metav1.ConditionTrue {
		vmbda.Status.Phase = virtv2.BlockDeviceAttachmentPhasePending
		cb.
			Status(metav1.ConditionFalse).
			Reason(vmbdacondition.NotAttached).
			Message("Waiting for virtual machine to be ready.")
		return reconcile.Result{}, nil
	}

	if vd == nil {
		vmbda.Status.Phase = virtv2.BlockDeviceAttachmentPhasePending
		cb.
			Status(metav1.ConditionFalse).
			Reason(vmbdacondition.NotAttached).
			Message(fmt.Sprintf("VirtualDisk %q not found.", vmbda.Spec.BlockDeviceRef.Name))
		return reconcile.Result{}, nil
	}

	if vm == nil {
		vmbda.Status.Phase = virtv2.BlockDeviceAttachmentPhasePending
		cb.
			Status(metav1.ConditionFalse).
			Reason(vmbdacondition.NotAttached).
			Message(fmt.Sprintf("VirtualMachine %q not found.", vmbda.Spec.VirtualMachineName))
		return reconcile.Result{}, nil
	}

	if kvvm == nil {
		vmbda.Status.Phase = virtv2.BlockDeviceAttachmentPhasePending
		cb.
			Status(metav1.ConditionFalse).
			Reason(vmbdacondition.NotAttached).
			Message(fmt.Sprintf("InternalVirtualizationVirtualMachine %q not found.", vm.Name))
		return reconcile.Result{}, nil
	}

	kvvmi, err := h.attacher.GetKVVMI(ctx, vm)
	if err != nil {
		return reconcile.Result{}, err
	}

	if kvvmi == nil {
		vmbda.Status.Phase = virtv2.BlockDeviceAttachmentPhasePending
		cb.
			Status(metav1.ConditionFalse).
			Reason(vmbdacondition.NotAttached).
			Message(fmt.Sprintf("InternalVirtualizationVirtualMachineInstance %q not found.", vm.Name))
		return reconcile.Result{}, nil
	}

	log = log.With("vmName", vm.Name, "vdName", vd.Name)
	log.Info("Check if hot plug is completed and disk is attached")

	isHotPlugged, err := h.attacher.IsHotPlugged(vd, vm, kvvmi)
	if err != nil {
		if errors.Is(err, service.ErrVolumeStatusNotReady) {
			vmbda.Status.Phase = virtv2.BlockDeviceAttachmentPhaseInProgress
			cb.
				Status(metav1.ConditionFalse).
				Reason(vmbdacondition.AttachmentRequestSent).
				Message(service.CapitalizeFirstLetter(err.Error() + "."))
			return reconcile.Result{}, nil
		}

		return reconcile.Result{}, err
	}

	if isHotPlugged {
		log.Info("Hot plug is completed and disk is attached")

		vmbda.Status.Phase = virtv2.BlockDeviceAttachmentPhaseAttached
		cb.Status(metav1.ConditionTrue).Reason(vmbdacondition.Attached)

		vmbda.Status.VirtualMachineName = vm.Name

		return reconcile.Result{}, nil
	}

	_, err = h.attacher.CanHotPlug(vd, vm, kvvm)
	switch {
	case err == nil:
		log.Info("Send attachment request")

		err = h.attacher.HotPlugDisk(ctx, vd, vm, kvvm)
		if err != nil {
			return reconcile.Result{}, err
		}

		vmbda.Status.Phase = virtv2.BlockDeviceAttachmentPhaseInProgress
		cb.
			Status(metav1.ConditionFalse).
			Reason(vmbdacondition.AttachmentRequestSent).
			Message("Attachment request has sent: attachment is in progress.")
		return reconcile.Result{}, nil
	case errors.Is(err, service.ErrDiskIsSpecAttached):
		log.Info("VirtualDisk is already attached to the virtual machine spec")

		vmbda.Status.Phase = virtv2.BlockDeviceAttachmentPhaseFailed
		cb.
			Status(metav1.ConditionFalse).
			Reason(vmbdacondition.Conflict).
			Message(service.CapitalizeFirstLetter(err.Error()))
		return reconcile.Result{}, nil
	case errors.Is(err, service.ErrHotPlugRequestAlreadySent):
		log.Info("Attachment request sent: attachment is in progress.")

		vmbda.Status.Phase = virtv2.BlockDeviceAttachmentPhaseInProgress
		cb.
			Status(metav1.ConditionFalse).
			Reason(vmbdacondition.AttachmentRequestSent).
			Message("Attachment request sent: attachment is in progress.")
		return reconcile.Result{}, nil
	case errors.Is(err, service.ErrVirtualMachineWaitsForRestartApproval):
		log.Info("Virtual machine waits for restart approval")

		vmbda.Status.Phase = virtv2.BlockDeviceAttachmentPhasePending
		cb.
			Status(metav1.ConditionFalse).
			Reason(vmbdacondition.NotAttached).
			Message(service.CapitalizeFirstLetter(err.Error()))
		return reconcile.Result{}, nil
	default:
		return reconcile.Result{}, err
	}
}
