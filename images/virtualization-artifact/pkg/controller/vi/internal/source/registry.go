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
	"k8s.io/apimachinery/pkg/types"
	cdiv1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/common"
	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/datasource"
	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	podutil "github.com/deckhouse/virtualization-controller/pkg/common/pod"
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

const registryDataSource = "registry"

type RegistryDataSource struct {
	statService         Stat
	importerService     Importer
	dvcrSettings        *dvcr.Settings
	client              client.Client
	diskService         *service.DiskService
	storageClassService *service.VirtualImageStorageClassService
	recorder            eventrecord.EventRecorderLogger
}

func NewRegistryDataSource(
	recorder eventrecord.EventRecorderLogger,
	statService Stat,
	importerService Importer,
	dvcrSettings *dvcr.Settings,
	client client.Client,
	diskService *service.DiskService,
	storageClassService *service.VirtualImageStorageClassService,
) *RegistryDataSource {
	return &RegistryDataSource{
		statService:         statService,
		importerService:     importerService,
		dvcrSettings:        dvcrSettings,
		client:              client,
		diskService:         diskService,
		storageClassService: storageClassService,
		recorder:            recorder,
	}
}

func (ds RegistryDataSource) StoreToPVC(ctx context.Context, vi *virtv2.VirtualImage) (reconcile.Result, error) {
	log, ctx := logger.GetDataSourceContext(ctx, registryDataSource)

	condition, _ := conditions.GetCondition(vicondition.ReadyType, vi.Status.Conditions)
	cb := conditions.NewConditionBuilder(vicondition.ReadyType).Generation(vi.Generation)
	defer func() { conditions.SetCondition(cb, &vi.Status.Conditions) }()

	supgen := supplements.NewGenerator(annotations.VIShortName, vi.Name, vi.Namespace, vi.UID)
	pod, err := ds.importerService.GetPod(ctx, supgen)
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

	clusterDefaultSC, _ := ds.diskService.GetDefaultStorageClass(ctx)
	sc, err := ds.storageClassService.GetStorageClass(vi.Spec.PersistentVolumeClaim.StorageClass, clusterDefaultSC)
	if updated, err := setConditionFromStorageClassError(err, cb); err != nil || updated {
		return reconcile.Result{}, err
	}

	var quotaNotExceededCondition *metav1.Condition
	if dv != nil {
		quotaNotExceededCondition = getDVNotExceededCondition(dv.Status.Conditions)
	}

	switch {
	case isDiskProvisioningFinished(condition):
		log.Info("Disk provisioning finished: clean up")

		setPhaseConditionForFinishedImage(pvc, cb, &vi.Status.Phase, supgen)

		// Protect Ready Disk and underlying PVC.
		err = ds.diskService.Protect(ctx, vi, nil, pvc)
		if err != nil {
			return reconcile.Result{}, err
		}

		// Unprotect import time supplements to delete them later.
		err = ds.importerService.Unprotect(ctx, pod)
		if err != nil {
			return reconcile.Result{}, err
		}

		err = ds.diskService.Unprotect(ctx, dv)
		if err != nil {
			return reconcile.Result{}, err
		}

		return CleanUpSupplements(ctx, vi, ds)
	case object.AnyTerminating(pod, dv, pvc):
		log.Info("Waiting for supplements to be terminated")
	case pod == nil:
		log.Info("Start import to DVCR")

		vi.Status.Progress = "0%"

		envSettings := ds.getEnvSettings(vi, supgen)
		err = ds.importerService.Start(ctx, envSettings, vi, supgen, datasource.NewCABundleForVMI(vi.GetNamespace(), vi.Spec.DataSource))
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

		return reconcile.Result{Requeue: true}, nil
	case !podutil.IsPodComplete(pod):
		log.Info("Provisioning to DVCR is in progress", "podPhase", pod.Status.Phase)

		err = ds.statService.CheckPod(pod)
		if err != nil {
			return reconcile.Result{}, setPhaseConditionFromPodError(cb, vi, err)
		}

		vi.Status.Phase = virtv2.ImageProvisioning
		cb.
			Status(metav1.ConditionFalse).
			Reason(vicondition.Provisioning).
			Message("DVCR Provisioner not found: create the new one.")

		vi.Status.Progress = ds.statService.GetProgress(vi.GetUID(), pod, vi.Status.Progress, service.NewScaleOption(0, 50))

		err = ds.importerService.Protect(ctx, pod)
		if err != nil {
			return reconcile.Result{}, err
		}
	case dv == nil:
		log.Info("Start import to PVC")

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

		vi.Status.Progress = "50.0%"

		var diskSize resource.Quantity
		diskSize, err = ds.getPVCSize(pod)
		if err != nil {
			setPhaseConditionToFailed(cb, &vi.Status.Phase, err)

			if errors.Is(err, service.ErrInsufficientPVCSize) {
				return reconcile.Result{}, nil
			}

			return reconcile.Result{}, err
		}

		source := ds.getSource(supgen, ds.statService.GetDVCRImageName(pod))

		err = ds.diskService.StartImmediate(ctx, diskSize, sc, source, vi, supgen)
		if updated, err := setPhaseConditionFromStorageError(err, vi, cb); err != nil || updated {
			return reconcile.Result{}, err
		}

		vi.Status.Phase = virtv2.ImageProvisioning
		cb.
			Status(metav1.ConditionFalse).
			Reason(vicondition.Provisioning).
			Message("PVC Provisioner not found: create the new one.")

		return reconcile.Result{Requeue: true}, nil
	case quotaNotExceededCondition != nil && quotaNotExceededCondition.Status == metav1.ConditionFalse:
		cb.
			Status(metav1.ConditionFalse).
			Reason(vicondition.QuotaExceeded).
			Message(quotaNotExceededCondition.Message)
		return reconcile.Result{}, nil
	case pvc == nil:
		vi.Status.Phase = virtv2.ImageProvisioning
		cb.
			Status(metav1.ConditionFalse).
			Reason(vicondition.Provisioning).
			Message("PVC not found: waiting for creation.")
		return reconcile.Result{Requeue: true}, nil
	case ds.diskService.IsImportDone(dv, pvc):
		log.Info("Import has completed", "dvProgress", dv.Status.Progress, "dvPhase", dv.Status.Phase, "pvcPhase", pvc.Status.Phase)

		vi.Status.Phase = virtv2.ImageReady
		cb.
			Status(metav1.ConditionTrue).
			Reason(vicondition.Ready).
			Message("")

		vi.Status.Progress = "100%"
		vi.Status.Size = ds.statService.GetSize(pod)
		vi.Status.DownloadSpeed = ds.statService.GetDownloadSpeed(vi.GetUID(), pod)
		vi.Status.Target.PersistentVolumeClaim = dv.Status.ClaimName
	default:
		log.Info("Provisioning to PVC is in progress", "dvProgress", dv.Status.Progress, "dvPhase", dv.Status.Phase, "pvcPhase", pvc.Status.Phase)

		vi.Status.Progress = ds.diskService.GetProgress(dv, vi.Status.Progress, service.NewScaleOption(50, 100))
		vi.Status.Target.PersistentVolumeClaim = dv.Status.ClaimName

		err = ds.diskService.Protect(ctx, vi, dv, pvc)
		if err != nil {
			return reconcile.Result{}, err
		}

		err = setPhaseConditionForPVCProvisioningImage(ctx, dv, vi, pvc, cb, ds.diskService)
		if err != nil {
			return reconcile.Result{}, err
		}

		return reconcile.Result{}, nil
	}

	return reconcile.Result{Requeue: true}, nil
}

