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
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/common"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/supplements"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	"github.com/deckhouse/virtualization-controller/pkg/util"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdcondition"
)

type ObjectRefVirtualDiskSnapshot struct {
	diskService *service.DiskService
}

func NewObjectRefVirtualDiskSnapshot(diskService *service.DiskService) *ObjectRefVirtualDiskSnapshot {
	return &ObjectRefVirtualDiskSnapshot{
		diskService: diskService,
	}
}

func (ds ObjectRefVirtualDiskSnapshot) Sync(ctx context.Context, vd *virtv2.VirtualDisk) (reconcile.Result, error) {
	if vd.Spec.DataSource == nil || vd.Spec.DataSource.ObjectRef == nil {
		return reconcile.Result{}, errors.New("object ref missed for data source")
	}

	log, ctx := logger.GetDataSourceContext(ctx, objectRefDataSource)

	supgen := supplements.NewGenerator(common.VDShortName, vd.Name, vd.Namespace, vd.UID)

	condition, _ := service.GetCondition(vdcondition.ReadyType, vd.Status.Conditions)
	defer func() { service.SetCondition(condition, &vd.Status.Conditions) }()

	pvc, err := ds.diskService.GetPersistentVolumeClaim(ctx, supgen)
	if err != nil {
		return reconcile.Result{}, err
	}
	vs, err := ds.diskService.GetVolumeSnapshot(ctx, vd.Spec.DataSource.ObjectRef.Name, vd.Namespace)
	if err != nil {
		return reconcile.Result{}, err
	}
	if vs == nil {
		return reconcile.Result{}, errors.New("the source volume snapshot not found")
	}

	switch {
	case isDiskProvisioningFinished(condition):
		log.Debug("Disk provisioning finished: clean up")

		setPhaseConditionForFinishedDisk(pvc, &condition, &vd.Status.Phase, supgen)

		// Protect Ready Disk and underlying PVC.
		err = ds.diskService.Protect(ctx, vd, nil, pvc)
		if err != nil {
			return reconcile.Result{}, err
		}

		return reconcile.Result{}, nil
	case common.IsTerminating(pvc):
		log.Info("Waiting for supplements to be terminated")
		return reconcile.Result{Requeue: true}, nil
	case pvc == nil:
		log.Info("Start import to PVC")

		namespacedName := supplements.NewGenerator(common.VDShortName, vd.Name, vd.Namespace, vd.UID).PersistentVolumeClaim()

		storageClassName := vs.Annotations["storageClass"]
		volumeMode := vs.Annotations["volumeMode"]
		accessModesStr := strings.Split(vs.Annotations["accessModes"], ",")
		accessModes := make([]corev1.PersistentVolumeAccessMode, 0, len(accessModesStr))
		for _, accessModeStr := range accessModesStr {
			accessModes = append(accessModes, corev1.PersistentVolumeAccessMode(accessModeStr))
		}

		spec := corev1.PersistentVolumeClaimSpec{
			AccessModes: accessModes,
			DataSource: &corev1.TypedLocalObjectReference{
				APIGroup: ptr.To(vs.GroupVersionKind().Group),
				Kind:     vs.Kind,
				Name:     vd.Spec.DataSource.ObjectRef.Name,
			},
		}

		if storageClassName != "" {
			spec.StorageClassName = &storageClassName
			vd.Status.StorageClassName = storageClassName
		}

		if volumeMode != "" {
			spec.VolumeMode = ptr.To(corev1.PersistentVolumeMode(volumeMode))
		}

		if vs.Status != nil && vs.Status.RestoreSize != nil {
			spec.Resources = corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: *vs.Status.RestoreSize,
				},
			}
		}

		pvc = &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:      namespacedName.Name,
				Namespace: namespacedName.Namespace,
				OwnerReferences: []metav1.OwnerReference{
					service.MakeOwnerReference(vd),
				},
			},
			Spec: spec,
		}

		err = ds.diskService.CreatePersistentVolumeClaim(ctx, pvc)
		if err != nil {
			setPhaseConditionToFailed(&condition, &vd.Status.Phase, err)
			return reconcile.Result{}, err
		}

		vd.Status.Phase = virtv2.DiskProvisioning
		condition.Status = metav1.ConditionFalse
		condition.Reason = vdcondition.Provisioning
		condition.Message = "PVC has created: waiting to be Bound."

		vd.Status.Progress = "0%"
		vd.Status.SourceUID = util.GetPointer(vs.UID)
		vd.Status.Capacity = ds.diskService.GetCapacity(pvc)
		vd.Status.Target.PersistentVolumeClaim = pvc.Name

		return reconcile.Result{}, nil
	case pvc.Status.Phase == corev1.ClaimBound:
		vd.Status.Phase = virtv2.DiskReady
		condition.Status = metav1.ConditionTrue
		condition.Reason = vdcondition.Ready
		condition.Message = ""

		vd.Status.Progress = "100%"
		vd.Status.Capacity = ds.diskService.GetCapacity(pvc)
		vd.Status.Target.PersistentVolumeClaim = pvc.Name

		return reconcile.Result{Requeue: true}, nil
	default:
		vd.Status.Phase = virtv2.DiskProvisioning
		condition.Status = metav1.ConditionFalse
		condition.Reason = vdcondition.Provisioning
		condition.Message = fmt.Sprintf("Waiting for the PVC %s to be Bound.", pvc.Name)

		return reconcile.Result{}, nil
	}
}

func (ds ObjectRefVirtualDiskSnapshot) Validate(ctx context.Context, vd *virtv2.VirtualDisk) error {
	if vd.Spec.DataSource == nil || vd.Spec.DataSource.ObjectRef == nil {
		return errors.New("object ref missed for data source")
	}

	vdSnapshot, err := ds.diskService.GetVirtualDiskSnapshot(ctx, vd.Spec.DataSource.ObjectRef.Name, vd.Namespace)
	if err != nil {
		return err
	}

	if vdSnapshot == nil || vdSnapshot.Status.Phase != virtv2.VirtualDiskSnapshotPhaseReady {
		return NewVirtualDiskSnapshotNotReadyError(vd.Spec.DataSource.ObjectRef.Name)
	}

	vs, err := ds.diskService.GetVolumeSnapshot(ctx, vdSnapshot.Status.VolumeSnapshotName, vdSnapshot.Namespace)
	if err != nil {
		return err
	}

	if vs == nil || vs.Status.ReadyToUse == nil || !*vs.Status.ReadyToUse {
		return NewVirtualDiskSnapshotNotReadyError(vd.Spec.DataSource.ObjectRef.Name)
	}

	return nil
}
