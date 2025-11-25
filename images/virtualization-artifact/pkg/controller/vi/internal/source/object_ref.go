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
	"k8s.io/apimachinery/pkg/types"
	cdiv1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/common"
	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/datasource"
	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	podutil "github.com/deckhouse/virtualization-controller/pkg/common/pod"
	"github.com/deckhouse/virtualization-controller/pkg/common/pointer"
	"github.com/deckhouse/virtualization-controller/pkg/controller"
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

const objectRefDataSource = "objectref"

type ObjectRefDataSource struct {
	statService     Stat
	importerService Importer
	bounderService  Bounder
	dvcrSettings    *dvcr.Settings
	client          client.Client
	diskService     *service.DiskService
	recorder        eventrecord.EventRecorderLogger

	viObjectRefOnPvc    *ObjectRefDataVirtualImageOnPVC
	vdSyncer            *ObjectRefVirtualDisk
	vdSnapshotCRSyncer  *ObjectRefVirtualDiskSnapshotCR
	vdSnapshotPVCSyncer *ObjectRefVirtualDiskSnapshotPVC
}

func NewObjectRefDataSource(
	recorder eventrecord.EventRecorderLogger,
	statService Stat,
	importerService Importer,
	bounderService *service.BounderPodService,
	dvcrSettings *dvcr.Settings,
	client client.Client,
	diskService *service.DiskService,
) *ObjectRefDataSource {
	return &ObjectRefDataSource{
		statService:         statService,
		importerService:     importerService,
		bounderService:      bounderService,
		dvcrSettings:        dvcrSettings,
		client:              client,
		diskService:         diskService,
		recorder:            recorder,
		viObjectRefOnPvc:    NewObjectRefDataVirtualImageOnPVC(recorder, statService, importerService, dvcrSettings, client, diskService),
		vdSyncer:            NewObjectRefVirtualDisk(recorder, importerService, client, diskService, dvcrSettings, statService),
		vdSnapshotCRSyncer:  NewObjectRefVirtualDiskSnapshotCR(importerService, statService, diskService, client, dvcrSettings, recorder),
		vdSnapshotPVCSyncer: NewObjectRefVirtualDiskSnapshotPVC(importerService, statService, bounderService, diskService, client, dvcrSettings, recorder),
	}
}