func (ds RegistryDataSource) StoreToDVCR(ctx context.Context, vi *virtv2.VirtualImage) (reconcile.Result, error) {
	log, ctx := logger.GetDataSourceContext(ctx, "registry")

	condition, _ := conditions.GetCondition(vicondition.ReadyType, vi.Status.Conditions)
	cb := conditions.NewConditionBuilder(vicondition.ReadyType).Generation(vi.Generation)
	defer func() { conditions.SetCondition(cb, &vi.Status.Conditions) }()

	supgen := supplements.NewGenerator(annotations.VIShortName, vi.Name, vi.Namespace, vi.UID)
	pod, err := ds.importerService.GetPod(ctx, supgen)
	if err != nil {
		return reconcile.Result{}, err
	}

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
	case object.IsTerminating(pod):
		vi.Status.Phase = virtv2.ImagePending

		log.Info("Cleaning up...")
	case pod == nil:
		vi.Status.Progress = "0%"

		envSettings := ds.getEnvSettings(vi, supgen)
		err = ds.importerService.Start(ctx, envSettings, vi, supgen, datasource.NewCABundleForVMI(vi.GetNamespace(), vi.Spec.DataSource))
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

		ds.recorder.Event(
			vi,
			corev1.EventTypeNormal,
			virtv2.ReasonDataSourceSyncCompleted,
			"The Registry DataSource import has completed",
		)

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
			return reconcile.Result{}, setPhaseConditionFromPodError(cb, vi, err)
		}

		cb.
			Status(metav1.ConditionFalse).
			Reason(vicondition.Provisioning).
			Message("Import is in the process of provisioning to DVCR.")

		vi.Status.Phase = virtv2.ImageProvisioning
		vi.Status.Progress = "0%"
		vi.Status.Target.RegistryURL = ds.statService.GetDVCRImageName(pod)

		log.Info("Provisioning...", "progress", vi.Status.Progress, "pod.phase", pod.Status.Phase)
	}

	return reconcile.Result{Requeue: true}, nil
}

