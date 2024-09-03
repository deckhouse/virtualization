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
	cc "github.com/deckhouse/virtualization-controller/pkg/controller/common"
	"github.com/deckhouse/virtualization-controller/pkg/controller/importer"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/supplements"
	"github.com/deckhouse/virtualization-controller/pkg/dvcr"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vicondition"
)

type ObjectRefVirtualImageOnPvc struct {
	importerService     Importer
	diskService         *service.DiskService
	statService         Stat
	dvcrSettings        *dvcr.Settings
	controllerNamespace string
}

func NewObjectRefVirtualImageOnPvc(importerService Importer, diskService *service.DiskService, controllerNamespace string, statService Stat) *ObjectRefVirtualImageOnPvc {
	return &ObjectRefVirtualImageOnPvc{
		importerService:     importerService,
		diskService:         diskService,
		statService:         statService,
		controllerNamespace: controllerNamespace,
	}
}

func (ds ObjectRefVirtualImageOnPvc) Sync(ctx context.Context, cvi *virtv2.ClusterVirtualImage, viRef *virtv2.VirtualImage, condition *metav1.Condition) (bool, error) {
	log, ctx := logger.GetDataSourceContext(ctx, "objectref")

	supgen := supplements.NewGenerator(cc.CVIShortName, cvi.Name, ds.controllerNamespace, cvi.UID)
	pod, err := ds.importerService.GetPod(ctx, supgen)
	if err != nil {
		return false, err
	}

	refSupgen := supplements.NewGenerator(cc.VIShortName, viRef.Name, viRef.Namespace, viRef.UID)

	refPvc, err := ds.diskService.GetPersistentVolumeClaim(ctx, refSupgen)
	if err != nil {
		return false, err
	}

	switch {
	case isDiskProvisioningFinished(*condition):
		log.Info("Cluster virtual image provisioning finished: clean up")

		condition.Status = metav1.ConditionTrue
		condition.Reason = vicondition.Ready
		condition.Message = ""

		cvi.Status.Phase = virtv2.ImageReady

		err = ds.importerService.Unprotect(ctx, pod)
		if err != nil {
			return false, err
		}

		return CleanUp(ctx, cvi, ds)
	case cc.IsTerminating(pod):
		cvi.Status.Phase = virtv2.ImagePending

		log.Info("Cleaning up...")
	case pod == nil:
		cvi.Status.Progress = ds.statService.GetProgress(cvi.GetUID(), pod, cvi.Status.Progress)
		cvi.Status.Target.RegistryURL = ds.statService.GetDVCRImageName(pod)

		envSettings := ds.getEnvSettings(cvi, supgen)

		err = ds.importerService.StartFromPVC(ctx, envSettings, cvi, supgen, datasource.NewCABundleForCVMI(cvi.Spec.DataSource), refPvc.Name)
		var requeue bool
		requeue, err = setPhaseConditionForImporterStart(condition, &cvi.Status.Phase, err)
		if err != nil {
			return false, err
		}

		log.Info("Create importer pod...", "progress", cvi.Status.Progress, "pod.phase", "nil")

		return requeue, nil
	case cc.IsPodComplete(pod):
		err = ds.statService.CheckPod(pod)
		if err != nil {
			cvi.Status.Phase = virtv2.ImageFailed

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

		cvi.Status.Phase = virtv2.ImageProvisioning
		cvi.Status.Progress = ds.statService.GetProgress(cvi.GetUID(), pod, cvi.Status.Progress)
		cvi.Status.Target.RegistryURL = ds.statService.GetDVCRImageName(pod)

		log.Info("Provisioning...", "progress", cvi.Status.Progress, "pod.phase", pod.Status.Phase)
	}

	return true, nil
}

func (ds ObjectRefVirtualImageOnPvc) CleanUpSupplements(ctx context.Context, vd *virtv2.VirtualDisk) (bool, error) {
	supgen := supplements.NewGenerator(cc.VIShortName, vd.Name, vd.Namespace, vd.UID)

	diskRequeue, err := ds.diskService.CleanUpSupplements(ctx, supgen)
	if err != nil {
		return false, err
	}

	return diskRequeue, nil
}

func (ds ObjectRefVirtualImageOnPvc) CleanUp(ctx context.Context, cvi *virtv2.ClusterVirtualImage) (bool, error) {
	supgen := supplements.NewGenerator(cc.CVIShortName, cvi.Name, ds.controllerNamespace, cvi.UID)

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
