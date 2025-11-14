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
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
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
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vicondition"
)

type ObjectRefVirtualDiskSnapshot struct {
	importerService     Importer
	diskService         *service.DiskService
	statService         Stat
	dvcrSettings        *dvcr.Settings
	client              client.Client
	controllerNamespace string
	recorder            eventrecord.EventRecorderLogger
}

func NewObjectRefVirtualDiskSnapshot(
	recorder eventrecord.EventRecorderLogger,
	importerService Importer,
	diskService *service.DiskService,
	client client.Client,
	controllerNamespace string,
	dvcrSettings *dvcr.Settings,
	statService Stat,
) *ObjectRefVirtualDiskSnapshot {
	return &ObjectRefVirtualDiskSnapshot{
		importerService:     importerService,
		diskService:         diskService,
		client:              client,
		recorder:            recorder,
		controllerNamespace: controllerNamespace,
		statService:         statService,
		dvcrSettings:        dvcrSettings,
	}
}

func (ds ObjectRefVirtualDiskSnapshot) Sync(ctx context.Context, cvi *v1alpha2.ClusterVirtualImage, vdSnapshotRef *v1alpha2.VirtualDiskSnapshot, cb *conditions.ConditionBuilder) (reconcile.Result, error) {
	log, ctx := logger.GetDataSourceContext(ctx, "objectref")

	supgen := supplements.NewGenerator(annotations.CVIShortName, cvi.Name, vdSnapshotRef.Namespace, cvi.UID)
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

	condition, _ := conditions.GetCondition(vicondition.ReadyType, cvi.Status.Conditions)
	switch {
	case isDiskProvisioningFinished(condition):
		log.Info("Virtual image provisioning finished: clean up")

		cb.
			Status(metav1.ConditionTrue).
			Reason(vicondition.Ready).
			Message("")

		cvi.Status.Phase = v1alpha2.ImageReady

		err = ds.importerService.Unprotect(ctx, pod, supgen)
		if err != nil {
			return reconcile.Result{}, err
		}

		return ds.CleanUpSupplements(ctx, cvi)
	case object.AnyTerminating(pod, pvc):
		cvi.Status.Phase = v1alpha2.ImagePending

		cb.
			Status(metav1.ConditionTrue).
			Reason(vicondition.Ready).
			Message("")

		log.Info("Cleaning up...")
	case pvc == nil:
		ds.recorder.Event(
			cvi,
			corev1.EventTypeNormal,
			v1alpha2.ReasonDataSourceSyncStarted,
			"The ObjectRef DataSource import has started",
		)

		pvcKey := supplements.NewGenerator(annotations.CVIShortName, cvi.Name, cvi.Spec.DataSource.ObjectRef.Namespace, cvi.UID).PersistentVolumeClaim()

		storageClassName := vs.Annotations[annotations.AnnStorageClassName]
		if storageClassName == "" {
			storageClassName = vs.Annotations[annotations.AnnStorageClassNameDeprecated]
		}
		volumeMode := vs.Annotations[annotations.AnnVolumeMode]
		if volumeMode == "" {
			volumeMode = vs.Annotations[annotations.AnnVolumeModeDeprecated]
		}
		accessModesRaw := vs.Annotations[annotations.AnnAccessModes]
		if accessModesRaw == "" {
			accessModesRaw = vs.Annotations[annotations.AnnAccessModesDeprecated]
		}

		accessModesStr := strings.Split(accessModesRaw, ",")
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
				Name:      pvcKey.Name,
				Namespace: pvcKey.Namespace,
				OwnerReferences: []metav1.OwnerReference{
					service.MakeOwnerReference(cvi),
				},
			},
			Spec: spec,
		}

		err = ds.diskService.CreatePersistentVolumeClaim(ctx, pvc)
		if err != nil {
			setPhaseConditionToFailed(cb, &cvi.Status.Phase, err)
			return reconcile.Result{}, err
		}

		cvi.Status.Phase = v1alpha2.ImageProvisioning
		cb.
			Status(metav1.ConditionFalse).
			Reason(vicondition.Provisioning).
			Message("PVC has created: waiting to be Bound.")

		cvi.Status.Progress = "0%"
		cvi.Status.SourceUID = pointer.GetPointer(vs.UID)

		return reconcile.Result{RequeueAfter: time.Second}, err
	case pod == nil:
		cvi.Status.Progress = ds.statService.GetProgress(cvi.GetUID(), pod, cvi.Status.Progress)
		cvi.Status.Target.RegistryURL = ds.statService.GetDVCRImageName(pod)

		envSettings := ds.getEnvSettings(cvi, supgen)

		ownerRef := metav1.NewControllerRef(cvi, cvi.GroupVersionKind())
		podSettings := ds.importerService.GetPodSettingsWithPVC(ownerRef, supgen, pvc.Name, pvc.Namespace)
		err = ds.importerService.StartWithPodSetting(ctx, envSettings, supgen, datasource.NewCABundleForCVMI(cvi.Spec.DataSource), podSettings)
		switch {
		case err == nil:
			// OK.
		case common.ErrQuotaExceeded(err):
			ds.recorder.Event(cvi, corev1.EventTypeWarning, v1alpha2.ReasonDataSourceQuotaExceeded, "DataSource quota exceed")
			return setQuotaExceededPhaseCondition(cb, &cvi.Status.Phase, err, cvi.CreationTimestamp), nil
		default:
			setPhaseConditionToFailed(cb, &cvi.Status.Phase, fmt.Errorf("unexpected error: %w", err))
			return reconcile.Result{}, err
		}

		cvi.Status.Phase = v1alpha2.ImageProvisioning
		cb.
			Status(metav1.ConditionFalse).
			Reason(vicondition.Provisioning).
			Message("DVCR Provisioner not found: create the new one.")

		log.Info("Create importer pod...", "progress", cvi.Status.Progress, "pod.phase", "nil")

		return reconcile.Result{RequeueAfter: time.Second}, nil
	case podutil.IsPodComplete(pod):
		err = ds.statService.CheckPod(pod)
		if err != nil {
			cvi.Status.Phase = v1alpha2.ImageFailed

			switch {
			case errors.Is(err, service.ErrProvisioningFailed):
				ds.recorder.Event(cvi, corev1.EventTypeWarning, v1alpha2.ReasonDataSourceDiskProvisioningFailed, "Disk provisioning failed")
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

		cvi.Status.Phase = v1alpha2.ImageReady
		cvi.Status.Size = ds.statService.GetSize(pod)
		cvi.Status.CDROM = ds.statService.GetCDROM(pod)
		cvi.Status.Format = ds.statService.GetFormat(pod)
		cvi.Status.Progress = "100%"
		cvi.Status.Target.RegistryURL = ds.statService.GetDVCRImageName(pod)

		log.Info("Ready", "progress", cvi.Status.Progress, "pod.phase", pod.Status.Phase)
	default:
		err = ds.statService.CheckPod(pod)
		if err != nil {
			cvi.Status.Phase = v1alpha2.ImageFailed

			switch {
			case errors.Is(err, service.ErrNotInitialized), errors.Is(err, service.ErrNotScheduled):
				if strings.Contains(err.Error(), "pod has unbound immediate PersistentVolumeClaims") {
					cvi.Status.Phase = v1alpha2.ImageProvisioning
					cb.
						Status(metav1.ConditionFalse).
						Reason(vicondition.Provisioning).
						Message("Waiting for PVC to be bound")

					return reconcile.Result{RequeueAfter: time.Second}, nil
				}

				cb.
					Status(metav1.ConditionFalse).
					Reason(vicondition.ProvisioningNotStarted).
					Message(service.CapitalizeFirstLetter(err.Error() + "."))
				return reconcile.Result{}, nil
			case errors.Is(err, service.ErrProvisioningFailed):
				ds.recorder.Event(cvi, corev1.EventTypeWarning, v1alpha2.ReasonDataSourceDiskProvisioningFailed, "Disk provisioning failed")
				cb.
					Status(metav1.ConditionFalse).
					Reason(vicondition.ProvisioningFailed).
					Message(service.CapitalizeFirstLetter(err.Error() + "."))
				return reconcile.Result{}, nil
			default:
				return reconcile.Result{}, err
			}
		}

		err = ds.importerService.Protect(ctx, pod, supgen)
		if err != nil {
			return reconcile.Result{}, err
		}

		cb.
			Status(metav1.ConditionFalse).
			Reason(vicondition.Provisioning).
			Message("Import is in the process of provisioning to DVCR.")

		cvi.Status.Phase = v1alpha2.ImageProvisioning
		cvi.Status.Progress = ds.statService.GetProgress(cvi.GetUID(), pod, cvi.Status.Progress)
		cvi.Status.Target.RegistryURL = ds.statService.GetDVCRImageName(pod)

		log.Info("Provisioning...", "progress", cvi.Status.Progress, "pod.phase", pod.Status.Phase)
	}

	return reconcile.Result{RequeueAfter: time.Second}, nil
}

