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
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	intsvc "github.com/deckhouse/virtualization-controller/pkg/controller/vmbda/internal/service"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmbdacondition"
)

type LifeCycleHandler struct {
	attacher *intsvc.AttachmentService
}

func NewLifeCycleHandler(attacher *intsvc.AttachmentService) *LifeCycleHandler {
	return &LifeCycleHandler{
		attacher: attacher,
	}
}

func (h LifeCycleHandler) Handle(ctx context.Context, vmbda *v1alpha2.VirtualMachineBlockDeviceAttachment) (reconcile.Result, error) {
	log := logger.FromContext(ctx).With(logger.SlogHandler("lifecycle"))

	// TODO protect vd.

	cb := conditions.NewConditionBuilder(vmbdacondition.AttachedType)
	defer func() { conditions.SetCondition(cb.Generation(vmbda.Generation), &vmbda.Status.Conditions) }()

	if !conditions.HasCondition(cb.GetType(), vmbda.Status.Conditions) {
		cb.Status(metav1.ConditionUnknown).Reason(conditions.ReasonUnknown)
	}

	var ad *intsvc.AttachmentDisk
	switch vmbda.Spec.BlockDeviceRef.Kind {
	case v1alpha2.VMBDAObjectRefKindVirtualDisk:
		vd, err := h.attacher.GetVirtualDisk(ctx, vmbda.Spec.BlockDeviceRef.Name, vmbda.Namespace)
		if err != nil {
			return reconcile.Result{}, err
		}
		if vd != nil {
			ad = intsvc.NewAttachmentDiskFromVirtualDisk(vd)
		}
	case v1alpha2.VMBDAObjectRefKindVirtualImage:
		vi, err := h.attacher.GetVirtualImage(ctx, vmbda.Spec.BlockDeviceRef.Name, vmbda.Namespace)
		if err != nil {
			return reconcile.Result{}, err
		}
		if vi != nil {
			ad = intsvc.NewAttachmentDiskFromVirtualImage(vi)
		}
	case v1alpha2.VMBDAObjectRefKindClusterVirtualImage:
		cvi, err := h.attacher.GetClusterVirtualImage(ctx, vmbda.Spec.BlockDeviceRef.Name)
		if err != nil {
			return reconcile.Result{}, err
		}
		if cvi != nil {
			ad = intsvc.NewAttachmentDiskFromClusterVirtualImage(cvi)
		}
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
		vmbda.Status.Phase = v1alpha2.BlockDeviceAttachmentPhaseTerminating
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

		vmbda.Status.Phase = v1alpha2.BlockDeviceAttachmentPhaseFailed
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
		vmbda.Status.Phase = v1alpha2.BlockDeviceAttachmentPhasePending
	}

	blockDeviceReady, _ := conditions.GetCondition(vmbdacondition.BlockDeviceReadyType, vmbda.Status.Conditions)
	if blockDeviceReady.Status != metav1.ConditionTrue {
		vmbda.Status.Phase = v1alpha2.BlockDeviceAttachmentPhasePending
		cb.
			Status(metav1.ConditionFalse).
			Reason(vmbdacondition.NotAttached).
			Message("Waiting for block device to be ready.")
		return reconcile.Result{}, nil
	}

	virtualMachineReady, _ := conditions.GetCondition(vmbdacondition.VirtualMachineReadyType, vmbda.Status.Conditions)
	if virtualMachineReady.Status != metav1.ConditionTrue {
		vmbda.Status.Phase = v1alpha2.BlockDeviceAttachmentPhasePending
		cb.
			Status(metav1.ConditionFalse).
			Reason(vmbdacondition.NotAttached).
			Message("Waiting for virtual machine to be ready.")
		return reconcile.Result{}, nil
	}

	if ad == nil {
		vmbda.Status.Phase = v1alpha2.BlockDeviceAttachmentPhasePending
		cb.
			Status(metav1.ConditionFalse).
			Reason(vmbdacondition.NotAttached).
			Message(fmt.Sprintf("AttachmentDisk %q not found.", vmbda.Spec.BlockDeviceRef.Name))
		return reconcile.Result{}, nil
	}

	if vm == nil {
		vmbda.Status.Phase = v1alpha2.BlockDeviceAttachmentPhasePending
		cb.
			Status(metav1.ConditionFalse).
			Reason(vmbdacondition.NotAttached).
			Message(fmt.Sprintf("VirtualMachine %q not found.", vmbda.Spec.VirtualMachineName))
		return reconcile.Result{}, nil
	}

	if kvvm == nil {
		vmbda.Status.Phase = v1alpha2.BlockDeviceAttachmentPhasePending
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
		vmbda.Status.Phase = v1alpha2.BlockDeviceAttachmentPhasePending
		cb.
			Status(metav1.ConditionFalse).
			Reason(vmbdacondition.NotAttached).
			Message(fmt.Sprintf("InternalVirtualizationVirtualMachineInstance %q not found.", vm.Name))
		return reconcile.Result{}, nil
	}

	log = log.With("vmName", vm.Name, "attachmentDiskName", ad.Name)
	log.Info("Check if hot plug is completed and disk is attached")

	isHotPlugged, err := h.attacher.IsHotPlugged(ad, vm, kvvmi)
	if err != nil {
		if errors.Is(err, intsvc.ErrVolumeStatusNotReady) {
			vmbda.Status.Phase = v1alpha2.BlockDeviceAttachmentPhaseInProgress
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

		vmbda.Status.Phase = v1alpha2.BlockDeviceAttachmentPhaseAttached
		cb.Status(metav1.ConditionTrue).Reason(vmbdacondition.Attached)

		vmbda.Status.VirtualMachineName = vm.Name

		return reconcile.Result{}, nil
	}

	_, err = h.attacher.CanHotPlug(ad, vm, kvvm)

	switch {
	case err == nil:
		blockDeviceLimitCondition, _ := conditions.GetCondition(vmbdacondition.DiskAttachmentCapacityAvailableType, vmbda.Status.Conditions)
		if blockDeviceLimitCondition.Status != metav1.ConditionTrue {
			log.Info("Virtual machine block device capacity reached")

			vmbda.Status.Phase = v1alpha2.BlockDeviceAttachmentPhasePending
			cb.
				Status(metav1.ConditionFalse).
				Reason(vmbdacondition.NotAttached).
				Message("Virtual machine block device capacity reached.")
			return reconcile.Result{}, nil
		}

		if ad.PVCName != "" {
			pvc, err := h.attacher.GetPersistentVolumeClaim(ctx, ad)
			if err != nil {
				return reconcile.Result{}, err
			}

			if pvc != nil {
				available, err := h.attacher.IsPVAvailableOnVMNode(ctx, pvc, kvvmi)
				if err != nil {
					return reconcile.Result{}, err
				}

				if !available {
					vmbda.Status.Phase = v1alpha2.BlockDeviceAttachmentPhaseFailed
					cb.
						Status(metav1.ConditionFalse).
						Reason(vmbdacondition.DeviceNotAvailableOnNode).
						Message(fmt.Sprintf("PersistentVolume %q is not available on node %q where the virtual machine is running", pvc.Spec.VolumeName, kvvmi.Status.NodeName))
					return reconcile.Result{}, nil
				}
			}
		}

		log.Info("Send attachment request")

		err = h.attacher.HotPlugDisk(ctx, ad, vm, kvvm)
		if err != nil {
			if IsOutdatedRequestError(err) {
				log.Debug("The server rejected our request, retry")

				return reconcile.Result{RequeueAfter: time.Second}, nil
			}

			return reconcile.Result{}, err
		}

		vmbda.Status.Phase = v1alpha2.BlockDeviceAttachmentPhaseInProgress
		cb.
			Status(metav1.ConditionFalse).
			Reason(vmbdacondition.AttachmentRequestSent).
			Message("Attachment request has sent: attachment is in progress.")
		return reconcile.Result{}, nil
	case errors.Is(err, intsvc.ErrBlockDeviceIsSpecAttached):
		log.Info("VirtualDisk is already attached to the virtual machine spec")

		vmbda.Status.Phase = v1alpha2.BlockDeviceAttachmentPhaseFailed
		cb.
			Status(metav1.ConditionFalse).
			Reason(vmbdacondition.Conflict).
			Message(service.CapitalizeFirstLetter(err.Error()))
		return reconcile.Result{}, nil
	case errors.Is(err, intsvc.ErrHotPlugRequestAlreadySent):
		log.Info("Attachment request sent: attachment is in progress.")

		vmbda.Status.Phase = v1alpha2.BlockDeviceAttachmentPhaseInProgress
		cb.
			Status(metav1.ConditionFalse).
			Reason(vmbdacondition.AttachmentRequestSent).
			Message("Attachment request sent: attachment is in progress.")
		return reconcile.Result{}, nil
	default:
		return reconcile.Result{}, err
	}
}
