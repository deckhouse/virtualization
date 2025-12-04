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
	"fmt"
	"strings"

	vsv1 "github.com/kubernetes-csi/external-snapshotter/client/v6/apis/volumesnapshot/v1"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	"github.com/deckhouse/virtualization-controller/pkg/common/pointer"
	commonvdsnapshot "github.com/deckhouse/virtualization-controller/pkg/common/vdsnapshot"
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
	recorder eventrecord.EventRecorderLogger
	client   client.Client
	cb       *conditions.ConditionBuilder
}

func NewCreatePVCFromVDSnapshotStep(
	pvc *corev1.PersistentVolumeClaim,
	recorder eventrecord.EventRecorderLogger,
	client client.Client,
	cb *conditions.ConditionBuilder,
) *CreatePVCFromVDSnapshotStep {
	return &CreatePVCFromVDSnapshotStep{
		pvc:      pvc,
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

	pvc := s.buildPVC(vd, vs)

	err = s.client.Create(ctx, pvc)
	if err != nil && !k8serrors.IsAlreadyExists(err) {
		return nil, fmt.Errorf("create pvc: %w", err)
	}

	vd.Status.Phase = v1alpha2.DiskProvisioning
	s.cb.
		Status(metav1.ConditionFalse).
		Reason(vdcondition.Provisioning).
		Message("PVC has created: waiting to be Bound.")

	vd.Status.Progress = "0%"
	vd.Status.SourceUID = pointer.GetPointer(vdSnapshot.UID)
	vdsupplements.SetPVCName(vd, pvc.Name)

	err = commonvdsnapshot.AddOriginalMetadata(vd, vs)
	if err != nil {
		return nil, fmt.Errorf("failed to add original metadata: %w", err)
	}

	return nil, nil
}

func (s CreatePVCFromVDSnapshotStep) buildPVC(vd *v1alpha2.VirtualDisk, vs *vsv1.VolumeSnapshot) *corev1.PersistentVolumeClaim {
	var storageClassName string
	if vd.Spec.PersistentVolumeClaim.StorageClass != nil && *vd.Spec.PersistentVolumeClaim.StorageClass != "" {
		storageClassName = *vd.Spec.PersistentVolumeClaim.StorageClass
	} else {
		storageClassName = vs.Annotations[annotations.AnnStorageClassName]
		if storageClassName == "" {
			storageClassName = vs.Annotations[annotations.AnnStorageClassNameDeprecated]
		}
	}

	volumeMode := vs.Annotations[annotations.AnnVolumeMode]
	if volumeMode == "" {
		volumeMode = vs.Annotations[annotations.AnnVolumeModeDeprecated]
	}
	accessModesRaw := vs.Annotations[annotations.AnnAccessModes]
	if accessModesRaw == "" {
		accessModesRaw = vs.Annotations[annotations.AnnAccessModesDeprecated]
	}

	accessModesStr := strings.Split(accessModesRaw, ",")
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

	pvcKey := vdsupplements.NewGenerator(vd).PersistentVolumeClaim()

	return &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pvcKey.Name,
			Namespace: pvcKey.Namespace,
			Finalizers: []string{
				v1alpha2.FinalizerVDProtection,
			},
			OwnerReferences: []metav1.OwnerReference{
				service.MakeOwnerReference(vd),
			},
		},
		Spec: spec,
	}
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