func (ds RegistryDataSource) CleanUp(ctx context.Context, vi *virtv2.VirtualImage) (bool, error) {
	supgen := supplements.NewGenerator(annotations.VIShortName, vi.Name, vi.Namespace, vi.UID)

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

func (ds RegistryDataSource) Validate(ctx context.Context, vi *virtv2.VirtualImage) error {
	if vi.Spec.DataSource.ContainerImage.ImagePullSecret.Name != "" {
		secretName := types.NamespacedName{
			Namespace: vi.GetNamespace(),
			Name:      vi.Spec.DataSource.ContainerImage.ImagePullSecret.Name,
		}
		secret, err := object.FetchObject[*corev1.Secret](ctx, secretName, ds.client, &corev1.Secret{})
		if err != nil {
			return fmt.Errorf("failed to get secret %s: %w", secretName, err)
		}

		if secret == nil {
			return ErrSecretNotFound
		}
	}

	return nil
}

func (ds RegistryDataSource) getEnvSettings(vi *virtv2.VirtualImage, supgen *supplements.Generator) *importer.Settings {
	var settings importer.Settings

	containerImage := &datasource.ContainerRegistry{
		Image: vi.Spec.DataSource.ContainerImage.Image,
		ImagePullSecret: types.NamespacedName{
			Name:      vi.Spec.DataSource.ContainerImage.ImagePullSecret.Name,
			Namespace: vi.GetNamespace(),
		},
		CABundle: vi.Spec.DataSource.ContainerImage.CABundle,
	}
	importer.ApplyRegistrySourceSettings(&settings, containerImage, supgen)
	importer.ApplyDVCRDestinationSettings(
		&settings,
		ds.dvcrSettings,
		supgen,
		ds.dvcrSettings.RegistryImageForVI(vi),
	)

	return &settings
}

func (ds RegistryDataSource) CleanUpSupplements(ctx context.Context, vi *virtv2.VirtualImage) (reconcile.Result, error) {
	supgen := supplements.NewGenerator(annotations.VIShortName, vi.Name, vi.Namespace, vi.UID)

	importerRequeue, err := ds.importerService.CleanUpSupplements(ctx, supgen)
	if err != nil {
		return reconcile.Result{}, err
	}

	diskRequeue, err := ds.diskService.CleanUpSupplements(ctx, supgen)
	if err != nil {
		return reconcile.Result{}, err
	}

	return reconcile.Result{Requeue: importerRequeue || diskRequeue}, nil
}

func (ds RegistryDataSource) getPVCSize(pod *corev1.Pod) (resource.Quantity, error) {
	// Get size from the importer Pod to detect if specified PVC size is enough.
	unpackedSize, err := resource.ParseQuantity(ds.statService.GetSize(pod).UnpackedBytes)
	if err != nil {
		return resource.Quantity{}, fmt.Errorf("failed to parse unpacked bytes %s: %w", ds.statService.GetSize(pod).UnpackedBytes, err)
	}

	if unpackedSize.IsZero() {
		return resource.Quantity{}, errors.New("got zero unpacked size from data source")
	}

	return service.GetValidatedPVCSize(&unpackedSize, unpackedSize)
}

func (ds RegistryDataSource) getSource(sup *supplements.Generator, dvcrSourceImageName string) *cdiv1.DataVolumeSource {
	// The image was preloaded from source into dvcr.
	// We can't use the same data source a second time, but we can set dvcr as the data source.
	// Use DV name for the Secret with DVCR auth and the ConfigMap with DVCR CA Bundle.
	url := common.DockerRegistrySchemePrefix + dvcrSourceImageName
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
