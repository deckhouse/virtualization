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
	"k8s.io/apimachinery/pkg/types"
	cdiv1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	cc "github.com/deckhouse/virtualization-controller/pkg/common"
	"github.com/deckhouse/virtualization-controller/pkg/common/datasource"
	vdutil "github.com/deckhouse/virtualization-controller/pkg/common/datavolume"
	"github.com/deckhouse/virtualization-controller/pkg/controller/common"
	"github.com/deckhouse/virtualization-controller/pkg/controller/importer"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/supplements"
	"github.com/deckhouse/virtualization-controller/pkg/dvcr"
	"github.com/deckhouse/virtualization-controller/pkg/sdk/framework/helper"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdcondition"
)

type RegistryDataSource struct {
	statService     *service.StatService
	importerService *service.ImporterService
	diskService     *service.DiskService
	dvcrSettings    *dvcr.Settings
	client          client.Client
	logger          *slog.Logger
}

func NewRegistryDataSource(
	statService *service.StatService,
	importerService *service.ImporterService,
	diskService *service.DiskService,
	dvcrSettings *dvcr.Settings,
	client client.Client,
) *RegistryDataSource {
	return &RegistryDataSource{
		statService:     statService,
		importerService: importerService,
		diskService:     diskService,
		dvcrSettings:    dvcrSettings,
		client:          client,
		logger:          slog.Default().With("controller", common.VDShortName, "ds", "http"),
	}
}

func (ds RegistryDataSource) Sync(ctx context.Context, vd *virtv2.VirtualDisk) (bool, error) {
	ds.logger.Info("Sync", "vd", vd.Name)

	condition, _ := service.GetCondition(vdcondition.ReadyType, vd.Status.Conditions)
	defer func() { service.SetCondition(condition, &vd.Status.Conditions) }()

	supgen := supplements.NewGenerator(common.VDShortName, vd.Name, vd.Namespace, vd.UID)
	pod, err := ds.importerService.GetPod(ctx, supgen)
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

		// Unprotect import time supplements to delete them later.
		err = ds.importerService.Unprotect(ctx, pod)
		if err != nil {
			return false, err
		}

		err = ds.diskService.Unprotect(ctx, dv)
		if err != nil {
			return false, err
		}

		return CleanUpSupplements(ctx, vd, ds)
	case common.AnyTerminating(pod, dv, pvc):
		vd.Status.Phase = virtv2.DiskPending

		ds.logger.Info("Cleaning up...", "vd", vd.Name)
	case pod == nil:
		envSettings := ds.getEnvSettings(vd, supgen)
		err = ds.importerService.Start(ctx, envSettings, vd, supgen, datasource.NewCABundleForVMD(vd.Spec.DataSource))
		if err != nil {
			return false, err
		}

		vd.Status.Phase = virtv2.DiskProvisioning
		condition.Status = metav1.ConditionFalse
		condition.Reason = vdcondition.ReadyReason_Provisioning
		condition.Message = "DVCR Provisioner not found: create the new one."

		vd.Status.Progress = "0%"

		ds.logger.Info("Create importer pod...", "vd", vd.Name, "progress", vd.Status.Progress, "pod.phase", "nil")
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

		vd.Status.Phase = virtv2.DiskProvisioning
		condition.Status = metav1.ConditionFalse
		condition.Reason = vdcondition.ReadyReason_Provisioning
		condition.Message = "DVCR Provisioner not found: create the new one."

		vd.Status.Progress = ds.statService.GetProgress(vd.GetUID(), pod, vd.Status.Progress, service.NewScaleOption(0, 50))

		err = ds.importerService.Protect(ctx, pod)
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

func (ds RegistryDataSource) CleanUp(ctx context.Context, vd *virtv2.VirtualDisk) (bool, error) {
	supgen := supplements.NewGenerator(common.VDShortName, vd.Name, vd.Namespace, vd.UID)

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

func (ds RegistryDataSource) CleanUpSupplements(ctx context.Context, vd *virtv2.VirtualDisk) (bool, error) {
	supgen := supplements.NewGenerator(common.VDShortName, vd.Name, vd.Namespace, vd.UID)

	importerRequeue, err := ds.importerService.CleanUpSupplements(ctx, supgen)
	if err != nil {
		return false, err
	}

	diskRequeue, err := ds.diskService.CleanUpSupplements(ctx, supgen)
	if err != nil {
		return false, err
	}

	return importerRequeue || diskRequeue, nil
}

func (ds RegistryDataSource) Validate(ctx context.Context, vd *virtv2.VirtualDisk) error {
	if vd.Spec.DataSource == nil || vd.Spec.DataSource.ContainerImage == nil {
		return errors.New("container image missed for data source")
	}

	if vd.Spec.DataSource.ContainerImage.ImagePullSecret.Name != "" {
		secretName := types.NamespacedName{
			Namespace: vd.Spec.DataSource.ContainerImage.ImagePullSecret.Namespace,
			Name:      vd.Spec.DataSource.ContainerImage.ImagePullSecret.Name,
		}
		secret, err := helper.FetchObject[*corev1.Secret](ctx, secretName, ds.client, &corev1.Secret{})
		if err != nil {
			return fmt.Errorf("failed to get secret %s: %w", secretName, err)
		}

		if secret == nil {
			return ErrSecretNotFound
		}
	}

	return nil
}

func (ds RegistryDataSource) getEnvSettings(vd *virtv2.VirtualDisk, supgen *supplements.Generator) *importer.Settings {
	var settings importer.Settings

	importer.ApplyRegistrySourceSettings(&settings, vd.Spec.DataSource.ContainerImage, supgen)
	importer.ApplyDVCRDestinationSettings(
		&settings,
		ds.dvcrSettings,
		supgen,
		ds.dvcrSettings.RegistryImageForVMD(vd.Name, vd.Namespace),
	)

	return &settings
}

func (ds RegistryDataSource) getSource(vd *virtv2.VirtualDisk, sup *supplements.Generator) *cdiv1.DataVolumeSource {
	// The image was preloaded from source into dvcr.
	// We can't use the same data source a second time, but we can set dvcr as the data source.
	// Use DV name for the Secret with DVCR auth and the ConfigMap with DVCR CA Bundle.
	dvcrSourceImageName := ds.dvcrSettings.RegistryImageForVMD(vd.Name, vd.Namespace)

	url := cc.DockerRegistrySchemePrefix + dvcrSourceImageName
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

func (ds RegistryDataSource) getPVCSize(vd *virtv2.VirtualDisk, pod *corev1.Pod) (resource.Quantity, error) {
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