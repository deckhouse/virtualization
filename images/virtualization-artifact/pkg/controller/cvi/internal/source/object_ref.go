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
	"github.com/deckhouse/virtualization-controller/pkg/controller"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/importer"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/supplements"
	"github.com/deckhouse/virtualization-controller/pkg/dvcr"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/cvicondition"
)

type ObjectRefDataSource struct {
	statService         Stat
	importerService     Importer
	diskService         *service.DiskService
	dvcrSettings        *dvcr.Settings
	client              client.Client
	controllerNamespace string
	recorder            eventrecord.EventRecorderLogger

	viOnPvcSyncer    *ObjectRefVirtualImageOnPvc
	vdSyncer         *ObjectRefVirtualDisk
	vdSnapshotSyncer *ObjectRefVirtualDiskSnapshot
}

func NewObjectRefDataSource(
	recorder eventrecord.EventRecorderLogger,
	statService Stat,
	importerService Importer,
	diskService *service.DiskService,
	dvcrSettings *dvcr.Settings,
	client client.Client,
	controllerNamespace string,
) *ObjectRefDataSource {
	return &ObjectRefDataSource{
		statService:         statService,
		importerService:     importerService,
		diskService:         diskService,
		dvcrSettings:        dvcrSettings,
		client:              client,
		controllerNamespace: controllerNamespace,
		recorder:            recorder,

		viOnPvcSyncer:    NewObjectRefVirtualImageOnPvc(recorder, importerService, dvcrSettings, statService),
		vdSyncer:         NewObjectRefVirtualDisk(recorder, importerService, client, controllerNamespace, dvcrSettings, statService),
		vdSnapshotSyncer: NewObjectRefVirtualDiskSnapshot(recorder, importerService, diskService, client, controllerNamespace, dvcrSettings, statService),
	}
}

