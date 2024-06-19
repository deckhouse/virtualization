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
	"log/slog"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/deckhouse/virtualization-controller/pkg/common/cvmi"
	"github.com/deckhouse/virtualization-controller/pkg/common/datasource"
	cc "github.com/deckhouse/virtualization-controller/pkg/controller/common"
	"github.com/deckhouse/virtualization-controller/pkg/controller/importer"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/supplements"
	"github.com/deckhouse/virtualization-controller/pkg/dvcr"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/cvicondition"
)

type HTTPDataSource struct {
	statService         Stat
	importerService     Importer
	dvcrSettings        *dvcr.Settings
	controllerNamespace string
	logger              *slog.Logger
}

func NewHTTPDataSource(
	statService Stat,
	importerService Importer,
	dvcrSettings *dvcr.Settings,
	controllerNamespace string,
) *HTTPDataSource {
	return &HTTPDataSource{
		statService:         statService,
		importerService:     importerService,
		dvcrSettings:        dvcrSettings,
		controllerNamespace: controllerNamespace,
		logger:              slog.Default().With("controller", cvmi.ShortName, "ds", "http"),
	}
}

func (ds HTTPDataSource) Sync(ctx context.Context, cvi *virtv2.ClusterVirtualImage) (bool, error) {
	ds.logger.Info("Sync", "cvi", cvi.Name)

	condition, _ := service.GetCondition(cvicondition.ReadyType, cvi.Status.Conditions)
	defer func() { service.SetCondition(condition, &cvi.Status.Conditions) }()

	supgen := supplements.NewGenerator(cvmi.ShortName, cvi.Name, ds.controllerNamespace, cvi.UID)
	pod, err := ds.importerService.GetPod(ctx, supgen)
	if err != nil {
		return false, err
	}

	switch {
	case isDiskProvisioningFinished(condition):
		ds.logger.Info("Finishing...", "cvi", cvi.Name)

		condition.Status = metav1.ConditionTrue
		condition.Reason = cvicondition.ReadyReason_Ready
		condition.Message = ""

		cvi.Status.Phase = virtv2.ImageReady

		err = ds.importerService.Unprotect(ctx, pod)
		if err != nil {
			return false, err
		}

		return CleanUp(ctx, cvi, ds)
	case cc.IsTerminating(pod):
		cvi.Status.Phase = virtv2.ImagePending

		ds.logger.Info("Cleaning up...", "cvi", cvi.Name)
	case pod == nil:
		condition.Status = metav1.ConditionFalse
		condition.Reason = cvicondition.ReadyReason_Provisioning
		condition.Message = "DVCR Provisioner not found: create the new one."

		cvi.Status.Phase = virtv2.ImageProvisioning
		cvi.Status.Progress = ds.statService.GetProgress(cvi.GetUID(), pod, cvi.Status.Progress)
		cvi.Status.DownloadSpeed = ds.statService.GetDownloadSpeed(cvi.GetUID(), pod)
		cvi.Status.Target.RegistryURL = ds.dvcrSettings.RegistryImageForCVMI(cvi.Name)

		envSettings := ds.getEnvSettings(cvi, supgen)
		err = ds.importerService.Start(ctx, envSettings, cvi, supgen, datasource.NewCABundleForCVMI(cvi.Spec.DataSource))
		if err != nil {
			return false, err
		}

		ds.logger.Info("Create importer pod...", "cvi", cvi.Name, "progress", cvi.Status.Progress, "pod.phase", "nil")
	case cc.IsPodComplete(pod):
		condition.Status = metav1.ConditionTrue
		condition.Reason = cvicondition.ReadyReason_Ready
		condition.Message = ""

		cvi.Status.Phase = virtv2.ImageReady
		cvi.Status.Size = ds.statService.GetSize(pod)
		cvi.Status.CDROM = ds.statService.GetCDROM(pod)
		cvi.Status.Format = ds.statService.GetFormat(pod)
		cvi.Status.Progress = ds.statService.GetProgress(cvi.GetUID(), pod, cvi.Status.Progress)
		cvi.Status.DownloadSpeed = ds.statService.GetDownloadSpeed(cvi.GetUID(), pod)
		cvi.Status.Target.RegistryURL = ds.dvcrSettings.RegistryImageForCVMI(cvi.Name)

		ds.logger.Info("Ready", "cvi", cvi.Name, "progress", cvi.Status.Progress, "pod.phase", pod.Status.Phase)
	default:
		err = ds.statService.CheckPod(pod)
		if err != nil {
			cvi.Status.Phase = virtv2.ImageFailed

			switch {
			case errors.Is(err, service.ErrNotInitialized), errors.Is(err, service.ErrNotScheduled):
				condition.Status = metav1.ConditionFalse
				condition.Reason = cvicondition.ReadyReason_ProvisioningNotStarted
				condition.Message = service.CapitalizeFirstLetter(err.Error() + ".")
				return false, nil
			case errors.Is(err, service.ErrProvisioningFailed):
				condition.Status = metav1.ConditionFalse
				condition.Reason = cvicondition.ReadyReason_ProvisioningFailed
				condition.Message = service.CapitalizeFirstLetter(err.Error() + ".")
				return false, nil
			default:
				return false, err
			}
		}

		err = ds.importerService.Protect(ctx, pod)
		if err != nil {
			return false, err
		}

		condition.Status = metav1.ConditionFalse
		condition.Reason = cvicondition.ReadyReason_Provisioning
		condition.Message = "Import is in the process of provisioning to DVCR."

		cvi.Status.Phase = virtv2.ImageProvisioning
		cvi.Status.Progress = ds.statService.GetProgress(cvi.GetUID(), pod, cvi.Status.Progress)
		cvi.Status.DownloadSpeed = ds.statService.GetDownloadSpeed(cvi.GetUID(), pod)
		cvi.Status.Target.RegistryURL = ds.dvcrSettings.RegistryImageForCVMI(cvi.Name)

		ds.logger.Info("Provisioning...", "cvi", cvi.Name, "progress", cvi.Status.Progress, "pod.phase", pod.Status.Phase)
	}

	return true, nil
}

func (ds HTTPDataSource) CleanUp(ctx context.Context, cvi *virtv2.ClusterVirtualImage) (bool, error) {
	supgen := supplements.NewGenerator(cvmi.ShortName, cvi.Name, ds.controllerNamespace, cvi.UID)

	requeue, err := ds.importerService.CleanUp(ctx, supgen)
	if err != nil {
		return false, err
	}

	return requeue, nil
}

func (ds HTTPDataSource) Validate(_ context.Context, _ *virtv2.ClusterVirtualImage) error {
	return nil
}

func (ds HTTPDataSource) getEnvSettings(cvi *virtv2.ClusterVirtualImage, supgen *supplements.Generator) *importer.Settings {
	var settings importer.Settings

	importer.ApplyHTTPSourceSettings(&settings, cvi.Spec.DataSource.HTTP, supgen)
	importer.ApplyDVCRDestinationSettings(
		&settings,
		ds.dvcrSettings,
		supgen,
		ds.dvcrSettings.RegistryImageForCVMI(cvi.Name),
	)

	return &settings
}
