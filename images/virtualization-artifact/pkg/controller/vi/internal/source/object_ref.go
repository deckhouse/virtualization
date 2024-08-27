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

	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	cdiv1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	common "github.com/deckhouse/virtualization-controller/pkg/common"
	"github.com/deckhouse/virtualization-controller/pkg/common/datasource"
	"github.com/deckhouse/virtualization-controller/pkg/controller"
	cc "github.com/deckhouse/virtualization-controller/pkg/controller/common"
	"github.com/deckhouse/virtualization-controller/pkg/controller/importer"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/supplements"
	"github.com/deckhouse/virtualization-controller/pkg/dvcr"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	"github.com/deckhouse/virtualization-controller/pkg/sdk/framework/helper"
	"github.com/deckhouse/virtualization-controller/pkg/util"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdcondition"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vicondition"
)

const objectRefDataSource = "objectref"

type ObjectRefDataSource struct {
	statService     Stat
	importerService Importer
	dvcrSettings    *dvcr.Settings
	client          client.Client
	diskService     *service.DiskService
	cloneService    *service.PVCCloneService
}

func NewObjectRefDataSource(
	statService Stat,
	importerService Importer,
	dvcrSettings *dvcr.Settings,
	client client.Client,
	diskService *service.DiskService,
	cloneService *service.PVCCloneService,
) *ObjectRefDataSource {
	return &ObjectRefDataSource{
		statService:     statService,
		importerService: importerService,
		dvcrSettings:    dvcrSettings,
		client:          client,
		diskService:     diskService,
		cloneService:    cloneService,
	}
}