func (ds ObjectRefDataSource) StoreToPVC(ctx context.Context, vi *v1alpha2.VirtualImage) (reconcile.Result, error) {
	if vi.Spec.DataSource.ObjectRef.Kind == v1alpha2.VirtualDiskSnapshotKind {
		return ds.vdSnapshotPVCSyncer.Sync(ctx, vi)
	}

	log, ctx := logger.GetDataSourceContext(ctx, objectRefDataSource)

	condition, _ := conditions.GetCondition(vicondition.ReadyType, vi.Status.Conditions)
	cb := conditions.NewConditionBuilder(vicondition.ReadyType).Generation(vi.Generation)
	defer func() { conditions.SetCondition(cb, &vi.Status.Conditions) }()

	switch vi.Spec.DataSource.ObjectRef.Kind {
	case v1alpha2.VirtualImageKind:
		viKey := types.NamespacedName{Name: vi.Spec.DataSource.ObjectRef.Name, Namespace: vi.Namespace}
		viRef, err := object.FetchObject(ctx, viKey, ds.client, &v1alpha2.VirtualImage{})
		if err != nil {
			return reconcile.Result{}, fmt.Errorf("unable to get VI %s: %w", viKey, err)
		}

		if viRef == nil {
			return reconcile.Result{}, fmt.Errorf("VI object ref %s is nil", viKey)
		}

		if viRef.Spec.Storage == v1alpha2.StorageKubernetes || viRef.Spec.Storage == v1alpha2.StoragePersistentVolumeClaim {
			return ds.viObjectRefOnPvc.StoreToPVC(ctx, vi, viRef, cb)
		}
	case v1alpha2.VirtualDiskKind:
		vdKey := types.NamespacedName{Name: vi.Spec.DataSource.ObjectRef.Name, Namespace: vi.Namespace}
		vd, err := object.FetchObject(ctx, vdKey, ds.client, &v1alpha2.VirtualDisk{})
		if err != nil {
			return reconcile.Result{}, fmt.Errorf("unable to get VD %s: %w", vdKey, err)
		}

		if vd == nil {
			return reconcile.Result{}, fmt.Errorf("VD object ref %s is nil", vdKey)
		}

		return ds.vdSyncer.StoreToPVC(ctx, vi, vd, cb)
	}

	supgen := supplements.NewGenerator(annotations.VIShortName, vi.Name, vi.Namespace, vi.UID)
	dv, err := ds.diskService.GetDataVolume(ctx, supgen)
	if err != nil {
		return reconcile.Result{}, err
	}
	pvc, err := ds.diskService.GetPersistentVolumeClaim(ctx, supgen)
	if err != nil {
		return reconcile.Result{}, err
	}

	var dvQuotaNotExceededCondition *cdiv1.DataVolumeCondition
	var dvRunningCondition *cdiv1.DataVolumeCondition
	if dv != nil {
		dvQuotaNotExceededCondition = service.GetDataVolumeCondition(DVQoutaNotExceededConditionType, dv.Status.Conditions)
		dvRunningCondition = service.GetDataVolumeCondition(DVRunningConditionType, dv.Status.Conditions)
	}

	switch {
	case IsImageProvisioningFinished(condition):
		log.Info("Disk provisioning finished: clean up")

		setPhaseConditionForFinishedImage(pvc, cb, &vi.Status.Phase, supgen)

		// Protect Ready Disk and underlying PVC.
		err = ds.diskService.Protect(ctx, supgen, vi, nil, pvc)
		if err != nil {
			return reconcile.Result{}, err
		}

		err = ds.diskService.Unprotect(ctx, supgen, dv)
		if err != nil {
			return reconcile.Result{}, err
		}

		return CleanUpSupplements(ctx, vi, ds)
	case object.AnyTerminating(dv, pvc):
		log.Info("Waiting for supplements to be terminated")
	case dv == nil:
		ds.recorder.Event(
			vi,
			corev1.EventTypeNormal,
			v1alpha2.ReasonDataSourceSyncStarted,
			"The ObjectRef DataSource import has started",
		)

		var dvcrDataSource controller.DVCRDataSource
		dvcrDataSource, err = controller.NewDVCRDataSourcesForVMI(ctx, vi.Spec.DataSource, vi, ds.client)
		if err != nil {
			return reconcile.Result{}, err
		}

		if !dvcrDataSource.IsReady() {
			cb.
				Status(metav1.ConditionFalse).
				Reason(vicondition.ProvisioningFailed).
				Message("Failed to get stats from non-ready datasource: waiting for the DataSource to be ready.")
			return reconcile.Result{}, nil
		}

		vi.Status.Progress = "0%"
		vi.Status.SourceUID = pointer.GetPointer(dvcrDataSource.GetUID())

		var diskSize resource.Quantity
		diskSize, err = ds.getPVCSize(dvcrDataSource)
		if err != nil {
			setPhaseConditionToFailed(cb, &vi.Status.Phase, err)

			if errors.Is(err, service.ErrInsufficientPVCSize) {
				return reconcile.Result{}, nil
			}

			return reconcile.Result{}, err
		}

		var source *cdiv1.DataVolumeSource
		source, err = ds.getSource(supgen, dvcrDataSource)
		if err != nil {
			return reconcile.Result{}, err
		}

		var sc *storagev1.StorageClass
		sc, err = ds.diskService.GetStorageClass(ctx, vi.Status.StorageClassName)
		if err != nil {
			return reconcile.Result{}, err
		}
		err = ds.diskService.StartImmediate(ctx, diskSize, sc, source, vi, supgen)
		if updated, err := setPhaseConditionFromStorageError(err, vi, cb); err != nil || updated {
			return reconcile.Result{}, err
		}

		vi.Status.Phase = v1alpha2.ImageProvisioning
		cb.
			Status(metav1.ConditionFalse).
			Reason(vicondition.Provisioning).
			Message("PVC Provisioner not found: create the new one.")

		return reconcile.Result{RequeueAfter: time.Second}, nil
	case dvQuotaNotExceededCondition != nil && dvQuotaNotExceededCondition.Status == corev1.ConditionFalse:
		vi.Status.Phase = v1alpha2.ImagePending
		cb.
			Status(metav1.ConditionFalse).
			Reason(vicondition.QuotaExceeded).
			Message(dvQuotaNotExceededCondition.Message)
		return reconcile.Result{}, nil
	case dvRunningCondition != nil && dvRunningCondition.Status != corev1.ConditionTrue && dvRunningCondition.Reason == DVImagePullFailedReason:
		vi.Status.Phase = v1alpha2.ImagePending
		cb.
			Status(metav1.ConditionFalse).
			Reason(vicondition.ImagePullFailed).
			Message(dvRunningCondition.Message)
		ds.recorder.Event(vi, corev1.EventTypeWarning, vicondition.ImagePullFailed.String(), dvRunningCondition.Message)
		return reconcile.Result{}, nil
	case pvc == nil:
		vi.Status.Phase = v1alpha2.ImageProvisioning
		cb.
			Status(metav1.ConditionFalse).
			Reason(vicondition.Provisioning).
			Message("PVC not found: waiting for creation.")
		return reconcile.Result{RequeueAfter: time.Second}, nil
	case ds.diskService.IsImportDone(dv, pvc):
		log.Info("Import has completed", "dvProgress", dv.Status.Progress, "dvPhase", dv.Status.Phase, "pvcPhase", pvc.Status.Phase)
		ds.recorder.Event(
			vi,
			corev1.EventTypeNormal,
			v1alpha2.ReasonDataSourceSyncCompleted,
			"The ObjectRef DataSource import has completed",
		)

		vi.Status.Phase = v1alpha2.ImageReady
		cb.
			Status(metav1.ConditionTrue).
			Reason(vicondition.Ready).
			Message("")

		var dvcrDataSource controller.DVCRDataSource
		dvcrDataSource, err = controller.NewDVCRDataSourcesForVMI(ctx, vi.Spec.DataSource, vi, ds.client)
		if err != nil {
			return reconcile.Result{}, err
		}

		vi.Status.Size = dvcrDataSource.GetSize()
		vi.Status.CDROM = dvcrDataSource.IsCDROM()
		vi.Status.Format = dvcrDataSource.GetFormat()
		vi.Status.Progress = "100%"
		vi.Status.Target.PersistentVolumeClaim = dv.Status.ClaimName
	default:
		log.Info("Provisioning to PVC is in progress", "dvProgress", dv.Status.Progress, "dvPhase", dv.Status.Phase, "pvcPhase", pvc.Status.Phase)

		vi.Status.Progress = ds.diskService.GetProgress(dv, vi.Status.Progress, service.NewScaleOption(0, 100))
		vi.Status.Target.PersistentVolumeClaim = dv.Status.ClaimName

		err = ds.diskService.Protect(ctx, supgen, vi, dv, pvc)
		if err != nil {
			return reconcile.Result{}, err
		}

		err = setPhaseConditionForPVCProvisioningImage(ctx, dv, vi, pvc, cb, ds.diskService)
		if err != nil {
			return reconcile.Result{}, err
		}

		return reconcile.Result{}, nil
	}

	return reconcile.Result{RequeueAfter: time.Second}, nil
}

