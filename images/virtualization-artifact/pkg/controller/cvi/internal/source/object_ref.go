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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/common/datasource"
	"github.com/deckhouse/virtualization-controller/pkg/controller"
	"github.com/deckhouse/virtualization-controller/pkg/controller/common"
	"github.com/deckhouse/virtualization-controller/pkg/controller/importer"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/supplements"
	"github.com/deckhouse/virtualization-controller/pkg/dvcr"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	"github.com/deckhouse/virtualization-controller/pkg/sdk/framework/helper"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/cvicondition"
)

type ObjectRefDataSource struct {
	statService         Stat
	importerService     Importer
	dvcrSettings        *dvcr.Settings
	client              client.Client
	controllerNamespace string

	viOnPvcSyncer *ObjectRefVirtualImageOnPvc
	vdSyncer      *ObjectRefVirtualDisk
}

func NewObjectRefDataSource(
	statService Stat,
	importerService Importer,
	dvcrSettings *dvcr.Settings,
	client client.Client,
	controllerNamespace string,
) *ObjectRefDataSource {
	return &ObjectRefDataSource{
		statService:         statService,
		importerService:     importerService,
		dvcrSettings:        dvcrSettings,
		client:              client,
		controllerNamespace: controllerNamespace,

		viOnPvcSyncer: NewObjectRefVirtualImageOnPvc(importerService, dvcrSettings, statService),
		vdSyncer:      NewObjectRefVirtualDisk(importerService, client, controllerNamespace, dvcrSettings, statService),
	}
}

func (ds ObjectRefDataSource) Sync(ctx context.Context, cvi *virtv2.ClusterVirtualImage) (reconcile.Result, error) {
	log, ctx := logger.GetDataSourceContext(ctx, "objectref")

	condition := metav1.Condition{
		Type:   cvicondition.ReadyType,
		Status: metav1.ConditionUnknown,
	}

	defer func() { service.SetCondition(condition, &cvi.Status.Conditions) }()

	switch cvi.Spec.DataSource.ObjectRef.Kind {
	case virtv2.VirtualImageKind:
		viKey := types.NamespacedName{Name: cvi.Spec.DataSource.ObjectRef.Name, Namespace: cvi.Spec.DataSource.ObjectRef.Namespace}
		vi, err := helper.FetchObject(ctx, viKey, ds.client, &virtv2.VirtualImage{})
		if err != nil {
			return reconcile.Result{}, fmt.Errorf("unable to get VI %s: %w", viKey, err)
		}

		if vi == nil {
			return reconcile.Result{}, fmt.Errorf("VI object ref source %s is nil", cvi.Spec.DataSource.ObjectRef.Name)
		}

		if vi.Spec.Storage == virtv2.StorageKubernetes {
			return ds.viOnPvcSyncer.Sync(ctx, cvi, vi, &condition)
		}
	case virtv2.VirtualDiskKind:
		vdKey := types.NamespacedName{Name: cvi.Spec.DataSource.ObjectRef.Name, Namespace: cvi.Spec.DataSource.ObjectRef.Namespace}
		vd, err := helper.FetchObject(ctx, vdKey, ds.client, &virtv2.VirtualDisk{})
		if err != nil {
			return reconcile.Result{}, fmt.Errorf("unable to get VD %s: %w", vdKey, err)
		}

		if vd == nil {
			return reconcile.Result{}, fmt.Errorf("VD object ref source %s is nil", cvi.Spec.DataSource.ObjectRef.Name)
		}

		return ds.vdSyncer.Sync(ctx, cvi, vd, &condition)
	}

	supgen := supplements.NewGenerator(common.CVIShortName, cvi.Name, ds.controllerNamespace, cvi.UID)
	pod, err := ds.importerService.GetPod(ctx, supgen)
	if err != nil {
		return reconcile.Result{}, err
	}

	switch {
	case isDiskProvisioningFinished(condition):
		log.Info("Cluster virtual image provisioning finished: clean up")

		condition.Status = metav1.ConditionTrue
		condition.Reason = cvicondition.Ready
		condition.Message = ""

		cvi.Status.Phase = virtv2.ImageReady

		// Unprotect import time supplements to delete them later.
		err = ds.importerService.Unprotect(ctx, pod)
		if err != nil {
			return reconcile.Result{}, err
		}

		return CleanUp(ctx, cvi, ds)
	case common.IsTerminating(pod):
		cvi.Status.Phase = virtv2.ImagePending

		log.Info("Cleaning up...")
	case pod == nil:
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

		err = ds.importerService.Start(ctx, envSettings, cvi, supgen, datasource.NewCABundleForCVMI(cvi.Spec.DataSource))
		switch {
		case err == nil:
			// OK.
		case common.ErrQuotaExceeded(err):
			return setQuotaExceededPhaseCondition(&condition, &cvi.Status.Phase, err, cvi.CreationTimestamp), nil
		default:
			setPhaseConditionToFailed(&condition, &cvi.Status.Phase, fmt.Errorf("unexpected error: %w", err))
			return reconcile.Result{}, err
		}

		cvi.Status.Phase = virtv2.ImageProvisioning
		condition.Status = metav1.ConditionFalse
		condition.Reason = cvicondition.Provisioning
		condition.Message = "DVCR Provisioner not found: create the new one."

		log.Info("Ready", "progress", cvi.Status.Progress, "pod.phase", "nil")

		return reconcile.Result{Requeue: true}, nil
	case common.IsPodComplete(pod):
		err = ds.statService.CheckPod(pod)
		if err != nil {
			cvi.Status.Phase = virtv2.ImageFailed

			switch {
			case errors.Is(err, service.ErrProvisioningFailed):
				condition.Status = metav1.ConditionFalse
				condition.Reason = cvicondition.ProvisioningFailed
				condition.Message = service.CapitalizeFirstLetter(err.Error() + ".")
				return reconcile.Result{}, nil
			default:
				return reconcile.Result{}, err
			}
		}

		condition.Status = metav1.ConditionTrue
		condition.Reason = cvicondition.Ready
		condition.Message = ""

		cvi.Status.Phase = virtv2.ImageReady

		var dvcrDataSource controller.DVCRDataSource
		dvcrDataSource, err = controller.NewDVCRDataSourcesForCVMI(ctx, cvi.Spec.DataSource, ds.client)
		if err != nil {
			return reconcile.Result{}, err
		}

		if !dvcrDataSource.IsReady() {
			condition.Status = metav1.ConditionFalse
			condition.Reason = cvicondition.ProvisioningFailed
			condition.Message = "Failed to get stats from non-ready datasource: waiting for the DataSource to be ready."
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
			cvi.Status.Phase = virtv2.ImageFailed

			switch {
			case errors.Is(err, service.ErrNotInitialized), errors.Is(err, service.ErrNotScheduled):
				condition.Status = metav1.ConditionFalse
				condition.Reason = cvicondition.ProvisioningNotStarted
				condition.Message = service.CapitalizeFirstLetter(err.Error() + ".")
				return reconcile.Result{}, nil
			case errors.Is(err, service.ErrProvisioningFailed):
				condition.Status = metav1.ConditionFalse
				condition.Reason = cvicondition.ProvisioningFailed
				condition.Message = service.CapitalizeFirstLetter(err.Error() + ".")
				return reconcile.Result{}, nil
			default:
				return reconcile.Result{}, err
			}
		}

		condition.Status = metav1.ConditionFalse
		condition.Reason = cvicondition.Provisioning
		condition.Message = "Import is in the process of provisioning to DVCR."

		cvi.Status.Phase = virtv2.ImageProvisioning
		cvi.Status.Target.RegistryURL = ds.statService.GetDVCRImageName(pod)

		log.Info("Ready", "progress", cvi.Status.Progress, "pod.phase", pod.Status.Phase)
	}

	return reconcile.Result{Requeue: true}, nil
}

