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

	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	cdiv1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"

	"github.com/deckhouse/virtualization-controller/pkg/controller/common"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/supplements"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	"github.com/deckhouse/virtualization-controller/pkg/util"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vicondition"
)

type ObjectRefVirtualImageOnPvc struct {
	diskService *service.DiskService
}

func NewObjectRefVirtualImageOnPvc(diskService *service.DiskService) *ObjectRefVirtualImageOnPvc {
	return &ObjectRefVirtualImageOnPvc{
		diskService: diskService,
	}
}

func (ds ObjectRefVirtualImageOnPvc) Sync(ctx context.Context, vd *virtv2.VirtualDisk, viRef *virtv2.VirtualImage, condition *metav1.Condition) (bool, error) {
	log, _ := logger.GetDataSourceContext(ctx, objectRefDataSource)

	supgen := supplements.NewGenerator(common.VDShortName, vd.Name, vd.Namespace, vd.UID)
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

		setPhaseConditionForFinishedDisk(pvc, condition, &vd.Status.Phase, supgen)

		// Protect Ready Disk and underlying PVC.
		err = ds.diskService.Protect(ctx, vd, nil, pvc)
		if err != nil {
			return false, err
		}

		err = ds.diskService.Unprotect(ctx, dv)
		if err != nil {
			return false, err
		}

		return CleanUpSupplements(ctx, vd, ds)
	case common.AnyTerminating(dv, pvc):
		log.Info("Waiting for supplements to be terminated")
	case dv == nil:
		log.Info("Start import to PVC")

		vd.Status.Progress = "0%"
		vd.Status.SourceUID = util.GetPointer(viRef.GetUID())

		refSupgen := supplements.NewGenerator(common.VIShortName, viRef.Name, viRef.Namespace, viRef.UID)

		refPvc, err := ds.diskService.GetPersistentVolumeClaim(ctx, refSupgen)
		if err != nil {
			return false, err
		}

		var size resource.Quantity
		size, err = ds.getPVCSize(vd, viRef.Status.Size)
		if err != nil {
			setPhaseConditionToFailed(condition, &vd.Status.Phase, err)

			if errors.Is(err, service.ErrInsufficientPVCSize) {
				return false, nil
			}

			return false, err
		}

		source := &cdiv1.DataVolumeSource{
			PVC: &cdiv1.DataVolumeSourcePVC{
				Name:      refPvc.Name,
				Namespace: refPvc.Namespace,
			},
		}

		err = ds.diskService.StartClone(ctx, size, vd.Spec.PersistentVolumeClaim.StorageClass, source, vd, supgen)
		if err != nil {
			return false, err
		}

		vd.Status.Phase = virtv2.DiskProvisioning
		condition.Status = metav1.ConditionFalse
		condition.Reason = vicondition.Provisioning
		condition.Message = "PVC Provisioner not found: create the new one."

		return true, nil
	case pvc == nil:
		vd.Status.Phase = virtv2.DiskProvisioning
		condition.Status = metav1.ConditionFalse
		condition.Reason = vicondition.Provisioning
		condition.Message = "PVC not found: waiting for creation."
		return true, nil
	case ds.diskService.IsImportDone(dv, pvc):
		log.Info("Import has completed", "dvProgress", dv.Status.Progress, "dvPhase", dv.Status.Phase, "pvcPhase", pvc.Status.Phase)

		vd.Status.Phase = virtv2.DiskReady
		condition.Status = metav1.ConditionTrue
		condition.Reason = vicondition.Ready
		condition.Message = ""

		vd.Status.Progress = "100%"
		vd.Status.Capacity = ds.diskService.GetCapacity(pvc)
		vd.Status.Target.PersistentVolumeClaim = dv.Status.ClaimName
	default:
		log.Info("Provisioning to PVC is in progress", "dvProgress", dv.Status.Progress, "dvPhase", dv.Status.Phase, "pvcPhase", pvc.Status.Phase)

		vd.Status.Progress = ds.diskService.GetProgress(dv, vd.Status.Progress, service.NewScaleOption(0, 100))
		vd.Status.Target.PersistentVolumeClaim = dv.Status.ClaimName

		err = ds.diskService.Protect(ctx, vd, dv, pvc)
		if err != nil {
			return false, err
		}

		err = setPhaseConditionForPVCProvisioningDisk(ctx, dv, vd, pvc, condition, ds.diskService)
		if err != nil {
			return false, err
		}

		return false, nil
	}

	return true, nil
}

func (ds ObjectRefVirtualImageOnPvc) CleanUpSupplements(ctx context.Context, vd *virtv2.VirtualDisk) (bool, error) {
	supgen := supplements.NewGenerator(common.VIShortName, vd.Name, vd.Namespace, vd.UID)

	diskRequeue, err := ds.diskService.CleanUpSupplements(ctx, supgen)
	if err != nil {
		return false, err
	}

	return diskRequeue, nil
}

func (ds ObjectRefVirtualImageOnPvc) getPVCSize(vd *virtv2.VirtualDisk, is virtv2.ImageStatusSize) (resource.Quantity, error) {
	unpackedSize, err := resource.ParseQuantity(is.UnpackedBytes)
	if err != nil {
		return resource.Quantity{}, fmt.Errorf("failed to parse unpacked bytes %s: %w", is.UnpackedBytes, err)
	}

	if unpackedSize.IsZero() {
		return resource.Quantity{}, errors.New("got zero unpacked size from data source")
	}

	return service.GetValidatedPVCSize(vd.Spec.PersistentVolumeClaim.Size, unpackedSize)
}
