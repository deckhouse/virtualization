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
	"log/slog"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/common/datasource"
	"github.com/deckhouse/virtualization-controller/pkg/controller"
	"github.com/deckhouse/virtualization-controller/pkg/controller/common"
	"github.com/deckhouse/virtualization-controller/pkg/controller/importer"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/supplements"
	"github.com/deckhouse/virtualization-controller/pkg/dvcr"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/cvicondition"
)

type ObjectRefDataSource struct {
	statService         Stat
	importerService     Importer
	dvcrSettings        *dvcr.Settings
	client              client.Client
	controllerNamespace string
	logger              *slog.Logger
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
		logger:              slog.Default().With("controller", common.CVIShortName, "ds", "objectref"),
	}
}

func (ds ObjectRefDataSource) Sync(ctx context.Context, cvi *virtv2.ClusterVirtualImage) (bool, error) {
	ds.logger.Info("Sync", "cvi", cvi.Name)

	condition, _ := service.GetCondition(cvicondition.ReadyType, cvi.Status.Conditions)
	defer func() { service.SetCondition(condition, &cvi.Status.Conditions) }()

	supgen := supplements.NewGenerator(common.CVIShortName, cvi.Name, ds.controllerNamespace, cvi.UID)
	pod, err := ds.importerService.GetPod(ctx, supgen)
	if err != nil {
		return false, err
	}

	switch {
	case isDiskProvisioningFinished(condition):
		ds.logger.Info("Finishing...", "cvi", cvi.Name)

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

		ds.logger.Info("Cleaning up...", "cvi", cvi.Name)
	case pod == nil:
		condition.Status = metav1.ConditionFalse
		condition.Reason = cvicondition.Provisioning
		condition.Message = "DVCR Provisioner not found: create the new one."

		var envSettings *importer.Settings
		envSettings, err = ds.getEnvSettings(cvi, supgen)
		if err != nil {
			return false, err
		}

		err = ds.importerService.Start(ctx, envSettings, cvi, supgen, datasource.NewCABundleForCVMI(cvi.Spec.DataSource))
		if err != nil {
			return false, err
		}

		cvi.Status.Phase = virtv2.ImageProvisioning
		cvi.Status.Target.RegistryURL = ds.dvcrSettings.RegistryImageForCVMI(cvi.Name)

		ds.logger.Info("Ready", "cvi", cvi.Name, "progress", cvi.Status.Progress, "pod.phase", "nil")
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
		cvi.Status.Size = ds.statService.GetSize(pod)
		cvi.Status.CDROM = ds.statService.GetCDROM(pod)
		cvi.Status.Format = ds.statService.GetFormat(pod)
		cvi.Status.Progress = "100%"
		cvi.Status.Target.RegistryURL = ds.dvcrSettings.RegistryImageForCVMI(cvi.Name)

		ds.logger.Info("Ready", "cvi", cvi.Name, "progress", cvi.Status.Progress, "pod.phase", pod.Status.Phase)
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
		cvi.Status.Target.RegistryURL = ds.dvcrSettings.RegistryImageForCVMI(cvi.Name)

		ds.logger.Info("Ready", "cvi", cvi.Name, "progress", cvi.Status.Progress, "pod.phase", pod.Status.Phase)
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

func (ds ObjectRefDataSource) getEnvSettings(cvi *virtv2.ClusterVirtualImage, supgen *supplements.Generator) (*importer.Settings, error) {
	var settings importer.Settings

	switch cvi.Spec.DataSource.ObjectRef.Kind {
	case virtv2.ClusterVirtualImageObjectRefKindVirtualImage:
		dvcrSourceImageName := ds.dvcrSettings.RegistryImageForVMI(
			cvi.Spec.DataSource.ObjectRef.Name,
			cvi.Spec.DataSource.ObjectRef.Namespace,
		)
		importer.ApplyDVCRSourceSettings(&settings, dvcrSourceImageName)
	case virtv2.ClusterVirtualImageObjectRefKindClusterVirtualImage:
		dvcrSourceImageName := ds.dvcrSettings.RegistryImageForCVMI(cvi.Spec.DataSource.ObjectRef.Name)
		importer.ApplyDVCRSourceSettings(&settings, dvcrSourceImageName)
	default:
		return nil, fmt.Errorf("unknown objectRef kind: %s", cvi.Spec.DataSource.ObjectRef.Kind)
	}

	importer.ApplyDVCRDestinationSettings(
		&settings,
		ds.dvcrSettings,
		supgen,
		ds.dvcrSettings.RegistryImageForCVMI(cvi.Name),
	)

	return &settings, nil
}
