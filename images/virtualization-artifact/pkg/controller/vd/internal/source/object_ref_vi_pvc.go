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

package source

import (
	"context"
	"errors"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	cdiv1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/imageformat"
	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	"github.com/deckhouse/virtualization-controller/pkg/common/pointer"
	"github.com/deckhouse/virtualization-controller/pkg/common/provisioner"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/supplements"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdcondition"
)

type ObjectRefVirtualImagePVC struct {
	diskService *service.DiskService
	client      client.Client
	recorder    eventrecord.EventRecorderLogger
}

func NewObjectRefVirtualImagePVC(
	recorder eventrecord.EventRecorderLogger,
	diskService *service.DiskService,
	client client.Client,
) *ObjectRefVirtualImagePVC {
	return &ObjectRefVirtualImagePVC{
		diskService: diskService,
		client:      client,
		recorder:    recorder,
	}
}

func (ds ObjectRefVirtualImagePVC) Sync(ctx context.Context, vd *virtv2.VirtualDisk) (reconcile.Result, error) {
	if vd.Spec.DataSource == nil || vd.Spec.DataSource.ObjectRef == nil {
		return reconcile.Result{}, errors.New("object ref missed for data source")
	}

	log, _ := logger.GetDataSourceContext(ctx, objectRefDataSource)

	condition, _ := conditions.GetCondition(vdcondition.ReadyType, vd.Status.Conditions)
	cb := conditions.NewConditionBuilder(vdcondition.ReadyType).Generation(vd.Generation)
	defer func() { conditions.SetCondition(cb, &vd.Status.Conditions) }()

	supgen := supplements.NewGenerator(annotations.VDShortName, vd.Name, vd.Namespace, vd.UID)
	dv, err := ds.diskService.GetDataVolume(ctx, supgen)
	if err != nil {
		return reconcile.Result{}, err
	}
	pvc, err := ds.diskService.GetPersistentVolumeClaim(ctx, supgen)
	if err != nil {
		return reconcile.Result{}, err
	}
	vi, err := ds.diskService.GetVirtualImage(ctx, vd.Spec.DataSource.ObjectRef.Name, vd.Namespace)
	if err != nil {
		return reconcile.Result{}, err
	}
	if vi == nil {
		return reconcile.Result{}, errors.New("the source virtual image not found")
	}

	var dvQuotaNotExceededCondition *cdiv1.DataVolumeCondition
	var dvRunningCondition *cdiv1.DataVolumeCondition
	if dv != nil {
		dvQuotaNotExceededCondition = service.GetDataVolumeCondition(DVQoutaNotExceededConditionType, dv.Status.Conditions)
		dvRunningCondition = service.GetDataVolumeCondition(DVRunningConditionType, dv.Status.Conditions)
	}

	switch {
	case IsDiskProvisioningFinished(condition):
		log.Info("Disk provisioning finished: clean up")

		setPhaseConditionForFinishedDisk(pvc, cb, &vd.Status.Phase, supgen)

		// Protect Ready Disk and underlying PVC.
		err = ds.diskService.Protect(ctx, vd, nil, pvc)
		if err != nil {
			return reconcile.Result{}, err
		}

		err = ds.diskService.Unprotect(ctx, dv)
		if err != nil {
			return reconcile.Result{}, err
		}

		return CleanUpSupplements(ctx, vd, ds)
	case object.AnyTerminating(dv, pvc):
		log.Info("Waiting for supplements to be terminated")
	case dv == nil:
		ds.recorder.Event(
			vd,
			corev1.EventTypeNormal,
			virtv2.ReasonDataSourceSyncStarted,
			"The ObjectRef DataSource import to PVC has started",
		)

		vd.Status.Progress = "0%"
		vd.Status.SourceUID = pointer.GetPointer(vi.GetUID())

		if imageformat.IsISO(vi.Status.Format) {
			setPhaseConditionToFailed(cb, &vd.Status.Phase, ErrISOSourceNotSupported)
			return reconcile.Result{}, nil
		}

		var size resource.Quantity
		size, err = ds.getPVCSize(vd, vi.Status.Size)
		if err != nil {
			setPhaseConditionToFailed(cb, &vd.Status.Phase, err)

			if errors.Is(err, service.ErrInsufficientPVCSize) {
				return reconcile.Result{}, nil
			}

			return reconcile.Result{}, err
		}

		source := &cdiv1.DataVolumeSource{
			PVC: &cdiv1.DataVolumeSourcePVC{
				Name:      vi.Status.Target.PersistentVolumeClaim,
				Namespace: vi.Namespace,
			},
		}

		var nodePlacement *provisioner.NodePlacement
		nodePlacement, err = getNodePlacement(ctx, ds.client, vd)
		if err != nil {
			setPhaseConditionToFailed(cb, &vd.Status.Phase, fmt.Errorf("unexpected error: %w", err))
			return reconcile.Result{}, fmt.Errorf("failed to get importer tolerations: %w", err)
		}

		var sc *storagev1.StorageClass
		sc, err = ds.diskService.GetStorageClass(ctx, vd.Status.StorageClassName)
		if err != nil {
			return reconcile.Result{}, err
		}

		err = ds.diskService.Start(ctx, size, sc, source, vd, supgen, service.WithNodePlacement(nodePlacement))
		if updated, err := setPhaseConditionFromStorageError(err, vd, cb); err != nil || updated {
			return reconcile.Result{}, err
		}

		vd.Status.Phase = virtv2.DiskProvisioning
		cb.
			Status(metav1.ConditionFalse).
			Reason(vdcondition.Provisioning).
			Message("PVC Provisioner not found: create the new one.")

		return reconcile.Result{RequeueAfter: time.Second}, nil
	case dvQuotaNotExceededCondition != nil && dvQuotaNotExceededCondition.Status == corev1.ConditionFalse:
		vd.Status.Phase = virtv2.DiskPending
		cb.
			Status(metav1.ConditionFalse).
			Reason(vdcondition.QuotaExceeded).
			Message(dvQuotaNotExceededCondition.Message)
		return reconcile.Result{}, nil
	case dvRunningCondition != nil && dvRunningCondition.Status != corev1.ConditionTrue && dvRunningCondition.Reason == DVImagePullFailedReason:
		vd.Status.Phase = virtv2.DiskPending
		cb.
			Status(metav1.ConditionFalse).
			Reason(vdcondition.ImagePullFailed).
			Message(dvRunningCondition.Message)
		ds.recorder.Event(vd, corev1.EventTypeWarning, vdcondition.ImagePullFailed.String(), dvRunningCondition.Message)
		return reconcile.Result{}, nil
	case pvc == nil:
		vd.Status.Phase = virtv2.DiskProvisioning
		cb.
			Status(metav1.ConditionFalse).
			Reason(vdcondition.Provisioning).
			Message("PVC not found: waiting for creation.")
		return reconcile.Result{RequeueAfter: time.Second}, nil
	case ds.diskService.IsImportDone(dv, pvc):
		log.Info("Import has completed", "dvProgress", dv.Status.Progress, "dvPhase", dv.Status.Phase, "pvcPhase", pvc.Status.Phase)
		ds.recorder.Event(
			vd,
			corev1.EventTypeNormal,
			virtv2.ReasonDataSourceSyncCompleted,
			"The ObjectRef DataSource import has completed",
		)

		vd.Status.Phase = virtv2.DiskReady
		cb.
			Status(metav1.ConditionTrue).
			Reason(vdcondition.Ready).
			Message("")

		vd.Status.Progress = "100%"
		vd.Status.Capacity = ds.diskService.GetCapacity(pvc)
		vd.Status.Target.PersistentVolumeClaim = dv.Status.ClaimName
	default:
		log.Info("Provisioning to PVC is in progress", "dvProgress", dv.Status.Progress, "dvPhase", dv.Status.Phase, "pvcPhase", pvc.Status.Phase)

		err = ds.diskService.CheckProvisioning(ctx, pvc)
		if err != nil {
			return reconcile.Result{}, setPhaseConditionFromProvisioningError(ctx, err, cb, vd, dv, ds.diskService, ds.client)
		}

		vd.Status.Progress = ds.diskService.GetProgress(dv, vd.Status.Progress, service.NewScaleOption(0, 100))
		vd.Status.Target.PersistentVolumeClaim = dv.Status.ClaimName

		err = ds.diskService.Protect(ctx, vd, dv, pvc)
		if err != nil {
			return reconcile.Result{}, err
		}

		var sc *storagev1.StorageClass
		sc, err = ds.diskService.GetStorageClass(ctx, ptr.Deref(pvc.Spec.StorageClassName, ""))
		if err != nil {
			return reconcile.Result{}, err
		}

		if err = setPhaseConditionForPVCProvisioningDisk(ctx, dv, vd, pvc, sc, cb, ds.diskService); err != nil {
			return reconcile.Result{}, err
		}

		return reconcile.Result{}, nil
	}

	return reconcile.Result{RequeueAfter: time.Second}, nil
}

