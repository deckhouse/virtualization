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
	cdiv1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/common/datasource"
	"github.com/deckhouse/virtualization-controller/pkg/controller/common"
	"github.com/deckhouse/virtualization-controller/pkg/controller/importer"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/supplements"
	"github.com/deckhouse/virtualization-controller/pkg/dvcr"
	"github.com/deckhouse/virtualization-controller/pkg/imageformat"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	"github.com/deckhouse/virtualization-controller/pkg/sdk/framework/helper"
	"github.com/deckhouse/virtualization-controller/pkg/util"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vicondition"
)

type ObjectRefVirtualDisk struct {
	importerService    Importer
	diskService        *service.DiskService
	statService        Stat
	dvcrSettings       *dvcr.Settings
	client             client.Client
	storageClassForPVC string
}

func NewObjectRefVirtualDisk(importerService Importer, client client.Client, diskService *service.DiskService, dvcrSettings *dvcr.Settings, statService Stat, storageClassForPVC string) *ObjectRefVirtualDisk {
	return &ObjectRefVirtualDisk{
		importerService:    importerService,
		client:             client,
		diskService:        diskService,
		statService:        statService,
		dvcrSettings:       dvcrSettings,
		storageClassForPVC: storageClassForPVC,
	}
}

func (ds ObjectRefVirtualDisk) StoreToDVCR(ctx context.Context, vi *virtv2.VirtualImage, vdRef *virtv2.VirtualDisk, condition *metav1.Condition) (bool, error) {
	log, ctx := logger.GetDataSourceContext(ctx, "objectref")

	supgen := supplements.NewGenerator(common.VIShortName, vi.Name, vdRef.Namespace, vi.UID)
	pod, err := ds.importerService.GetPod(ctx, supgen)
	if err != nil {
		return false, err
	}

	switch {
	case isDiskProvisioningFinished(*condition):
		log.Info("Virtual image provisioning finished: clean up")

		condition.Status = metav1.ConditionTrue
		condition.Reason = vicondition.Ready
		condition.Message = ""

		vi.Status.Phase = virtv2.ImageReady

		err = ds.importerService.Unprotect(ctx, pod)
		if err != nil {
			return false, err
		}

		return CleanUp(ctx, vi, ds)
	case common.IsTerminating(pod):
		vi.Status.Phase = virtv2.ImagePending

		log.Info("Cleaning up...")
	case pod == nil:
		vi.Status.Progress = ds.statService.GetProgress(vi.GetUID(), pod, vi.Status.Progress)
		vi.Status.Target.RegistryURL = ds.statService.GetDVCRImageName(pod)

		envSettings := ds.getEnvSettings(vi, supgen)

		ownerRef := metav1.NewControllerRef(vi, vi.GroupVersionKind())
		podSettings := ds.importerService.GetPodSettingsWithPVC(ownerRef, supgen, vdRef.Status.Target.PersistentVolumeClaim, vdRef.Namespace)
		err = ds.importerService.StartWithPodSetting(ctx, envSettings, supgen, datasource.NewCABundleForVMI(vi.Spec.DataSource), podSettings)

		var requeue bool
		requeue, err = setPhaseConditionForImporterStart(condition, &vi.Status.Phase, err)
		if err != nil {
			return false, err
		}

		log.Info("Create importer pod...", "progress", vi.Status.Progress, "pod.phase", "nil")

		return requeue, nil
	case common.IsPodComplete(pod):
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
		vi.Status.Progress = "100%"
		vi.Status.Target.RegistryURL = ds.statService.GetDVCRImageName(pod)

		log.Info("Ready", "progress", vi.Status.Progress, "pod.phase", pod.Status.Phase)
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
		vi.Status.Target.RegistryURL = ds.statService.GetDVCRImageName(pod)

		log.Info("Provisioning...", "progress", vi.Status.Progress, "pod.phase", pod.Status.Phase)
	}

	return true, nil
}

