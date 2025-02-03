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
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"
	storev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/common"
	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/datasource"
	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	podutil "github.com/deckhouse/virtualization-controller/pkg/common/pod"
	"github.com/deckhouse/virtualization-controller/pkg/common/pointer"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/importer"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/supplements"
	"github.com/deckhouse/virtualization-controller/pkg/dvcr"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vicondition"
)

type ObjectRefVirtualDiskSnapshot struct {
	importerService     Importer
	bounderService      Bounder
	diskService         *service.DiskService
	statService         Stat
	dvcrSettings        *dvcr.Settings
	client              client.Client
	storageClassService *service.VirtualImageStorageClassService
	recorder            eventrecord.EventRecorderLogger
}

func NewObjectRefVirtualDiskSnapshot(
	recorder eventrecord.EventRecorderLogger,
	importerService Importer,
	bounderService Bounder,
	client client.Client,
	diskService *service.DiskService,
	dvcrSettings *dvcr.Settings,
	statService Stat,
	storageClassService *service.VirtualImageStorageClassService,
) *ObjectRefVirtualDiskSnapshot {
	return &ObjectRefVirtualDiskSnapshot{
		importerService:     importerService,
		bounderService:      bounderService,
		client:              client,
		recorder:            recorder,
		diskService:         diskService,
		statService:         statService,
		dvcrSettings:        dvcrSettings,
		storageClassService: storageClassService,
	}
}