func (ds ObjectRefDataSource) StoreToDVCR(ctx context.Context, vi *v1alpha2.VirtualImage) (reconcile.Result, error) {
	if vi.Spec.DataSource.ObjectRef.Kind == v1alpha2.VirtualDiskSnapshotKind {
		return ds.vdSnapshotCRSyncer.Sync(ctx, vi)
	}

	log, ctx := logger.GetDataSourceContext(ctx, "objectref")

	condition, _ := conditions.GetCondition(vicondition.ReadyType, vi.Status.Conditions)
	cb := conditions.NewConditionBuilder(vicondition.ReadyType).Generation(vi.Generation)
	defer func() { conditions.SetCondition(cb, &vi.Status.Conditions) }()

	switch vi.Spec.DataSource.ObjectRef.Kind {
	case v1alpha2.VirtualImageKind:
		viKey := types.NamespacedName{Name: vi.Spec.DataSource.ObjectRef.Name, Namespace: vi.Namespace}
		viRef, err := object.FetchObject(ctx, viKey, ds.client, &v1alpha2.VirtualImage{})
		if err != nil {
			return reconcile.Result{}, fmt.Errorf("unable to get VI %s: %w", viKey, err)
		}

		if viRef == nil {
			return reconcile.Result{}, fmt.Errorf("VI object ref source %s is nil", vi.Spec.DataSource.ObjectRef.Name)
		}

		if viRef.Spec.Storage == v1alpha2.StorageKubernetes || viRef.Spec.Storage == v1alpha2.StoragePersistentVolumeClaim {
			return ds.viObjectRefOnPvc.StoreToDVCR(ctx, vi, viRef, cb)
		}
	case v1alpha2.VirtualDiskKind:
		viKey := types.NamespacedName{Name: vi.Spec.DataSource.ObjectRef.Name, Namespace: vi.Namespace}
		vd, err := object.FetchObject(ctx, viKey, ds.client, &v1alpha2.VirtualDisk{})
		if err != nil {
			return reconcile.Result{}, fmt.Errorf("unable to get VD %s: %w", viKey, err)
		}

		if vd == nil {
			return reconcile.Result{}, fmt.Errorf("VD object ref %s is nil", viKey)
		}

		return ds.vdSyncer.StoreToDVCR(ctx, vi, vd, cb)
	}

	supgen := supplements.NewGenerator(annotations.VIShortName, vi.Name, vi.Namespace, vi.UID)
	pod, err := ds.importerService.GetPod(ctx, supgen)
	if err != nil {
		return reconcile.Result{}, err
	}

	switch {
	case IsImageProvisioningFinished(condition):
		log.Info("Virtual image provisioning finished: clean up")

		cb.
			Status(metav1.ConditionTrue).
			Reason(vicondition.Ready).
			Message("")

		vi.Status.Phase = v1alpha2.ImageReady

		err = ds.importerService.Unprotect(ctx, pod, supgen)
		if err != nil {
			return reconcile.Result{}, err
		}

		return CleanUpSupplements(ctx, vi, ds)
	case object.IsTerminating(pod):
		vi.Status.Phase = v1alpha2.ImagePending

		log.Info("Cleaning up...")
	case pod == nil:
		vi.Status.Progress = "0%"

		var dvcrDataSource controller.DVCRDataSource
		dvcrDataSource, err = controller.NewDVCRDataSourcesForVMI(ctx, vi.Spec.DataSource, vi, ds.client)
		if err != nil {
			return reconcile.Result{}, err
		}

		vi.Status.SourceUID = pointer.GetPointer(dvcrDataSource.GetUID())

		var envSettings *importer.Settings
		envSettings, err = ds.getEnvSettings(vi, supgen, dvcrDataSource)
		if err != nil {
			return reconcile.Result{}, err
		}

		err = ds.importerService.Start(ctx, envSettings, vi, supgen, datasource.NewCABundleForVMI(vi.GetNamespace(), vi.Spec.DataSource), service.WithSystemNodeToleration())
		switch {
		case err == nil:
			// OK.
		case common.ErrQuotaExceeded(err):
			ds.recorder.Event(vi, corev1.EventTypeWarning, v1alpha2.ReasonDataSourceQuotaExceeded, "DataSource quota exceed")
			return setQuotaExceededPhaseCondition(cb, &vi.Status.Phase, err, vi.CreationTimestamp), nil
		default:
			setPhaseConditionToFailed(cb, &vi.Status.Phase, fmt.Errorf("unexpected error: %w", err))
			return reconcile.Result{}, err
		}

		vi.Status.Phase = v1alpha2.ImageProvisioning
		cb.
			Status(metav1.ConditionFalse).
			Reason(vicondition.Provisioning).
			Message("DVCR Provisioner not found: create the new one.")

		log.Info("Ready", "progress", vi.Status.Progress, "pod.phase", "nil")

		return reconcile.Result{RequeueAfter: time.Second}, nil
	case podutil.IsPodComplete(pod):
		err = ds.statService.CheckPod(pod)
		if err != nil {
			vi.Status.Phase = v1alpha2.ImageFailed

			switch {
			case errors.Is(err, service.ErrProvisioningFailed):
				ds.recorder.Event(vi, corev1.EventTypeWarning, v1alpha2.ReasonDataSourceDiskProvisioningFailed, "Disk provisioning failed")
				cb.
					Status(metav1.ConditionFalse).
					Reason(vicondition.ProvisioningFailed).
					Message(service.CapitalizeFirstLetter(err.Error() + "."))
				return reconcile.Result{}, nil
			default:
				return reconcile.Result{}, err
			}
		}
		var dvcrDataSource controller.DVCRDataSource
		dvcrDataSource, err = controller.NewDVCRDataSourcesForVMI(ctx, vi.Spec.DataSource, vi, ds.client)
		if err != nil {
			return reconcile.Result{}, err
		}

		if !dvcrDataSource.IsReady() {
			cb.
				Status(metav1.ConditionFalse).
				Reason(vicondition.ProvisioningFailed).
				Message("Failed to get stats from non-ready datasource: waiting for the DataSource to be ready.")
			return reconcile.Result{}, nil
		}

		cb.
			Status(metav1.ConditionTrue).
			Reason(vicondition.Ready).
			Message("")

		vi.Status.Phase = v1alpha2.ImageReady
		vi.Status.Size = dvcrDataSource.GetSize()
		vi.Status.CDROM = dvcrDataSource.IsCDROM()
		vi.Status.Format = dvcrDataSource.GetFormat()
		vi.Status.Progress = "100%"
		vi.Status.Target.RegistryURL = ds.statService.GetDVCRImageName(pod)

		log.Info("Ready", "progress", vi.Status.Progress, "pod.phase", pod.Status.Phase)
	default:
		err = ds.statService.CheckPod(pod)
		if err != nil {
			return reconcile.Result{}, setPhaseConditionFromPodError(cb, vi, err)
		}

		cb.
			Status(metav1.ConditionFalse).
			Reason(vicondition.Provisioning).
			Message("Import is in the process of provisioning to DVCR.")

		vi.Status.Phase = v1alpha2.ImageProvisioning
		vi.Status.Target.RegistryURL = ds.statService.GetDVCRImageName(pod)

		log.Info("Ready", "progress", vi.Status.Progress, "pod.phase", pod.Status.Phase)
	}

	return reconcile.Result{RequeueAfter: time.Second}, nil
}

