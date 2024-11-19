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

	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	cdiv1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/common"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/supplements"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdcondition"
)

const blankDataSource = "blank"

type BlankDataSource struct {
	statService         *service.StatService
	diskService         *service.DiskService
	storageClassService *service.VirtualDiskStorageClassService
}

func NewBlankDataSource(
	statService *service.StatService,
	diskService *service.DiskService,
	storageClassService *service.VirtualDiskStorageClassService,
) *BlankDataSource {
	return &BlankDataSource{
		statService:         statService,
		diskService:         diskService,
		storageClassService: storageClassService,
	}
}

func (ds BlankDataSource) Sync(ctx context.Context, vd *virtv2.VirtualDisk) (reconcile.Result, error) {
	log, ctx := logger.GetDataSourceContext(ctx, blankDataSource)

	condition, _ := conditions.GetCondition(vdcondition.ReadyType, vd.Status.Conditions)
	cb := conditions.NewConditionBuilder(vdcondition.ReadyType).Generation(vd.Generation)
	defer func() { conditions.SetCondition(cb, &vd.Status.Conditions) }()

	supgen := supplements.NewGenerator(common.VDShortName, vd.Name, vd.Namespace, vd.UID)
	dv, err := ds.diskService.GetDataVolume(ctx, supgen)
	if err != nil {
		return reconcile.Result{}, err
	}
	pvc, err := ds.diskService.GetPersistentVolumeClaim(ctx, supgen)
	if err != nil {
		return reconcile.Result{}, err
	}

	clusterDefaultSC, _ := ds.diskService.GetDefaultStorageClass(ctx)
	sc, err := ds.storageClassService.GetStorageClass(vd.Spec.PersistentVolumeClaim.StorageClass, clusterDefaultSC)
	if updated, err := setConditionFromStorageClassError(err, cb); err != nil || updated {
		return reconcile.Result{}, err
	}

	switch {
	case isDiskProvisioningFinished(condition):
		log.Debug("Disk provisioning finished: clean up")

		setPhaseConditionForFinishedDisk(pvc, cb, &vd.Status.Phase, supgen)

		// Protect Ready Disk and underlying PVC.
		err = ds.diskService.Protect(ctx, vd, nil, pvc)
		if err != nil {
			return reconcile.Result{}, err
		}

		err = ds.diskService.Unprotect(ctx, dv)
		if err != nil {
			return reconcile.Result{}, err
		}

		return CleanUpSupplements(ctx, vd, ds)
	case common.AnyTerminating(dv, pvc):
		log.Info("Waiting for supplements to be terminated")
	case dv == nil:
		log.Info("Start import to PVC")

		vd.Status.Progress = "0%"

		var diskSize resource.Quantity
		diskSize, err = ds.getPVCSize(vd)
		if err != nil {
			setPhaseConditionToFailed(cb, &vd.Status.Phase, err)

			return reconcile.Result{}, err
		}

		source := ds.getSource()

		err = ds.diskService.Start(ctx, diskSize, sc, source, vd, supgen)

		if updated, err := setPhaseConditionFromStorageError(err, vd, cb); err != nil || updated {
			return reconcile.Result{}, err
		}

		vd.Status.Phase = virtv2.DiskProvisioning
		cb.
			Status(metav1.ConditionFalse).
			Reason(vdcondition.Provisioning).
			Message("PVC Provisioner not found: create the new one.")

		return reconcile.Result{Requeue: true}, nil
	case pvc == nil:
		vd.Status.Phase = virtv2.DiskProvisioning
		cb.
			Status(metav1.ConditionFalse).
			Reason(vdcondition.Provisioning).
			Message("PVC not found: waiting for creation.")
		return reconcile.Result{Requeue: true}, nil
	case ds.diskService.IsImportDone(dv, pvc):
		log.Info("Import has completed", "dvProgress", dv.Status.Progress, "dvPhase", dv.Status.Phase, "pvcPhase", pvc.Status.Phase)

		vd.Status.Phase = virtv2.DiskReady
		cb.
			Status(metav1.ConditionTrue).
			Reason(vdcondition.Ready).
			Message("")

		vd.Status.Progress = "100%"
		vd.Status.Capacity = ds.diskService.GetCapacity(pvc)
		vd.Status.Target.PersistentVolumeClaim = dv.Status.ClaimName
	default:
		log.Info("Provisioning to PVC is in progress", "dvProgress", dv.Status.Progress, "dvPhase", dv.Status.Phase, "pvcPhase", pvc.Status.Phase)

		vd.Status.Progress = ds.diskService.GetProgress(dv, vd.Status.Progress, service.NewScaleOption(0, 100))
		vd.Status.Capacity = ds.diskService.GetCapacity(pvc)
		vd.Status.Target.PersistentVolumeClaim = dv.Status.ClaimName

		err = ds.diskService.Protect(ctx, vd, dv, pvc)
		if err != nil {
			return reconcile.Result{}, err
		}
		sc, err := ds.diskService.GetStorageClass(ctx, pvc.Spec.StorageClassName)
		if updated, err := setPhaseConditionFromStorageError(err, vd, cb); err != nil || updated {
			return reconcile.Result{}, err
		}
		if err = setPhaseConditionForPVCProvisioningDisk(ctx, dv, vd, pvc, sc, cb, ds.diskService); err != nil {
			return reconcile.Result{}, err
		}
		return reconcile.Result{}, nil
	}

	return reconcile.Result{Requeue: true}, nil
}

func (ds BlankDataSource) CleanUp(ctx context.Context, vd *virtv2.VirtualDisk) (bool, error) {
	supgen := supplements.NewGenerator(common.VDShortName, vd.Name, vd.Namespace, vd.UID)

	requeue, err := ds.diskService.CleanUp(ctx, supgen)
	if err != nil {
		return false, err
	}

	return requeue, nil
}

func (ds BlankDataSource) CleanUpSupplements(ctx context.Context, vd *virtv2.VirtualDisk) (reconcile.Result, error) {
	supgen := supplements.NewGenerator(common.VDShortName, vd.Name, vd.Namespace, vd.UID)

	requeue, err := ds.diskService.CleanUpSupplements(ctx, supgen)
	if err != nil {
		return reconcile.Result{}, err
	}

	return reconcile.Result{Requeue: requeue}, nil
}

func (ds BlankDataSource) Validate(_ context.Context, _ *virtv2.VirtualDisk) error {
	return nil
}

func (ds BlankDataSource) Name() string {
	return blankDataSource
}

func (ds BlankDataSource) getSource() *cdiv1.DataVolumeSource {
	return &cdiv1.DataVolumeSource{
		Blank: &cdiv1.DataVolumeBlankImage{},
	}
}

func (ds BlankDataSource) getPVCSize(vd *virtv2.VirtualDisk) (resource.Quantity, error) {
	pvcSize := vd.Spec.PersistentVolumeClaim.Size
	if pvcSize == nil || pvcSize.IsZero() {
		return resource.Quantity{}, errors.New("spec.persistentVolumeClaim.size should be set for blank virtual disk")
	}

	return *pvcSize, nil
}