func (ds ObjectRefDataSource) SyncDVCRFromPVC(ctx context.Context, vi *virtv2.VirtualImage, viRef *virtv2.VirtualImage) (bool, error) {
	log, ctx := logger.GetDataSourceContext(ctx, "objectref")

	condition, _ := service.GetCondition(vicondition.ReadyType, vi.Status.Conditions)
	defer func() { service.SetCondition(condition, &vi.Status.Conditions) }()

	supgen := supplements.NewGenerator(cc.VIShortName, vi.Name, vi.Namespace, vi.UID)
	pod, err := ds.importerService.GetPod(ctx, supgen)
	if err != nil {
		return false, err
	}

	refSupgen := supplements.NewGenerator(cc.VIShortName, viRef.Name, viRef.Namespace, viRef.UID)

	refPvc, err := ds.cloneService.GetPersistentVolumeClaim(ctx, refSupgen)
	if err != nil {
		return false, err
	}

	switch {
	case isDiskProvisioningFinished(condition):
		log.Info("Virtual image provisioning finished: clean up")

		condition.Status = metav1.ConditionTrue
		condition.Reason = vicondition.Ready
		condition.Message = ""

		vi.Status.Phase = virtv2.ImageReady

		err = ds.importerService.Unprotect(ctx, pod)
		if err != nil {
			return false, err
		}

		return CleanUp(ctx, vi, ds)
	case cc.IsTerminating(pod):
		vi.Status.Phase = virtv2.ImagePending

		log.Info("Cleaning up...")
	case pod == nil:
		var dvcrDataSource controller.DVCRDataSource
		dvcrDataSource, err = controller.NewDVCRDataSourcesForVMI(ctx, vi.Spec.DataSource, vi, ds.client)
		if err != nil {
			return false, err
		}

		var envSettings *importer.Settings
		envSettings, err = ds.getEnvSettings(vi, supgen, dvcrDataSource)
		if err != nil {
			return false, err
		}

		err = ds.importerService.Start(ctx, envSettings, vi, supgen, datasource.NewCABundleForVMI(vi.Spec.DataSource), refPvc.Name)
		var requeue bool
		requeue, err = setPhaseConditionForImporterStart(&condition, &vi.Status.Phase, err)
		if err != nil {
			return false, err
		}

		vi.Status.Target.RegistryURL = ds.dvcrSettings.RegistryImageForVMI(vi.Name, vi.Namespace)
		vi.Status.SourceUID = util.GetPointer(dvcrDataSource.GetUID())

		log.Info("Ready", "progress", vi.Status.Progress, "pod.phase", "nil")

		return requeue, nil
	case cc.IsPodComplete(pod):
		err = ds.statService.CheckPod(pod)
		if err != nil {
			vi.Status.Phase = virtv2.ImageFailed

			switch {
			case errors.Is(err, service.ErrProvisioningFailed):
				condition.Status = metav1.ConditionFalse
				condition.Reason = vicondition.ProvisioningFailed
				condition.Message = service.CapitalizeFirstLetter(err.Error() + ".")
				return false, nil
			default:
				return false, err
			}
		}

		var dvcrDataSource controller.DVCRDataSource
		dvcrDataSource, err = controller.NewDVCRDataSourcesForVMI(ctx, vi.Spec.DataSource, vi, ds.client)
		if err != nil {
			return false, err
		}

		if !dvcrDataSource.IsReady() {
			condition.Status = metav1.ConditionFalse
			condition.Reason = vicondition.ProvisioningFailed
			condition.Message = "Failed to get stats from non-ready datasource: waiting for the DataSource to be ready."
			return false, nil
		}

		condition.Status = metav1.ConditionTrue
		condition.Reason = vicondition.Ready
		condition.Message = ""

		vi.Status.Phase = virtv2.ImageReady
		vi.Status.Size = dvcrDataSource.GetSize()
		vi.Status.CDROM = dvcrDataSource.IsCDROM()
		vi.Status.Format = dvcrDataSource.GetFormat()
		vi.Status.Progress = "100%"
		vi.Status.Target.RegistryURL = ds.dvcrSettings.RegistryImageForVMI(vi.Name, vi.Namespace)

		log.Info("Ready", "progress", vi.Status.Progress, "pod.phase", pod.Status.Phase)
	default:
		err = ds.statService.CheckPod(pod)
		if err != nil {
			vi.Status.Phase = virtv2.ImageFailed

			switch {
			case errors.Is(err, service.ErrNotInitialized), errors.Is(err, service.ErrNotScheduled):
				condition.Status = metav1.ConditionFalse
				condition.Reason = vicondition.ProvisioningNotStarted
				condition.Message = service.CapitalizeFirstLetter(err.Error() + ".")
				return false, nil
			case errors.Is(err, service.ErrProvisioningFailed):
				condition.Status = metav1.ConditionFalse
				condition.Reason = vicondition.ProvisioningFailed
				condition.Message = service.CapitalizeFirstLetter(err.Error() + ".")
				return false, nil
			default:
				return false, err
			}
		}

		condition.Status = metav1.ConditionFalse
		condition.Reason = vicondition.Provisioning
		condition.Message = "Import is in the process of provisioning to DVCR."

		vi.Status.Phase = virtv2.ImageProvisioning
		vi.Status.Target.RegistryURL = ds.dvcrSettings.RegistryImageForVMI(vi.Name, vi.Namespace)

		log.Info("Ready", "progress", vi.Status.Progress, "pod.phase", pod.Status.Phase)
	}

	return true, nil
}

