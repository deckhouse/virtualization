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

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdcondition"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmbdacondition"
)

type BlockDeviceReadyHandler struct {
	attachment AttachmentService
}

func NewBlockDeviceReadyHandler(attachment AttachmentService) *BlockDeviceReadyHandler {
	return &BlockDeviceReadyHandler{
		attachment: attachment,
	}
}

func (h BlockDeviceReadyHandler) Handle(ctx context.Context, vmbda *v1alpha2.VirtualMachineBlockDeviceAttachment) (reconcile.Result, error) {
	cb := conditions.NewConditionBuilder(vmbdacondition.BlockDeviceReadyType)
	defer func() { conditions.SetCondition(cb.Generation(vmbda.Generation), &vmbda.Status.Conditions) }()

	if !conditions.HasCondition(cb.GetType(), vmbda.Status.Conditions) {
		cb.Status(metav1.ConditionUnknown).Reason(conditions.ReasonUnknown)
	}

	if vmbda.DeletionTimestamp != nil {
		cb.Status(metav1.ConditionUnknown).Reason(conditions.ReasonUnknown)
		return reconcile.Result{}, nil
	}

	switch vmbda.Spec.BlockDeviceRef.Kind {
	case v1alpha2.VMBDAObjectRefKindVirtualDisk:
		return reconcile.Result{}, h.ValidateVirtualDiskReady(ctx, vmbda, cb)
	case v1alpha2.VMBDAObjectRefKindVirtualImage:
		viKey := types.NamespacedName{
			Name:      vmbda.Spec.BlockDeviceRef.Name,
			Namespace: vmbda.Namespace,
		}

		vi, err := h.attachment.GetVirtualImage(ctx, viKey.Name, viKey.Namespace)
		if err != nil {
			return reconcile.Result{}, err
		}

		if vi == nil {
			cb.
				Status(metav1.ConditionFalse).
				Reason(vmbdacondition.BlockDeviceNotReady).
				Message(fmt.Sprintf("VirtualImage %q not found.", viKey.String()))
			return reconcile.Result{}, nil
		}

		if vi.GetDeletionTimestamp() != nil {
			cb.
				Status(metav1.ConditionFalse).
				Reason(vmbdacondition.BlockDeviceNotReady).
				Message(fmt.Sprintf("VirtualImage %q is being deleted. Delete VirtualMachineBlockDeviceAttachment to detach image from the VirtualMachine.", viKey.String()))
			return reconcile.Result{}, nil
		}

		if vi.Generation != vi.Status.ObservedGeneration {
			cb.
				Status(metav1.ConditionFalse).
				Reason(vmbdacondition.BlockDeviceNotReady).
				Message(fmt.Sprintf("Waiting for the VirtualImage %q to be observed in its latest state generation.", viKey.String()))
			return reconcile.Result{}, nil
		}

		if vi.Status.Phase != v1alpha2.ImageReady {
			cb.
				Status(metav1.ConditionFalse).
				Reason(vmbdacondition.BlockDeviceNotReady).
				Message(fmt.Sprintf("VirtualImage %q is not ready to be attached to the virtual machine: waiting for the VirtualImage to be ready for attachment.", viKey.String()))
			return reconcile.Result{}, nil
		}
		switch vi.Spec.Storage {
		case v1alpha2.StorageKubernetes, v1alpha2.StoragePersistentVolumeClaim:
			if vi.Status.Target.PersistentVolumeClaim == "" {
				cb.
					Status(metav1.ConditionFalse).
					Reason(vmbdacondition.BlockDeviceNotReady).
					Message("Waiting until VirtualImage has associated PersistentVolumeClaim name.")
				return reconcile.Result{}, nil
			}
			ad := service.NewAttachmentDiskFromVirtualImage(vi)
			pvc, err := h.attachment.GetPersistentVolumeClaim(ctx, ad)
			if err != nil {
				return reconcile.Result{}, err
			}

			if pvc == nil {
				cb.
					Status(metav1.ConditionFalse).
					Reason(vmbdacondition.BlockDeviceNotReady).
					Message(fmt.Sprintf("Underlying PersistentVolumeClaim %q not found.", vi.Status.Target.PersistentVolumeClaim))
				return reconcile.Result{}, nil
			}

			if vi.Status.Phase == v1alpha2.ImageReady && pvc.Status.Phase != corev1.ClaimBound {
				cb.
					Status(metav1.ConditionFalse).
					Reason(vmbdacondition.BlockDeviceNotReady).
					Message(fmt.Sprintf("Underlying PersistentVolumeClaim %q not bound.", vi.Status.Target.PersistentVolumeClaim))
				return reconcile.Result{}, nil
			}

			cb.Status(metav1.ConditionTrue).Reason(vmbdacondition.BlockDeviceReady)

		case v1alpha2.StorageContainerRegistry:
			if vi.Status.Target.RegistryURL == "" {
				cb.
					Status(metav1.ConditionFalse).
					Reason(vmbdacondition.BlockDeviceNotReady).
					Message("Waiting until VirtualImage has associated RegistryUrl.")
				return reconcile.Result{}, nil
			}
		}

		cb.Status(metav1.ConditionTrue).Reason(vmbdacondition.BlockDeviceReady)
		return reconcile.Result{}, nil
	case v1alpha2.VMBDAObjectRefKindClusterVirtualImage:
		cviKey := types.NamespacedName{
			Name: vmbda.Spec.BlockDeviceRef.Name,
		}

		cvi, err := h.attachment.GetClusterVirtualImage(ctx, cviKey.Name)
		if err != nil {
			return reconcile.Result{}, err
		}

		if cvi == nil {
			cb.
				Status(metav1.ConditionFalse).
				Reason(vmbdacondition.BlockDeviceNotReady).
				Message(fmt.Sprintf("ClusterVirtualImage %q not found.", cviKey.String()))
			return reconcile.Result{}, nil
		}

		if cvi.GetDeletionTimestamp() != nil {
			cb.
				Status(metav1.ConditionFalse).
				Reason(vmbdacondition.BlockDeviceNotReady).
				Message(fmt.Sprintf("ClusterVirtualImage %q is being deleted. Delete VirtualMachineBlockDeviceAttachment to detach image from the VirtualMachine.", cviKey.String()))
			return reconcile.Result{}, nil
		}

		if cvi.Generation != cvi.Status.ObservedGeneration {
			cb.
				Status(metav1.ConditionFalse).
				Reason(vmbdacondition.BlockDeviceNotReady).
				Message(fmt.Sprintf("Waiting for the ClusterVirtualImage %q to be observed in its latest state generation.", cviKey.String()))
			return reconcile.Result{}, nil
		}

		if cvi.Status.Phase != v1alpha2.ImageReady {
			cb.
				Status(metav1.ConditionFalse).
				Reason(vmbdacondition.BlockDeviceNotReady).
				Message(fmt.Sprintf("ClusterVirtualImage %q is not ready to be attached to the virtual machine: waiting for the ClusterVirtualImage to be ready for attachment.", cviKey.String()))
			return reconcile.Result{}, nil
		}

		if cvi.Status.Target.RegistryURL == "" {
			cb.
				Status(metav1.ConditionFalse).
				Reason(vmbdacondition.BlockDeviceNotReady).
				Message("Waiting until VirtualImage has associated RegistryUrl.")
			return reconcile.Result{}, nil
		}

		cb.Status(metav1.ConditionTrue).Reason(vmbdacondition.BlockDeviceReady)
		return reconcile.Result{}, nil
	default:
		return reconcile.Result{}, fmt.Errorf("unknown block device kind %s", vmbda.Spec.BlockDeviceRef.Kind)
	}
}

