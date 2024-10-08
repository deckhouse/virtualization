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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/deckhouse/virtualization-controller/pkg/common/datasource"
	"github.com/deckhouse/virtualization-controller/pkg/controller/common"
	"github.com/deckhouse/virtualization-controller/pkg/controller/importer"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/supplements"
	"github.com/deckhouse/virtualization-controller/pkg/dvcr"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/cvicondition"
)

const controllerName = "cvi-controller"

type ObjectRefVirtualImageOnPvc struct {
	importerService Importer
	statService     Stat
	dvcrSettings    *dvcr.Settings
}

func NewObjectRefVirtualImageOnPvc(importerService Importer, dvcrSettings *dvcr.Settings, statService Stat) *ObjectRefVirtualImageOnPvc {
	return &ObjectRefVirtualImageOnPvc{
		importerService: importerService,
		statService:     statService,
		dvcrSettings:    dvcrSettings,
	}
}

func (ds ObjectRefVirtualImageOnPvc) Sync(ctx context.Context, cvi *virtv2.ClusterVirtualImage, viRef *virtv2.VirtualImage, condition *metav1.Condition) (bool, error) {
	log, ctx := logger.GetDataSourceContext(ctx, "objectref")

	supgen := supplements.NewGenerator(common.CVIShortName, cvi.Name, viRef.Namespace, cvi.UID)
	pod, err := ds.importerService.GetPod(ctx, supgen)
	if err != nil {
		return false, err
	}

	switch {
	case isDiskProvisioningFinished(*condition):
		log.Info("Cluster virtual image provisioning finished: clean up")

		condition.Status = metav1.ConditionTrue
		condition.Reason = cvicondition.Ready
		condition.Message = ""

		cvi.Status.Phase = virtv2.ImageReady

		err = ds.importerService.Unprotect(ctx, pod)
		if err != nil {
			return false, err
		}

		return CleanUp(ctx, cvi, ds)
	case common.IsTerminating(pod):
		cvi.Status.Phase = virtv2.ImagePending

		log.Info("Cleaning up...")
	case pod == nil:
		cvi.Status.Progress = ds.statService.GetProgress(cvi.GetUID(), pod, cvi.Status.Progress)
		cvi.Status.Target.RegistryURL = ds.statService.GetDVCRImageName(pod)

		envSettings := ds.getEnvSettings(cvi, supgen)

		ownerRef := metav1.NewControllerRef(cvi, cvi.GroupVersionKind())
		podSettings := ds.importerService.GetPodSettingsWithPVC(ownerRef, supgen, viRef.Status.Target.PersistentVolumeClaim, viRef.Namespace)
		err = ds.importerService.StartWithPodSetting(ctx, envSettings, supgen, datasource.NewCABundleForCVMI(cvi.Spec.DataSource), podSettings)

		var requeue bool
		requeue, err = setPhaseConditionForImporterStart(condition, &cvi.Status.Phase, err)
		if err != nil {
			return false, err
		}

		log.Info("Create importer pod...", "progress", cvi.Status.Progress, "pod.phase", "nil")

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
		cvi.Status.Size = viRef.Status.Size
		cvi.Status.CDROM = viRef.Status.CDROM
		cvi.Status.Format = viRef.Status.Format
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

		err = ds.importerService.Protect(ctx, pod)
		if err != nil {
			return false, err
		}

		condition.Status = metav1.ConditionFalse
		condition.Reason = cvicondition.Provisioning
		condition.Message = "Import is in the process of provisioning to DVCR."

		cvi.Status.Phase = virtv2.ImageProvisioning
		cvi.Status.Progress = ds.statService.GetProgress(cvi.GetUID(), pod, cvi.Status.Progress)
		cvi.Status.Target.RegistryURL = ds.statService.GetDVCRImageName(pod)

		log.Info("Provisioning...", "progress", cvi.Status.Progress, "pod.phase", pod.Status.Phase)
	}

	return true, nil
}

func (ds ObjectRefVirtualImageOnPvc) CleanUp(ctx context.Context, cvi *virtv2.ClusterVirtualImage) (bool, error) {
	importerRequeue, err := ds.importerService.DeletePod(ctx, cvi, controllerName)
	if err != nil {
		return false, err
	}

	return importerRequeue, nil
}

func (ds ObjectRefVirtualImageOnPvc) getEnvSettings(cvi *virtv2.ClusterVirtualImage, sup *supplements.Generator) *importer.Settings {
	var settings importer.Settings
	importer.ApplyBlockDeviceSourceSettings(&settings)
	importer.ApplyDVCRDestinationSettings(
		&settings,
		ds.dvcrSettings,
		sup,
		ds.dvcrSettings.RegistryImageForCVI(cvi),
	)

	return &settings
}
