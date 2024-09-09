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
}

func NewObjectRefDataSource(
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
		dvcrSettings:        dvcrSettings,
		client:              client,
		controllerNamespace: controllerNamespace,

		viOnPvcSyncer: NewObjectRefVirtualImageOnPvc(importerService, diskService, controllerNamespace, dvcrSettings, statService),
	}
}

func (ds ObjectRefDataSource) Sync(ctx context.Context, cvi *virtv2.ClusterVirtualImage) (bool, error) {
	log, ctx := logger.GetDataSourceContext(ctx, "objectref")

	condition, _ := service.GetCondition(cvicondition.ReadyType, cvi.Status.Conditions)
	defer func() { service.SetCondition(condition, &cvi.Status.Conditions) }()

	if cvi.Spec.DataSource.ObjectRef.Kind == virtv2.VirtualImageKind {
		viKey := types.NamespacedName{Name: cvi.Spec.DataSource.ObjectRef.Name, Namespace: cvi.Spec.DataSource.ObjectRef.Namespace}
		vi, err := helper.FetchObject(ctx, viKey, ds.client, &virtv2.VirtualImage{})
		if err != nil {
			return false, fmt.Errorf("unable to get VI %s: %w", viKey, err)
		}

		if vi == nil {
			return false, fmt.Errorf("VI object ref source %s is nil", cvi.Spec.DataSource.ObjectRef.Name)
		}

		if vi.Spec.Storage == virtv2.StorageKubernetes {
			return ds.viOnPvcSyncer.Sync(ctx, cvi, vi, &condition)
		}
	}

	supgen := supplements.NewGenerator(common.CVIShortName, cvi.Name, ds.controllerNamespace, cvi.UID)
	pod, err := ds.importerService.GetPod(ctx, supgen)
	if err != nil {
		return false, err
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
			return false, err
		}

		return CleanUp(ctx, cvi, ds)
	case common.IsTerminating(pod):
		cvi.Status.Phase = virtv2.ImagePending

		log.Info("Cleaning up...")
	case pod == nil:
		var dvcrDataSource controller.DVCRDataSource
		dvcrDataSource, err = controller.NewDVCRDataSourcesForCVMI(ctx, cvi.Spec.DataSource, ds.client)
		if err != nil {
			return false, err
		}

		var envSettings *importer.Settings
		envSettings, err = ds.getEnvSettings(cvi, supgen, dvcrDataSource)
		if err != nil {
			return false, err
		}

		err = ds.importerService.Start(ctx, envSettings, cvi, supgen, datasource.NewCABundleForCVMI(cvi.Spec.DataSource))
		var requeue bool
		requeue, err = setPhaseConditionForImporterStart(&condition, &cvi.Status.Phase, err)
		if err != nil {
			return false, err
		}

		log.Info("Ready", "progress", cvi.Status.Progress, "pod.phase", "nil")

		return requeue, nil
	case common.IsPodComplete(pod):
		err = ds.statService.CheckPod(pod)
		if err != nil {
			cvi.Status.Phase = virtv2.ImageFailed

			switch {
			case errors.Is(err, service.ErrProvisioningFailed):
				condition.Status = metav1.ConditionFalse
				condition.Reason = cvicondition.ProvisioningFailed
				condition.Message = service.CapitalizeFirstLetter(err.Error() + ".")
				return false, nil
			default:
				return false, err
			}
		}

		condition.Status = metav1.ConditionTrue
		condition.Reason = cvicondition.Ready
		condition.Message = ""

		cvi.Status.Phase = virtv2.ImageReady

		var dvcrDataSource controller.DVCRDataSource
		dvcrDataSource, err = controller.NewDVCRDataSourcesForCVMI(ctx, cvi.Spec.DataSource, ds.client)
		if err != nil {
			return false, err
		}

		if !dvcrDataSource.IsReady() {
			condition.Status = metav1.ConditionFalse
			condition.Reason = cvicondition.ProvisioningFailed
			condition.Message = "Failed to get stats from non-ready datasource: waiting for the DataSource to be ready."
			return false, nil
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
				return false, nil
			case errors.Is(err, service.ErrProvisioningFailed):
				condition.Status = metav1.ConditionFalse
				condition.Reason = cvicondition.ProvisioningFailed
				condition.Message = service.CapitalizeFirstLetter(err.Error() + ".")
				return false, nil
			default:
				return false, err
			}
		}

		condition.Status = metav1.ConditionFalse
		condition.Reason = cvicondition.Provisioning
		condition.Message = "Import is in the process of provisioning to DVCR."

		cvi.Status.Phase = virtv2.ImageProvisioning
		cvi.Status.Target.RegistryURL = ds.statService.GetDVCRImageName(pod)

		log.Info("Ready", "progress", cvi.Status.Progress, "pod.phase", pod.Status.Phase)
	}

	return true, nil
}

func (ds ObjectRefDataSource) CleanUp(ctx context.Context, cvi *virtv2.ClusterVirtualImage) (bool, error) {
	supgen := supplements.NewGenerator(common.CVIShortName, cvi.Name, ds.controllerNamespace, cvi.UID)

	requeue, err := ds.importerService.CleanUp(ctx, supgen)
	if err != nil {
		return false, err
	}

	return requeue, nil
}

func (ds ObjectRefDataSource) Validate(ctx context.Context, cvi *virtv2.ClusterVirtualImage) error {
	if cvi.Spec.DataSource.ObjectRef == nil {
		return fmt.Errorf("nil object ref: %s", cvi.Spec.DataSource.Type)
	}

	if cvi.Spec.DataSource.ObjectRef.Kind == virtv2.VirtualImageKind {
		viKey := types.NamespacedName{Name: cvi.Spec.DataSource.ObjectRef.Name, Namespace: cvi.Spec.DataSource.ObjectRef.Namespace}
		vi, err := helper.FetchObject(ctx, viKey, ds.client, &virtv2.VirtualImage{})
		if err != nil {
			return fmt.Errorf("unable to get VI %s: %w", viKey, err)
		}

		if vi == nil {
			return fmt.Errorf("VI object ref source %s is nil", cvi.Spec.DataSource.ObjectRef.Name)
		}

		if vi.Spec.Storage == virtv2.StorageKubernetes {
			if vi.Status.Phase != virtv2.ImageReady {
				return NewImageNotReadyError(cvi.Spec.DataSource.ObjectRef.Name)
			}
			return nil
		}
	}

	dvcrDataSource, err := controller.NewDVCRDataSourcesForCVMI(ctx, cvi.Spec.DataSource, ds.client)
	if err != nil {
		return err
	}

	if dvcrDataSource.IsReady() {
		return nil
	}

	switch cvi.Spec.DataSource.ObjectRef.Kind {
	case virtv2.ClusterVirtualImageObjectRefKindVirtualImage:
		return NewImageNotReadyError(cvi.Spec.DataSource.ObjectRef.Name)
	case virtv2.ClusterVirtualImageObjectRefKindClusterVirtualImage:
		return NewClusterImageNotReadyError(cvi.Spec.DataSource.ObjectRef.Name)
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