func (ds ObjectRefDataSource) CleanUp(ctx context.Context, vi *v1alpha2.VirtualImage) (bool, error) {
	supgen := supplements.NewGenerator(annotations.VIShortName, vi.Name, vi.Namespace, vi.UID)

	importerRequeue, err := ds.importerService.CleanUp(ctx, supgen)
	if err != nil {
		return false, err
	}

	bounderRequeue, err := ds.bounderService.CleanUp(ctx, supgen)
	if err != nil {
		return false, err
	}

	diskRequeue, err := ds.diskService.CleanUp(ctx, supgen)
	if err != nil {
		return false, err
	}

	return importerRequeue || bounderRequeue || diskRequeue, nil
}

func (ds ObjectRefDataSource) Validate(ctx context.Context, vi *v1alpha2.VirtualImage) error {
	if vi.Spec.DataSource.ObjectRef == nil {
		return fmt.Errorf("nil object ref: %s", vi.Spec.DataSource.Type)
	}

	switch vi.Spec.DataSource.ObjectRef.Kind {
	case v1alpha2.VirtualImageObjectRefKindVirtualImage:
		viKey := types.NamespacedName{Name: vi.Spec.DataSource.ObjectRef.Name, Namespace: vi.Namespace}
		viRef, err := object.FetchObject(ctx, viKey, ds.client, &v1alpha2.VirtualImage{})
		if err != nil {
			return fmt.Errorf("unable to get VI %s: %w", viKey, err)
		}

		if viRef == nil {
			return NewImageNotReadyError(vi.Spec.DataSource.ObjectRef.Name)
		}

		if viRef.Spec.Storage == v1alpha2.StorageKubernetes || viRef.Spec.Storage == v1alpha2.StoragePersistentVolumeClaim {
			if viRef.Status.Phase != v1alpha2.ImageReady {
				return NewImageNotReadyError(vi.Spec.DataSource.ObjectRef.Name)
			}
			return nil
		}

		dvcrDataSource, err := controller.NewDVCRDataSourcesForVMI(ctx, vi.Spec.DataSource, vi, ds.client)
		if err != nil {
			return err
		}

		if dvcrDataSource.IsReady() {
			return nil
		}

		return NewImageNotReadyError(vi.Spec.DataSource.ObjectRef.Name)
	case v1alpha2.VirtualImageObjectRefKindClusterVirtualImage:
		dvcrDataSource, err := controller.NewDVCRDataSourcesForVMI(ctx, vi.Spec.DataSource, vi, ds.client)
		if err != nil {
			return err
		}

		if dvcrDataSource.IsReady() {
			return nil
		}

		return NewClusterImageNotReadyError(vi.Spec.DataSource.ObjectRef.Name)
	case v1alpha2.VirtualImageObjectRefKindVirtualDisk:
		return ds.vdSyncer.Validate(ctx, vi)
	case v1alpha2.VirtualImageObjectRefKindVirtualDiskSnapshot:
		switch vi.Spec.Storage {
		case v1alpha2.StorageKubernetes, v1alpha2.StoragePersistentVolumeClaim:
			return ds.vdSnapshotPVCSyncer.Validate(ctx, vi)
		case v1alpha2.StorageContainerRegistry:
			return ds.vdSnapshotCRSyncer.Validate(ctx, vi)
		}

		return fmt.Errorf("unexpected object ref kind: %s", vi.Spec.DataSource.ObjectRef.Kind)
	default:
		return fmt.Errorf("unexpected object ref kind: %s", vi.Spec.DataSource.ObjectRef.Kind)
	}
}

