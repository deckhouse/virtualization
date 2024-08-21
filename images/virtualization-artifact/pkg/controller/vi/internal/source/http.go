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

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	cdiv1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"

	cc "github.com/deckhouse/virtualization-controller/pkg/common"
	"github.com/deckhouse/virtualization-controller/pkg/common/datasource"
	"github.com/deckhouse/virtualization-controller/pkg/controller/common"
	"github.com/deckhouse/virtualization-controller/pkg/controller/importer"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/supplements"
	"github.com/deckhouse/virtualization-controller/pkg/dvcr"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdcondition"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vicondition"
)

type HTTPDataSource struct {
	statService     Stat
	importerService Importer
	dvcrSettings    *dvcr.Settings
	diskService     *service.DiskService
}

func NewHTTPDataSource(
	statService Stat,
	importerService Importer,
	dvcrSettings *dvcr.Settings,
	diskService *service.DiskService,
) *HTTPDataSource {
	return &HTTPDataSource{
		statService:     statService,
		importerService: importerService,
		dvcrSettings:    dvcrSettings,
		diskService:     diskService,
	}
}

var storageClass = "linstor-thin-r2" // FIXME add seting settings in controller

func (ds HTTPDataSource) Sync(ctx context.Context, vi *virtv2.VirtualImage) (bool, error) {
	log, ctx := logger.GetDataSourceContext(ctx, "http")

	condition, _ := service.GetCondition(vicondition.ReadyType, vi.Status.Conditions)
	defer func() { service.SetCondition(condition, &vi.Status.Conditions) }()

	supgen := supplements.NewGenerator(common.VIShortName, vi.Name, vi.Namespace, vi.UID)
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
	case common.IsTerminating(pod):
		vi.Status.Phase = virtv2.ImagePending

		log.Info("Cleaning up...")
	case pod == nil:
		vi.Status.Progress = ds.statService.GetProgress(vi.GetUID(), pod, vi.Status.Progress)

		envSettings := ds.getEnvSettings(vi, supgen)
		err = ds.importerService.Start(ctx, envSettings, vi, supgen, datasource.NewCABundleForVMI(vi.Spec.DataSource))
		var requeue bool
		requeue, err = setPhaseConditionForImporterStart(&condition, &vi.Status.Phase, err)
		if err != nil {
			return false, err
		}

		log.Info("Create importer pod...", "progress", vi.Status.Progress, "pod.phase", "nil")

		return requeue, nil
	case common.IsPodComplete(pod):
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

		condition.Status = metav1.ConditionTrue
		condition.Reason = vicondition.Ready
		condition.Message = ""

		vi.Status.Phase = virtv2.ImageReady
		vi.Status.Size = ds.statService.GetSize(pod)
		vi.Status.CDROM = ds.statService.GetCDROM(pod)
		vi.Status.Format = ds.statService.GetFormat(pod)
		vi.Status.Progress = ds.statService.GetProgress(vi.GetUID(), pod, vi.Status.Progress)
		vi.Status.Target.RegistryURL = ds.statService.GetDVCRImageName(pod)
		vi.Status.DownloadSpeed = ds.statService.GetDownloadSpeed(vi.GetUID(), pod)

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

		err = ds.importerService.Protect(ctx, pod)
		if err != nil {
			return false, err
		}

		condition.Status = metav1.ConditionFalse
		condition.Reason = vicondition.Provisioning
		condition.Message = "Import is in the process of provisioning to DVCR."

		vi.Status.Phase = virtv2.ImageProvisioning
		vi.Status.Progress = ds.statService.GetProgress(vi.GetUID(), pod, vi.Status.Progress)
		vi.Status.Target.RegistryURL = ds.statService.GetDVCRImageName(pod)
		vi.Status.DownloadSpeed = ds.statService.GetDownloadSpeed(vi.GetUID(), pod)

		log.Info("Provisioning...", "progress", vi.Status.Progress, "pod.phase", pod.Status.Phase)
	}

	return true, nil
}