func (ds ObjectRefVirtualDiskSnapshot) StoreToDVCR(ctx context.Context, vi *virtv2.VirtualImage, vdSnapshotRef *virtv2.VirtualDiskSnapshot, cb *conditions.ConditionBuilder) (reconcile.Result, error) {
	log, ctx := logger.GetDataSourceContext(ctx, "objectref")

	supgen := supplements.NewGenerator(annotations.VIShortName, vi.Name, vdSnapshotRef.Namespace, vi.UID)
	pod, err := ds.importerService.GetPod(ctx, supgen)
	if err != nil {
		return reconcile.Result{}, err
	}

	pvc, err := ds.diskService.GetPersistentVolumeClaim(ctx, supgen)
	if err != nil {
		return reconcile.Result{}, err
	}

	vs, err := ds.diskService.GetVolumeSnapshot(ctx, vdSnapshotRef.Status.VolumeSnapshotName, vdSnapshotRef.Namespace)
	if err != nil {
		return reconcile.Result{}, err
	}

	condition, _ := conditions.GetCondition(vicondition.ReadyType, vi.Status.Conditions)
	switch {
	case isDiskProvisioningFinished(condition):
		log.Info("Virtual image provisioning finished: clean up")

		cb.
			Status(metav1.ConditionTrue).
			Reason(vicondition.Ready).
			Message("")

		vi.Status.Phase = virtv2.ImageReady

		err = ds.importerService.Unprotect(ctx, pod)
		if err != nil {
			return reconcile.Result{}, err
		}

		return CleanUpSupplements(ctx, vi, ds)
	case object.AnyTerminating(pod, pvc):
		vi.Status.Phase = virtv2.ImagePending

		log.Info("Cleaning up...")
	case pvc == nil:
		ds.recorder.Event(
			vi,
			corev1.EventTypeNormal,
			virtv2.ReasonDataSourceSyncStarted,
			"The ObjectRef DataSource import has started",
		)

		namespacedName := supplements.NewGenerator(annotations.VIShortName, vi.Name, vi.Namespace, vi.UID).PersistentVolumeClaim()

		storageClassName := vs.Annotations["storageClass"]
		volumeMode := vs.Annotations["volumeMode"]
		accessModesStr := strings.Split(vs.Annotations["accessModes"], ",")
		accessModes := make([]corev1.PersistentVolumeAccessMode, 0, len(accessModesStr))
		for _, accessModeStr := range accessModesStr {
			accessModes = append(accessModes, corev1.PersistentVolumeAccessMode(accessModeStr))
		}

		spec := corev1.PersistentVolumeClaimSpec{
			AccessModes: accessModes,
			DataSource: &corev1.TypedLocalObjectReference{
				APIGroup: ptr.To(vs.GroupVersionKind().Group),
				Kind:     vs.Kind,
				Name:     vs.Name,
			},
		}

		if storageClassName != "" {
			spec.StorageClassName = &storageClassName
			vi.Status.StorageClassName = storageClassName
		}

		if volumeMode != "" {
			spec.VolumeMode = ptr.To(corev1.PersistentVolumeMode(volumeMode))
		}

		if vs.Status != nil && vs.Status.RestoreSize != nil {
			spec.Resources = corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: *vs.Status.RestoreSize,
				},
			}
		}

		pvc = &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:      namespacedName.Name,
				Namespace: namespacedName.Namespace,
				OwnerReferences: []metav1.OwnerReference{
					service.MakeOwnerReference(vi),
				},
			},
			Spec: spec,
		}

		err = ds.diskService.CreatePersistentVolumeClaim(ctx, pvc)
		if err != nil {
			setPhaseConditionToFailed(cb, &vi.Status.Phase, err)
			return reconcile.Result{}, err
		}

		vi.Status.Phase = virtv2.ImageProvisioning
		cb.
			Status(metav1.ConditionFalse).
			Reason(vicondition.Provisioning).
			Message("PVC has created: waiting to be Bound.")

		vi.Status.Progress = "0%"
		vi.Status.SourceUID = pointer.GetPointer(vs.UID)

		return reconcile.Result{Requeue: true}, err
	case pod == nil:
		vi.Status.Progress = ds.statService.GetProgress(vi.GetUID(), pod, vi.Status.Progress)
		vi.Status.Target.RegistryURL = ds.statService.GetDVCRImageName(pod)

		envSettings := ds.getEnvSettings(vi, supgen)

		ownerRef := metav1.NewControllerRef(vi, vi.GroupVersionKind())
		podSettings := ds.importerService.GetPodSettingsWithPVC(ownerRef, supgen, pvc.Name, pvc.Namespace)
		err = ds.importerService.StartWithPodSetting(ctx, envSettings, supgen, datasource.NewCABundleForVMI(vi.GetNamespace(), vi.Spec.DataSource), podSettings)
		switch {
		case err == nil:
			// OK.
		case common.ErrQuotaExceeded(err):
			ds.recorder.Event(vi, corev1.EventTypeWarning, virtv2.ReasonDataSourceQuotaExceeded, "DataSource quota exceed")
			return setQuotaExceededPhaseCondition(cb, &vi.Status.Phase, err, vi.CreationTimestamp), nil
		default:
			setPhaseConditionToFailed(cb, &vi.Status.Phase, fmt.Errorf("unexpected error: %w", err))
			return reconcile.Result{}, err
		}

		vi.Status.Phase = virtv2.ImageProvisioning
		cb.
			Status(metav1.ConditionFalse).
			Reason(vicondition.Provisioning).
			Message("DVCR Provisioner not found: create the new one.")

		log.Info("Create importer pod...", "progress", vi.Status.Progress, "pod.phase", "nil")

		return reconcile.Result{Requeue: true}, nil
	case podutil.IsPodComplete(pod):
		err = ds.statService.CheckPod(pod)
		if err != nil {
			vi.Status.Phase = virtv2.ImageFailed

			switch {
			case errors.Is(err, service.ErrProvisioningFailed):
				ds.recorder.Event(vi, corev1.EventTypeWarning, virtv2.ReasonDataSourceDiskProvisioningFailed, "Disk provisioning failed")
				cb.
					Status(metav1.ConditionFalse).
					Reason(vicondition.ProvisioningFailed).
					Message(service.CapitalizeFirstLetter(err.Error() + "."))
				return reconcile.Result{}, nil
			default:
				return reconcile.Result{}, err
			}
		}

		cb.
			Status(metav1.ConditionTrue).
			Reason(vicondition.Ready).
			Message("")

		vi.Status.Phase = virtv2.ImageReady
		vi.Status.Size = ds.statService.GetSize(pod)
		vi.Status.CDROM = ds.statService.GetCDROM(pod)
		vi.Status.Format = ds.statService.GetFormat(pod)
		vi.Status.Progress = "100%"
		vi.Status.Target.RegistryURL = ds.statService.GetDVCRImageName(pod)

		log.Info("Ready", "progress", vi.Status.Progress, "pod.phase", pod.Status.Phase)
	default:
		err = ds.statService.CheckPod(pod)
		if err != nil {
			vi.Status.Phase = virtv2.ImageFailed

			switch {
			case errors.Is(err, service.ErrNotInitialized), errors.Is(err, service.ErrNotScheduled):
				if strings.Contains(err.Error(), "pod has unbound immediate PersistentVolumeClaims") {
					vi.Status.Phase = virtv2.ImageProvisioning
					cb.
						Status(metav1.ConditionFalse).
						Reason(vicondition.Provisioning).
						Message("Waiting for PVC to be bound")

					return reconcile.Result{Requeue: true}, nil
				}

				cb.
					Status(metav1.ConditionFalse).
					Reason(vicondition.ProvisioningNotStarted).
					Message(service.CapitalizeFirstLetter(err.Error() + "."))
				return reconcile.Result{}, nil
			case errors.Is(err, service.ErrProvisioningFailed):
				ds.recorder.Event(vi, corev1.EventTypeWarning, virtv2.ReasonDataSourceDiskProvisioningFailed, "Disk provisioning failed")
				cb.
					Status(metav1.ConditionFalse).
					Reason(vicondition.ProvisioningFailed).
					Message(service.CapitalizeFirstLetter(err.Error() + "."))
				return reconcile.Result{}, nil
			default:
				return reconcile.Result{}, err
			}
		}

		err = ds.importerService.Protect(ctx, pod)
		if err != nil {
			return reconcile.Result{}, err
		}

		cb.
			Status(metav1.ConditionFalse).
			Reason(vicondition.Provisioning).
			Message("Import is in the process of provisioning to DVCR.")

		vi.Status.Phase = virtv2.ImageProvisioning
		vi.Status.Progress = ds.statService.GetProgress(vi.GetUID(), pod, vi.Status.Progress)
		vi.Status.Target.RegistryURL = ds.statService.GetDVCRImageName(pod)

		log.Info("Provisioning...", "progress", vi.Status.Progress, "pod.phase", pod.Status.Phase)
	}

	return reconcile.Result{Requeue: true}, nil
}

