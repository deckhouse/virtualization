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
	"log/slog"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	cdiv1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"

	common2 "github.com/deckhouse/virtualization-controller/pkg/common"
	"github.com/deckhouse/virtualization-controller/pkg/common/datasource"
	vdutil "github.com/deckhouse/virtualization-controller/pkg/common/datavolume"
	"github.com/deckhouse/virtualization-controller/pkg/controller/common"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/supplements"
	"github.com/deckhouse/virtualization-controller/pkg/controller/uploader"
	"github.com/deckhouse/virtualization-controller/pkg/dvcr"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdcondition"
)

type UploadDataSource struct {
	statService     *service.StatService
	uploaderService *service.UploaderService
	diskService     *service.DiskService
	dvcrSettings    *dvcr.Settings
	logger          *slog.Logger
}

func NewUploadDataSource(
	statService *service.StatService,
	uploaderService *service.UploaderService,
	diskService *service.DiskService,
	dvcrSettings *dvcr.Settings,
) *UploadDataSource {
	return &UploadDataSource{
		statService:     statService,
		uploaderService: uploaderService,
		diskService:     diskService,
		dvcrSettings:    dvcrSettings,
		logger:          slog.Default().With("controller", common.VDShortName, "ds", "upload"),
	}
}

func (ds UploadDataSource) Sync(ctx context.Context, vd *virtv2.VirtualDisk) (bool, error) {
	ds.logger.Info("Sync", "vd", vd.Name)

	condition, _ := service.GetCondition(vdcondition.ReadyType, vd.Status.Conditions)
	defer func() { service.SetCondition(condition, &vd.Status.Conditions) }()

	supgen := supplements.NewGenerator(common.VDShortName, vd.Name, vd.Namespace, vd.UID)
	pod, err := ds.uploaderService.GetPod(ctx, supgen)
	if err != nil {
		return false, err
	}
	svc, err := ds.uploaderService.GetService(ctx, supgen)
	if err != nil {
		return false, err
	}
	ing, err := ds.uploaderService.GetIngress(ctx, supgen)
	if err != nil {
		return false, err
	}
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

	if vd.Status.UploadCommand == "" {
		if ing != nil && ing.Annotations[common.AnnUploadURL] != "" {
			vd.Status.UploadCommand = fmt.Sprintf("curl %s -T example.iso", ing.Annotations[common.AnnUploadURL])
		}
	}

	switch {
	case isDiskProvisioningFinished(condition):
		ds.logger.Info("Finishing...", "vd", vd.Name)

		switch {
		case pvc == nil:
			condition.Status = metav1.ConditionFalse
			condition.Reason = vdcondition.ReadyReason_Lost
			condition.Message = fmt.Sprintf("PVC %s not found.", supgen.PersistentVolumeClaim().String())
		case pv == nil:
			condition.Status = metav1.ConditionFalse
			condition.Reason = vdcondition.ReadyReason_Lost
			condition.Message = fmt.Sprintf("PV %s not found.", pvc.Spec.VolumeName)
		default:
			condition.Status = metav1.ConditionTrue
			condition.Reason = vdcondition.ReadyReason_Ready
			condition.Message = ""
		}

		// Protect Ready Disk and underlying PVC and PV.
		err = ds.diskService.Protect(ctx, vd, nil, pvc, pv)
		if err != nil {
			return false, err
		}

		// Unprotect upload time supplements to delete them later.
		err = ds.uploaderService.Unprotect(ctx, pod, svc, ing)
		if err != nil {
			return false, err
		}

		err = ds.diskService.Unprotect(ctx, dv)
		if err != nil {
			return false, err
		}

		return CleanUpSupplements(ctx, vd, ds)
	case common.AnyTerminating(pod, svc, ing, dv, pvc):
		vd.Status.Phase = virtv2.DiskPending
		condition.Status = metav1.ConditionUnknown
		condition.Reason = ""
		condition.Message = ""

		ds.logger.Info("Cleaning up...", "vd", vd.Name)
	case pod == nil && svc == nil && ing == nil:
		envSettings := ds.getEnvSettings(supgen)
		err = ds.uploaderService.Start(ctx, envSettings, vd, supgen, datasource.NewCABundleForVMD(vd.Spec.DataSource))
		if err != nil {
			return false, err
		}

		vd.Status.Phase = virtv2.DiskPending
		condition.Status = metav1.ConditionFalse
		condition.Reason = vdcondition.ReadyReason_Provisioning
		condition.Message = "DVCR Provisioner not found: create the new one."

		vd.Status.Progress = "0%"

		ds.logger.Info("Create uploader pod...", "vd", vd.Name, "progress", vd.Status.Progress, "pod.phase", nil)
	case !common.IsPodComplete(pod):
		err = ds.statService.CheckPod(pod)
		if err != nil {
			vd.Status.Phase = virtv2.DiskFailed

			switch {
			case errors.Is(err, service.ErrNotInitialized), errors.Is(err, service.ErrNotScheduled):
				condition.Status = metav1.ConditionFalse
				condition.Reason = vdcondition.ReadyReason_ProvisioningNotStarted
				condition.Message = service.CapitalizeFirstLetter(err.Error() + ".")
				return false, nil
			case errors.Is(err, service.ErrProvisioningFailed):
				condition.Status = metav1.ConditionFalse
				condition.Reason = vdcondition.ReadyReason_ProvisioningFailed
				condition.Message = service.CapitalizeFirstLetter(err.Error() + ".")
				return false, nil
			default:
				return false, err
			}
		}

		if !ds.statService.IsUploadStarted(vd.GetUID(), pod) {
			vd.Status.Phase = virtv2.DiskWaitForUserUpload
			condition.Status = metav1.ConditionFalse
			condition.Reason = vdcondition.ReadyReason_WaitForUserUpload
			condition.Message = "Waiting for the user upload."

			ds.logger.Info("WaitForUserUpload...", "vd", vd.Name, "progress", vd.Status.Progress, "pod.phase", pod.Status.Phase)

			return false, nil
		}

		vd.Status.Phase = virtv2.DiskProvisioning
		condition.Status = metav1.ConditionFalse
		condition.Reason = vdcondition.ReadyReason_Provisioning
		condition.Message = "Import is in the process of provisioning to DVCR."

		vd.Status.Progress = ds.statService.GetProgress(vd.GetUID(), pod, vd.Status.Progress, service.NewScaleOption(0, 50))

		err = ds.uploaderService.Protect(ctx, pod, svc, ing)
		if err != nil {
			return false, err
		}

		ds.logger.Info("Provisioning...", "vd", vd.Name, "progress", vd.Status.Progress, "pod.phase", pod.Status.Phase)
	case dv == nil:
		err = ds.statService.CheckPod(pod)
		if err != nil {
			vd.Status.Phase = virtv2.DiskFailed

			switch {
			case errors.Is(err, service.ErrProvisioningFailed):
				condition.Status = metav1.ConditionFalse
				condition.Reason = vdcondition.ReadyReason_ProvisioningFailed
				condition.Message = service.CapitalizeFirstLetter(err.Error() + ".")
				return false, nil
			default:
				return false, err
			}
		}

		var diskSize resource.Quantity
		diskSize, err = ds.getPVCSize(vd, pod)
		if err != nil {
			return false, err
		}

		source := ds.getSource(vd, supgen)

		err = ds.diskService.Start(ctx, diskSize, vd.Spec.PersistentVolumeClaim.StorageClass, source, vd, supgen)
		if err != nil {
			return false, err
		}

		vd.Status.Phase = virtv2.DiskProvisioning
		condition.Status = metav1.ConditionFalse
		condition.Reason = vdcondition.ReadyReason_Provisioning
		condition.Message = "PVC Provisioner not found: create the new one."

		vd.Status.Progress = "50%"

		ds.logger.Info("Create data volume...", "vd", vd.Name, "progress", vd.Status.Progress, "dv.phase", "nil")

		return true, nil
	case common.IsDataVolumeComplete(dv):
		vd.Status.Phase = virtv2.DiskReady
		condition.Status = metav1.ConditionTrue
		condition.Reason = vdcondition.ReadyReason_Ready
		condition.Message = ""

		vd.Status.Progress = "100%"
		vd.Status.Capacity = ds.diskService.GetCapacity(pvc)
		vd.Status.Target.PersistentVolumeClaim = dv.Status.ClaimName

		ds.logger.Info("Ready", "vd", vd.Name, "progress", vd.Status.Progress, "dv.phase", dv.Status.Phase)
	default:
		ds.logger.Info("Provisioning...", "vd", vd.Name, "progress", vd.Status.Progress, "dv.phase", dv.Status.Phase)

		vd.Status.Progress = ds.diskService.GetProgress(dv, vd.Status.Progress, service.NewScaleOption(50, 100))
		vd.Status.Capacity = ds.diskService.GetCapacity(pvc)
		vd.Status.Target.PersistentVolumeClaim = dv.Status.ClaimName

		err = ds.diskService.Protect(ctx, vd, dv, pvc, pv)
		if err != nil {
			return false, err
		}

		err = ds.diskService.CheckStorageClass(ctx, vd.Spec.PersistentVolumeClaim.StorageClass)
		switch {
		case err == nil:
			vd.Status.Phase = virtv2.DiskProvisioning
			condition.Status = metav1.ConditionFalse
			condition.Reason = vdcondition.ReadyReason_Provisioning
			condition.Message = "Import is in the process of provisioning to PVC."
			return false, nil
		case errors.Is(err, service.ErrStorageClassNotFound):
			vd.Status.Phase = virtv2.DiskFailed
			condition.Status = metav1.ConditionFalse
			condition.Reason = vdcondition.ReadyReason_ProvisioningFailed
			condition.Message = "Provided StorageClass not found in the cluster."
			return false, nil
		case errors.Is(err, service.ErrDefaultStorageClassNotFound):
			vd.Status.Phase = virtv2.DiskFailed
			condition.Status = metav1.ConditionFalse
			condition.Reason = vdcondition.ReadyReason_ProvisioningFailed
			condition.Message = "Default StorageClass not found in the cluster: please provide a StorageClass name or set a default StorageClass."
			return false, nil
		default:
			return false, err
		}
	}

	return true, nil
}