func (ds ObjectRefDataSource) Sync(ctx context.Context, cvi *v1alpha2.ClusterVirtualImage) (reconcile.Result, error) {
	log, ctx := logger.GetDataSourceContext(ctx, "objectref")

	condition, _ := conditions.GetCondition(cvicondition.ReadyType, cvi.Status.Conditions)
	cb := conditions.NewConditionBuilder(cvicondition.ReadyType).Generation(cvi.Generation)
	defer func() {
		// It is necessary to avoid setting unknown for the ready condition if it was already set to true.
		if !(cb.Condition().Status == metav1.ConditionUnknown && condition.Status == metav1.ConditionTrue) {
			conditions.SetCondition(cb, &cvi.Status.Conditions)
		}
	}()

	switch cvi.Spec.DataSource.ObjectRef.Kind {
	case v1alpha2.VirtualImageKind:
		viKey := types.NamespacedName{Name: cvi.Spec.DataSource.ObjectRef.Name, Namespace: cvi.Spec.DataSource.ObjectRef.Namespace}
		vi, err := object.FetchObject(ctx, viKey, ds.client, &v1alpha2.VirtualImage{})
		if err != nil {
			return reconcile.Result{}, fmt.Errorf("unable to get VI %s: %w", viKey, err)
		}

		if vi == nil {
			return reconcile.Result{}, fmt.Errorf("VI object ref source %s is nil", cvi.Spec.DataSource.ObjectRef.Name)
		}

		if vi.Spec.Storage == v1alpha2.StorageKubernetes || vi.Spec.Storage == v1alpha2.StoragePersistentVolumeClaim {
			return ds.viOnPvcSyncer.Sync(ctx, cvi, vi, cb)
		}
	case v1alpha2.VirtualDiskKind:
		vdKey := types.NamespacedName{Name: cvi.Spec.DataSource.ObjectRef.Name, Namespace: cvi.Spec.DataSource.ObjectRef.Namespace}
		vd, err := object.FetchObject(ctx, vdKey, ds.client, &v1alpha2.VirtualDisk{})
		if err != nil {
			return reconcile.Result{}, fmt.Errorf("unable to get VD %s: %w", vdKey, err)
		}

		if vd == nil {
			return reconcile.Result{}, fmt.Errorf("VD object ref source %s is nil", cvi.Spec.DataSource.ObjectRef.Name)
		}

		return ds.vdSyncer.Sync(ctx, cvi, vd, cb)

	case v1alpha2.VirtualDiskSnapshotKind:
		vdSnapshotKey := types.NamespacedName{Name: cvi.Spec.DataSource.ObjectRef.Name, Namespace: cvi.Spec.DataSource.ObjectRef.Namespace}
		vdSnapshot, err := object.FetchObject(ctx, vdSnapshotKey, ds.client, &v1alpha2.VirtualDiskSnapshot{})
		if err != nil {
			return reconcile.Result{}, fmt.Errorf("unable to get VDSnapshot %s: %w", vdSnapshotKey, err)
		}

		if vdSnapshot == nil {
			return reconcile.Result{}, fmt.Errorf("VDSnapshot object ref %s is nil", vdSnapshotKey)
		}

		return ds.vdSnapshotSyncer.Sync(ctx, cvi, vdSnapshot, cb)
	}

	supgen := supplements.NewGenerator(annotations.CVIShortName, cvi.Name, ds.controllerNamespace, cvi.UID)
	pod, err := ds.importerService.GetPod(ctx, supgen)
	if err != nil {
		return reconcile.Result{}, err
	}

	switch {
	case isDiskProvisioningFinished(condition):
		log.Info("Cluster virtual image provisioning finished: clean up")

		cb.
			Status(metav1.ConditionTrue).
			Reason(cvicondition.Ready).
			Message("")

		cvi.Status.Phase = v1alpha2.ImageReady

		// Unprotect import time supplements to delete them later.
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
		var dvcrDataSource controller.DVCRDataSource
		dvcrDataSource, err = controller.NewDVCRDataSourcesForCVMI(ctx, cvi.Spec.DataSource, ds.client)
		if err != nil {
			return reconcile.Result{}, err
		}

		var envSettings *importer.Settings
		envSettings, err = ds.getEnvSettings(cvi, supgen, dvcrDataSource)
		if err != nil {
			return reconcile.Result{}, err
		}

		err = ds.importerService.Start(ctx, envSettings, cvi, supgen, datasource.NewCABundleForCVMI(cvi.Spec.DataSource), service.WithSystemNodeToleration())
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

		log.Info("Ready", "progress", cvi.Status.Progress, "pod.phase", "nil")

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

		var dvcrDataSource controller.DVCRDataSource
		dvcrDataSource, err = controller.NewDVCRDataSourcesForCVMI(ctx, cvi.Spec.DataSource, ds.client)
		if err != nil {
			return reconcile.Result{}, err
		}

		if !dvcrDataSource.IsReady() {
			cb.
				Status(metav1.ConditionFalse).
				Reason(cvicondition.ProvisioningFailed).
				Message("Failed to get stats from non-ready datasource: waiting for the DataSource to be ready.")
			return reconcile.Result{}, nil
		}

		cvi.Status.Size = dvcrDataSource.GetSize()
		cvi.Status.CDROM = dvcrDataSource.IsCDROM()
		cvi.Status.Format = dvcrDataSource.GetFormat()
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

		cb.
			Status(metav1.ConditionFalse).
			Reason(cvicondition.Provisioning).
			Message("Import is in the process of provisioning to DVCR.")

		cvi.Status.Phase = v1alpha2.ImageProvisioning
		cvi.Status.Target.RegistryURL = ds.statService.GetDVCRImageName(pod)

		log.Info("Ready", "progress", cvi.Status.Progress, "pod.phase", pod.Status.Phase)
	}

	return reconcile.Result{RequeueAfter: time.Second}, nil
}

func (ds ObjectRefDataSource) CleanUp(ctx context.Context, cvi *v1alpha2.ClusterVirtualImage) (bool, error) {
	viRefResult, err := ds.viOnPvcSyncer.CleanUp(ctx, cvi)
	if err != nil {
		return false, err
	}

	vdRefResult, err := ds.vdSyncer.CleanUp(ctx, cvi)
	if err != nil {
		return false, err
	}

	supgen := supplements.NewGenerator(annotations.CVIShortName, cvi.Name, ds.controllerNamespace, cvi.UID)

	objRefRequeue, err := ds.importerService.CleanUp(ctx, supgen)
	if err != nil {
		return false, err
	}

	return viRefResult || vdRefResult || objRefRequeue, nil
}

