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
	"github.com/deckhouse/virtualization-controller/pkg/common/imageformat"
	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	podutil "github.com/deckhouse/virtualization-controller/pkg/common/pod"
	"github.com/deckhouse/virtualization-controller/pkg/common/provisioner"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/importer"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/supplements"
	"github.com/deckhouse/virtualization-controller/pkg/dvcr"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdcondition"
)

const registryDataSource = "registry"

type RegistryDataSource struct {
	statService         *service.StatService
	importerService     *service.ImporterService
	diskService         *service.DiskService
	dvcrSettings        *dvcr.Settings
	client              client.Client
	storageClassService *service.VirtualDiskStorageClassService
	recorder            eventrecord.EventRecorderLogger
}

func NewRegistryDataSource(
	recorder eventrecord.EventRecorderLogger,
	statService *service.StatService,
	importerService *service.ImporterService,
	diskService *service.DiskService,
	dvcrSettings *dvcr.Settings,
	client client.Client,
	storageClassService *service.VirtualDiskStorageClassService,
) *RegistryDataSource {
	return &RegistryDataSource{
		statService:         statService,
		importerService:     importerService,
		diskService:         diskService,
		dvcrSettings:        dvcrSettings,
		client:              client,
		storageClassService: storageClassService,
		recorder:            recorder,
	}
}

