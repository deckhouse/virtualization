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

	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/supplements"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdcondition"
)

type StorageClassReadyHandler struct {
	svc StorageClassService
}

func NewStorageClassReadyHandler(svc StorageClassService) *StorageClassReadyHandler {
	return &StorageClassReadyHandler{
		svc: svc,
	}
}

func (h StorageClassReadyHandler) Handle(ctx context.Context, vd *virtv2.VirtualDisk) (reconcile.Result, error) {
	cb := conditions.NewConditionBuilder(vdcondition.StorageClassReadyType).Generation(vd.Generation)

	defer func() { conditions.SetCondition(cb, &vd.Status.Conditions) }()

	if vd.DeletionTimestamp != nil {
		cb.
			Status(metav1.ConditionUnknown).
			Reason(conditions.ReasonUnknown).
			Message("")
		return reconcile.Result{}, nil
	}

	sup := supplements.NewGenerator(annotations.VDShortName, vd.Name, vd.Namespace, vd.UID)
	pvc, err := h.svc.GetPersistentVolumeClaim(ctx, sup)
	if err != nil {
		return reconcile.Result{}, err
	}

	// Reset storage class every time.
	vd.Status.StorageClassName = ""

	// 1. PVC already exists: used storage class is known.
	if pvc != nil {
		return reconcile.Result{}, h.setFromExistingPVC(ctx, vd, pvc, cb)
	}

	// 2. VirtualDisk has storage class in the spec.
	if vd.Spec.PersistentVolumeClaim.StorageClass != nil && *vd.Spec.PersistentVolumeClaim.StorageClass != "" {
		return reconcile.Result{}, h.setFromSpec(ctx, vd, cb)
	}

	// 3. Try to use default storage class from the module settings.
	moduleStorageClass, err := h.svc.GetModuleStorageClass(ctx)
	if err != nil && !errors.Is(err, service.ErrDefaultStorageClassNotFound) {
		return reconcile.Result{}, fmt.Errorf("get module storage class: %w", err)
	}

	if moduleStorageClass != nil {
		h.setFromModuleSettings(vd, moduleStorageClass, cb)
		return reconcile.Result{}, nil
	}

	// 4. Try to use default storage class from the cluster.
	defaultStorageClass, err := h.svc.GetDefaultStorageClass(ctx)
	if err != nil && !errors.Is(err, service.ErrDefaultStorageClassNotFound) {
		return reconcile.Result{}, fmt.Errorf("get default storage class: %w", err)
	}

	if defaultStorageClass != nil {
		h.setFromDefault(vd, defaultStorageClass, cb)
		return reconcile.Result{}, nil
	}

	cb.
		Status(metav1.ConditionFalse).
		Reason(vdcondition.StorageClassNotReady).
		Message("The default StorageClass was not found in either the cluster or the module settings. Please specify a StorageClass name explicitly in the spec.")
	return reconcile.Result{}, nil
}

func (h StorageClassReadyHandler) setFromExistingPVC(ctx context.Context, vd *virtv2.VirtualDisk, pvc *corev1.PersistentVolumeClaim, cb *conditions.ConditionBuilder) error {
	if pvc.Spec.StorageClassName == nil || *pvc.Spec.StorageClassName == "" {
		return fmt.Errorf("pvc does not have storage class")
	}

	vd.Status.StorageClassName = *pvc.Spec.StorageClassName

	sc, err := h.svc.GetStorageClass(ctx, *pvc.Spec.StorageClassName)
	if err != nil {
		return fmt.Errorf("get storage class used by pvc: %w", err)
	}

	if sc == nil {
		cb.
			Status(metav1.ConditionFalse).
			Reason(vdcondition.StorageClassNotReady).
			Message(fmt.Sprintf("The StorageClass %q used by the underlying PersistentVolumeClaim was not found.", vd.Status.StorageClassName))
		return nil
	}

	if sc.DeletionTimestamp != nil {
		cb.
			Status(metav1.ConditionFalse).
			Reason(vdcondition.StorageClassNotReady).
			Message(fmt.Sprintf("The StorageClass %q used by the underlying PersistentVolumeClaim is terminating and cannot be used.", vd.Status.StorageClassName))
		return nil
	}

	cb.
		Status(metav1.ConditionTrue).
		Reason(vdcondition.StorageClassReady).
		Message("")
	return nil
}

func (h StorageClassReadyHandler) setFromSpec(ctx context.Context, vd *virtv2.VirtualDisk, cb *conditions.ConditionBuilder) error {
	vd.Status.StorageClassName = *vd.Spec.PersistentVolumeClaim.StorageClass

	if !h.svc.IsStorageClassAllowed(*vd.Spec.PersistentVolumeClaim.StorageClass) {
		cb.
			Status(metav1.ConditionFalse).
			Reason(vdcondition.StorageClassNotReady).
			Message(fmt.Sprintf("The specified StorageClass %q is not allowed. Please check the module settings.", vd.Status.StorageClassName))
		return nil
	}

	sc, err := h.svc.GetStorageClass(ctx, *vd.Spec.PersistentVolumeClaim.StorageClass)
	if err != nil {
		return fmt.Errorf("get storage class specified in spec: %w", err)
	}

	if sc == nil {
		cb.
			Status(metav1.ConditionFalse).
			Reason(vdcondition.StorageClassNotReady).
			Message(fmt.Sprintf("The specified StorageClass %q was not found.", vd.Status.StorageClassName))
		return nil
	}

	if !sc.DeletionTimestamp.IsZero() {
		cb.
			Status(metav1.ConditionFalse).
			Reason(vdcondition.StorageClassNotReady).
			Message(fmt.Sprintf("The specified StorageClass %q is terminating and cannot be used.", vd.Status.StorageClassName))
		return nil
	}

	cb.
		Status(metav1.ConditionTrue).
		Reason(vdcondition.StorageClassReady).
		Message("")
	return nil
}

func (h StorageClassReadyHandler) setFromModuleSettings(vd *virtv2.VirtualDisk, moduleStorageClass *storagev1.StorageClass, cb *conditions.ConditionBuilder) {
	vd.Status.StorageClassName = moduleStorageClass.Name

	if moduleStorageClass.DeletionTimestamp.IsZero() {
		cb.
			Status(metav1.ConditionTrue).
			Reason(vdcondition.StorageClassReady).
			Message("")
	} else {
		cb.
			Status(metav1.ConditionFalse).
			Reason(vdcondition.StorageClassNotReady).
			Message(fmt.Sprintf("The default StorageClass %q, defined in the module settings, is terminating and cannot be used.", vd.Status.StorageClassName))
	}
}

func (h StorageClassReadyHandler) setFromDefault(vd *virtv2.VirtualDisk, defaultStorageClass *storagev1.StorageClass, cb *conditions.ConditionBuilder) {
	vd.Status.StorageClassName = defaultStorageClass.Name

	if defaultStorageClass.DeletionTimestamp.IsZero() {
		cb.
			Status(metav1.ConditionTrue).
			Reason(vdcondition.StorageClassReady).
			Message("")
	} else {
		cb.
			Status(metav1.ConditionFalse).
			Reason(vdcondition.StorageClassNotReady).
			Message(fmt.Sprintf("The default StorageClass %q is terminating and cannot be used.", vd.Status.StorageClassName))
	}
}