func (ds ObjectRefDataSource) getEnvSettings(vi *v1alpha2.VirtualImage, sup supplements.Generator, dvcrDataSource controller.DVCRDataSource) (*importer.Settings, error) {
	if !dvcrDataSource.IsReady() {
		return nil, errors.New("dvcr data source is not ready")
	}

	var settings importer.Settings
	importer.ApplyDVCRSourceSettings(&settings, dvcrDataSource.GetTarget())
	importer.ApplyDVCRDestinationSettings(
		&settings,
		ds.dvcrSettings,
		sup,
		ds.dvcrSettings.RegistryImageForVI(vi),
	)

	return &settings, nil
}

func (ds ObjectRefDataSource) CleanUpSupplements(ctx context.Context, vi *v1alpha2.VirtualImage) (reconcile.Result, error) {
	supgen := supplements.NewGenerator(annotations.VIShortName, vi.Name, vi.Namespace, vi.UID)

	importerRequeue, err := ds.importerService.CleanUpSupplements(ctx, supgen)
	if err != nil {
		return reconcile.Result{}, err
	}

	diskRequeue, err := ds.diskService.CleanUpSupplements(ctx, supgen)
	if err != nil {
		return reconcile.Result{}, err
	}

	if importerRequeue || diskRequeue {
		return reconcile.Result{RequeueAfter: time.Second}, nil
	} else {
		return reconcile.Result{}, nil
	}
}

