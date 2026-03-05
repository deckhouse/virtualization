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
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vicondition"
)

type StorageClassReadyHandler struct {
	svc      StorageClassService
	recorder eventrecord.EventRecorderLogger
}

func (h StorageClassReadyHandler) Name() string {
	return "StorageClassReadyHandler"
}

func NewStorageClassReadyHandler(recorder eventrecord.EventRecorderLogger, svc StorageClassService) *StorageClassReadyHandler {
	return &StorageClassReadyHandler{
		svc:      svc,
		recorder: recorder,
	}
}

func (h StorageClassReadyHandler) Handle(ctx context.Context, vi *v1alpha2.VirtualImage) (reconcile.Result, error) {
	cb := conditions.NewConditionBuilder(vicondition.StorageClassReadyType).Generation(vi.Generation)

	if vi.DeletionTimestamp != nil {
		conditions.RemoveCondition(cb.GetType(), &vi.Status.Conditions)
		return reconcile.Result{}, nil
	}

	if vi.Spec.Storage == v1alpha2.StorageContainerRegistry {
		conditions.RemoveCondition(cb.GetType(), &vi.Status.Conditions)
		return reconcile.Result{}, nil
	}

	supgen := supplements.NewGenerator(annotations.VIShortName, vi.Name, vi.Namespace, vi.UID)
	pvc, err := h.svc.GetPersistentVolumeClaim(ctx, supgen)
	if err != nil {
		return reconcile.Result{}, err
	}

	// Reset storage class every time.
	vi.Status.StorageClassName = ""

	// 1. PVC already exists: used storage class is known.
	if pvc != nil {
		return reconcile.Result{}, h.setFromExistingPVC(ctx, vi, pvc, cb)
	}

	// 2. VirtualImage has storage class in the spec.
	if vi.Spec.PersistentVolumeClaim.StorageClass != nil && *vi.Spec.PersistentVolumeClaim.StorageClass != "" {
		return reconcile.Result{}, h.setFromSpec(ctx, vi, cb)
	}

	// 3. Try to use default storage class from the module settings.
	moduleStorageClass, err := h.svc.GetModuleStorageClass(ctx)
	if err != nil && !errors.Is(err, service.ErrDefaultStorageClassNotFound) {
		return reconcile.Result{}, fmt.Errorf("get module storage class: %w", err)
	}

	if moduleStorageClass != nil {
		storageProfile, err := h.svc.GetStorageProfile(ctx, moduleStorageClass.Name)
		if err != nil {
			return reconcile.Result{}, fmt.Errorf("get storage profile for the storage profile specified in the module config: %w", err)
		}

		err = h.svc.ValidateClaimPropertySets(storageProfile)
		if err != nil {
			cb.
				Status(metav1.ConditionFalse).
				Reason(vicondition.StorageClassNotReady).
				Message(service.CapitalizeFirstLetter(err.Error() + "."))
			conditions.SetCondition(cb, &vi.Status.Conditions)
			return reconcile.Result{}, nil
		}
		h.setFromModuleSettings(vi, moduleStorageClass, cb)
		return reconcile.Result{}, nil
	}

	// 4. Try to use default storage class from the cluster.
	defaultStorageClass, err := h.svc.GetDefaultStorageClass(ctx)
	if err != nil && !errors.Is(err, service.ErrDefaultStorageClassNotFound) {
		return reconcile.Result{}, fmt.Errorf("get default storage class: %w", err)
	}

	if defaultStorageClass != nil {
		storageProfile, err := h.svc.GetStorageProfile(ctx, defaultStorageClass.Name)
		if err != nil {
			return reconcile.Result{}, fmt.Errorf("get storage profile for the storage class specified in the global config: %w", err)
		}

		err = h.svc.ValidateClaimPropertySets(storageProfile)
		if err != nil {
			cb.
				Status(metav1.ConditionFalse).
				Reason(vicondition.StorageClassNotReady).
				Message(service.CapitalizeFirstLetter(err.Error() + "."))
			conditions.SetCondition(cb, &vi.Status.Conditions)
			return reconcile.Result{}, nil
		}
		h.setFromDefault(vi, defaultStorageClass, cb)
		return reconcile.Result{}, nil
	}

	msg := "The default StorageClass was not found in either the cluster or the module settings. Please specify a StorageClass name explicitly in the spec."
	h.recorder.Event(
		vi,
		corev1.EventTypeWarning,
		v1alpha2.ReasonVIStorageClassNotFound,
		msg,
	)
	cb.
		Status(metav1.ConditionFalse).
		Reason(vicondition.StorageClassNotFound).
		Message(msg)
	conditions.SetCondition(cb, &vi.Status.Conditions)

	return reconcile.Result{}, nil
}