func (ds ObjectRefDataSource) SyncPVCFromPVC(ctx context.Context, vi *virtv2.VirtualImage, viRef *virtv2.VirtualImage) (bool, error) {
	log, ctx := logger.GetDataSourceContext(ctx, objectRefDataSource)

	condition, _ := service.GetCondition(vdcondition.ReadyType, vi.Status.Conditions)
	defer func() { service.SetCondition(condition, &vi.Status.Conditions) }()

	supgen := supplements.NewGenerator(cc.VIShortName, vi.Name, vi.Namespace, vi.UID)
	dv, err := ds.cloneService.GetDataVolume(ctx, supgen)
	if err != nil {
		return false, err
	}
	pvc, err := ds.cloneService.GetPersistentVolumeClaim(ctx, supgen)
	if err != nil {
		return false, err
	}
	pv, err := ds.cloneService.GetPersistentVolume(ctx, pvc)
	if err != nil {
		return false, err
	}

	switch {
	case isDiskProvisioningFinished(condition):
		log.Info("Disk provisioning finished: clean up")

		setPhaseConditionForFinishedImage(pv, pvc, &condition, &vi.Status.Phase, supgen)

		// Protect Ready Disk and underlying PVC and PV.
		err = ds.cloneService.Protect(ctx, vi, nil, pvc, pv)
		if err != nil {
			return false, err
		}

		err = ds.cloneService.Unprotect(ctx, dv)
		if err != nil {
			return false, err
		}

		return CleanUpSupplements(ctx, vi, ds)
	case cc.AnyTerminating(dv, pvc, pv):
		log.Info("Waiting for supplements to be terminated")
	case dv == nil:
		log.Info("Start import to PVC")

		vi.Status.Progress = "0%"
		vi.Status.SourceUID = util.GetPointer(viRef.GetUID())

		refSupgen := supplements.NewGenerator(cc.VIShortName, viRef.Name, viRef.Namespace, viRef.UID)

		refPvc, err := ds.cloneService.GetPersistentVolumeClaim(ctx, refSupgen)
		if err != nil {
			return false, err
		}

		var size resource.Quantity
		size, err = ds.getPVCSize2(viRef.Status.Size)
		if err != nil {
			setPhaseConditionToFailed(&condition, &vi.Status.Phase, err)

			if errors.Is(err, service.ErrInsufficientPVCSize) {
				return false, nil
			}

			return false, err
		}

		source := &cdiv1.DataVolumeSource{
			PVC: &cdiv1.DataVolumeSourcePVC{
				Name:      refPvc.Name,
				Namespace: refPvc.Namespace,
			},
		}

		err = ds.cloneService.Start(ctx, size, &storageClass, source, vi, supgen, false)
		if err != nil {
			return false, err
		}

		vi.Status.Phase = virtv2.ImageProvisioning
		condition.Status = metav1.ConditionFalse
		condition.Reason = vicondition.Provisioning
		condition.Message = "PVC Provisioner not found: create the new one."

		return true, nil
	case pvc == nil:
		vi.Status.Phase = virtv2.ImageProvisioning
		condition.Status = metav1.ConditionFalse
		condition.Reason = vicondition.Provisioning
		condition.Message = "PVC not found: waiting for creation."
		return true, nil
	case ds.cloneService.IsImportDone(dv, pvc):
		log.Info("Import has completed", "dvProgress", dv.Status.Progress, "dvPhase", dv.Status.Phase, "pvcPhase", pvc.Status.Phase)

		vi.Status.Phase = virtv2.ImageReady
		condition.Status = metav1.ConditionTrue
		condition.Reason = vicondition.Ready
		condition.Message = ""

		vi.Status.Progress = "100%"
		vi.Status.Target.PersistentVolumeClaim = dv.Status.ClaimName
	default:
		log.Info("Provisioning to PVC is in progress", "dvProgress", dv.Status.Progress, "dvPhase", dv.Status.Phase, "pvcPhase", pvc.Status.Phase)

		vi.Status.Progress = ds.cloneService.GetProgress(dv, vi.Status.Progress, service.NewScaleOption(0, 100))
		vi.Status.Target.PersistentVolumeClaim = dv.Status.ClaimName

		err = ds.cloneService.Protect(ctx, vi, dv, pvc, pv)
		if err != nil {
			return false, err
		}

		err = setPhaseConditionForPVCProvisioningImage(ctx, dv, vi, pvc, &condition, ds.cloneService)
		if err != nil {
			return false, err
		}

		return false, nil
	}

	return true, nil
}