func (ds ObjectRefVirtualImagePVC) Validate(ctx context.Context, vd *virtv2.VirtualDisk) error {
	if vd.Spec.DataSource == nil || vd.Spec.DataSource.ObjectRef == nil {
		return errors.New("object ref missed for data source")
	}

	vi, err := ds.diskService.GetVirtualImage(ctx, vd.Spec.DataSource.ObjectRef.Name, vd.Namespace)
	if err != nil {
		return fmt.Errorf("unable to get VI: %w", err)
	}

	if vi == nil || vi.Status.Phase != virtv2.ImageReady || vi.Status.Target.PersistentVolumeClaim == "" {
		return NewImageNotReadyError(vd.Spec.DataSource.ObjectRef.Name)
	}

	return nil
}

func (ds ObjectRefVirtualImagePVC) CleanUpSupplements(ctx context.Context, vd *virtv2.VirtualDisk) (reconcile.Result, error) {
	supgen := supplements.NewGenerator(annotations.VDShortName, vd.Name, vd.Namespace, vd.UID)

	diskRequeue, err := ds.diskService.CleanUpSupplements(ctx, supgen)
	if err != nil {
		return reconcile.Result{}, err
	}

	if diskRequeue {
		return reconcile.Result{RequeueAfter: time.Second}, nil
	} else {
		return reconcile.Result{}, nil
	}
}

func (ds ObjectRefVirtualImagePVC) getPVCSize(vd *virtv2.VirtualDisk, imageSize virtv2.ImageStatusSize) (resource.Quantity, error) {
	unpackedSize, err := resource.ParseQuantity(imageSize.UnpackedBytes)
	if err != nil {
		return resource.Quantity{}, fmt.Errorf("failed to parse unpacked bytes %s: %w", imageSize.UnpackedBytes, err)
	}

	if unpackedSize.IsZero() {
		return resource.Quantity{}, errors.New("got zero unpacked size from data source")
	}

	return service.GetValidatedPVCSize(vd.Spec.PersistentVolumeClaim.Size, unpackedSize)
}