func (h StorageClassReadyHandler) setFromSpec(ctx context.Context, vi *v1alpha2.VirtualImage, cb *conditions.ConditionBuilder) error {
	vi.Status.StorageClassName = *vi.Spec.PersistentVolumeClaim.StorageClass

	sc, err := h.svc.GetStorageClass(ctx, *vi.Spec.PersistentVolumeClaim.StorageClass)
	if err != nil {
		return fmt.Errorf("get storage class specified in the spec: %w", err)
	}

	if sc != nil {
		sp, err := h.svc.GetStorageProfile(ctx, sc.Name)
		if err != nil {
			return fmt.Errorf("get storage profile for the storage class specified in the spec: %w", err)
		}

		err = h.svc.ValidateClaimPropertySets(sp)
		if err != nil {
			cb.
				Status(metav1.ConditionFalse).
				Reason(vicondition.StorageClassNotReady).
				Message(service.CapitalizeFirstLetter(err.Error() + "."))
			conditions.SetCondition(cb, &vi.Status.Conditions)
			return nil
		}
	}

	if h.svc.IsStorageClassDeprecated(sc) {
		cb.
			Status(metav1.ConditionFalse).
			Reason(vicondition.StorageClassNotReady).
			Message(fmt.Sprintf("The provisioner of the %q storage class is deprecated; please use a different one.", *vi.Spec.PersistentVolumeClaim.StorageClass))
		conditions.SetCondition(cb, &vi.Status.Conditions)
		return nil
	}

	if !h.svc.IsStorageClassAllowed(*vi.Spec.PersistentVolumeClaim.StorageClass) {
		cb.
			Status(metav1.ConditionFalse).
			Reason(vicondition.StorageClassNotReady).
			Message(fmt.Sprintf(
				"The specified StorageClass %q is not permitted by the virtualization module configuration. Check settings in \"spec.settings.virtualImages\" section of the ModuleConfig.",
				vi.Status.StorageClassName,
			))
		conditions.SetCondition(cb, &vi.Status.Conditions)
		return nil
	}

	if sc == nil {
		cb.
			Status(metav1.ConditionFalse).
			Reason(vicondition.StorageClassNotFound).
			Message(fmt.Sprintf("The specified StorageClass %q was not found.", vi.Status.StorageClassName))
		conditions.SetCondition(cb, &vi.Status.Conditions)
		return nil
	}

	if !sc.DeletionTimestamp.IsZero() {
		cb.
			Status(metav1.ConditionFalse).
			Reason(vicondition.StorageClassNotReady).
			Message(fmt.Sprintf("The specified StorageClass %q is terminating and cannot be used.", vi.Status.StorageClassName))
		conditions.SetCondition(cb, &vi.Status.Conditions)
		return nil
	}

	cb.
		Status(metav1.ConditionTrue).
		Reason(vicondition.StorageClassReady).
		Message("")
	conditions.SetCondition(cb, &vi.Status.Conditions)
	return nil
}

