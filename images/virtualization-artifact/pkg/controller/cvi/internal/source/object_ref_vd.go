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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
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
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/cvicondition"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdcondition"
)

type ObjectRefVirtualDisk struct {
	importerService     Importer
	client              client.Client
	statService         Stat
	dvcrSettings        *dvcr.Settings
	controllerNamespace string
	recorder            eventrecord.EventRecorderLogger
}

func NewObjectRefVirtualDisk(recorder eventrecord.EventRecorderLogger, importerService Importer, client client.Client, controllerNamespace string, dvcrSettings *dvcr.Settings, statService Stat) *ObjectRefVirtualDisk {
	return &ObjectRefVirtualDisk{
		importerService:     importerService,
		statService:         statService,
		dvcrSettings:        dvcrSettings,
		client:              client,
		controllerNamespace: controllerNamespace,
		recorder:            recorder,
	}
}

func (ds ObjectRefVirtualDisk) Sync(ctx context.Context, cvi *v1alpha2.ClusterVirtualImage, vdRef *v1alpha2.VirtualDisk, cb *conditions.ConditionBuilder) (reconcile.Result, error) {
	log, ctx := logger.GetDataSourceContext(ctx, "objectref")

	supgen := supplements.NewGenerator(annotations.CVIShortName, cvi.Name, vdRef.Namespace, cvi.UID)
	pod, err := ds.importerService.GetPod(ctx, supgen)
	if err != nil {
		return reconcile.Result{}, err
	}

	condition, _ := conditions.GetCondition(cvicondition.ReadyType, cvi.Status.Conditions)
	switch {
	case isDiskProvisioningFinished(condition):
		log.Info("Cluster virtual image provisioning finished: clean up")

		cb.
			Status(metav1.ConditionTrue).
			Reason(cvicondition.Ready).
			Message("")

		cvi.Status.Phase = v1alpha2.ImageReady

		err = ds.importerService.Unprotect(ctx, pod, supgen)
		if err != nil {
			return reconcile.Result{}, err
		}

		_, err = CleanUp(ctx, cvi, ds)
		if err != nil {
			return reconcile.Result{}, err
		}

		return reconcile.Result{}, nil
	case object.IsTerminating(pod):
		cvi.Status.Phase = v1alpha2.ImagePending

		log.Info("Cleaning up...")
	case pod == nil:
		ds.recorder.Event(
			cvi,
			corev1.EventTypeNormal,
			v1alpha2.ReasonDataSourceSyncStarted,
			"The ObjectRef DataSource import has started",
		)
		cvi.Status.Progress = ds.statService.GetProgress(cvi.GetUID(), pod, cvi.Status.Progress)
		cvi.Status.Target.RegistryURL = ds.statService.GetDVCRImageName(pod)

		pvc := &corev1.PersistentVolumeClaim{}
		err := ds.client.Get(ctx, types.NamespacedName{Name: vdRef.Status.Target.PersistentVolumeClaim, Namespace: vdRef.Namespace}, pvc)
		if err != nil {
			return reconcile.Result{}, err
		}

		envSettings := ds.getEnvSettings(cvi, supgen, pvc.Spec.VolumeMode)
		ownerRef := metav1.NewControllerRef(cvi, cvi.GroupVersionKind())
		podSettings := ds.importerService.GetPodSettingsWithPVC(ownerRef, supgen, vdRef.Status.Target.PersistentVolumeClaim, vdRef.Namespace)
		err = ds.importerService.StartWithPodSetting(ctx, envSettings, supgen, datasource.NewCABundleForCVMI(cvi.Spec.DataSource), podSettings, service.WithSystemNodeToleration())
		switch {
		case err == nil:
			// OK.
		case common.ErrQuotaExceeded(err):
			ds.recorder.Event(cvi, corev1.EventTypeWarning, v1alpha2.ReasonDataSourceQuotaExceeded, "DataSource quota exceed")
			return setQuotaExceededPhaseCondition(cb, &cvi.Status.Phase, err, cvi.CreationTimestamp), nil
		default:
			setPhaseConditionToFailed(cb, &cvi.Status.Phase, fmt.Errorf("unexpected error: %w", err))
			return reconcile.Result{}, err
		}

		cvi.Status.Phase = v1alpha2.ImageProvisioning
		cb.
			Status(metav1.ConditionFalse).
			Reason(cvicondition.Provisioning).
			Message("DVCR Provisioner not found: create the new one.")

		log.Info("Create importer pod...", "progress", cvi.Status.Progress, "pod.phase", "nil")

		return reconcile.Result{RequeueAfter: time.Second}, nil
	case podutil.IsPodComplete(pod):
		err = ds.statService.CheckPod(pod)
		if err != nil {
			cvi.Status.Phase = v1alpha2.ImageFailed

			switch {
			case errors.Is(err, service.ErrProvisioningFailed):
				ds.recorder.Event(cvi, corev1.EventTypeWarning, v1alpha2.ReasonDataSourceDiskProvisioningFailed, "Disk provisioning failed")
				cb.
					Status(metav1.ConditionFalse).
					Reason(cvicondition.ProvisioningFailed).
					Message(service.CapitalizeFirstLetter(err.Error() + "."))
				return reconcile.Result{}, nil
			default:
				return reconcile.Result{}, err
			}
		}

		cb.
			Status(metav1.ConditionTrue).
			Reason(cvicondition.Ready).
			Message("")

		cvi.Status.Phase = v1alpha2.ImageReady
		cvi.Status.Size = ds.statService.GetSize(pod)
		cvi.Status.CDROM = ds.statService.GetCDROM(pod)
		cvi.Status.Format = ds.statService.GetFormat(pod)
		cvi.Status.Progress = "100%"
		cvi.Status.Target.RegistryURL = ds.statService.GetDVCRImageName(pod)

		log.Info("Ready", "progress", cvi.Status.Progress, "pod.phase", pod.Status.Phase)
	default:
		err = ds.statService.CheckPod(pod)
		if err != nil {
			cvi.Status.Phase = v1alpha2.ImageFailed

			switch {
			case errors.Is(err, service.ErrNotInitialized), errors.Is(err, service.ErrNotScheduled):
				cb.
					Status(metav1.ConditionFalse).
					Reason(cvicondition.ProvisioningNotStarted).
					Message(service.CapitalizeFirstLetter(err.Error() + "."))
				return reconcile.Result{}, nil
			case errors.Is(err, service.ErrProvisioningFailed):
				ds.recorder.Event(cvi, corev1.EventTypeWarning, v1alpha2.ReasonDataSourceDiskProvisioningFailed, "Disk provisioning failed")
				cb.
					Status(metav1.ConditionFalse).
					Reason(cvicondition.ProvisioningFailed).
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
			Reason(cvicondition.Provisioning).
			Message("Import is in the process of provisioning to DVCR.")

		cvi.Status.Phase = v1alpha2.ImageProvisioning
		cvi.Status.Progress = ds.statService.GetProgress(cvi.GetUID(), pod, cvi.Status.Progress)
		cvi.Status.Target.RegistryURL = ds.statService.GetDVCRImageName(pod)

		log.Info("Provisioning...", "progress", cvi.Status.Progress, "pod.phase", pod.Status.Phase)
	}

	return reconcile.Result{RequeueAfter: time.Second}, nil
}

func (ds ObjectRefVirtualDisk) CleanUp(ctx context.Context, cvi *v1alpha2.ClusterVirtualImage) (bool, error) {
	supgen := supplements.NewGenerator(annotations.CVIShortName, cvi.Name, ds.controllerNamespace, cvi.UID)
	return ds.importerService.DeletePod(ctx, cvi, controllerName, supgen)
}

func (ds ObjectRefVirtualDisk) getEnvSettings(cvi *v1alpha2.ClusterVirtualImage, sup supplements.Generator, volumeMode *corev1.PersistentVolumeMode) *importer.Settings {
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
		ds.dvcrSettings.RegistryImageForCVI(cvi),
	)

	return &settings
}

func (ds ObjectRefVirtualDisk) Validate(ctx context.Context, cvi *v1alpha2.ClusterVirtualImage) error {
	if cvi.Spec.DataSource.ObjectRef == nil || cvi.Spec.DataSource.ObjectRef.Kind != v1alpha2.ClusterVirtualImageObjectRefKindVirtualDisk {
		return fmt.Errorf("not a %s data source", v1alpha2.ClusterVirtualImageObjectRefKindVirtualDisk)
	}

	vd, err := object.FetchObject(ctx, types.NamespacedName{Name: cvi.Spec.DataSource.ObjectRef.Name, Namespace: cvi.Spec.DataSource.ObjectRef.Namespace}, ds.client, &v1alpha2.VirtualDisk{})
	if err != nil {
		return err
	}

	if vd == nil || vd.Status.Phase != v1alpha2.DiskReady {
		return NewVirtualDiskNotReadyError(cvi.Spec.DataSource.ObjectRef.Name)
	}
	if cvi.Status.Phase != v1alpha2.ImageReady {
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
