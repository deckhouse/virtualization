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

	"github.com/deckhouse/virtualization-controller/pkg/common/datasource"
	"github.com/deckhouse/virtualization-controller/pkg/controller/common"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/supplements"
	"github.com/deckhouse/virtualization-controller/pkg/controller/uploader"
	"github.com/deckhouse/virtualization-controller/pkg/dvcr"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/cvicondition"
)

type UploadDataSource struct {
	statService         Stat
	uploaderService     Uploader
	dvcrSettings        *dvcr.Settings
	controllerNamespace string
}

func NewUploadDataSource(
	statService Stat,
	uploaderService Uploader,
	dvcrSettings *dvcr.Settings,
	controllerNamespace string,
) *UploadDataSource {
	return &UploadDataSource{
		statService:         statService,
		uploaderService:     uploaderService,
		dvcrSettings:        dvcrSettings,
		controllerNamespace: controllerNamespace,
	}
}

func (ds UploadDataSource) Sync(ctx context.Context, cvi *virtv2.ClusterVirtualImage) (bool, error) {
	log, ctx := logger.GetDataSourceContext(ctx, "upload")

	condition, _ := service.GetCondition(cvicondition.ReadyType, cvi.Status.Conditions)
	defer func() { service.SetCondition(condition, &cvi.Status.Conditions) }()

	supgen := supplements.NewGenerator(common.CVIShortName, cvi.Name, ds.controllerNamespace, cvi.UID)
	pod, err := ds.uploaderService.GetPod(ctx, supgen)
	if err != nil {
		return false, err
	}
	svc, err := ds.uploaderService.GetService(ctx, supgen)
	if err != nil {
		return false, err
	}
	ing, err := ds.uploaderService.GetIngress(ctx, supgen)
	if err != nil {
		return false, err
	}

	if cvi.Status.UploadCommand == "" {
		if ing != nil && ing.Annotations[common.AnnUploadURL] != "" {
			cvi.Status.UploadCommand = fmt.Sprintf("curl %s -T example.iso", ing.Annotations[common.AnnUploadURL])
		}
	}

	switch {
	case isDiskProvisioningFinished(condition):
		log.Info("Cluster virtual image provisioning finished: clean up")

		condition.Status = metav1.ConditionTrue
		condition.Reason = cvicondition.Ready
		condition.Message = ""

		cvi.Status.Phase = virtv2.ImageReady

		// Unprotect upload time supplements to delete them later.
		err = ds.uploaderService.Unprotect(ctx, pod, svc, ing)
		if err != nil {
			return false, err
		}

		return CleanUp(ctx, cvi, ds)
	case common.AnyTerminating(pod, svc, ing):
		cvi.Status.Phase = virtv2.ImagePending

		log.Info("Cleaning up...")
	case pod == nil || svc == nil || ing == nil:
		envSettings := ds.getEnvSettings(cvi, supgen)
		err = ds.uploaderService.Start(ctx, envSettings, cvi, supgen, datasource.NewCABundleForCVMI(cvi.Spec.DataSource))
		var requeue bool
		requeue, err = setPhaseConditionForUploaderStart(&condition, &cvi.Status.Phase, err)
		if err != nil {
			return false, err
		}

		log.Info("Create uploader pod...", "progress", cvi.Status.Progress, "pod.phase", nil)

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
		cvi.Status.Size = ds.statService.GetSize(pod)
		cvi.Status.CDROM = ds.statService.GetCDROM(pod)
		cvi.Status.Format = ds.statService.GetFormat(pod)
		cvi.Status.Progress = "100%"
		cvi.Status.Target.RegistryURL = ds.statService.GetDVCRImageName(pod)
		cvi.Status.DownloadSpeed = ds.statService.GetDownloadSpeed(cvi.GetUID(), pod)

		log.Info("Ready", "progress", cvi.Status.Progress, "pod.phase", pod.Status.Phase)
	case ds.statService.IsUploadStarted(cvi.GetUID(), pod):
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
		cvi.Status.Progress = ds.statService.GetProgress(cvi.GetUID(), pod, cvi.Status.Progress)
		cvi.Status.Target.RegistryURL = ds.statService.GetDVCRImageName(pod)
		cvi.Status.DownloadSpeed = ds.statService.GetDownloadSpeed(cvi.GetUID(), pod)

		err = ds.uploaderService.Protect(ctx, pod, svc, ing)
		if err != nil {
			return false, err
		}

		log.Info("Provisioning...", "progress", cvi.Status.Progress, "pod.phase", pod.Status.Phase)
	default:
		condition.Status = metav1.ConditionFalse
		condition.Reason = cvicondition.WaitForUserUpload
		condition.Message = "Waiting for the user upload."

		cvi.Status.Phase = virtv2.ImageWaitForUserUpload
		cvi.Status.Target.RegistryURL = ds.statService.GetDVCRImageName(pod)

		log.Info("WaitForUserUpload...", "progress", cvi.Status.Progress, "pod.phase", pod.Status.Phase)
	}

	return true, nil
}

func (ds UploadDataSource) CleanUp(ctx context.Context, cvi *virtv2.ClusterVirtualImage) (bool, error) {
	supgen := supplements.NewGenerator(common.CVIShortName, cvi.Name, ds.controllerNamespace, cvi.UID)

	requeue, err := ds.uploaderService.CleanUp(ctx, supgen)
	if err != nil {
		return false, err
	}

	return requeue, nil
}

func (ds UploadDataSource) Validate(_ context.Context, _ *virtv2.ClusterVirtualImage) error {
	return nil
}

func (ds UploadDataSource) getEnvSettings(cvi *virtv2.ClusterVirtualImage, supgen *supplements.Generator) *uploader.Settings {
	var settings uploader.Settings

	uploader.ApplyDVCRDestinationSettings(
		&settings,
		ds.dvcrSettings,
		supgen,
		ds.dvcrSettings.RegistryImageForCVI(cvi),
	)

	return &settings
}