func (h StorageClassReadyHandler) setFromExistingPVC(ctx context.Context, vi *v1alpha2.VirtualImage, pvc *corev1.PersistentVolumeClaim, cb *conditions.ConditionBuilder) error {
	if pvc.Spec.StorageClassName == nil || *pvc.Spec.StorageClassName == "" {
		return fmt.Errorf("pvc does not have storage class")
	}

	vi.Status.StorageClassName = *pvc.Spec.StorageClassName

	sc, err := h.svc.GetStorageClass(ctx, *pvc.Spec.StorageClassName)
	if err != nil {
		return fmt.Errorf("get storage class used by pvc: %w", err)
	}

	if sc == nil {
		cb.
			Status(metav1.ConditionFalse).
			Reason(vicondition.StorageClassNotFound).
			Message(fmt.Sprintf("The StorageClass %q used by the underlying PersistentVolumeClaim was not found.", vi.Status.StorageClassName))
		conditions.SetCondition(cb, &vi.Status.Conditions)
		return nil
	}

	if sc.DeletionTimestamp != nil {
		cb.
			Status(metav1.ConditionFalse).
			Reason(vicondition.StorageClassNotReady).
			Message(fmt.Sprintf("The StorageClass %q used by the underlying PersistentVolumeClaim is terminating and cannot be used.", vi.Status.StorageClassName))
		conditions.SetCondition(cb, &vi.Status.Conditions)
		return nil
	}

	cb.
		Status(metav1.ConditionTrue).
		Reason(vicondition.StorageClassReady).
		Message("")
	conditions.SetCondition(cb, &vi.Status.Conditions)
	return nil
}

func (h StorageClassReadyHandler) setFromModuleSettings(vi *v1alpha2.VirtualImage, moduleStorageClass *storagev1.StorageClass, cb *conditions.ConditionBuilder) {
	vi.Status.StorageClassName = moduleStorageClass.Name

	if h.svc.IsStorageClassDeprecated(moduleStorageClass) {
		cb.
			Status(metav1.ConditionFalse).
			Reason(vicondition.StorageClassNotReady).
			Message(fmt.Sprintf("The provisioner of the %q storage class is deprecated; please use a different one.", moduleStorageClass.Name))
		conditions.SetCondition(cb, &vi.Status.Conditions)
		return
	}

	if moduleStorageClass.DeletionTimestamp.IsZero() {
		cb.
			Status(metav1.ConditionTrue).
			Reason(vicondition.StorageClassReady).
			Message("")
		conditions.SetCondition(cb, &vi.Status.Conditions)
		return
	} else {
		cb.
			Status(metav1.ConditionFalse).
			Reason(vicondition.StorageClassNotReady).
			Message(fmt.Sprintf("The default StorageClass %q, defined in the module settings, is terminating and cannot be used.", vi.Status.StorageClassName))
		conditions.SetCondition(cb, &vi.Status.Conditions)
		return
	}
}

func (h StorageClassReadyHandler) setFromDefault(vi *v1alpha2.VirtualImage, defaultStorageClass *storagev1.StorageClass, cb *conditions.ConditionBuilder) {
	vi.Status.StorageClassName = defaultStorageClass.Name

	if h.svc.IsStorageClassDeprecated(defaultStorageClass) {
		cb.
			Status(metav1.ConditionFalse).
			Reason(vicondition.StorageClassNotReady).
			Message(fmt.Sprintf("The provisioner of the %q storage class is deprecated; please use a different one.", defaultStorageClass.Name))
		conditions.SetCondition(cb, &vi.Status.Conditions)
		return
	}

	if defaultStorageClass.DeletionTimestamp.IsZero() {
		cb.
			Status(metav1.ConditionTrue).
			Reason(vicondition.StorageClassReady).
			Message("")
		conditions.SetCondition(cb, &vi.Status.Conditions)
		return
	} else {
		cb.
			Status(metav1.ConditionFalse).
			Reason(vicondition.StorageClassNotReady).
			Message(fmt.Sprintf("The default StorageClass %q is terminating and cannot be used.", vi.Status.StorageClassName))
		conditions.SetCondition(cb, &vi.Status.Conditions)
		return
	}
}