func (ds ObjectRefDataSource) SyncPVC(ctx context.Context, vi *virtv2.VirtualImage) (bool, error) {
	log, ctx := logger.GetDataSourceContext(ctx, objectRefDataSource)

	condition, _ := service.GetCondition(vdcondition.ReadyType, vi.Status.Conditions)
	defer func() { service.SetCondition(condition, &vi.Status.Conditions) }()

	if vi.Spec.DataSource.ObjectRef.Kind == virtv2.VirtualImageKind {
		// fetch ref объекта и смотрим в его спеку
		viKey := types.NamespacedName{Name: vi.Spec.DataSource.ObjectRef.Name, Namespace: vi.Namespace}
		viObjetcRef, err := helper.FetchObject(ctx, viKey, ds.client, &virtv2.VirtualImage{})
		if err != nil {
			return false, fmt.Errorf("unable to get VM %s: %w", viKey, err)
		}

		// если в spec указан испочник Kubernetes  ->  новая логика c созданием clone service
		if viObjetcRef.Spec.Storage == virtv2.StorageKubernetes {
			return ds.SyncPVCFromPVC(ctx, vi, viObjetcRef)
		}
	}

	// дальше логика по работе с CVI (PVC нальется из DVCR)
	supgen := supplements.NewGenerator(cc.VIShortName, vi.Name, vi.Namespace, vi.UID)
	dv, err := ds.diskService.GetDataVolume(ctx, supgen)
	if err != nil {
		return false, err
	}
	pvc, err := ds.diskService.GetPersistentVolumeClaim(ctx, supgen)
	if err != nil {
		return false, err
	}
	pv, err := ds.diskService.GetPersistentVolume(ctx, pvc)
	if err != nil {
		return false, err
	}

	switch {
	case isDiskProvisioningFinished(condition):
		log.Info("Disk provisioning finished: clean up")

		setPhaseConditionForFinishedImage(pv, pvc, &condition, &vi.Status.Phase, supgen)

		// Protect Ready Disk and underlying PVC and PV.
		err = ds.diskService.Protect(ctx, vi, nil, pvc, pv)
		if err != nil {
			return false, err
		}

		err = ds.diskService.Unprotect(ctx, dv)
		if err != nil {
			return false, err
		}

		return CleanUpSupplements(ctx, vi, ds)
	case cc.AnyTerminating(dv, pvc, pv):
		log.Info("Waiting for supplements to be terminated")
	case dv == nil:
		log.Info("Start import to PVC")

		var dvcrDataSource controller.DVCRDataSource
		dvcrDataSource, err = controller.NewDVCRDataSourcesForVMI(ctx, vi.Spec.DataSource, vi, ds.client)
		if err != nil {
			return false, err
		}

		if !dvcrDataSource.IsReady() {
			condition.Status = metav1.ConditionFalse
			condition.Reason = vicondition.ProvisioningFailed
			condition.Message = "Failed to get stats from non-ready datasource: waiting for the DataSource to be ready."
			return false, nil
		}

		vi.Status.Progress = "0%"
		vi.Status.SourceUID = util.GetPointer(dvcrDataSource.GetUID())

		var diskSize resource.Quantity
		diskSize, err = ds.getPVCSize(dvcrDataSource)
		if err != nil {
			setPhaseConditionToFailed(&condition, &vi.Status.Phase, err)

			if errors.Is(err, service.ErrInsufficientPVCSize) {
				return false, nil
			}

			return false, err
		}

		var source *cdiv1.DataVolumeSource
		source, err = ds.getSource(supgen, dvcrDataSource)
		if err != nil {
			return false, err
		}

		err = ds.diskService.Start(ctx, diskSize, &storageClass, source, vi, supgen, false)
		if err != nil {
			return false, err
		}

		vi.Status.Phase = virtv2.ImageProvisioning
		condition.Status = metav1.ConditionFalse
		condition.Reason = vicondition.Provisioning
		condition.Message = "PVC Provisioner not found: create the new one."

		return true, nil
	case pvc == nil:
		vi.Status.Phase = virtv2.ImageProvisioning
		condition.Status = metav1.ConditionFalse
		condition.Reason = vicondition.Provisioning
		condition.Message = "PVC not found: waiting for creation."
		return true, nil
	case ds.diskService.IsImportDone(dv, pvc):
		log.Info("Import has completed", "dvProgress", dv.Status.Progress, "dvPhase", dv.Status.Phase, "pvcPhase", pvc.Status.Phase)

		vi.Status.Phase = virtv2.ImageReady
		condition.Status = metav1.ConditionTrue
		condition.Reason = vicondition.Ready
		condition.Message = ""

		vi.Status.Progress = "100%"
		vi.Status.Target.PersistentVolumeClaim = dv.Status.ClaimName
	default:
		log.Info("Provisioning to PVC is in progress", "dvProgress", dv.Status.Progress, "dvPhase", dv.Status.Phase, "pvcPhase", pvc.Status.Phase)

		vi.Status.Progress = ds.diskService.GetProgress(dv, vi.Status.Progress, service.NewScaleOption(0, 100))
		vi.Status.Target.PersistentVolumeClaim = dv.Status.ClaimName

		err = ds.diskService.Protect(ctx, vi, dv, pvc, pv)
		if err != nil {
			return false, err
		}

		err = setPhaseConditionForPVCProvisioningImage(ctx, dv, vi, pvc, &condition, ds.diskService)
		if err != nil {
			return false, err
		}

		return false, nil
	}

	return true, nil
}