func (ds ObjectRefVirtualDiskSnapshot) StoreToPVC(ctx context.Context, vi *virtv2.VirtualImage, vdSnapshotRef *virtv2.VirtualDiskSnapshot, cb *conditions.ConditionBuilder) (reconcile.Result, error) {
	log, ctx := logger.GetDataSourceContext(ctx, objectRefDataSource)

	supgen := supplements.NewGenerator(annotations.VIShortName, vi.Name, vi.Namespace, vi.UID)

	pod, err := ds.bounderService.GetPod(ctx, supgen)
	if err != nil {
		return reconcile.Result{}, err
	}

	pvc, err := ds.diskService.GetPersistentVolumeClaim(ctx, supgen)
	if err != nil {
		return reconcile.Result{}, err
	}

	vs, err := ds.diskService.GetVolumeSnapshot(ctx, vdSnapshotRef.Status.VolumeSnapshotName, vdSnapshotRef.Namespace)
	if err != nil {
		return reconcile.Result{}, err
	}

	clusterDefaultSC, _ := ds.diskService.GetDefaultStorageClass(ctx)
	sc, err := ds.storageClassService.GetStorageClass(vi.Spec.PersistentVolumeClaim.StorageClass, clusterDefaultSC)
	if updated, err := setConditionFromStorageClassError(err, cb); err != nil || updated {
		return reconcile.Result{}, err
	}

	storageClass, err := ds.diskService.GetStorageClass(ctx, sc)
	if err != nil {
		return reconcile.Result{}, err
	}

	condition, _ := conditions.GetCondition(vicondition.ReadyType, vi.Status.Conditions)
	switch {
	case isDiskProvisioningFinished(condition):
		log.Info("Disk provisioning finished: clean up")

		setPhaseConditionForFinishedImage(pvc, cb, &vi.Status.Phase, supgen)

		// Protect Ready Disk and underlying PVC.
		err = ds.diskService.Protect(ctx, vi, nil, pvc)
		if err != nil {
			return reconcile.Result{}, err
		}

		err = ds.bounderService.Unprotect(ctx, pod)
		if err != nil {
			return reconcile.Result{}, err
		}

		return CleanUpSupplements(ctx, vi, ds)
	case object.AnyTerminating(pvc, pod):
		log.Info("Waiting for supplements to be terminated")
	case pvc == nil:
		ds.recorder.Event(
			vi,
			corev1.EventTypeNormal,
			virtv2.ReasonDataSourceSyncStarted,
			"The ObjectRef DataSource import has started",
		)

		namespacedName := supplements.NewGenerator(annotations.VIShortName, vi.Name, vi.Namespace, vi.UID).PersistentVolumeClaim()

		storageClassName := vs.Annotations["storageClass"]
		volumeMode := vs.Annotations["volumeMode"]
		accessModesStr := strings.Split(vs.Annotations["accessModes"], ",")
		accessModes := make([]corev1.PersistentVolumeAccessMode, 0, len(accessModesStr))
		for _, accessModeStr := range accessModesStr {
			accessModes = append(accessModes, corev1.PersistentVolumeAccessMode(accessModeStr))
		}

		spec := corev1.PersistentVolumeClaimSpec{
			AccessModes: accessModes,
			DataSource: &corev1.TypedLocalObjectReference{
				APIGroup: ptr.To(vs.GroupVersionKind().Group),
				Kind:     vs.Kind,
				Name:     vs.Name,
			},
		}

		if storageClassName != "" {
			spec.StorageClassName = &storageClassName
			vi.Status.StorageClassName = storageClassName
		}

		if volumeMode != "" {
			spec.VolumeMode = ptr.To(corev1.PersistentVolumeMode(volumeMode))
		}

		if vs.Status != nil && vs.Status.RestoreSize != nil {
			spec.Resources = corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: *vs.Status.RestoreSize,
				},
			}
		}

		pvc = &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:      namespacedName.Name,
				Namespace: namespacedName.Namespace,
				OwnerReferences: []metav1.OwnerReference{
					service.MakeOwnerReference(vi),
				},
			},
			Spec: spec,
		}

		err = ds.diskService.CreatePersistentVolumeClaim(ctx, pvc)
		if err != nil {
			setPhaseConditionToFailed(cb, &vi.Status.Phase, err)
			return reconcile.Result{}, err
		}

		vi.Status.Phase = virtv2.ImageProvisioning
		cb.
			Status(metav1.ConditionFalse).
			Reason(vicondition.Provisioning).
			Message("PVC has created: waiting to be Bound.")

		vi.Status.Progress = "0%"
		vi.Status.SourceUID = pointer.GetPointer(vs.UID)
		vi.Status.Target.PersistentVolumeClaim = pvc.Name

		return reconcile.Result{Requeue: true}, err
	case pvc.Status.Phase == corev1.ClaimPending:
		isWFFC := storageClass != nil && storageClass.VolumeBindingMode != nil && *storageClass.VolumeBindingMode == storev1.VolumeBindingWaitForFirstConsumer

		if !isWFFC {
			return reconcile.Result{Requeue: true}, nil
		}

		ownerRef := metav1.NewControllerRef(vi, vi.GroupVersionKind())
		err = ds.bounderService.Start(ctx, ownerRef, supgen, pvc)
		switch {
		case err == nil:
			// OK.
		case common.ErrQuotaExceeded(err):
			ds.recorder.Event(vi, corev1.EventTypeWarning, virtv2.ReasonDataSourceQuotaExceeded, "DataSource quota exceed")
			return setQuotaExceededPhaseCondition(cb, &vi.Status.Phase, err, vi.CreationTimestamp), nil
		default:
			setPhaseConditionToFailed(cb, &vi.Status.Phase, fmt.Errorf("unexpected error: %w", err))
			return reconcile.Result{}, err
		}

		cb.
			Status(metav1.ConditionFalse).
			Reason(vicondition.Provisioning).
			Message("Bounder pod has created: waiting to be Bound.")

		return reconcile.Result{Requeue: true}, err
	case pvc.Status.Phase == corev1.ClaimBound:
		ds.recorder.Event(
			vi,
			corev1.EventTypeNormal,
			virtv2.ReasonDataSourceSyncCompleted,
			"The ObjectRef DataSource import has completed",
		)

		vi.Status.Phase = virtv2.ImageReady
		cb.
			Status(metav1.ConditionTrue).
			Reason(vicondition.Ready).
			Message("")

		q, err := resource.ParseQuantity(vs.Status.RestoreSize.String())
		if err != nil {
			return reconcile.Result{}, err
		}

		intQ, ok := q.AsInt64()
		if !ok {
			return reconcile.Result{}, errors.New("fail to convert quantity to int64")
		}

		vi.Status.Size = virtv2.ImageStatusSize{
			Stored:        vs.Status.RestoreSize.String(),
			StoredBytes:   strconv.FormatInt(intQ, 10),
			Unpacked:      vs.Status.RestoreSize.String(),
			UnpackedBytes: strconv.FormatInt(intQ, 10),
		}

		vi.Status.Progress = "100%"
	default:
		vi.Status.Phase = virtv2.ImageProvisioning
		cb.
			Status(metav1.ConditionFalse).
			Reason(vicondition.Provisioning).
			Message("Import is in the process of provisioning to PVC.")

		return reconcile.Result{}, nil
	}

	return reconcile.Result{Requeue: true}, nil
}