func (ds ObjectRefVirtualDisk) StoreToPVC(ctx context.Context, vi *virtv2.VirtualImage, vdRef *virtv2.VirtualDisk, condition *metav1.Condition) (bool, error) {
	log, _ := logger.GetDataSourceContext(ctx, objectRefDataSource)

	supgen := supplements.NewGenerator(common.VIShortName, vi.Name, vi.Namespace, vi.UID)
	dv, err := ds.diskService.GetDataVolume(ctx, supgen)
	if err != nil {
		return false, err
	}

	pvc, err := ds.diskService.GetPersistentVolumeClaim(ctx, supgen)
	if err != nil {
		return false, err
	}

	switch {
	case isDiskProvisioningFinished(*condition):
		log.Info("Disk provisioning finished: clean up")

		setPhaseConditionForFinishedImage(pvc, condition, &vi.Status.Phase, supgen)

		// Protect Ready Disk and underlying PVC.
		err = ds.diskService.Protect(ctx, vi, nil, pvc)
		if err != nil {
			return false, err
		}

		err = ds.diskService.Unprotect(ctx, dv)
		if err != nil {
			return false, err
		}

		return CleanUpSupplements(ctx, vi, ds)
	case common.AnyTerminating(dv, pvc):
		log.Info("Waiting for supplements to be terminated")
	case dv == nil:
		log.Info("Start import to PVC")

		vi.Status.Progress = "0%"
		vi.Status.SourceUID = util.GetPointer(vdRef.GetUID())

		source := &cdiv1.DataVolumeSource{
			PVC: &cdiv1.DataVolumeSourcePVC{
				Name:      vdRef.Status.Target.PersistentVolumeClaim,
				Namespace: vdRef.Namespace,
			},
		}

		sc, err := ds.diskService.GetStorageClass(ctx, &ds.storageClassForPVC)
		if err != nil {
			setPhaseConditionToFailed(condition, &vi.Status.Phase, err)
			return false, err
		}

		err = ds.diskService.StartImmediate(ctx, *vdRef.Spec.PersistentVolumeClaim.Size, sc.GetName(), source, vi, supgen)
		if err != nil {
			return false, err
		}

		vi.Status.Phase = virtv2.ImageProvisioning
		condition.Status = metav1.ConditionFalse
		condition.Reason = vicondition.Provisioning
		condition.Message = "PVC Provisioner not found: create the new one."

		return true, nil
	case pvc == nil:
		vi.Status.Phase = virtv2.ImageProvisioning
		condition.Status = metav1.ConditionFalse
		condition.Reason = vicondition.Provisioning
		condition.Message = "PVC not found: waiting for creation."
		return true, nil
	case ds.diskService.IsImportDone(dv, pvc):
		log.Info("Import has completed", "dvProgress", dv.Status.Progress, "dvPhase", dv.Status.Phase, "pvcPhase", pvc.Status.Phase)

		vi.Status.Phase = virtv2.ImageReady
		condition.Status = metav1.ConditionTrue
		condition.Reason = vicondition.Ready
		condition.Message = ""

		var imageStatus virtv2.ImageStatusSize
		imageStatus.Stored = vdRef.Spec.PersistentVolumeClaim.Size.String()
		imageStatus.Unpacked = vdRef.Spec.PersistentVolumeClaim.Size.String()
		vi.Status.Size = imageStatus

		vi.Status.Format = imageformat.FormatRAW
		vi.Status.Progress = "100%"
		vi.Status.Target.PersistentVolumeClaim = dv.Status.ClaimName
	default:
		log.Info("Provisioning to PVC is in progress", "dvProgress", dv.Status.Progress, "dvPhase", dv.Status.Phase, "pvcPhase", pvc.Status.Phase)

		vi.Status.Progress = ds.diskService.GetProgress(dv, vi.Status.Progress, service.NewScaleOption(0, 100))
		vi.Status.Target.PersistentVolumeClaim = dv.Status.ClaimName

		err = ds.diskService.Protect(ctx, vi, dv, pvc)
		if err != nil {
			return false, err
		}

		err = setPhaseConditionForPVCProvisioningImage(ctx, dv, vi, pvc, condition, ds.diskService)
		if err != nil {
			return false, err
		}

		return false, nil
	}

	return true, nil
}

func (ds ObjectRefVirtualDisk) CleanUpSupplements(ctx context.Context, vi *virtv2.VirtualImage) (bool, error) {
	supgen := supplements.NewGenerator(common.VIShortName, vi.Name, vi.Namespace, vi.UID)

	importerRequeue, err := ds.importerService.CleanUpSupplements(ctx, supgen)
	if err != nil {
		return false, err
	}

	diskRequeue, err := ds.diskService.CleanUpSupplements(ctx, supgen)
	if err != nil {
		return false, err
	}

	return importerRequeue || diskRequeue, nil
}

func (ds ObjectRefVirtualDisk) CleanUp(ctx context.Context, vi *virtv2.VirtualImage) (bool, error) {
	supgen := supplements.NewGenerator(common.VIShortName, vi.Name, vi.Namespace, vi.UID)

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

func (ds ObjectRefVirtualDisk) getEnvSettings(vi *virtv2.VirtualImage, sup *supplements.Generator) *importer.Settings {
	var settings importer.Settings
	importer.ApplyBlockDeviceSourceSettings(&settings)
	importer.ApplyDVCRDestinationSettings(
		&settings,
		ds.dvcrSettings,
		sup,
		ds.dvcrSettings.RegistryImageForVI(vi),
	)

	return &settings
}

func (ds ObjectRefVirtualDisk) Validate(ctx context.Context, vi *virtv2.VirtualImage) error {
	if vi.Spec.DataSource.ObjectRef == nil || vi.Spec.DataSource.ObjectRef.Kind != virtv2.VirtualImageObjectRefKindVirtualDisk {
		return fmt.Errorf("not a %s data source", virtv2.ClusterVirtualImageObjectRefKindVirtualDisk)
	}

	vd, err := helper.FetchObject(ctx, types.NamespacedName{Name: vi.Spec.DataSource.ObjectRef.Name, Namespace: vi.Namespace}, ds.client, &virtv2.VirtualDisk{})
	if err != nil {
		return err
	}

	if vd == nil || vd.Status.Phase != virtv2.DiskReady {
		return NewVirtualDiskNotReadyError(vi.Spec.DataSource.ObjectRef.Name)
	}

	if len(vd.Status.AttachedToVirtualMachines) > 0 {
		vmName := vd.Status.AttachedToVirtualMachines[0]
		vm, err := helper.FetchObject(ctx, types.NamespacedName{Name: vmName.Name, Namespace: vd.Namespace}, ds.client, &virtv2.VirtualMachine{})
		if err != nil {
			return err
		}

		if vm.Status.Phase != virtv2.MachineStopped {
			return NewVirtualDiskAttachedToRunningVMError(vd.Name, vmName.Name)
		}
	}

	return nil
}