func (ds ObjectRefDataSource) Sync(ctx context.Context, vi *virtv2.VirtualImage) (bool, error) {
	log, ctx := logger.GetDataSourceContext(ctx, "objectref")

	condition, _ := service.GetCondition(vicondition.ReadyType, vi.Status.Conditions)
	defer func() { service.SetCondition(condition, &vi.Status.Conditions) }()

	if vi.Spec.DataSource.ObjectRef.Kind == virtv2.VirtualImageKind {
		// fetch ref объекта и смотрим в его спеку
		viKey := types.NamespacedName{Name: vi.Spec.DataSource.ObjectRef.Name, Namespace: vi.Namespace}
		viObjetcRef, err := helper.FetchObject(ctx, viKey, ds.client, &virtv2.VirtualImage{})
		if err != nil {
			return false, fmt.Errorf("unable to get VM %s: %w", viKey, err)
		}

		// если в spec указан испочник Kubernetes - новая логика по выгрузке образа с файловой системы на DVCR
		if viObjetcRef.Spec.Storage == virtv2.StorageKubernetes {
			return ds.SyncDVCRFromPVC(ctx, vi, viObjetcRef)
		}
	}

	supgen := supplements.NewGenerator(cc.VIShortName, vi.Name, vi.Namespace, vi.UID)
	pod, err := ds.importerService.GetPod(ctx, supgen)
	if err != nil {
		return false, err
	}

	switch {
	case isDiskProvisioningFinished(condition):
		log.Info("Virtual image provisioning finished: clean up")

		condition.Status = metav1.ConditionTrue
		condition.Reason = vicondition.Ready
		condition.Message = ""

		vi.Status.Phase = virtv2.ImageReady

		err = ds.importerService.Unprotect(ctx, pod)
		if err != nil {
			return false, err
		}

		return CleanUp(ctx, vi, ds)
	case cc.IsTerminating(pod):
		vi.Status.Phase = virtv2.ImagePending

		log.Info("Cleaning up...")
	case pod == nil:
		var dvcrDataSource controller.DVCRDataSource
		dvcrDataSource, err = controller.NewDVCRDataSourcesForVMI(ctx, vi.Spec.DataSource, vi, ds.client)
		if err != nil {
			return false, err
		}

		var envSettings *importer.Settings
		envSettings, err = ds.getEnvSettings(vi, supgen, dvcrDataSource)
		if err != nil {
			return false, err
		}

		err = ds.importerService.Start(ctx, envSettings, vi, supgen, datasource.NewCABundleForVMI(vi.Spec.DataSource), "")
		var requeue bool
		requeue, err = setPhaseConditionForImporterStart(&condition, &vi.Status.Phase, err)
		if err != nil {
			return false, err
		}

		vi.Status.SourceUID = util.GetPointer(dvcrDataSource.GetUID())

		log.Info("Ready", "progress", vi.Status.Progress, "pod.phase", "nil")

		return requeue, nil
	case cc.IsPodComplete(pod):
		err = ds.statService.CheckPod(pod)
		if err != nil {
			vi.Status.Phase = virtv2.ImageFailed

			switch {
			case errors.Is(err, service.ErrProvisioningFailed):
				condition.Status = metav1.ConditionFalse
				condition.Reason = vicondition.ProvisioningFailed
				condition.Message = service.CapitalizeFirstLetter(err.Error() + ".")
				return false, nil
			default:
				return false, err
			}
		}

		var dvcrDataSource controller.DVCRDataSource
		dvcrDataSource, err = controller.NewDVCRDataSourcesForVMI(ctx, vi.Spec.DataSource, vi, ds.client)
		if err != nil {
			return false, err
		}

		if !dvcrDataSource.IsReady() {
			condition.Status = metav1.ConditionFalse
			condition.Reason = vicondition.ProvisioningFailed
			condition.Message = "Failed to get stats from non-ready datasource: waiting for the DataSource to be ready."
			return false, nil
		}

		condition.Status = metav1.ConditionTrue
		condition.Reason = vicondition.Ready
		condition.Message = ""

		vi.Status.Phase = virtv2.ImageReady
		vi.Status.Size = dvcrDataSource.GetSize()
		vi.Status.CDROM = dvcrDataSource.IsCDROM()
		vi.Status.Format = dvcrDataSource.GetFormat()
		vi.Status.Progress = "100%"
		vi.Status.Target.RegistryURL = ds.statService.GetDVCRImageName(pod)

		log.Info("Ready", "progress", vi.Status.Progress, "pod.phase", pod.Status.Phase)
	default:
		err = ds.statService.CheckPod(pod)
		if err != nil {
			vi.Status.Phase = virtv2.ImageFailed

			switch {
			case errors.Is(err, service.ErrNotInitialized), errors.Is(err, service.ErrNotScheduled):
				condition.Status = metav1.ConditionFalse
				condition.Reason = vicondition.ProvisioningNotStarted
				condition.Message = service.CapitalizeFirstLetter(err.Error() + ".")
				return false, nil
			case errors.Is(err, service.ErrProvisioningFailed):
				condition.Status = metav1.ConditionFalse
				condition.Reason = vicondition.ProvisioningFailed
				condition.Message = service.CapitalizeFirstLetter(err.Error() + ".")
				return false, nil
			default:
				return false, err
			}
		}

		condition.Status = metav1.ConditionFalse
		condition.Reason = vicondition.Provisioning
		condition.Message = "Import is in the process of provisioning to DVCR."

		vi.Status.Phase = virtv2.ImageProvisioning
		vi.Status.Target.RegistryURL = ds.statService.GetDVCRImageName(pod)

		log.Info("Ready", "progress", vi.Status.Progress, "pod.phase", pod.Status.Phase)
	}

	return true, nil
}