func (h BlockDeviceReadyHandler) ValidateVirtualDiskReady(ctx context.Context, vmbda *v1alpha2.VirtualMachineBlockDeviceAttachment, cb *conditions.ConditionBuilder) error {
	vdKey := types.NamespacedName{
		Name:      vmbda.Spec.BlockDeviceRef.Name,
		Namespace: vmbda.Namespace,
	}

	vd, err := h.attachment.GetVirtualDisk(ctx, vdKey.Name, vdKey.Namespace)
	if err != nil {
		return err
	}

	if vd == nil {
		cb.
			Status(metav1.ConditionFalse).
			Reason(vmbdacondition.BlockDeviceNotReady).
			Message(fmt.Sprintf("VirtualDisk %q not found.", vdKey.String()))
		return nil
	}

	if vd.GetDeletionTimestamp() != nil {
		cb.
			Status(metav1.ConditionFalse).
			Reason(vmbdacondition.BlockDeviceNotReady).
			Message(fmt.Sprintf("VirtualDisk %q is being deleted. Delete VirtualMachineBlockDeviceAttachment to detach disk from the VirtualMachine.", vdKey.String()))
		return nil
	}

	if vd.Generation != vd.Status.ObservedGeneration {
		cb.
			Status(metav1.ConditionFalse).
			Reason(vmbdacondition.BlockDeviceNotReady).
			Message(fmt.Sprintf("Waiting for the VirtualDisk %q to be observed in its latest state generation.", vdKey.String()))
		return nil
	}

	diskPhaseReadyOrMigrating := vd.Status.Phase == v1alpha2.DiskReady || vd.Status.Phase == v1alpha2.DiskMigrating
	if !diskPhaseReadyOrMigrating && vd.Status.Phase != v1alpha2.DiskWaitForFirstConsumer {
		cb.
			Status(metav1.ConditionFalse).
			Reason(vmbdacondition.BlockDeviceNotReady).
			Message(fmt.Sprintf("VirtualDisk %q is not ready to be attached to the virtual machine: waiting for the VirtualDisk to be ready for attachment.", vdKey.String()))
		return nil
	}

	if diskPhaseReadyOrMigrating {
		diskReadyCondition, _ := conditions.GetCondition(vdcondition.ReadyType, vd.Status.Conditions)
		if diskReadyCondition.Status != metav1.ConditionTrue {
			cb.
				Status(metav1.ConditionFalse).
				Reason(vmbdacondition.BlockDeviceNotReady).
				Message(fmt.Sprintf("VirtualDisk %q is not Ready: waiting for the VirtualDisk to be Ready.", vdKey.String()))
			return nil
		}
	}

	if vd.Status.Target.PersistentVolumeClaim == "" {
		cb.
			Status(metav1.ConditionFalse).
			Reason(vmbdacondition.BlockDeviceNotReady).
			Message("Waiting until VirtualDisk has associated PersistentVolumeClaim name.")
		return nil
	}

	ad := service.NewAttachmentDiskFromVirtualDisk(vd)
	pvc, err := h.attachment.GetPersistentVolumeClaim(ctx, ad)
	if err != nil {
		return err
	}

	if pvc == nil {
		cb.
			Status(metav1.ConditionFalse).
			Reason(vmbdacondition.BlockDeviceNotReady).
			Message(fmt.Sprintf("Underlying PersistentVolumeClaim %q not found.", vd.Status.Target))
		return nil
	}

	if diskPhaseReadyOrMigrating && pvc.Status.Phase != corev1.ClaimBound {
		cb.
			Status(metav1.ConditionFalse).
			Reason(vmbdacondition.BlockDeviceNotReady).
			Message(fmt.Sprintf("Underlying PersistentVolumeClaim %q not bound.", vd.Status.Target))
		return nil
	}

	cb.Status(metav1.ConditionTrue).Reason(vmbdacondition.BlockDeviceReady)
	return nil
}