func (ds RegistryDataSource) Sync(ctx context.Context, vd *virtv2.VirtualDisk) (reconcile.Result, error) {
	log, ctx := logger.GetDataSourceContext(ctx, registryDataSource)

	condition, _ := conditions.GetCondition(vdcondition.ReadyType, vd.Status.Conditions)
	cb := conditions.NewConditionBuilder(vdcondition.ReadyType).Generation(vd.Generation)
	defer func() { conditions.SetCondition(cb, &vd.Status.Conditions) }()

	supgen := supplements.NewGenerator(annotations.VDShortName, vd.Name, vd.Namespace, vd.UID)
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
	sc, err := ds.storageClassService.GetStorageClass(vd.Spec.PersistentVolumeClaim.StorageClass, clusterDefaultSC)
	if updated, err := setConditionFromStorageClassError(err, cb); err != nil || updated {
		return reconcile.Result{}, err
	}

	var quotaNotExceededCondition *metav1.Condition
	if dv != nil {
		quotaNotExceededCondition = getDVNotExceededCondition(dv.Status.Conditions)
	}

	switch {
	case isDiskProvisioningFinished(condition):
		log.Debug("Disk provisioning finished: clean up")

		setPhaseConditionForFinishedDisk(pvc, cb, &vd.Status.Phase, supgen)

		// Protect Ready Disk and underlying PVC.
		err = ds.diskService.Protect(ctx, vd, nil, pvc)
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

		return CleanUpSupplements(ctx, vd, ds)
	case object.AnyTerminating(pod, dv, pvc):
		log.Info("Waiting for supplements to be terminated")
	case pod == nil:
		ds.recorder.Event(
			vd,
			corev1.EventTypeNormal,
			v1alpha2.ReasonDataSourceSyncStarted,
			"The Registry DataSource import to DVCR has started",
		)

		vd.Status.Progress = "0%"

		envSettings := ds.getEnvSettings(vd, supgen)

		var nodePlacement *provisioner.NodePlacement
		nodePlacement, err = getNodePlacement(ctx, ds.client, vd)
		if err != nil {
			setPhaseConditionToFailed(cb, &vd.Status.Phase, fmt.Errorf("unexpected error: %w", err))
			return reconcile.Result{}, fmt.Errorf("failed to get importer tolerations: %w", err)
		}

		err = ds.importerService.Start(ctx, envSettings, vd, supgen, datasource.NewCABundleForVMD(vd.GetNamespace(), vd.Spec.DataSource), service.WithNodePlacement(nodePlacement))
		switch {
		case err == nil:
			// OK.
		case common.ErrQuotaExceeded(err):
			ds.recorder.Event(vd, corev1.EventTypeWarning, virtv2.ReasonDataSourceQuotaExceeded, "DataSource quota exceed")
			return setQuotaExceededPhaseCondition(cb, &vd.Status.Phase, err, vd.CreationTimestamp), nil
		default:
			setPhaseConditionToFailed(cb, &vd.Status.Phase, fmt.Errorf("unexpected error: %w", err))
			return reconcile.Result{}, err
		}

		vd.Status.Phase = virtv2.DiskPending
		cb.
			Status(metav1.ConditionFalse).
			Reason(vdcondition.WaitForUserUpload).
			Message("DVCR Provisioner not found: create the new one.")

		return reconcile.Result{Requeue: true}, nil
	case !podutil.IsPodComplete(pod):
		log.Info("Provisioning to DVCR is in progress", "podPhase", pod.Status.Phase)

		err = ds.statService.CheckPod(pod)
		if err != nil {
			return reconcile.Result{}, setPhaseConditionFromPodError(ctx, err, pod, vd, cb, ds.client)
		}

		vd.Status.Phase = virtv2.DiskProvisioning
		cb.
			Status(metav1.ConditionFalse).
			Reason(vdcondition.Provisioning).
			Message("DVCR Provisioner not found: create the new one.")

		vd.Status.Progress = ds.statService.GetProgress(vd.GetUID(), pod, vd.Status.Progress, service.NewScaleOption(0, 50))

		err = ds.importerService.Protect(ctx, pod)
		if err != nil {
			return reconcile.Result{}, err
		}
	case dv == nil:
		ds.recorder.Event(
			vd,
			corev1.EventTypeNormal,
			v1alpha2.ReasonDataSourceSyncStarted,
			"The Registry DataSource import to PVC has started",
		)

		err = ds.statService.CheckPod(pod)
		if err != nil {
			vd.Status.Phase = virtv2.DiskFailed

			switch {
			case errors.Is(err, service.ErrProvisioningFailed):
				ds.recorder.Event(vd, corev1.EventTypeWarning, virtv2.ReasonDataSourceDiskProvisioningFailed, "Disk provisioning failed")
				cb.
					Status(metav1.ConditionFalse).
					Reason(vdcondition.ProvisioningFailed).
					Message(service.CapitalizeFirstLetter(err.Error() + "."))
				return reconcile.Result{}, nil
			default:
				return reconcile.Result{}, err
			}
		}

		vd.Status.Progress = "50%"

		if imageformat.IsISO(ds.statService.GetFormat(pod)) {
			setPhaseConditionToFailed(cb, &vd.Status.Phase, ErrISOSourceNotSupported)
			return reconcile.Result{}, nil
		}

		var diskSize resource.Quantity
		diskSize, err = ds.getPVCSize(vd, pod)
		if err != nil {
			setPhaseConditionToFailed(cb, &vd.Status.Phase, err)

			if errors.Is(err, service.ErrInsufficientPVCSize) {
				return reconcile.Result{}, nil
			}

			return reconcile.Result{}, err
		}

		source := ds.getSource(supgen, ds.statService.GetDVCRImageName(pod))

		var nodePlacement *provisioner.NodePlacement
		nodePlacement, err = getNodePlacement(ctx, ds.client, vd)
		if err != nil {
			setPhaseConditionToFailed(cb, &vd.Status.Phase, fmt.Errorf("unexpected error: %w", err))
			return reconcile.Result{}, fmt.Errorf("failed to get importer tolerations: %w", err)
		}

		err = ds.diskService.Start(ctx, diskSize, sc, source, vd, supgen, service.WithNodePlacement(nodePlacement))
		if updated, err := setPhaseConditionFromStorageError(err, vd, cb); err != nil || updated {
			return reconcile.Result{}, err
		}
		vd.Status.Phase = virtv2.DiskProvisioning
		cb.
			Status(metav1.ConditionFalse).
			Reason(vdcondition.Provisioning).
			Message("PVC Provisioner not found: create the new one.")

		return reconcile.Result{Requeue: true}, nil
	case quotaNotExceededCondition != nil && quotaNotExceededCondition.Status == metav1.ConditionFalse:
		vd.Status.Phase = virtv2.DiskPending
		cb.
			Status(metav1.ConditionFalse).
			Reason(vdcondition.QuotaExceeded).
			Message(quotaNotExceededCondition.Message)
		return reconcile.Result{}, nil
	case pvc == nil:
		vd.Status.Phase = virtv2.DiskProvisioning
		cb.
			Status(metav1.ConditionFalse).
			Reason(vdcondition.Provisioning).
			Message("PVC not found: waiting for creation.")
		return reconcile.Result{Requeue: true}, nil
	case ds.diskService.IsImportDone(dv, pvc):
		log.Info("Import has completed", "dvProgress", dv.Status.Progress, "dvPhase", dv.Status.Phase, "pvcPhase", pvc.Status.Phase)

		ds.recorder.Event(
			vd,
			corev1.EventTypeNormal,
			v1alpha2.ReasonDataSourceSyncCompleted,
			"The Registry DataSource import has completed",
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

		vd.Status.Progress = ds.diskService.GetProgress(dv, vd.Status.Progress, service.NewScaleOption(50, 100))
		vd.Status.Capacity = ds.diskService.GetCapacity(pvc)
		vd.Status.Target.PersistentVolumeClaim = dv.Status.ClaimName

		err = ds.diskService.Protect(ctx, vd, dv, pvc)
		if err != nil {
			return reconcile.Result{}, err
		}
		sc, err := ds.diskService.GetStorageClass(ctx, pvc.Spec.StorageClassName)
		if updated, err := setPhaseConditionFromStorageError(err, vd, cb); err != nil || updated {
			return reconcile.Result{}, err
		}
		if err = setPhaseConditionForPVCProvisioningDisk(ctx, dv, vd, pvc, sc, cb, ds.diskService); err != nil {
			return reconcile.Result{}, err
		}
		return reconcile.Result{}, nil
	}

	return reconcile.Result{Requeue: true}, nil
}

func (ds RegistryDataSource) CleanUp(ctx context.Context, vd *virtv2.VirtualDisk) (bool, error) {
	supgen := supplements.NewGenerator(annotations.VDShortName, vd.Name, vd.Namespace, vd.UID)

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

func (ds RegistryDataSource) CleanUpSupplements(ctx context.Context, vd *virtv2.VirtualDisk) (reconcile.Result, error) {
	supgen := supplements.NewGenerator(annotations.VDShortName, vd.Name, vd.Namespace, vd.UID)

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

func (ds RegistryDataSource) Validate(ctx context.Context, vd *virtv2.VirtualDisk) error {
	if vd.Spec.DataSource == nil || vd.Spec.DataSource.ContainerImage == nil {
		return errors.New("container image missed for data source")
	}

	if vd.Spec.DataSource.ContainerImage.ImagePullSecret.Name != "" {
		secretName := types.NamespacedName{
			Namespace: vd.GetNamespace(),
			Name:      vd.Spec.DataSource.ContainerImage.ImagePullSecret.Name,
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

func (ds RegistryDataSource) Name() string {
	return registryDataSource
}

func (ds RegistryDataSource) getEnvSettings(vd *virtv2.VirtualDisk, supgen *supplements.Generator) *importer.Settings {
	var settings importer.Settings

	containerImage := &datasource.ContainerRegistry{
		Image: vd.Spec.DataSource.ContainerImage.Image,
		ImagePullSecret: types.NamespacedName{
			Name:      vd.Spec.DataSource.ContainerImage.ImagePullSecret.Name,
			Namespace: vd.GetNamespace(),
		},
	}
	importer.ApplyRegistrySourceSettings(&settings, containerImage, supgen)
	importer.ApplyDVCRDestinationSettings(
		&settings,
		ds.dvcrSettings,
		supgen,
		ds.dvcrSettings.RegistryImageForVD(vd),
	)

	return &settings
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

func (ds RegistryDataSource) getPVCSize(vd *virtv2.VirtualDisk, pod *corev1.Pod) (resource.Quantity, error) {
	// Get size from the importer Pod to detect if specified PVC size is enough.
	unpackedSize, err := resource.ParseQuantity(ds.statService.GetSize(pod).UnpackedBytes)
	if err != nil {
		return resource.Quantity{}, fmt.Errorf("failed to parse unpacked bytes %s: %w", ds.statService.GetSize(pod).UnpackedBytes, err)
	}

	return service.GetValidatedPVCSize(vd.Spec.PersistentVolumeClaim.Size, unpackedSize)
}
