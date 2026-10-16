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
	"slices"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/indexer"
	"github.com/deckhouse/virtualization-controller/pkg/controller/kvbuilder"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmbdacondition"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
)

type LifeCycleHandler struct {
	attacher AttachmentService
	client   client.Client
}

func NewLifeCycleHandler(attacher AttachmentService, client client.Client) *LifeCycleHandler {
	return &LifeCycleHandler{
		attacher: attacher,
		client:   client,
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

	var ad *service.AttachmentDisk
	switch vmbda.Spec.BlockDeviceRef.Kind {
	case v1alpha2.VMBDAObjectRefKindVirtualDisk:
		vd, err := h.attacher.GetVirtualDisk(ctx, vmbda.Spec.BlockDeviceRef.Name, vmbda.Namespace)
		if err != nil {
			return reconcile.Result{}, err
		}
		if vd != nil {
			ad = service.NewAttachmentDiskFromVirtualDisk(vd)
		}
	case v1alpha2.VMBDAObjectRefKindVirtualImage:
		vi, err := h.attacher.GetVirtualImage(ctx, vmbda.Spec.BlockDeviceRef.Name, vmbda.Namespace)
		if err != nil {
			return reconcile.Result{}, err
		}
		if vi != nil {
			ad = service.NewAttachmentDiskFromVirtualImage(vi)
		}
	case v1alpha2.VMBDAObjectRefKindClusterVirtualImage:
		cvi, err := h.attacher.GetClusterVirtualImage(ctx, vmbda.Spec.BlockDeviceRef.Name)
		if err != nil {
			return reconcile.Result{}, err
		}
		if cvi != nil {
			ad = service.NewAttachmentDiskFromClusterVirtualImage(cvi)
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
			Message(fmt.Sprintf("Block device %q not found.", vmbda.Spec.BlockDeviceRef.Name))
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
			Message(fmt.Sprintf("VirtualMachine %q is Running, but its underlying virtualization instance is not ready yet.", vm.Name))
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
			Message(fmt.Sprintf("VirtualMachine %q is Running, but its underlying virtualization instance is not ready yet.", vm.Name))
		return reconcile.Result{}, nil
	}

	// An attached attachment has already waited, and no other one of the disk can be created while it exists.
	var unplugMessage string
	if vmbda.Status.Phase != v1alpha2.BlockDeviceAttachmentPhaseAttached {
		unplugMessage, err = h.previousUnplugMessage(ctx, vmbda, kvvm, kvvmi)
		if err != nil {
			return reconcile.Result{}, err
		}
	}
	if unplugMessage != "" {
		vmbda.Status.Phase = v1alpha2.BlockDeviceAttachmentPhasePending
		cb.
			Status(metav1.ConditionFalse).
			Reason(vmbdacondition.NotAttached).
			Message(unplugMessage)
		return reconcile.Result{}, nil
	}

	log = log.With("vmName", vm.Name, "attachmentDiskName", ad.Name)

	_, canErr := h.attacher.CanHotPlug(ad, vm, kvvm)
	if errors.Is(canErr, service.ErrBlockDeviceIsSpecAttached) {
		log.Info("VirtualDisk is already attached to the virtual machine spec")

		vmbda.Status.Phase = v1alpha2.BlockDeviceAttachmentPhaseFailed
		cb.
			Status(metav1.ConditionFalse).
			Reason(vmbdacondition.Conflict).
			Message(service.CapitalizeFirstLetter(canErr.Error()))
		return reconcile.Result{}, nil
	}

	log.Info("Check if hot plug is completed and disk is attached")

	isHotPlugged, err := h.attacher.IsHotPlugged(ad, vm, kvvmi)
	if err != nil {
		if errors.Is(err, service.ErrVolumeStatusNotReady) {
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

	switch {
	case canErr == nil:
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

		if conditions.HasCondition(vmcondition.TypeMigrating, vm.Status.Conditions) {
			queued, err := migrationIsQueued(ctx, h.client, vmbda.GetNamespace(), vmbda.Spec.VirtualMachineName)
			if err != nil {
				return reconcile.Result{}, fmt.Errorf("check migration state: %w", err)
			}

			if !queued {
				log.Info("Cannot hot-plug the block device while the virtual machine is migrating")

				vmbda.Status.Phase = v1alpha2.BlockDeviceAttachmentPhasePending
				cb.
					Status(metav1.ConditionFalse).
					Reason(vmbdacondition.BlockedByMigration).
					Message(migrationBlockedHotPlugMessage(vmbda))
				return reconcile.Result{}, nil
			}
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
						Message(fmt.Sprintf("The disk storage is not available on node %q where the VirtualMachine is running.", kvvmi.Status.NodeName))
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

			if errors.Is(err, service.ErrDiskNotGivenToVM) {
				log.Info("The disk has been given to another virtual machine since it was checked, wait")

				vmbda.Status.Phase = v1alpha2.BlockDeviceAttachmentPhasePending
				cb.
					Status(metav1.ConditionFalse).
					Reason(vmbdacondition.NotAttached).
					Message("Waiting for block device to be ready.")
				return reconcile.Result{}, nil
			}

			return reconcile.Result{}, err
		}

		vmbda.Status.Phase = v1alpha2.BlockDeviceAttachmentPhaseInProgress
		cb.
			Status(metav1.ConditionFalse).
			Reason(vmbdacondition.AttachmentRequestSent).
			Message("Attachment request has been sent: attachment is in progress.")
		return reconcile.Result{}, nil
	case errors.Is(canErr, service.ErrHotPlugRequestAlreadySent):
		log.Info("Attachment request sent: attachment is in progress.")

		vmbda.Status.Phase = v1alpha2.BlockDeviceAttachmentPhaseInProgress
		cb.
			Status(metav1.ConditionFalse).
			Reason(vmbdacondition.AttachmentRequestSent).
			Message("Attachment request sent: attachment is in progress.")
		return reconcile.Result{}, nil
	default:
		return reconcile.Result{}, canErr
	}
}

// previousUnplugMessage tells why the attachment waits for an earlier unplug of the same block device
// from the same VM, or returns an empty string. The unplug removes the volume by the block device name,
// so a hot-plug sent before it is over is undone, and a volume status left from it reads as attached.
func (h LifeCycleHandler) previousUnplugMessage(ctx context.Context, vmbda *v1alpha2.VirtualMachineBlockDeviceAttachment, kvvm *virtv1.VirtualMachine, kvvmi *virtv1.VirtualMachineInstance) (string, error) {
	var vmbdas v1alpha2.VirtualMachineBlockDeviceAttachmentList
	// Read only: the attachments are not copied out of the cache.
	err := h.client.List(ctx, &vmbdas, client.InNamespace(vmbda.Namespace),
		client.MatchingFields{indexer.IndexFieldVMBDAByVM: vmbda.Spec.VirtualMachineName}, client.UnsafeDisableDeepCopy)
	if err != nil {
		return "", fmt.Errorf("list virtual machine block device attachments: %w", err)
	}

	for _, other := range vmbdas.Items {
		if other.Name == vmbda.Name || other.DeletionTimestamp == nil ||
			other.Spec.BlockDeviceRef.Kind != vmbda.Spec.BlockDeviceRef.Kind ||
			other.Spec.BlockDeviceRef.Name != vmbda.Spec.BlockDeviceRef.Name {
			continue
		}

		return fmt.Sprintf("Waiting for the VirtualMachineBlockDeviceAttachment %q to detach the %s %q from the VirtualMachine %q.",
			other.Name, vmbda.Spec.BlockDeviceRef.Kind, vmbda.Spec.BlockDeviceRef.Name, vmbda.Spec.VirtualMachineName), nil
	}

	name := kvbuilder.GenerateVMBDADiskName(vmbda.Spec.BlockDeviceRef)
	if volumeRequested(kvvm, name) || slices.ContainsFunc(kvvmi.Spec.Volumes, func(v virtv1.Volume) bool { return v.Name == name }) {
		return "", nil
	}

	for _, vs := range kvvmi.Status.VolumeStatus {
		// An unmounted volume is already reported as not attached.
		if vs.Name == name && vs.HotplugVolume != nil && vs.Phase != virtv1.HotplugVolumeUnMounted {
			return fmt.Sprintf("Waiting for the %s %q to finish detaching from the VirtualMachine %q.",
				vmbda.Spec.BlockDeviceRef.Kind, vmbda.Spec.BlockDeviceRef.Name, vmbda.Spec.VirtualMachineName), nil
		}
	}

	return "", nil
}

// volumeRequested reports whether the internal VM has the volume or an attach request for it.
func volumeRequested(kvvm *virtv1.VirtualMachine, name string) bool {
	if kvvm.Spec.Template != nil && slices.ContainsFunc(kvvm.Spec.Template.Spec.Volumes, func(v virtv1.Volume) bool { return v.Name == name }) {
		return true
	}

	return slices.ContainsFunc(kvvm.Status.VolumeRequests, func(vr virtv1.VirtualMachineVolumeRequest) bool {
		return vr.AddVolumeOptions != nil && vr.AddVolumeOptions.Name == name
	})
}