func (ds ObjectRefDataSource) Validate(ctx context.Context, cvi *v1alpha2.ClusterVirtualImage) error {
	if cvi.Spec.DataSource.ObjectRef == nil {
		return fmt.Errorf("nil object ref: %s", cvi.Spec.DataSource.Type)
	}

	switch cvi.Spec.DataSource.ObjectRef.Kind {
	case v1alpha2.ClusterVirtualImageObjectRefKindVirtualImage:
		viKey := types.NamespacedName{Name: cvi.Spec.DataSource.ObjectRef.Name, Namespace: cvi.Spec.DataSource.ObjectRef.Namespace}
		vi, err := object.FetchObject(ctx, viKey, ds.client, &v1alpha2.VirtualImage{})
		if err != nil {
			return fmt.Errorf("unable to get VI %s: %w", viKey, err)
		}

		if vi == nil {
			return NewImageNotReadyError(cvi.Spec.DataSource.ObjectRef.Name)
		}

		if vi.Spec.Storage == v1alpha2.StorageKubernetes || vi.Spec.Storage == v1alpha2.StoragePersistentVolumeClaim {
			if vi.Status.Phase != v1alpha2.ImageReady {
				return NewImageNotReadyError(cvi.Spec.DataSource.ObjectRef.Name)
			}
			return nil
		}

		dvcrDataSource, err := controller.NewDVCRDataSourcesForCVMI(ctx, cvi.Spec.DataSource, ds.client)
		if err != nil {
			return err
		}

		if dvcrDataSource.IsReady() {
			return nil
		}

		return NewImageNotReadyError(cvi.Spec.DataSource.ObjectRef.Name)
	case v1alpha2.ClusterVirtualImageObjectRefKindClusterVirtualImage:
		dvcrDataSource, err := controller.NewDVCRDataSourcesForCVMI(ctx, cvi.Spec.DataSource, ds.client)
		if err != nil {
			return err
		}

		if dvcrDataSource.IsReady() {
			return nil
		}

		return NewClusterImageNotReadyError(cvi.Spec.DataSource.ObjectRef.Name)
	case v1alpha2.VirtualDiskSnapshotKind:
		vdSnapshotKey := types.NamespacedName{Name: cvi.Spec.DataSource.ObjectRef.Name, Namespace: cvi.Spec.DataSource.ObjectRef.Namespace}
		vdSnapshot, err := object.FetchObject(ctx, vdSnapshotKey, ds.client, &v1alpha2.VirtualDiskSnapshot{})
		if err != nil {
			return fmt.Errorf("unable to get VDSnapshot %s: %w", vdSnapshotKey, err)
		}

		if vdSnapshot == nil {
			return NewVirtualDiskSnapshotNotReadyError(cvi.Spec.DataSource.ObjectRef.Name)
		}

		return ds.vdSnapshotSyncer.Validate(ctx, cvi)
	case v1alpha2.ClusterVirtualImageObjectRefKindVirtualDisk:
		return ds.vdSyncer.Validate(ctx, cvi)
	default:
		return fmt.Errorf("unexpected object ref kind: %s", cvi.Spec.DataSource.ObjectRef.Kind)
	}
}

func (ds ObjectRefDataSource) getEnvSettings(cvi *v1alpha2.ClusterVirtualImage, sup supplements.Generator, dvcrDataSource controller.DVCRDataSource) (*importer.Settings, error) {
	if !dvcrDataSource.IsReady() {
		return nil, errors.New("dvcr data source is not ready")
	}

	var settings importer.Settings
	importer.ApplyDVCRSourceSettings(&settings, dvcrDataSource.GetTarget())
	importer.ApplyDVCRDestinationSettings(
		&settings,
		ds.dvcrSettings,
		sup,
		ds.dvcrSettings.RegistryImageForCVI(cvi),
	)

	return &settings, nil
}