func (ds ObjectRefVirtualDiskSnapshot) CleanUpSupplements(ctx context.Context, cvi *v1alpha2.ClusterVirtualImage) (reconcile.Result, error) {
	supgen := supplements.NewGenerator(annotations.CVIShortName, cvi.Name, cvi.Spec.DataSource.ObjectRef.Namespace, cvi.UID)

	importerRequeue, err := ds.importerService.CleanUpSupplements(ctx, supgen)
	if err != nil {
		return reconcile.Result{}, err
	}

	diskRequeue, err := ds.diskService.CleanUpSupplements(ctx, supgen)
	if err != nil {
		return reconcile.Result{}, err
	}

	pvcCleanupRequeue, err := ds.diskService.CleanUp(ctx, supgen)
	if err != nil {
		return reconcile.Result{}, err
	}

	if importerRequeue || diskRequeue || pvcCleanupRequeue {
		return reconcile.Result{RequeueAfter: time.Second}, nil
	} else {
		return reconcile.Result{}, nil
	}
}

func (ds ObjectRefVirtualDiskSnapshot) CleanUp(ctx context.Context, cvi *v1alpha2.ClusterVirtualImage) (bool, error) {
	supgen := supplements.NewGenerator(annotations.CVIShortName, cvi.Name, cvi.Spec.DataSource.ObjectRef.Namespace, cvi.UID)

	importerRequeue, err := ds.importerService.CleanUp(ctx, supgen)
	if err != nil {
		return false, err
	}

	diskRequeue, err := ds.diskService.CleanUp(ctx, supgen)
	if err != nil {
		return false, err
	}

	return importerRequeue || diskRequeue, nil
}

