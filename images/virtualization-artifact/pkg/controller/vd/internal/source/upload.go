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
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	common2 "github.com/deckhouse/virtualization-controller/pkg/common"
	"github.com/deckhouse/virtualization-controller/pkg/common/datasource"
	"github.com/deckhouse/virtualization-controller/pkg/controller/common"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/supplements"
	"github.com/deckhouse/virtualization-controller/pkg/controller/uploader"
	"github.com/deckhouse/virtualization-controller/pkg/dvcr"
	"github.com/deckhouse/virtualization-controller/pkg/imageformat"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdcondition"
)

const uploadDataSource = "upload"

type UploadDataSource struct {
	statService     *service.StatService
	uploaderService *service.UploaderService
	diskService     *service.DiskService
	dvcrSettings    *dvcr.Settings
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
	}
}

func (ds UploadDataSource) Sync(ctx context.Context, vd *virtv2.VirtualDisk) (reconcile.Result, error) {
	log, ctx := logger.GetDataSourceContext(ctx, uploadDataSource)

	condition, ok := service.GetCondition(vdcondition.ReadyType, vd.Status.Conditions)
	if !ok {
		condition = metav1.Condition{
			Type:   vdcondition.ReadyType,
			Status: metav1.ConditionUnknown,
		}
	}
	defer func() { service.SetCondition(condition, &vd.Status.Conditions) }()

	supgen := supplements.NewGenerator(common.VDShortName, vd.Name, vd.Namespace, vd.UID)
	pod, err := ds.uploaderService.GetPod(ctx, supgen)
	if err != nil {
		return reconcile.Result{}, err
	}
	svc, err := ds.uploaderService.GetService(ctx, supgen)
	if err != nil {
		return reconcile.Result{}, err
	}
	ing, err := ds.uploaderService.GetIngress(ctx, supgen)
	if err != nil {
		return reconcile.Result{}, err
	}
	dv, err := ds.diskService.GetDataVolume(ctx, supgen)
	if err != nil {
		return reconcile.Result{}, err
	}
	pvc, err := ds.diskService.GetPersistentVolumeClaim(ctx, supgen)
	if err != nil {
		return reconcile.Result{}, err
	}

	switch {
	case isDiskProvisioningFinished(condition):
		log.Debug("Disk provisioning finished: clean up")

		setPhaseConditionForFinishedDisk(pvc, &condition, &vd.Status.Phase, supgen)

		// Protect Ready Disk and underlying PVC.
		err = ds.diskService.Protect(ctx, vd, nil, pvc)
		if err != nil {
			return reconcile.Result{}, err
		}

		// Unprotect upload time supplements to delete them later.
		err = ds.uploaderService.Unprotect(ctx, pod, svc, ing)
		if err != nil {
			return reconcile.Result{}, err
		}

		err = ds.diskService.Unprotect(ctx, dv)
		if err != nil {
			return reconcile.Result{}, err
		}

		return CleanUpSupplements(ctx, vd, ds)
	case common.AnyTerminating(pod, svc, ing, dv, pvc):
		log.Info("Waiting for supplements to be terminated")
	case pod == nil || svc == nil || ing == nil:
		log.Info("Start import to DVCR")

		vd.Status.Progress = "0%"

		envSettings := ds.getEnvSettings(vd, supgen)
		err = ds.uploaderService.Start(ctx, envSettings, vd, supgen, datasource.NewCABundleForVMD(vd.Spec.DataSource))
		switch {
		case err == nil:
			// OK.
		case common.ErrQuotaExceeded(err):
			return setQuotaExceededPhaseCondition(&condition, &vd.Status.Phase, err, vd.CreationTimestamp), nil
		default:
			setPhaseConditionToFailed(&condition, &vd.Status.Phase, fmt.Errorf("unexpected error: %w", err))
			return reconcile.Result{}, err
		}

		vd.Status.Phase = virtv2.DiskPending
		condition.Status = metav1.ConditionFalse
		condition.Reason = vdcondition.WaitForUserUpload
		condition.Message = "DVCR Provisioner not found: create the new one."

		return reconcile.Result{Requeue: true}, nil
	case !common.IsPodComplete(pod):
		err = ds.statService.CheckPod(pod)
		if err != nil {
			vd.Status.Phase = virtv2.DiskFailed

			switch {
			case errors.Is(err, service.ErrNotInitialized), errors.Is(err, service.ErrNotScheduled):
				condition.Status = metav1.ConditionFalse
				condition.Reason = vdcondition.ProvisioningNotStarted
				condition.Message = service.CapitalizeFirstLetter(err.Error() + ".")
				return reconcile.Result{}, nil
			case errors.Is(err, service.ErrProvisioningFailed):
				condition.Status = metav1.ConditionFalse
				condition.Reason = vdcondition.ProvisioningFailed
				condition.Message = service.CapitalizeFirstLetter(err.Error() + ".")
				return reconcile.Result{}, nil
			default:
				return reconcile.Result{}, err
			}
		}

		if !ds.statService.IsUploadStarted(vd.GetUID(), pod) {
			if ds.statService.IsUploaderReady(pod, svc, ing) {
				log.Info("Waiting for the user upload", "pod.phase", pod.Status.Phase)

				vd.Status.Phase = virtv2.DiskWaitForUserUpload
				condition.Status = metav1.ConditionFalse
				condition.Reason = vdcondition.WaitForUserUpload
				condition.Message = "Waiting for the user upload."
				vd.Status.ImageUploadURLs = &virtv2.ImageUploadURLs{
					External:  ds.uploaderService.GetExternalURL(ctx, ing),
					InCluster: ds.uploaderService.GetInClusterURL(ctx, svc),
				}
			} else {
				log.Info("Waiting for the uploader to be ready to process the user's upload", "pod.phase", pod.Status.Phase)

				vd.Status.Phase = virtv2.DiskPending
				condition.Status = metav1.ConditionFalse
				condition.Reason = vdcondition.ProvisioningNotStarted
				condition.Message = fmt.Sprintf("Waiting for the uploader %q to be ready to process the user's upload.", pod.Name)
			}

			return reconcile.Result{Requeue: true}, nil
		}

		log.Info("Provisioning to DVCR is in progress", "podPhase", pod.Status.Phase)

		vd.Status.Phase = virtv2.DiskProvisioning
		condition.Status = metav1.ConditionFalse
		condition.Reason = vdcondition.Provisioning
		condition.Message = "Import is in the process of provisioning to DVCR."

		vd.Status.Progress = ds.statService.GetProgress(vd.GetUID(), pod, vd.Status.Progress, service.NewScaleOption(0, 50))
		vd.Status.DownloadSpeed = ds.statService.GetDownloadSpeed(vd.GetUID(), pod)

		err = ds.uploaderService.Protect(ctx, pod, svc, ing)
		if err != nil {
			return reconcile.Result{}, err
		}
	case dv == nil:
		log.Info("Start import to PVC")

		err = ds.statService.CheckPod(pod)
		if err != nil {
			vd.Status.Phase = virtv2.DiskFailed

			switch {
			case errors.Is(err, service.ErrProvisioningFailed):
				condition.Status = metav1.ConditionFalse
				condition.Reason = vdcondition.ProvisioningFailed
				condition.Message = service.CapitalizeFirstLetter(err.Error() + ".")
				return reconcile.Result{}, nil
			default:
				return reconcile.Result{}, err
			}
		}

		vd.Status.Progress = "50%"
		vd.Status.DownloadSpeed = ds.statService.GetDownloadSpeed(vd.GetUID(), pod)

		if imageformat.IsISO(ds.statService.GetFormat(pod)) {
			setPhaseConditionToFailed(&condition, &vd.Status.Phase, ErrISOSourceNotSupported)
			return reconcile.Result{}, nil
		}

		var diskSize resource.Quantity
		diskSize, err = ds.getPVCSize(vd, pod)
		if err != nil {
			setPhaseConditionToFailed(&condition, &vd.Status.Phase, err)

			if errors.Is(err, service.ErrInsufficientPVCSize) {
				return reconcile.Result{}, nil
			}

			return reconcile.Result{}, err
		}

		source := ds.getSource(supgen, ds.statService.GetDVCRImageName(pod))

		err = ds.diskService.Start(ctx, diskSize, vd.Spec.PersistentVolumeClaim.StorageClass, source, vd, supgen)
		if updated, err := setPhaseConditionFromStorageError(err, vd, &condition); err != nil || updated {
			return reconcile.Result{}, err
		}

		vd.Status.Phase = virtv2.DiskProvisioning
		condition.Status = metav1.ConditionFalse
		condition.Reason = vdcondition.Provisioning
		condition.Message = "PVC Provisioner not found: create the new one."

		return reconcile.Result{Requeue: true}, nil
	case pvc == nil:
		vd.Status.Phase = virtv2.DiskProvisioning
		condition.Status = metav1.ConditionFalse
		condition.Reason = vdcondition.Provisioning
		condition.Message = "PVC not found: waiting for creation."
		return reconcile.Result{Requeue: true}, nil
	case ds.diskService.IsImportDone(dv, pvc):
		log.Info("Import has completed", "dvProgress", dv.Status.Progress, "dvPhase", dv.Status.Phase, "pvcPhase", pvc.Status.Phase)

		vd.Status.Phase = virtv2.DiskReady
		condition.Status = metav1.ConditionTrue
		condition.Reason = vdcondition.Ready
		condition.Message = ""

		vd.Status.Progress = "100%"
		vd.Status.Capacity = ds.diskService.GetCapacity(pvc)
		vd.Status.Target.PersistentVolumeClaim = dv.Status.ClaimName

		log.Info("Ready", "vd", vd.Name, "progress", vd.Status.Progress, "dv.phase", dv.Status.Phase)
	default:
		log.Info("Provisioning to PVC is in progress", "dvProgress", dv.Status.Progress, "dvPhase", dv.Status.Phase, "pvcPhase", pvc.Status.Phase)

		vd.Status.Progress = ds.diskService.GetProgress(dv, vd.Status.Progress, service.NewScaleOption(50, 100))
		vd.Status.Capacity = ds.diskService.GetCapacity(pvc)
		vd.Status.Target.PersistentVolumeClaim = dv.Status.ClaimName

		err = ds.diskService.Protect(ctx, vd, dv, pvc)
		if err != nil {
			return reconcile.Result{}, err
		}
		sc, err := ds.diskService.GetStorageClass(ctx, pvc.Spec.StorageClassName)
		if updated, err := setPhaseConditionFromStorageError(err, vd, &condition); err != nil || updated {
			return reconcile.Result{}, err
		}
		if err = setPhaseConditionForPVCProvisioningDisk(ctx, dv, vd, pvc, sc, &condition, ds.diskService); err != nil {
			return reconcile.Result{}, err
		}
		return reconcile.Result{}, nil
	}

	return reconcile.Result{Requeue: true}, nil
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

func (ds UploadDataSource) CleanUpSupplements(ctx context.Context, vd *virtv2.VirtualDisk) (reconcile.Result, error) {
	supgen := supplements.NewGenerator(common.VDShortName, vd.Name, vd.Namespace, vd.UID)

	uploaderRequeue, err := ds.uploaderService.CleanUpSupplements(ctx, supgen)
	if err != nil {
		return reconcile.Result{}, err
	}

	diskRequeue, err := ds.diskService.CleanUpSupplements(ctx, supgen)
	if err != nil {
		return reconcile.Result{}, err
	}

	return reconcile.Result{Requeue: uploaderRequeue || diskRequeue}, nil
}

func (ds UploadDataSource) Validate(_ context.Context, _ *virtv2.VirtualDisk) error {
	return nil
}

func (ds UploadDataSource) Name() string {
	return uploadDataSource
}

func (ds UploadDataSource) getEnvSettings(vd *virtv2.VirtualDisk, supgen *supplements.Generator) *uploader.Settings {
	var settings uploader.Settings

	uploader.ApplyDVCRDestinationSettings(
		&settings,
		ds.dvcrSettings,
		supgen,
		ds.dvcrSettings.RegistryImageForVD(vd),
	)

	return &settings
}

func (ds UploadDataSource) getSource(sup *supplements.Generator, dvcrSourceImageName string) *cdiv1.DataVolumeSource {
	// The image was preloaded from source into dvcr.
	// We can't use the same data source a second time, but we can set dvcr as the data source.
	// Use DV name for the Secret with DVCR auth and the ConfigMap with DVCR CA Bundle.
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
		return resource.Quantity{}, fmt.Errorf("failed to parse unpacked bytes %s: %w", ds.statService.GetSize(pod).UnpackedBytes, err)
	}

	return service.GetValidatedPVCSize(vd.Spec.PersistentVolumeClaim.Size, unpackedSize)
}