func (ds ObjectRefDataSource) CleanUp(ctx context.Context, cvi *virtv2.ClusterVirtualImage) (reconcile.Result, error) {
	viRefResult, err := ds.viOnPvcSyncer.CleanUp(ctx, cvi)
	if err != nil {
		return reconcile.Result{}, err
	}

	vdRefResult, err := ds.vdSyncer.CleanUp(ctx, cvi)
	if err != nil {
		return reconcile.Result{}, err
	}

	supgen := supplements.NewGenerator(common.CVIShortName, cvi.Name, ds.controllerNamespace, cvi.UID)

	objRefRequeue, err := ds.importerService.CleanUp(ctx, supgen)
	if err != nil {
		return reconcile.Result{}, err
	}

	return service.MergeResults(viRefResult, vdRefResult, reconcile.Result{Requeue: objRefRequeue}), nil
}

func (ds ObjectRefDataSource) Validate(ctx context.Context, cvi *virtv2.ClusterVirtualImage) error {
	if cvi.Spec.DataSource.ObjectRef == nil {
		return fmt.Errorf("nil object ref: %s", cvi.Spec.DataSource.Type)
	}

	switch cvi.Spec.DataSource.ObjectRef.Kind {
	case virtv2.ClusterVirtualImageObjectRefKindVirtualImage:
		viKey := types.NamespacedName{Name: cvi.Spec.DataSource.ObjectRef.Name, Namespace: cvi.Spec.DataSource.ObjectRef.Namespace}
		vi, err := helper.FetchObject(ctx, viKey, ds.client, &virtv2.VirtualImage{})
		if err != nil {
			return fmt.Errorf("unable to get VI %s: %w", viKey, err)
		}

		if vi == nil {
			return NewImageNotReadyError(cvi.Spec.DataSource.ObjectRef.Name)
		}

		if vi.Spec.Storage == virtv2.StorageKubernetes {
			if vi.Status.Phase != virtv2.ImageReady {
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
	case virtv2.ClusterVirtualImageObjectRefKindClusterVirtualImage:
		dvcrDataSource, err := controller.NewDVCRDataSourcesForCVMI(ctx, cvi.Spec.DataSource, ds.client)
		if err != nil {
			return err
		}

		if dvcrDataSource.IsReady() {
			return nil
		}

		return NewClusterImageNotReadyError(cvi.Spec.DataSource.ObjectRef.Name)
	case virtv2.ClusterVirtualImageObjectRefKindVirtualDisk:
		return ds.vdSyncer.Validate(ctx, cvi)
	default:
		return fmt.Errorf("unexpected object ref kind: %s", cvi.Spec.DataSource.ObjectRef.Kind)
	}
}

func (ds ObjectRefDataSource) getEnvSettings(cvi *virtv2.ClusterVirtualImage, sup *supplements.Generator, dvcrDataSource controller.DVCRDataSource) (*importer.Settings, error) {
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
