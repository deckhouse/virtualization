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
	cc "github.com/deckhouse/virtualization-controller/pkg/controller/common"
	"github.com/deckhouse/virtualization-controller/pkg/controller/importer"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/supplements"
	"github.com/deckhouse/virtualization-controller/pkg/dvcr"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vicondition"
)

type ObjectRefVirtualDisk struct {
	importerService     Importer
	diskService         *service.DiskService
	statService         Stat
	dvcrSettings        *dvcr.Settings
	controllerNamespace string
}

func NewObjectRefVirtualDisk(importerService Importer, diskService *service.DiskService, controllerNamespace string, dvcrSettings *dvcr.Settings, statService Stat) *ObjectRefVirtualDisk {
	return &ObjectRefVirtualDisk{
		importerService:     importerService,
		diskService:         diskService,
		statService:         statService,
		dvcrSettings:        dvcrSettings,
		controllerNamespace: controllerNamespace,
	}
}

func (ds ObjectRefVirtualDisk) Sync(ctx context.Context, cvi *virtv2.ClusterVirtualImage, vdRef *virtv2.VirtualDisk, condition *metav1.Condition) (bool, error) {
	log, ctx := logger.GetDataSourceContext(ctx, "objectref")

	supgen := supplements.NewGenerator(cc.CVIShortName, cvi.Name, vdRef.Namespace, cvi.UID)
	pod, err := ds.importerService.GetPod(ctx, supgen)
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

		ownerRef := metav1.NewControllerRef(cvi, cvi.GroupVersionKind())
		podSettings := ds.importerService.GetPodSettingsWithPVC(ownerRef, supgen, vdRef.Status.Target.PersistentVolumeClaim, vdRef.Namespace)
		err = ds.importerService.StartWithPodSetting(ctx, envSettings, supgen, datasource.NewCABundleForCVMI(cvi.Spec.DataSource), podSettings)

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
		cvi.Status.Size = ds.statService.GetSize(pod)
		cvi.Status.CDROM = ds.statService.GetCDROM(pod)
		cvi.Status.Format = ds.statService.GetFormat(pod)
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

func (ds ObjectRefVirtualDisk) CleanUp(ctx context.Context, cvi *virtv2.ClusterVirtualImage) (bool, error) {
	supgen := supplements.NewGenerator(cc.CVIShortName, cvi.Name, cvi.Spec.DataSource.ObjectRef.Namespace, cvi.UID)

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

func (ds ObjectRefVirtualDisk) getEnvSettings(cvi *virtv2.ClusterVirtualImage, sup *supplements.Generator) *importer.Settings {
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

func (ds ObjectRefVirtualDisk) Validate(ctx context.Context, cvi *virtv2.ClusterVirtualImage) error {
	if cvi.Spec.DataSource.ObjectRef == nil || cvi.Spec.DataSource.ObjectRef.Kind != virtv2.ClusterVirtualImageObjectRefKindVirtualDisk {
		return fmt.Errorf("not a %s data source", virtv2.ClusterVirtualImageObjectRefKindVirtualDisk)
	}

	vd, err := ds.diskService.GetVirtualDisk(ctx, cvi.Spec.DataSource.ObjectRef.Name, cvi.Spec.DataSource.ObjectRef.Namespace)
	if err != nil {
		return err
	}

	if vd == nil || vd.Status.Phase != virtv2.DiskReady {
		return NewVirtualDiskNotReadyError(cvi.Spec.DataSource.ObjectRef.Name)
	}

	if len(vd.Status.AttachedToVirtualMachines) != 0 {
		vmName := vd.Status.AttachedToVirtualMachines[0]
		vm, err := ds.diskService.GetVirtualMachine(ctx, vmName.Name, vd.Namespace)
		if err != nil {
			return err
		}

		if vm.Status.Phase != virtv2.MachineStopped {
			return NewVirtualDiskAttachedToRunningVMError(vd.Name, vmName.Name)
		}
	}

	return nil
}
