/*
Copyright 2025 Flant JSC

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

package step

import (
	"context"
	"errors"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	cdiv1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	"github.com/deckhouse/virtualization-controller/pkg/common/pointer"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/supplements"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vicondition"
)

type CreateDataVolumeStep struct {
	pvc         *corev1.PersistentVolumeClaim
	dv          *cdiv1.DataVolume
	recorder    eventrecord.EventRecorderLogger
	diskService *service.DiskService
	scService   *service.VirtualImageStorageClassService
	client      client.Client
	cb          *conditions.ConditionBuilder
}

func NewCreateDataVolumeStep(
	pvc *corev1.PersistentVolumeClaim,
	dv *cdiv1.DataVolume,
	recorder eventrecord.EventRecorderLogger,
	diskService *service.DiskService,
	scService *service.VirtualImageStorageClassService,
	client client.Client,
	cb *conditions.ConditionBuilder,
) *CreateDataVolumeStep {
	return &CreateDataVolumeStep{
		pvc:         pvc,
		dv:          dv,
		recorder:    recorder,
		diskService: diskService,
		scService:   scService,
		client:      client,
		cb:          cb,
	}
}

func (s CreateDataVolumeStep) Take(ctx context.Context, vi *virtv2.VirtualImage) (*reconcile.Result, error) {
	if s.dv != nil {
		return nil, nil
	}

	s.recorder.Event(
		vi,
		corev1.EventTypeNormal,
		virtv2.ReasonDataSourceSyncStarted,
		"The ObjectRef DataSource import has started",
	)

	vdRefKey := types.NamespacedName{Name: vi.Spec.DataSource.ObjectRef.Name, Namespace: vi.Namespace}
	vdRef, err := object.FetchObject(ctx, vdRefKey, s.client, &virtv2.VirtualDisk{})
	if err != nil {
		return nil, fmt.Errorf("fetch vd %q: %w", vdRefKey, err)
	}

	if vdRef == nil {
		return nil, fmt.Errorf("vd object ref %q is nil", vdRefKey)
	}

	vi.Status.Progress = "0%"
	vi.Status.SourceUID = pointer.GetPointer(vdRef.GetUID())

	sc, res, err := s.getStorageClassName(ctx, vi)
	if err != nil {
		return nil, fmt.Errorf("get sc name: %w", err)
	}
	if res != nil {
		return res, nil
	}

	res, err = s.startImmediate(ctx, vi, vdRef, sc)
	if err != nil {
		return nil, fmt.Errorf("start immediate: %w", err)
	}
	if res != nil {
		return res, nil
	}

	return nil, nil
}

func (s CreateDataVolumeStep) getStorageClassName(ctx context.Context, vi *virtv2.VirtualImage) (*string, *reconcile.Result, error) {
	clusterDefaultSC, err := s.diskService.GetDefaultStorageClass(ctx)
	if err != nil {
		return nil, nil, err
	}

	scName, err := s.scService.GetStorageClass(vi.Spec.PersistentVolumeClaim.StorageClass, clusterDefaultSC)
	switch {
	case err == nil:
		// OK.
	case errors.Is(err, service.ErrStorageClassNotFound):
		vi.Status.Phase = virtv2.ImagePending
		s.cb.
			Status(metav1.ConditionFalse).
			Reason(vicondition.ProvisioningFailed).
			Message("Provided StorageClass not found in the cluster.")
		return nil, &reconcile.Result{}, nil
	case errors.Is(err, service.ErrStorageClassNotAllowed):
		vi.Status.Phase = virtv2.ImagePending
		s.cb.
			Status(metav1.ConditionFalse).
			Reason(vicondition.ProvisioningFailed).
			Message("Specified StorageClass is not allowed: please change provided StorageClass name or check the module settings.")
		return nil, &reconcile.Result{}, nil
	default:
		return nil, nil, fmt.Errorf("get vi sc: %w", err)
	}

	return scName, nil, nil
}

func (s CreateDataVolumeStep) startImmediate(ctx context.Context, vi *virtv2.VirtualImage, vdRef *virtv2.VirtualDisk, sc *string) (*reconcile.Result, error) {
	supgen := supplements.NewGenerator(annotations.VIShortName, vi.Name, vi.Namespace, vi.UID)

	source := &cdiv1.DataVolumeSource{
		PVC: &cdiv1.DataVolumeSourcePVC{
			Name:      vdRef.Status.Target.PersistentVolumeClaim,
			Namespace: vdRef.Namespace,
		},
	}

	size, err := resource.ParseQuantity(vdRef.Status.Capacity)
	if err != nil {
		return nil, fmt.Errorf("parse quantity: %w", err)
	}

	err = s.diskService.StartImmediate(ctx, size, sc, source, vi, supgen)
	switch {
	case err == nil:
		// OK.
	case errors.Is(err, service.ErrStorageProfileNotFound):
		vi.Status.Phase = virtv2.ImageFailed
		s.cb.
			Status(metav1.ConditionFalse).
			Reason(vicondition.ProvisioningFailed).
			Message("StorageProfile not found in the cluster: Please check a StorageClass name in the cluster or set a default StorageClass.")
		return &reconcile.Result{}, nil
	case errors.Is(err, service.ErrStorageClassNotFound):
		vi.Status.Phase = virtv2.ImagePending
		s.cb.
			Status(metav1.ConditionFalse).
			Reason(vicondition.ProvisioningFailed).
			Message("Provided StorageClass not found in the cluster.")
		return &reconcile.Result{}, nil
	case errors.Is(err, service.ErrDefaultStorageClassNotFound):
		vi.Status.Phase = virtv2.ImagePending
		s.cb.
			Status(metav1.ConditionFalse).
			Reason(vicondition.ProvisioningFailed).
			Message("Default StorageClass not found in the cluster: please provide a StorageClass name or set a default StorageClass.")
		return &reconcile.Result{}, nil
	default:
		return nil, fmt.Errorf("start immediate: %w", err)
	}

	return nil, nil
}
