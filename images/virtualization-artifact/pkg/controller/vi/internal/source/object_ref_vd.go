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
	"strconv"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/common"
	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/datasource"
	"github.com/deckhouse/virtualization-controller/pkg/common/imageformat"
	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	podutil "github.com/deckhouse/virtualization-controller/pkg/common/pod"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/importer"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/supplements"
	"github.com/deckhouse/virtualization-controller/pkg/dvcr"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdcondition"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vicondition"
)

type ObjectRefVirtualDisk struct {
	importerService Importer
	diskService     *service.DiskService
	statService     Stat
	dvcrSettings    *dvcr.Settings
	client          client.Client
	recorder        eventrecord.EventRecorderLogger
}

func NewObjectRefVirtualDisk(
	recorder eventrecord.EventRecorderLogger,
	importerService Importer,
	client client.Client,
	diskService *service.DiskService,
	dvcrSettings *dvcr.Settings,
	statService Stat,
) *ObjectRefVirtualDisk {
	return &ObjectRefVirtualDisk{
		importerService: importerService,
		client:          client,
		recorder:        recorder,
		diskService:     diskService,
		statService:     statService,
		dvcrSettings:    dvcrSettings,
	}
}

func (ds ObjectRefVirtualDisk) StoreToDVCR(ctx context.Context, vi *v1alpha2.VirtualImage, vdRef *v1alpha2.VirtualDisk, cb *conditions.ConditionBuilder) (reconcile.Result, error) {
	log, ctx := logger.GetDataSourceContext(ctx, "objectref")

	supgen := supplements.NewGenerator(annotations.VIShortName, vi.Name, vi.Namespace, vi.UID)
	pod, err := ds.importerService.GetPod(ctx, supgen)
	if err != nil {
		return reconcile.Result{}, err
	}

	condition, _ := conditions.GetCondition(vicondition.ReadyType, vi.Status.Conditions)
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
		vi.Status.Progress = ds.statService.GetProgress(vi.GetUID(), pod, vi.Status.Progress)
		vi.Status.Target.RegistryURL = ds.statService.GetDVCRImageName(pod)

		pvc := &corev1.PersistentVolumeClaim{}
		err := ds.client.Get(ctx, types.NamespacedName{Name: vdRef.Status.Target.PersistentVolumeClaim, Namespace: vdRef.Namespace}, pvc)
		if err != nil {
			return reconcile.Result{}, err
		}

		envSettings := ds.getEnvSettings(vi, supgen, pvc.Spec.VolumeMode)
		ownerRef := metav1.NewControllerRef(vi, vi.GroupVersionKind())
		podSettings := ds.importerService.GetPodSettingsWithPVC(ownerRef, supgen, vdRef.Status.Target.PersistentVolumeClaim, vdRef.Namespace)
		err = ds.importerService.StartWithPodSetting(ctx, envSettings, supgen, datasource.NewCABundleForVMI(vi.GetNamespace(), vi.Spec.DataSource), podSettings, service.WithSystemNodeToleration())
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

		log.Info("Create importer pod...", "progress", vi.Status.Progress, "pod.phase", "nil")

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

		cb.
			Status(metav1.ConditionTrue).
			Reason(vicondition.Ready).
			Message("")

		vi.Status.Phase = v1alpha2.ImageReady
		vi.Status.Size = ds.statService.GetSize(pod)
		vi.Status.CDROM = ds.statService.GetCDROM(pod)
		vi.Status.Format = ds.statService.GetFormat(pod)
		vi.Status.Progress = "100%"
		vi.Status.Target.RegistryURL = ds.statService.GetDVCRImageName(pod)

		log.Info("Ready", "progress", vi.Status.Progress, "pod.phase", pod.Status.Phase)
	default:
		err = ds.statService.CheckPod(pod)
		if err != nil {
			vi.Status.Phase = v1alpha2.ImageFailed

			switch {
			case errors.Is(err, service.ErrNotInitialized), errors.Is(err, service.ErrNotScheduled):
				cb.
					Status(metav1.ConditionFalse).
					Reason(vicondition.ProvisioningNotStarted).
					Message(service.CapitalizeFirstLetter(err.Error() + "."))
				return reconcile.Result{}, nil
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

		err = ds.importerService.Protect(ctx, pod, supgen)
		if err != nil {
			return reconcile.Result{}, err
		}

		cb.
			Status(metav1.ConditionFalse).
			Reason(vicondition.Provisioning).
			Message("Import is in the process of provisioning to DVCR.")

		vi.Status.Phase = v1alpha2.ImageProvisioning
		vi.Status.Progress = ds.statService.GetProgress(vi.GetUID(), pod, vi.Status.Progress)
		vi.Status.Target.RegistryURL = ds.statService.GetDVCRImageName(pod)

		log.Info("Provisioning...", "progress", vi.Status.Progress, "pod.phase", pod.Status.Phase)
	}

	return reconcile.Result{RequeueAfter: time.Second}, nil
}

func (ds ObjectRefVirtualDisk) StoreToPVC(ctx context.Context, vi *v1alpha2.VirtualImage, vdRef *v1alpha2.VirtualDisk, cb *conditions.ConditionBuilder) (reconcile.Result, error) {
	log, ctx := logger.GetDataSourceContext(ctx, objectRefDataSource)

	supgen := supplements.NewGenerator(annotations.VIShortName, vi.Name, vi.Namespace, vi.UID)
	pvc, err := ds.diskService.GetPersistentVolumeClaim(ctx, supgen)
	if err != nil {
		return reconcile.Result{}, err
	}

	condition, _ := conditions.GetCondition(vicondition.ReadyType, vi.Status.Conditions)
	switch {
	case IsImageProvisioningFinished(condition):
		log.Info("Disk provisioning finished: clean up")

		setPhaseConditionForFinishedImage(pvc, cb, &vi.Status.Phase, supgen)

		err = ds.diskService.Unprotect(ctx, supgen)
		if err != nil {
			return reconcile.Result{}, err
		}

		return CleanUpSupplements(ctx, vi, ds)
	case object.AnyTerminating(pvc):
		log.Info("Waiting for supplements to be terminated")
	default:
		ds.recorder.Event(
			vi,
			corev1.EventTypeNormal,
			v1alpha2.ReasonDataSourceSyncStarted,
			"The ObjectRef DataSource import has started",
		)

		vi.Status.Progress = "0%"
		vi.Status.SourceUID = ptr.To(vdRef.GetUID())

		source := service.NewPVCPVCImportSource(vdRef.Status.Target.PersistentVolumeClaim, vdRef.Namespace)

		var size resource.Quantity
		size, err = resource.ParseQuantity(vdRef.Status.Capacity)
		if err != nil {
			return reconcile.Result{}, err
		}

		return reconcilePVCImportFromReadySource(ctx, vi, pvc, source, size, cb, supgen, ds.diskService, func() {
			ds.recorder.Event(vi, corev1.EventTypeNormal, v1alpha2.ReasonDataSourceSyncCompleted, "The ObjectRef DataSource import has completed")
			q, err := resource.ParseQuantity(vdRef.Status.Capacity)
			if err != nil {
				return
			}
			intQ, ok := q.AsInt64()
			if !ok {
				return
			}
			vi.Status.Size = v1alpha2.ImageStatusSize{
				Stored:        vdRef.Status.Capacity,
				StoredBytes:   strconv.FormatInt(intQ, 10),
				Unpacked:      vdRef.Status.Capacity,
				UnpackedBytes: strconv.FormatInt(intQ, 10),
			}
			vi.Status.Format = imageformat.FormatRAW
		})
	}

	return reconcile.Result{RequeueAfter: time.Second}, nil
}

func (ds ObjectRefVirtualDisk) CleanUpSupplements(ctx context.Context, vi *v1alpha2.VirtualImage) (reconcile.Result, error) {
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

func (ds ObjectRefVirtualDisk) CleanUp(ctx context.Context, vi *v1alpha2.VirtualImage) (bool, error) {
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

func (ds ObjectRefVirtualDisk) getEnvSettings(vi *v1alpha2.VirtualImage, sup supplements.Generator, volumeMode *corev1.PersistentVolumeMode) *importer.Settings {
	var settings importer.Settings

	if volumeMode != nil && *volumeMode == corev1.PersistentVolumeBlock {
		importer.ApplyBlockDeviceSourceSettings(&settings)
	} else {
		importer.ApplyFilesystemSourceSettings(&settings)
	}

	importer.ApplyDVCRDestinationSettings(
		&settings,
		ds.dvcrSettings,
		sup,
		ds.dvcrSettings.RegistryImageForVI(vi),
	)

	return &settings
}

func (ds ObjectRefVirtualDisk) Validate(ctx context.Context, vi *v1alpha2.VirtualImage) error {
	if vi.Spec.DataSource.ObjectRef == nil || vi.Spec.DataSource.ObjectRef.Kind != v1alpha2.VirtualImageObjectRefKindVirtualDisk {
		return fmt.Errorf("not a %s data source", v1alpha2.VirtualImageObjectRefKindVirtualDisk)
	}

	vd, err := object.FetchObject(ctx, types.NamespacedName{Name: vi.Spec.DataSource.ObjectRef.Name, Namespace: vi.Namespace}, ds.client, &v1alpha2.VirtualDisk{})
	if err != nil {
		return err
	}

	if vd == nil || vd.Status.Phase != v1alpha2.DiskReady {
		return NewVirtualDiskNotReadyError(vi.Spec.DataSource.ObjectRef.Name)
	}
	if vi.Status.Phase != v1alpha2.ImageReady {
		inUseCondition, _ := conditions.GetCondition(vdcondition.InUseType, vd.Status.Conditions)
		if inUseCondition.Status != metav1.ConditionTrue || !conditions.IsLastUpdated(inUseCondition, vd) {
			return NewVirtualDiskNotReadyForUseError(vd.Name)
		}

		switch inUseCondition.Reason {
		case vdcondition.UsedForImageCreation.String():
			return nil
		case vdcondition.AttachedToVirtualMachine.String():
			return NewVirtualDiskAttachedToVirtualMachineError(vd.Name)
		default:
			return NewVirtualDiskNotReadyForUseError(vd.Name)
		}
	}

	return nil
}
