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
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/supplements"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vicondition"
)

type CreatePersistentVolumeClaimStep struct {
	pvc      *corev1.PersistentVolumeClaim
	recorder eventrecord.EventRecorderLogger
	client   client.Client
	cb       *conditions.ConditionBuilder
}

func NewCreatePersistentVolumeClaimStep(
	pvc *corev1.PersistentVolumeClaim,
	recorder eventrecord.EventRecorderLogger,
	client client.Client,
	cb *conditions.ConditionBuilder,
) *CreatePersistentVolumeClaimStep {
	return &CreatePersistentVolumeClaimStep{
		pvc:      pvc,
		recorder: recorder,
		client:   client,
		cb:       cb,
	}
}

func (s CreatePersistentVolumeClaimStep) Take(ctx context.Context, vi *v1alpha2.VirtualImage) (*reconcile.Result, error) {
	if s.pvc != nil {
		return nil, nil
	}

	s.recorder.Event(
		vi,
		corev1.EventTypeNormal,
		v1alpha2.ReasonDataSourceSyncStarted,
		"The ObjectRef DataSource import has started",
	)

	vdSnapshot, err := object.FetchObject(ctx, types.NamespacedName{Name: vi.Spec.DataSource.ObjectRef.Name, Namespace: vi.Namespace}, s.client, &v1alpha2.VirtualDiskSnapshot{})
	if err != nil {
		return nil, fmt.Errorf("fetch virtual disk snapshot: %w", err)
	}

	if vdSnapshot == nil {
		vi.Status.Phase = v1alpha2.ImagePending
		s.cb.
			Status(metav1.ConditionFalse).
			Reason(vicondition.ProvisioningNotStarted).
			Message(fmt.Sprintf("VirtualDiskSnapshot %q not found.", vi.Spec.DataSource.ObjectRef.Name))
		return &reconcile.Result{}, nil
	}

	vs, err := object.FetchObject(ctx, types.NamespacedName{Name: vdSnapshot.Status.VolumeSnapshotName, Namespace: vdSnapshot.Namespace}, s.client, &vsv1.VolumeSnapshot{})
	if err != nil {
		return nil, fmt.Errorf("fetch volume snapshot: %w", err)
	}

	if vdSnapshot.Status.Phase != v1alpha2.VirtualDiskSnapshotPhaseReady || vs == nil || vs.Status == nil || vs.Status.ReadyToUse == nil || !*vs.Status.ReadyToUse {
		vi.Status.Phase = v1alpha2.ImagePending
		s.cb.
			Status(metav1.ConditionFalse).
			Reason(vicondition.ProvisioningNotStarted).
			Message(fmt.Sprintf("VirtualDiskSnapshot %q is not ready to use.", vdSnapshot.Name))
		return &reconcile.Result{}, nil
	}

	if err := s.validateStorageClassCompatibility(ctx, vi, vdSnapshot, vs); err != nil {
		vi.Status.Phase = v1alpha2.ImageFailed
		s.cb.
			Status(metav1.ConditionFalse).
			Reason(vicondition.ProvisioningFailed).
			Message(err.Error())
		s.recorder.Event(
			vi,
			corev1.EventTypeWarning,
			v1alpha2.ReasonDataSourceSyncFailed,
			err.Error(),
		)
		return &reconcile.Result{}, nil
	}

	pvc := s.buildPVC(vi, vs)

	err = s.client.Create(ctx, pvc)
	if err != nil && !k8serrors.IsAlreadyExists(err) {
		return nil, fmt.Errorf("create pvc: %w", err)
	}

	log, _ := logger.GetDataSourceContext(ctx, "objectref")
	log.With("pvc.name", pvc.Name).Debug("The underlying PVC has just been created.")

	if vi.Spec.Storage == v1alpha2.StoragePersistentVolumeClaim || vi.Spec.Storage == v1alpha2.StorageKubernetes {
		vi.Status.Target.PersistentVolumeClaim = pvc.Name
	}

	vi.Status.Progress = "0%"
	vi.Status.SourceUID = pointer.GetPointer(vdSnapshot.UID)

	return nil, nil
}

func (s CreatePersistentVolumeClaimStep) buildPVC(vi *v1alpha2.VirtualImage, vs *vsv1.VolumeSnapshot) *corev1.PersistentVolumeClaim {
	var storageClassName string
	if vi.Spec.PersistentVolumeClaim.StorageClass != nil && *vi.Spec.PersistentVolumeClaim.StorageClass != "" {
		storageClassName = *vi.Spec.PersistentVolumeClaim.StorageClass
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
			Name:     vs.Name,
		},
	}

	if storageClassName != "" {
		spec.StorageClassName = &storageClassName
		vi.Status.StorageClassName = storageClassName
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

	pvcKey := supplements.NewGenerator(annotations.VIShortName, vi.Name, vi.Namespace, vi.UID).PersistentVolumeClaim()

	return &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pvcKey.Name,
			Namespace: pvcKey.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				service.MakeOwnerReference(vi),
			},
			Finalizers: []string{
				v1alpha2.FinalizerVIProtection,
			},
		},
		Spec: spec,
	}
}

func (s CreatePersistentVolumeClaimStep) validateStorageClassCompatibility(ctx context.Context, vi *v1alpha2.VirtualImage, vdSnapshot *v1alpha2.VirtualDiskSnapshot, vs *vsv1.VolumeSnapshot) error {
	if vi.Spec.PersistentVolumeClaim.StorageClass == nil || *vi.Spec.PersistentVolumeClaim.StorageClass == "" {
		return nil
	}

	targetSCName := *vi.Spec.PersistentVolumeClaim.StorageClass

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