func (ds UploadDataSource) CleanUp(ctx context.Context, vd *virtv2.VirtualDisk) (bool, error) {
	supgen := supplements.NewGenerator(common.VDShortName, vd.Name, vd.Namespace, vd.UID)

	uploaderRequeue, err := ds.uploaderService.CleanUp(ctx, supgen)
	if err != nil {
		return false, err
	}

	diskRequeue, err := ds.diskService.CleanUp(ctx, supgen)
	if err != nil {
		return false, err
	}

	return uploaderRequeue || diskRequeue, nil
}

func (ds UploadDataSource) CleanUpSupplements(ctx context.Context, vd *virtv2.VirtualDisk) (bool, error) {
	supgen := supplements.NewGenerator(common.VDShortName, vd.Name, vd.Namespace, vd.UID)

	uploaderRequeue, err := ds.uploaderService.CleanUpSupplements(ctx, supgen)
	if err != nil {
		return false, err
	}

	diskRequeue, err := ds.diskService.CleanUpSupplements(ctx, supgen)
	if err != nil {
		return false, err
	}

	return uploaderRequeue || diskRequeue, nil
}

func (ds UploadDataSource) Validate(_ context.Context, _ *virtv2.VirtualDisk) error {
	return nil
}

func (ds UploadDataSource) getEnvSettings(supgen *supplements.Generator) *uploader.Settings {
	var settings uploader.Settings

	uploader.ApplyDVCRDestinationSettings(
		&settings,
		ds.dvcrSettings,
		supgen,
		ds.dvcrSettings.RegistryImageForVMD(supgen.Name, supgen.Namespace),
	)

	return &settings
}

