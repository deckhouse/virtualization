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
	"k8s.io/utils/ptr"
	cdiv1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/common"
	"github.com/deckhouse/virtualization-controller/pkg/common/datasource"
	"github.com/deckhouse/virtualization-controller/pkg/common/imageformat"
	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	podutil "github.com/deckhouse/virtualization-controller/pkg/common/pod"
	"github.com/deckhouse/virtualization-controller/pkg/common/provisioner"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/importer"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/supplements"
	vdsupplements "github.com/deckhouse/virtualization-controller/pkg/controller/vd/internal/supplements"
	"github.com/deckhouse/virtualization-controller/pkg/dvcr"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdcondition"
)

const httpDataSource = "http"

type HTTPDataSource struct {
	statService     *service.StatService
	importerService *service.ImporterService
	diskService     *service.DiskService
	dvcrSettings    *dvcr.Settings
	client          client.Client
	recorder        eventrecord.EventRecorderLogger
}

func NewHTTPDataSource(
	recorder eventrecord.EventRecorderLogger,
	statService *service.StatService,
	importerService *service.ImporterService,
	diskService *service.DiskService,
	dvcrSettings *dvcr.Settings,
	client client.Client,
) *HTTPDataSource {
	return &HTTPDataSource{
		statService:     statService,
		importerService: importerService,
		diskService:     diskService,
		dvcrSettings:    dvcrSettings,
		client:          client,
		recorder:        recorder,
	}
}

