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
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
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

func (s CreatePVCFromVDSnapshotStep) Take(ctx context.Context, vd *virtv2.VirtualDisk) (*reconcile.Result, error) {
	if s.pvc != nil {
		return nil, nil
	}

	s.recorder.Event(
		vd,
		corev1.EventTypeNormal,
		virtv2.ReasonDataSourceSyncStarted,
		"The ObjectRef DataSource import has started",
	)

	vdSnapshot, err := object.FetchObject(ctx, types.NamespacedName{Name: vd.Spec.DataSource.ObjectRef.Name, Namespace: vd.Namespace}, s.client, &virtv2.VirtualDiskSnapshot{})
	if err != nil {
		return nil, fmt.Errorf("fetch virtual disk snapshot: %w", err)
	}

	if vdSnapshot == nil {
		vd.Status.Phase = virtv2.DiskPending
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

	if vdSnapshot.Status.Phase != virtv2.VirtualDiskSnapshotPhaseReady || vs == nil || vs.Status == nil || vs.Status.ReadyToUse == nil || !*vs.Status.ReadyToUse {
		vd.Status.Phase = virtv2.DiskPending
		s.cb.
			Status(metav1.ConditionFalse).
			Reason(vdcondition.ProvisioningNotStarted).
			Message(fmt.Sprintf("VirtualDiskSnapshot %q is not ready to use.", vdSnapshot.Name))
		return &reconcile.Result{}, nil
	}

	pvc := s.buildPVC(vd, vs)

	err = s.client.Create(ctx, pvc)
	if err != nil && !k8serrors.IsAlreadyExists(err) {
		return nil, fmt.Errorf("create pvc: %w", err)
	}

	vd.Status.Phase = virtv2.DiskProvisioning
	s.cb.
		Status(metav1.ConditionFalse).
		Reason(vdcondition.Provisioning).
		Message("PVC has created: waiting to be Bound.")

	vd.Status.Progress = "0%"
	vd.Status.SourceUID = pointer.GetPointer(vdSnapshot.UID)
	vd.Status.Target.PersistentVolumeClaim = pvc.Name

	return nil, nil
}

func (s CreatePVCFromVDSnapshotStep) buildPVC(vd *virtv2.VirtualDisk, vs *vsv1.VolumeSnapshot) *corev1.PersistentVolumeClaim {
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

	pvcKey := supplements.NewGenerator(annotations.VDShortName, vd.Name, vd.Namespace, vd.UID).PersistentVolumeClaim()

	return &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pvcKey.Name,
			Namespace: pvcKey.Namespace,
			Finalizers: []string{
				virtv2.FinalizerVDProtection,
			},
			OwnerReferences: []metav1.OwnerReference{
				service.MakeOwnerReference(vd),
			},
		},
		Spec: spec,
	}
}