func (ds ObjectRefDataSource) CleanUp(ctx context.Context, vi *virtv2.VirtualImage) (bool, error) {
	supgen := supplements.NewGenerator(cc.VIShortName, vi.Name, vi.Namespace, vi.UID)

	requeue, err := ds.importerService.CleanUp(ctx, supgen)
	if err != nil {
		return false, err
	}

	// TODO clean up for clone Service or disk Service (еще один аргурмент объединить их) и тогда внутрь можно передвать нужный сервис для очистки

	return requeue, nil
}

func (ds ObjectRefDataSource) Validate(ctx context.Context, vi *virtv2.VirtualImage) error {
	if vi.Spec.DataSource.ObjectRef == nil {
		return fmt.Errorf("nil object ref: %s", vi.Spec.DataSource.Type)
	}

	dvcrDataSource, err := controller.NewDVCRDataSourcesForVMI(ctx, vi.Spec.DataSource, vi, ds.client)
	if err != nil {
		return err
	}

	if dvcrDataSource.IsReady() {
		return nil
	}

	switch vi.Spec.DataSource.ObjectRef.Kind {
	case virtv2.VirtualImageObjectRefKindVirtualImage:
		return NewImageNotReadyError(vi.Spec.DataSource.ObjectRef.Name)
	case virtv2.VirtualImageObjectRefKindClusterVirtualImage:
		return NewClusterImageNotReadyError(vi.Spec.DataSource.ObjectRef.Name)
	default:
		return fmt.Errorf("unexpected object ref kind: %s", vi.Spec.DataSource.ObjectRef.Kind)
	}
}

func (ds ObjectRefDataSource) getEnvSettings(vi *virtv2.VirtualImage, sup *supplements.Generator, dvcrDataSource controller.DVCRDataSource) (*importer.Settings, error) {
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

func (ds ObjectRefDataSource) CleanUpSupplements(ctx context.Context, vi *virtv2.VirtualImage) (bool, error) { // TODO передавать явно какой сервайс зачистить
	supgen := supplements.NewGenerator(cc.VIShortName, vi.Name, vi.Namespace, vi.UID)

	requeue, err := ds.diskService.CleanUpSupplements(ctx, supgen)
	if err != nil {
		return false, err
	}

	return requeue, nil
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

	return ds.diskService.AdjustPVCSize(&unpackedSize, unpackedSize)
}

func (ds ObjectRefDataSource) getPVCSize2(is virtv2.ImageStatusSize) (resource.Quantity, error) {
	unpackedSize, err := resource.ParseQuantity(is.UnpackedBytes)
	if err != nil {
		return resource.Quantity{}, fmt.Errorf("failed to parse unpacked bytes %s: %w", is.UnpackedBytes, err)
	}

	if unpackedSize.IsZero() {
		return resource.Quantity{}, errors.New("got zero unpacked size from data source")
	}

	return ds.diskService.AdjustPVCSize(&unpackedSize, unpackedSize)
}

func (ds ObjectRefDataSource) getSource(sup *supplements.Generator, dvcrDataSource controller.DVCRDataSource) (*cdiv1.DataVolumeSource, error) {
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
