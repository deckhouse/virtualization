/*
Copyright 2025 Flant JSC

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

package step

import (
	"context"
	"errors"
	"fmt"

	vsv1 "github.com/kubernetes-csi/external-snapshotter/client/v6/apis/volumesnapshot/v1"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	"github.com/deckhouse/virtualization-controller/pkg/common/provisioner"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	vdsupplements "github.com/deckhouse/virtualization-controller/pkg/controller/vd/internal/supplements"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdcondition"
)

type CreatePVCFromVDSnapshotStep struct {
	pvc      *corev1.PersistentVolumeClaim
	disk     service.VolumeAndAccessModesGetter
	pvcSvc   CreatePVCFromVDSnapshotStepPVCService
	recorder eventrecord.EventRecorderLogger
	client   client.Client
	cb       *conditions.ConditionBuilder
}

type CreatePVCFromVDSnapshotStepPVCService interface {
	CreateTargetFromVS(ctx context.Context, key types.NamespacedName, storageClassName string, size *resource.Quantity, owner client.Object, source *vsv1.VolumeSnapshot, modeGetter service.VolumeAndAccessModesGetter, nodePlacement *provisioner.NodePlacement) (corev1.PersistentVolumeClaim, error)
}

func NewCreatePVCFromVDSnapshotStep(
	pvc *corev1.PersistentVolumeClaim,
	disk service.VolumeAndAccessModesGetter,
	pvcSvc CreatePVCFromVDSnapshotStepPVCService,
	recorder eventrecord.EventRecorderLogger,
	client client.Client,
	cb *conditions.ConditionBuilder,
) *CreatePVCFromVDSnapshotStep {
	return &CreatePVCFromVDSnapshotStep{
		pvc:      pvc,
		disk:     disk,
		pvcSvc:   pvcSvc,
		recorder: recorder,
		client:   client,
		cb:       cb,
	}
}

func (s CreatePVCFromVDSnapshotStep) Take(ctx context.Context, vd *v1alpha2.VirtualDisk) (*reconcile.Result, error) {
	if s.pvc != nil {
		return nil, nil
	}

	s.recorder.Event(
		vd,
		corev1.EventTypeNormal,
		v1alpha2.ReasonDataSourceSyncStarted,
		"The ObjectRef DataSource import has started",
	)

	vdSnapshot, err := object.FetchObject(ctx, types.NamespacedName{Name: vd.Spec.DataSource.ObjectRef.Name, Namespace: vd.Namespace}, s.client, &v1alpha2.VirtualDiskSnapshot{})
	if err != nil {
		return nil, fmt.Errorf("fetch virtual disk snapshot: %w", err)
	}

	if vdSnapshot == nil {
		vd.Status.Phase = v1alpha2.DiskPending
		vd.Status.Progress = ""
		s.cb.
			Status(metav1.ConditionFalse).
			Reason(vdcondition.ProvisioningNotStarted).
			Message(fmt.Sprintf("VirtualDiskSnapshot %q not found.", vd.Spec.DataSource.ObjectRef.Name))
		return &reconcile.Result{}, nil
	}

	vs, err := object.FetchObject(ctx, types.NamespacedName{Name: vdSnapshot.Status.VolumeSnapshotName, Namespace: vdSnapshot.Namespace}, s.client, &vsv1.VolumeSnapshot{})
	if err != nil {
		return nil, fmt.Errorf("fetch volume snapshot: %w", err)
	}

	if vdSnapshot.Status.Phase != v1alpha2.VirtualDiskSnapshotPhaseReady || vs == nil || vs.Status == nil || vs.Status.ReadyToUse == nil || !*vs.Status.ReadyToUse {
		vd.Status.Phase = v1alpha2.DiskPending
		vd.Status.Progress = ""
		s.cb.
			Status(metav1.ConditionFalse).
			Reason(vdcondition.ProvisioningNotStarted).
			Message(fmt.Sprintf("VirtualDiskSnapshot %q is not ready to use.", vdSnapshot.Name))
		return &reconcile.Result{}, nil
	}

	if err := s.validateStorageClassCompatibility(ctx, vd, vdSnapshot, vs); err != nil {
		vd.Status.Phase = v1alpha2.DiskFailed
		s.cb.
			Status(metav1.ConditionFalse).
			Reason(vdcondition.ProvisioningFailed).
			Message(err.Error())
		s.recorder.Event(
			vd,
			corev1.EventTypeWarning,
			v1alpha2.ReasonDataSourceSyncFailed,
			err.Error(),
		)
		return &reconcile.Result{}, nil
	}

	storageClassName := s.storageClassName(vd, vs)
	if storageClassName != "" {
		vd.Status.StorageClassName = storageClassName
	}
	size, err := s.getPVCSize(vd, vs)
	if err != nil {
		if errors.Is(err, service.ErrInsufficientPVCSize) {
			vd.Status.Phase = v1alpha2.DiskFailed
			s.cb.
				Status(metav1.ConditionFalse).
				Reason(vdcondition.ProvisioningFailed).
				Message(service.CapitalizeFirstLetter(err.Error()) + ".")
			s.recorder.Event(
				vd,
				corev1.EventTypeWarning,
				v1alpha2.ReasonDataSourceSyncFailed,
				err.Error(),
			)
			return &reconcile.Result{}, nil
		}

		return nil, err
	}

	key := vdsupplements.NewGenerator(vd).PersistentVolumeClaim()
	pvc, err := s.pvcSvc.CreateTargetFromVS(ctx, key, storageClassName, size, vd, vs, s.disk, nil)
	if err != nil && !k8serrors.IsAlreadyExists(err) {
		return nil, fmt.Errorf("create pvc: %w", err)
	}

	vd.Status.Phase = v1alpha2.DiskProvisioning
	s.cb.
		Status(metav1.ConditionFalse).
		Reason(vdcondition.Provisioning).
		Message("The PersistentVolumeClaim has been created; waiting for it to be Bound.")

	vd.Status.Progress = "0%"
	vd.Status.SourceUID = ptr.To(vdSnapshot.UID)
	vdsupplements.SetPVCName(vd, pvc.Name)

	return nil, nil
}

func (s CreatePVCFromVDSnapshotStep) storageClassName(vd *v1alpha2.VirtualDisk, vs *vsv1.VolumeSnapshot) string {
	if vd.Spec.PersistentVolumeClaim.StorageClass != nil && *vd.Spec.PersistentVolumeClaim.StorageClass != "" {
		return *vd.Spec.PersistentVolumeClaim.StorageClass
	}
	storageClassName := vs.Annotations[annotations.AnnStorageClassName]
	if storageClassName == "" {
		storageClassName = vs.Annotations[annotations.AnnStorageClassNameDeprecated]
	}
	return storageClassName
}

func (s CreatePVCFromVDSnapshotStep) getPVCSize(vd *v1alpha2.VirtualDisk, vs *vsv1.VolumeSnapshot) (*resource.Quantity, error) {
	requestedSize := vd.Spec.PersistentVolumeClaim.Size
	if requestedSize == nil {
		originalSize := vs.Annotations[annotations.AnnVirtualDiskOriginalSize]
		if originalSize != "" {
			size, err := resource.ParseQuantity(originalSize)
			if err != nil {
				return nil, fmt.Errorf("failed to parse the original size %q: %w", originalSize, err)
			}
			requestedSize = &size
		}
	}

	if vs.Status == nil || vs.Status.RestoreSize == nil {
		return requestedSize, nil
	}

	// RestoreSize is a hard floor imposed by the CSI driver: a snapshot cannot be
	// restored into a PVC smaller than it (e.g. ceph-rbd rounds the snapshot size
	// up above the original disk's requested size). Grow the target to it instead
	// of failing provisioning.
	restoreSize := *vs.Status.RestoreSize
	if requestedSize == nil || requestedSize.Cmp(restoreSize) < 0 {
		return &restoreSize, nil
	}

	return requestedSize, nil
}

func (s CreatePVCFromVDSnapshotStep) validateStorageClassCompatibility(ctx context.Context, vd *v1alpha2.VirtualDisk, vdSnapshot *v1alpha2.VirtualDiskSnapshot, vs *vsv1.VolumeSnapshot) error {
	if vd.Spec.PersistentVolumeClaim.StorageClass == nil || *vd.Spec.PersistentVolumeClaim.StorageClass == "" {
		return nil
	}

	targetSCName := *vd.Spec.PersistentVolumeClaim.StorageClass

	var targetSC storagev1.StorageClass
	err := s.client.Get(ctx, types.NamespacedName{Name: targetSCName}, &targetSC)
	if err != nil {
		return fmt.Errorf("cannot fetch target storage class %q: %w", targetSCName, err)
	}

	log, _ := logger.GetDataSourceContext(ctx, "objectref")
	if vs.Spec.Source.PersistentVolumeClaimName == nil || *vs.Spec.Source.PersistentVolumeClaimName == "" {
		log.With("volumeSnapshot.name", vs.Name).Debug("Cannot determine original PVC from VolumeSnapshot, skipping storage class compatibility validation")
		return nil
	}

	pvcName := *vs.Spec.Source.PersistentVolumeClaimName

	var originalPVC corev1.PersistentVolumeClaim
	err = s.client.Get(ctx, types.NamespacedName{Name: pvcName, Namespace: vdSnapshot.Namespace}, &originalPVC)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			log.With("pvc.name", pvcName).Debug("Original PVC does not exist, skipping storage class compatibility validation")
			return nil
		}
		return fmt.Errorf("cannot fetch original PVC %q: %w", pvcName, err)
	}

	originalProvisioner := originalPVC.Annotations[annotations.AnnStorageProvisioner]
	if originalProvisioner == "" {
		originalProvisioner = originalPVC.Annotations[annotations.AnnStorageProvisionerDeprecated]
	}

	if originalProvisioner == "" {
		log.With("pvc.name", pvcName).Debug("Cannot determine original provisioner from PVC annotations, skipping storage class compatibility validation")
		return nil
	}

	if targetSC.Provisioner != originalProvisioner {
		return fmt.Errorf(
			"cannot restore snapshot to storage class %q: incompatible storage providers. "+
				"Original snapshot was created by %q, target storage class uses %q. "+
				"Cross-provider snapshot restore is not supported",
			targetSCName, originalProvisioner, targetSC.Provisioner,
		)
	}

	return nil
}