func (ds UploadDataSource) getSource(vd *virtv2.VirtualDisk, sup *supplements.Generator) *cdiv1.DataVolumeSource {
	// The image was preloaded from source into dvcr.
	// We can't use the same data source a second time, but we can set dvcr as the data source.
	// Use DV name for the Secret with DVCR auth and the ConfigMap with DVCR CA Bundle.
	dvcrSourceImageName := ds.dvcrSettings.RegistryImageForVMD(vd.Name, vd.Namespace)

	url := common2.DockerRegistrySchemePrefix + dvcrSourceImageName
	secretName := sup.DVCRAuthSecretForDV().Name
	certConfigMapName := sup.DVCRCABundleConfigMapForDV().Name

	return &cdiv1.DataVolumeSource{
		Registry: &cdiv1.DataVolumeSourceRegistry{
			URL:           &url,
			SecretRef:     &secretName,
			CertConfigMap: &certConfigMapName,
		},
	}
}

func (ds UploadDataSource) getPVCSize(vd *virtv2.VirtualDisk, pod *corev1.Pod) (resource.Quantity, error) {
	// Get size from the importer Pod to detect if specified PVC size is enough.
	unpackedSize, err := resource.ParseQuantity(ds.statService.GetSize(pod).UnpackedBytes)
	if err != nil {
		return resource.Quantity{}, err
	}

	if unpackedSize.IsZero() {
		return resource.Quantity{}, errors.New("got zero unpacked size from data source")
	}

	pvcSize := vd.Spec.PersistentVolumeClaim.Size
	if pvcSize != nil && !pvcSize.IsZero() && pvcSize.Cmp(unpackedSize) == -1 {
		return resource.Quantity{}, ErrPVCSizeSmallerImageVirtualSize
	}

	// Adjust PVC size to feat image onto scratch PVC.
	// TODO(future): remove size adjusting after get rid of scratch.
	adjustedSize := vdutil.AdjustPVCSize(unpackedSize)

	if pvcSize != nil && pvcSize.Cmp(adjustedSize) == 1 {
		return *pvcSize, nil
	}

	return adjustedSize, nil
}
