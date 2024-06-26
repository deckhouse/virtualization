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

	"github.com/deckhouse/virtualization-controller/pkg/common/datasource"
	cc "github.com/deckhouse/virtualization-controller/pkg/controller/common"
	"github.com/deckhouse/virtualization-controller/pkg/controller/importer"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/supplements"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vi/internal/util"
	"github.com/deckhouse/virtualization-controller/pkg/dvcr"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vicondition"
)

type HTTPDataSource struct {
	statService     Stat
	importerService Importer
	dvcrSettings    *dvcr.Settings
	logger          *slog.Logger
}

func NewHTTPDataSource(
	statService Stat,
	importerService Importer,
	dvcrSettings *dvcr.Settings,
) *HTTPDataSource {
	return &HTTPDataSource{
		statService:     statService,
		importerService: importerService,
		dvcrSettings:    dvcrSettings,
		logger:          slog.Default().With("controller", util.ControllerShortName, "ds", "http"),
	}
}

func (ds HTTPDataSource) Sync(ctx context.Context, vi *virtv2.VirtualImage) (bool, error) {
	ds.logger.Info("Sync", "vi", vi.Name)

	condition, _ := service.GetCondition(vicondition.ReadyType, vi.Status.Conditions)
	defer func() { service.SetCondition(condition, &vi.Status.Conditions) }()

	supgen := supplements.NewGenerator(util.ControllerShortName, vi.Name, vi.Namespace, vi.UID)
	pod, err := ds.importerService.GetPod(ctx, supgen)
	if err != nil {
		return false, err
	}

	switch {
	case isDiskProvisioningFinished(condition):
		ds.logger.Info("Finishing...", "vi", vi.Name)

		condition.Status = metav1.ConditionTrue
		condition.Reason = vicondition.Ready
		condition.Message = ""

		vi.Status.Phase = virtv2.ImageReady

		err = ds.importerService.Unprotect(ctx, pod)
		if err != nil {
			return false, err
		}

		return CleanUp(ctx, vi, ds)
	case cc.IsTerminating(pod):
		vi.Status.Phase = virtv2.ImagePending

		ds.logger.Info("Cleaning up...", "vi", vi.Name)
	case pod == nil:
		condition.Status = metav1.ConditionFalse
		condition.Reason = vicondition.Provisioning
		condition.Message = "DVCR Provisioner not found: create the new one."

		vi.Status.Phase = virtv2.ImageProvisioning
		vi.Status.Progress = ds.statService.GetProgress(vi.GetUID(), pod, vi.Status.Progress)
		vi.Status.DownloadSpeed = ds.statService.GetDownloadSpeed(vi.GetUID(), pod)
		vi.Status.Target.RegistryURL = ds.dvcrSettings.RegistryImageForCVMI(vi.Name)

		envSettings := ds.getEnvSettings(vi, supgen)
		err = ds.importerService.Start(ctx, envSettings, vi, supgen, datasource.NewCABundleForVMI(vi.Spec.DataSource))
		if err != nil {
			return false, err
		}

		ds.logger.Info("Create importer pod...", "vi", vi.Name, "progress", vi.Status.Progress, "pod.phase", "nil")
	case cc.IsPodComplete(pod):
		err = ds.statService.CheckPod(pod)
		if err != nil {
			vi.Status.Phase = virtv2.ImageFailed

			switch {
			case errors.Is(err, service.ErrProvisioningFailed):
				condition.Status = metav1.ConditionFalse
				condition.Reason = vicondition.ProvisioningFailed
				condition.Message = service.CapitalizeFirstLetter(err.Error() + ".")
				return false, nil
			default:
				return false, err
			}
		}

		condition.Status = metav1.ConditionTrue
		condition.Reason = vicondition.Ready
		condition.Message = ""

		vi.Status.Phase = virtv2.ImageReady
		vi.Status.Size = ds.statService.GetSize(pod)
		vi.Status.CDROM = ds.statService.GetCDROM(pod)
		vi.Status.Format = ds.statService.GetFormat(pod)
		vi.Status.Progress = ds.statService.GetProgress(vi.GetUID(), pod, vi.Status.Progress)
		vi.Status.DownloadSpeed = ds.statService.GetDownloadSpeed(vi.GetUID(), pod)
		vi.Status.Target.RegistryURL = ds.dvcrSettings.RegistryImageForCVMI(vi.Name)

		ds.logger.Info("Ready", "vi", vi.Name, "progress", vi.Status.Progress, "pod.phase", pod.Status.Phase)
	default:
		err = ds.statService.CheckPod(pod)
		if err != nil {
			vi.Status.Phase = virtv2.ImageFailed

			switch {
			case errors.Is(err, service.ErrNotInitialized), errors.Is(err, service.ErrNotScheduled):
				condition.Status = metav1.ConditionFalse
				condition.Reason = vicondition.ProvisioningNotStarted
				condition.Message = service.CapitalizeFirstLetter(err.Error() + ".")
				return false, nil
			case errors.Is(err, service.ErrProvisioningFailed):
				condition.Status = metav1.ConditionFalse
				condition.Reason = vicondition.ProvisioningFailed
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
		condition.Reason = vicondition.Provisioning
		condition.Message = "Import is in the process of provisioning to DVCR."

		vi.Status.Phase = virtv2.ImageProvisioning
		vi.Status.Progress = ds.statService.GetProgress(vi.GetUID(), pod, vi.Status.Progress)
		vi.Status.DownloadSpeed = ds.statService.GetDownloadSpeed(vi.GetUID(), pod)
		vi.Status.Target.RegistryURL = ds.dvcrSettings.RegistryImageForCVMI(vi.Name)

		ds.logger.Info("Provisioning...", "vi", vi.Name, "progress", vi.Status.Progress, "pod.phase", pod.Status.Phase)
	}

	return true, nil
}

func (ds HTTPDataSource) CleanUp(ctx context.Context, vi *virtv2.VirtualImage) (bool, error) {
	supgen := supplements.NewGenerator(util.ControllerShortName, vi.Name, vi.Namespace, vi.UID)

	requeue, err := ds.importerService.CleanUp(ctx, supgen)
	if err != nil {
		return false, err
	}

	return requeue, nil
}

func (ds HTTPDataSource) Validate(_ context.Context, _ *virtv2.VirtualImage) error {
	return nil
}

func (ds HTTPDataSource) getEnvSettings(vi *virtv2.VirtualImage, supgen *supplements.Generator) *importer.Settings {
	var settings importer.Settings

	importer.ApplyHTTPSourceSettings(&settings, vi.Spec.DataSource.HTTP, supgen)
	importer.ApplyDVCRDestinationSettings(
		&settings,
		ds.dvcrSettings,
		supgen,
		ds.dvcrSettings.RegistryImageForVMI(vi.Name, vi.Namespace),
	)

	return &settings
}
