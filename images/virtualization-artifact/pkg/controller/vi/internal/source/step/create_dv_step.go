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
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	cdiv1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/supplements"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vicondition"
)

type CreateDataVolumeStepDisk interface {
	StartImmediate(ctx context.Context, size resource.Quantity, sc *storagev1.StorageClass, source *cdiv1.DataVolumeSource, obj service.ObjectKind, sup *supplements.Generator) error
	GetStorageClass(ctx context.Context, scName string) (*storagev1.StorageClass, error)
}

type CreateDataVolumeStep struct {
	dv       *cdiv1.DataVolume
	recorder eventrecord.EventRecorderLogger
	disk     CreateDataVolumeStepDisk
	source   *cdiv1.DataVolumeSource
	size     resource.Quantity
	cb       *conditions.ConditionBuilder
}

func NewCreateDataVolumeStep(
	dv *cdiv1.DataVolume,
	recorder eventrecord.EventRecorderLogger,
	disk CreateDataVolumeStepDisk,
	source *cdiv1.DataVolumeSource,
	size resource.Quantity,
	cb *conditions.ConditionBuilder,
) *CreateDataVolumeStep {
	return &CreateDataVolumeStep{
		dv:       dv,
		recorder: recorder,
		disk:     disk,
		source:   source,
		size:     size,
		cb:       cb,
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

	supgen := supplements.NewGenerator(annotations.VIShortName, vi.Name, vi.Namespace, vi.UID)

	vi.Status.Progress = "0%"
	vi.Status.Target.PersistentVolumeClaim = supgen.PersistentVolumeClaim().Name

	sc, err := s.disk.GetStorageClass(ctx, vi.Status.StorageClassName)
	if err != nil {
		return nil, fmt.Errorf("get sc: %w", err)
	}

	err = s.disk.StartImmediate(ctx, s.size, sc, s.source, vi, supgen)
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