func (ds ObjectRefVirtualDiskSnapshot) CleanUpSupplements(ctx context.Context, vi *virtv2.VirtualImage) (reconcile.Result, error) {
	supgen := supplements.NewGenerator(annotations.VIShortName, vi.Name, vi.Namespace, vi.UID)

	importerRequeue, err := ds.importerService.CleanUpSupplements(ctx, supgen)
	if err != nil {
		return reconcile.Result{}, err
	}

	diskRequeue, err := ds.diskService.CleanUpSupplements(ctx, supgen)
	if err != nil {
		return reconcile.Result{}, err
	}

	bounderRequeue, err := ds.bounderService.CleanUpSupplements(ctx, supgen)
	if err != nil {
		return reconcile.Result{}, err
	}

	if vi.Spec.Storage == virtv2.StorageContainerRegistry {
		pvcCleanupRequeue, err := ds.diskService.CleanUp(ctx, supgen)
		if err != nil {
			return reconcile.Result{}, err
		}

		return reconcile.Result{Requeue: pvcCleanupRequeue}, nil
	}

	return reconcile.Result{Requeue: importerRequeue || diskRequeue || bounderRequeue}, nil
}

func (ds ObjectRefVirtualDiskSnapshot) CleanUp(ctx context.Context, vi *virtv2.VirtualImage) (bool, error) {
	supgen := supplements.NewGenerator(annotations.VIShortName, vi.Name, vi.Namespace, vi.UID)

	importerRequeue, err := ds.importerService.CleanUp(ctx, supgen)
	if err != nil {
		return false, err
	}

	diskRequeue, err := ds.diskService.CleanUp(ctx, supgen)
	if err != nil {
		return false, err
	}

	bounderRequeue, err := ds.bounderService.CleanUpSupplements(ctx, supgen)
	if err != nil {
		return false, err
	}

	return importerRequeue || diskRequeue || bounderRequeue, nil
}