func (ds ObjectRefVirtualDiskSnapshot) getEnvSettings(cvi *v1alpha2.ClusterVirtualImage, sup supplements.Generator) *importer.Settings {
	var settings importer.Settings
	importer.ApplyBlockDeviceSourceSettings(&settings)
	importer.ApplyDVCRDestinationSettings(
		&settings,
		ds.dvcrSettings,
		sup,
		ds.dvcrSettings.RegistryImageForVI(cvi),
	)

	return &settings
}

func (ds ObjectRefVirtualDiskSnapshot) Validate(ctx context.Context, cvi *v1alpha2.ClusterVirtualImage) error {
	if cvi.Spec.DataSource.ObjectRef == nil || cvi.Spec.DataSource.ObjectRef.Kind != v1alpha2.ClusterVirtualImageObjectRefKindVirtualDiskSnapshot {
		return fmt.Errorf("not a %s data source", v1alpha2.ClusterVirtualImageObjectRefKindVirtualDiskSnapshot)
	}

	vdSnapshot, err := ds.diskService.GetVirtualDiskSnapshot(ctx, cvi.Spec.DataSource.ObjectRef.Name, cvi.Spec.DataSource.ObjectRef.Namespace)
	if err != nil {
		return err
	}

	if vdSnapshot == nil || vdSnapshot.Status.Phase != v1alpha2.VirtualDiskSnapshotPhaseReady {
		return NewVirtualDiskSnapshotNotReadyError(cvi.Spec.DataSource.ObjectRef.Name)
	}

	volumeSnapshot, err := ds.diskService.GetVolumeSnapshot(ctx, vdSnapshot.Status.VolumeSnapshotName, vdSnapshot.Namespace)
	if err != nil {
		return err
	}

	if volumeSnapshot == nil || !*volumeSnapshot.Status.ReadyToUse {
		return NewVirtualDiskSnapshotNotReadyError(cvi.Spec.DataSource.ObjectRef.Name)
	}

	return nil
}