func (ds ObjectRefDataSource) getPVCSize(dvcrDataSource controller.DVCRDataSource) (resource.Quantity, error) {
	if !dvcrDataSource.IsReady() {
		return resource.Quantity{}, errors.New("dvcr data source is not ready")
	}

	unpackedSize, err := resource.ParseQuantity(dvcrDataSource.GetSize().UnpackedBytes)
	if err != nil {
		return resource.Quantity{}, fmt.Errorf("failed to parse unpacked bytes %s: %w", dvcrDataSource.GetSize().UnpackedBytes, err)
	}

	if unpackedSize.IsZero() {
		return resource.Quantity{}, errors.New("got zero unpacked size from data source")
	}

	return service.GetValidatedPVCSize(&unpackedSize, unpackedSize)
}

func (ds ObjectRefDataSource) getSource(sup supplements.Generator, dvcrDataSource controller.DVCRDataSource) (*cdiv1.DataVolumeSource, error) {
	if !dvcrDataSource.IsReady() {
		return nil, errors.New("dvcr data source is not ready")
	}

	url := common.DockerRegistrySchemePrefix + dvcrDataSource.GetTarget()
	secretName := sup.DVCRAuthSecretForDV().Name
	certConfigMapName := sup.DVCRCABundleConfigMapForDV().Name

	return &cdiv1.DataVolumeSource{
		Registry: &cdiv1.DataVolumeSourceRegistry{
			URL:           &url,
			SecretRef:     &secretName,
			CertConfigMap: &certConfigMapName,
		},
	}, nil
}