func (ds ObjectRefVirtualDiskSnapshot) getEnvSettings(vi *virtv2.VirtualImage, sup *supplements.Generator) *importer.Settings {
	var settings importer.Settings
	importer.ApplyBlockDeviceSourceSettings(&settings)
	importer.ApplyDVCRDestinationSettings(
		&settings,
		ds.dvcrSettings,
		sup,
		ds.dvcrSettings.RegistryImageForVI(vi),
	)

	return &settings
}

func (ds ObjectRefVirtualDiskSnapshot) Validate(ctx context.Context, vi *virtv2.VirtualImage) error {
	if vi.Spec.DataSource.ObjectRef == nil || vi.Spec.DataSource.ObjectRef.Kind != virtv2.VirtualImageObjectRefKindVirtualDiskSnapshot {
		return fmt.Errorf("not a %s data source", virtv2.VirtualImageObjectRefKindVirtualDiskSnapshot)
	}

	vdSnapshot, err := ds.diskService.GetVirtualDiskSnapshot(ctx, vi.Spec.DataSource.ObjectRef.Name, vi.Namespace)
	if err != nil {
		return err
	}

	if vdSnapshot == nil || vdSnapshot.Status.Phase != virtv2.VirtualDiskSnapshotPhaseReady {
		return NewVirtualDiskSnapshotNotReadyError(vi.Spec.DataSource.ObjectRef.Name)
	}

	volumeSnapshot, err := ds.diskService.GetVolumeSnapshot(ctx, vdSnapshot.Status.VolumeSnapshotName, vdSnapshot.Namespace)
	if err != nil {
		return err
	}

	if volumeSnapshot == nil || !*volumeSnapshot.Status.ReadyToUse {
		return NewVirtualDiskSnapshotNotReadyError(vi.Spec.DataSource.ObjectRef.Name)
	}

	return nil
}