func (ds HTTPDataSource) Sync(ctx context.Context, vd *v1alpha2.VirtualDisk) (reconcile.Result, error) {
	log, ctx := logger.GetDataSourceContext(ctx, httpDataSource)

	condition, _ := conditions.GetCondition(vdcondition.ReadyType, vd.Status.Conditions)
	cb := conditions.NewConditionBuilder(vdcondition.ReadyType).Generation(vd.Generation)
	defer func() { conditions.SetCondition(cb, &vd.Status.Conditions) }()

	supgen := vdsupplements.NewGenerator(vd)
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

	var dvQuotaNotExceededCondition *cdiv1.DataVolumeCondition
	var dvRunningCondition *cdiv1.DataVolumeCondition
	if dv != nil {
		dvQuotaNotExceededCondition = service.GetDataVolumeCondition(DVQoutaNotExceededConditionType, dv.Status.Conditions)
		dvRunningCondition = service.GetDataVolumeCondition(DVRunningConditionType, dv.Status.Conditions)
		vdsupplements.SetPVCName(vd, dv.Status.ClaimName)
	}

	var sc *storagev1.StorageClass
	sc, err = ds.diskService.GetStorageClass(ctx, vd.Status.StorageClassName)
	if err != nil {
		return reconcile.Result{}, err
	}

	switch {
	case IsDiskProvisioningFinished(condition):
		log.Debug("Disk provisioning finished: clean up")

		setPhaseConditionForFinishedDisk(pvc, cb, &vd.Status.Phase, supgen)

		// Protect Ready Disk and underlying PVC.
		err = ds.diskService.Protect(ctx, supgen, vd, nil, pvc)
		if err != nil {
			return reconcile.Result{}, err
		}

		// Unprotect import time supplements to delete them later.
		err = ds.importerService.Unprotect(ctx, pod, supgen)
		if err != nil {
			return reconcile.Result{}, err
		}

		err = ds.diskService.Unprotect(ctx, supgen, dv)
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
			"The HTTP DataSource import to DVCR has started",
		)

		vd.Status.Progress = "0%"

		envSettings := ds.getEnvSettings(vd, supgen)

		err = ds.importerService.Start(
			ctx, envSettings, vd, supgen,
			datasource.NewCABundleForVMD(vd.GetNamespace(), vd.Spec.DataSource),
			service.WithSystemNodeToleration(),
		)
		switch {
		case err == nil:
			// OK.
		case common.ErrQuotaExceeded(err):
			ds.recorder.Event(vd, corev1.EventTypeWarning, v1alpha2.ReasonDataSourceQuotaExceeded, "DataSource quota exceed")
			return setQuotaExceededPhaseCondition(cb, &vd.Status.Phase, err, vd.CreationTimestamp), nil
		default:
			setPhaseConditionToFailed(cb, &vd.Status.Phase, fmt.Errorf("unexpected error: %w", err))
			return reconcile.Result{}, err
		}

		vd.Status.Phase = v1alpha2.DiskProvisioning
		cb.
			Status(metav1.ConditionFalse).
			Reason(vdcondition.Provisioning).
			Message("DVCR Provisioner not found: create the new one.")

		return reconcile.Result{RequeueAfter: time.Second}, nil
	case !podutil.IsPodComplete(pod):
		log.Info("Provisioning to DVCR is in progress", "podPhase", pod.Status.Phase)

		err = ds.statService.CheckPod(pod)
		if err != nil {
			return reconcile.Result{}, setPhaseConditionFromPodError(ctx, err, pod, vd, cb, ds.client)
		}

		err = ds.importerService.Protect(ctx, pod, supgen)
		if err != nil {
			return reconcile.Result{}, err
		}

		vd.Status.Phase = v1alpha2.DiskProvisioning
		cb.
			Status(metav1.ConditionFalse).
			Reason(vdcondition.Provisioning).
			Message("Import is in the process of provisioning to DVCR.")

		vd.Status.Progress = ds.statService.GetProgress(vd.GetUID(), pod, vd.Status.Progress, service.NewScaleOption(0, 50))
		vd.Status.DownloadSpeed = ds.statService.GetDownloadSpeed(vd.GetUID(), pod)
	case dv == nil:
		if isStorageClassWFFC(sc) && len(vd.Status.AttachedToVirtualMachines) != 1 {
			vd.Status.Progress = "50%"
			vd.Status.Phase = v1alpha2.DiskWaitForFirstConsumer
			return reconcile.Result{}, nil
		}

		ds.recorder.Event(
			vd,
			corev1.EventTypeNormal,
			v1alpha2.ReasonDataSourceSyncStarted,
			"The HTTP DataSource import to PVC has started",
		)

		err = ds.statService.CheckPod(pod)
		if err != nil {
			vd.Status.Phase = v1alpha2.DiskFailed

			switch {
			case errors.Is(err, service.ErrProvisioningFailed):
				ds.recorder.Event(vd, corev1.EventTypeWarning, v1alpha2.ReasonDataSourceDiskProvisioningFailed, "Disk provisioning failed")
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
		vd.Status.DownloadSpeed = ds.statService.GetDownloadSpeed(vd.GetUID(), pod)

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

		vd.Status.Phase = v1alpha2.DiskProvisioning
		cb.
			Status(metav1.ConditionFalse).
			Reason(vdcondition.Provisioning).
			Message("PVC Provisioner not found: create the new one.")

		return reconcile.Result{RequeueAfter: time.Second}, nil
	case dvQuotaNotExceededCondition != nil && dvQuotaNotExceededCondition.Status == corev1.ConditionFalse:
		vd.Status.Phase = v1alpha2.DiskPending
		if dv.Status.ClaimName != "" && isStorageClassWFFC(sc) {
			vd.Status.Phase = v1alpha2.DiskWaitForFirstConsumer
		}
		cb.
			Status(metav1.ConditionFalse).
			Reason(vdcondition.QuotaExceeded).
			Message(dvQuotaNotExceededCondition.Message)
		return reconcile.Result{}, nil
	case dvRunningCondition != nil && dvRunningCondition.Status != corev1.ConditionTrue && dvRunningCondition.Reason == DVImagePullFailedReason:
		vd.Status.Phase = v1alpha2.DiskPending
		if dv.Status.ClaimName != "" && isStorageClassWFFC(sc) {
			vd.Status.Phase = v1alpha2.DiskWaitForFirstConsumer
		}
		cb.
			Status(metav1.ConditionFalse).
			Reason(vdcondition.ImagePullFailed).
			Message(dvRunningCondition.Message)
		ds.recorder.Event(vd, corev1.EventTypeWarning, vdcondition.ImagePullFailed.String(), dvRunningCondition.Message)
		return reconcile.Result{}, nil
	case pvc == nil:
		vd.Status.Phase = v1alpha2.DiskProvisioning
		cb.
			Status(metav1.ConditionFalse).
			Reason(vdcondition.Provisioning).
			Message("PVC not found: waiting for creation.")
		return reconcile.Result{RequeueAfter: time.Second}, nil
	case ds.diskService.IsImportDone(dv, pvc):
		log.Info("Import has completed", "dvProgress", dv.Status.Progress, "dvPhase", dv.Status.Phase, "pvcPhase", pvc.Status.Phase)

		ds.recorder.Event(
			vd,
			corev1.EventTypeNormal,
			v1alpha2.ReasonDataSourceSyncCompleted,
			"The HTTP DataSource import has completed",
		)

		vd.Status.Phase = v1alpha2.DiskReady
		cb.
			Status(metav1.ConditionTrue).
			Reason(vdcondition.Ready).
			Message("")

		vd.Status.Progress = "100%"
		vd.Status.Capacity = ds.diskService.GetCapacity(pvc)
	default:
		log.Info("Provisioning to PVC is in progress", "dvProgress", dv.Status.Progress, "dvPhase", dv.Status.Phase, "pvcPhase", pvc.Status.Phase)

		err = ds.diskService.CheckProvisioning(ctx, pvc)
		if err != nil {
			return reconcile.Result{}, setPhaseConditionFromProvisioningError(ctx, err, cb, vd, dv, ds.diskService, ds.client)
		}

		vd.Status.Progress = ds.diskService.GetProgress(dv, vd.Status.Progress, service.NewScaleOption(50, 100))
		vd.Status.Capacity = ds.diskService.GetCapacity(pvc)

		err = ds.diskService.Protect(ctx, supgen, vd, dv, pvc)
		if err != nil {
			return reconcile.Result{}, err
		}

		var sc *storagev1.StorageClass
		sc, err = ds.diskService.GetStorageClass(ctx, ptr.Deref(pvc.Spec.StorageClassName, ""))
		if err != nil {
			return reconcile.Result{}, err
		}

		if err = setPhaseConditionForPVCProvisioningDisk(ctx, dv, vd, pvc, sc, cb, ds.diskService); err != nil {
			return reconcile.Result{}, err
		}

		return reconcile.Result{}, nil
	}

	return reconcile.Result{RequeueAfter: time.Second}, nil
}

func (ds HTTPDataSource) CleanUp(ctx context.Context, vd *v1alpha2.VirtualDisk) (bool, error) {
	supgen := vdsupplements.NewGenerator(vd)

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

func (ds HTTPDataSource) Validate(_ context.Context, _ *v1alpha2.VirtualDisk) error {
	return nil
}

func (ds HTTPDataSource) CleanUpSupplements(ctx context.Context, vd *v1alpha2.VirtualDisk) (reconcile.Result, error) {
	supgen := vdsupplements.NewGenerator(vd)

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

func (ds HTTPDataSource) Name() string {
	return httpDataSource
}

func (ds HTTPDataSource) getEnvSettings(vd *v1alpha2.VirtualDisk, supgen supplements.Generator) *importer.Settings {
	var settings importer.Settings

	importer.ApplyHTTPSourceSettings(&settings, vd.Spec.DataSource.HTTP, supgen)
	importer.ApplyDVCRDestinationSettings(
		&settings,
		ds.dvcrSettings,
		supgen,
		ds.dvcrSettings.RegistryImageForVD(vd),
	)

	return &settings
}

func (ds HTTPDataSource) getSource(sup supplements.Generator, dvcrSourceImageName string) *cdiv1.DataVolumeSource {
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

func (ds HTTPDataSource) getPVCSize(vd *v1alpha2.VirtualDisk, pod *corev1.Pod) (resource.Quantity, error) {
	// Get size from the importer Pod to detect if specified PVC size is enough.
	unpackedSize, err := resource.ParseQuantity(ds.statService.GetSize(pod).UnpackedBytes)
	if err != nil {
		return resource.Quantity{}, fmt.Errorf("failed to parse unpacked bytes %s: %w", ds.statService.GetSize(pod).UnpackedBytes, err)
	}

	return service.GetValidatedPVCSize(vd.Spec.PersistentVolumeClaim.Size, unpackedSize)
}