func (ds HTTPDataSource) SyncPVC(ctx context.Context, vi *virtv2.VirtualImage) (bool, error) {
	log, ctx := logger.GetDataSourceContext(ctx, "http")

	condition, _ := service.GetCondition(vicondition.ReadyType, vi.Status.Conditions)
	defer func() { service.SetCondition(condition, &vi.Status.Conditions) }()

	supgen := supplements.NewGenerator(common.VIShortName, vi.Name, vi.Namespace, vi.UID)
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
		log.Info("Image provisioning finished: clean up")

		setPhaseConditionForFinishedImage(pv, pvc, &condition, &vi.Status.Phase, supgen)

		// Protect Ready Disk and underlying PVC and PV.
		err = ds.diskService.Protect(ctx, vi, nil, pvc, pv)
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

		return CleanUpSupplements(ctx, vi, ds)
	case common.AnyTerminating(pod, dv, pvc, pv):
		log.Info("Waiting for supplements to be terminated")
	case pod == nil:
		vi.Status.Progress = ds.statService.GetProgress(vi.GetUID(), pod, vi.Status.Progress)
		vi.Status.Target.RegistryURL = ds.dvcrSettings.RegistryImageForVMI(vi.Name, vi.Namespace)

		envSettings := ds.getEnvSettings(vi, supgen)
		err = ds.importerService.Start(ctx, envSettings, vi, supgen, datasource.NewCABundleForVMI(vi.Spec.DataSource))
		var requeue bool
		requeue, err = setPhaseConditionForImporterStart(&condition, &vi.Status.Phase, err)
		if err != nil {
			return false, err
		}

		log.Info("Create importer pod...", "progress", vi.Status.Progress, "pod.phase", "nil")

		return requeue, nil
	case !common.IsPodComplete(pod):
		log.Info("Provisioning to DVCR is in progress", "podPhase", pod.Status.Phase)

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

		err = ds.importerService.Protect(ctx, pod)
		if err != nil {
			return false, err
		}

		vi.Status.Phase = virtv2.ImageProvisioning
		condition.Status = metav1.ConditionFalse
		condition.Reason = vicondition.Provisioning
		condition.Message = "Import is in the process of provisioning to DVCR."

		vi.Status.Progress = ds.statService.GetProgress(vi.GetUID(), pod, vi.Status.Progress, service.NewScaleOption(0, 50))
		vi.Status.DownloadSpeed = ds.statService.GetDownloadSpeed(vi.GetUID(), pod)
	case dv == nil:
		log.Info("Start import to PVC")

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

		vi.Status.Progress = "50.0%"
		vi.Status.DownloadSpeed = ds.statService.GetDownloadSpeed(vi.GetUID(), pod)

		var diskSize resource.Quantity
		diskSize, err = ds.getPVCSize(pod)
		if err != nil {
			setPhaseConditionToFailed(&condition, &vi.Status.Phase, err)

			if errors.Is(err, service.ErrInsufficientPVCSize) {
				return false, nil
			}

			return false, err
		}

		source := ds.getSource(vi, supgen)

		err = ds.diskService.Start(ctx, diskSize, &storageClass, source, vi, supgen, false)
		if err != nil {
			return false, err
		}

		vi.Status.Phase = virtv2.ImageProvisioning
		condition.Status = metav1.ConditionFalse
		condition.Reason = vdcondition.Provisioning
		condition.Message = "PVC Provisioner not found: create the new one."

		return true, nil
	case pvc == nil:
		vi.Status.Phase = virtv2.ImageProvisioning
		condition.Status = metav1.ConditionFalse
		condition.Reason = vdcondition.Provisioning
		condition.Message = "PVC not found: waiting for creation."
		return true, nil
	case ds.diskService.IsImportDone(dv, pvc):
		log.Info("Import has completed", "dvProgress", dv.Status.Progress, "dvPhase", dv.Status.Phase, "pvcPhase", pvc.Status.Phase)

		vi.Status.Phase = virtv2.ImageReady
		condition.Status = metav1.ConditionTrue
		condition.Reason = vdcondition.Ready
		condition.Message = ""

		vi.Status.Progress = "100%"
		vi.Status.Target.PersistentVolumeClaim = dv.Status.ClaimName
	default:
		log.Info("Provisioning to PVC is in progress", "dvProgress", dv.Status.Progress, "dvPhase", dv.Status.Phase, "pvcPhase", pvc.Status.Phase)

		vi.Status.Progress = ds.diskService.GetProgress(dv, vi.Status.Progress, service.NewScaleOption(50, 100))
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

func (ds HTTPDataSource) CleanUp(ctx context.Context, vi *virtv2.VirtualImage) (bool, error) {
	supgen := supplements.NewGenerator(common.VIShortName, vi.Name, vi.Namespace, vi.UID)

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

func (ds HTTPDataSource) CleanUpSupplements(ctx context.Context, vi *virtv2.VirtualImage) (bool, error) {
	supgen := supplements.NewGenerator(common.VIShortName, vi.Name, vi.Namespace, vi.UID)

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

func (ds HTTPDataSource) Validate(_ context.Context, _ *virtv2.VirtualImage) error {
	return nil
}

func (ds HTTPDataSource) getEnvSettings(vi *virtv2.VirtualImage, supgen *supplements.Generator) *importer.Settings {
	var settings importer.Settings

	importer.ApplyHTTPSourceSettings(&settings, vi.Spec.DataSource.HTTP, supgen)
	importer.ApplyDVCRDestinationSettings(
		&settings,
		ds.dvcrSettings,
		supgen,
		ds.dvcrSettings.RegistryImageForVI(vi),
	)

	return &settings
}

func (ds HTTPDataSource) getPVCSize(pod *corev1.Pod) (resource.Quantity, error) {
	// Get size from the importer Pod to detect if specified PVC size is enough.
	unpackedSize, err := resource.ParseQuantity(ds.statService.GetSize(pod).UnpackedBytes)
	if err != nil {
		return resource.Quantity{}, fmt.Errorf("failed to parse unpacked bytes %s: %w", ds.statService.GetSize(pod).UnpackedBytes, err)
	}

	if unpackedSize.IsZero() {
		return resource.Quantity{}, errors.New("got zero unpacked size from data source")
	}

	return ds.diskService.AdjustPVCSize(&unpackedSize, unpackedSize)
}

func (ds HTTPDataSource) getSource(vi *virtv2.VirtualImage, sup *supplements.Generator) *cdiv1.DataVolumeSource {
	// The image was preloaded from source into dvcr.
	// We can't use the same data source a second time, but we can set dvcr as the data source.
	// Use DV name for the Secret with DVCR auth and the ConfigMap with DVCR CA Bundle.
	dvcrSourceImageName := ds.dvcrSettings.RegistryImageForVMI(vi.Name, vi.Namespace)

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
