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
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	vsv1 "github.com/kubernetes-csi/external-snapshotter/client/v6/apis/volumesnapshot/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
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
	vdsupplements "github.com/deckhouse/virtualization-controller/pkg/controller/vd/internal/supplements"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdcondition"
)

type CreatePVCFromVSStep struct {
	pvc    *corev1.PersistentVolumeClaim
	client client.Client
	cb     *conditions.ConditionBuilder
}

func NewCreatePVCFromVSStep(
	pvc *corev1.PersistentVolumeClaim,
	client client.Client,
	cb *conditions.ConditionBuilder,
) *CreatePVCFromVSStep {
	return &CreatePVCFromVSStep{
		pvc:    pvc,
		client: client,
		cb:     cb,
	}
}

func (s CreatePVCFromVSStep) Take(ctx context.Context, vd *v1alpha2.VirtualDisk) (*reconcile.Result, error) {
	if s.pvc != nil {
		return nil, nil
	}

	_, exists := vd.Annotations[annotations.AnnUseVolumeSnapshot]
	if !exists {
		return nil, nil
	}

	vi, err := object.FetchObject(ctx, types.NamespacedName{
		Namespace: vd.Namespace,
		Name:      vd.Spec.DataSource.ObjectRef.Name,
	}, s.client, &v1alpha2.VirtualImage{})
	if err != nil {
		vd.Status.Phase = v1alpha2.DiskFailed
		s.cb.
			Status(metav1.ConditionFalse).
			Reason(vdcondition.ProvisioningFailed).
			Message("The VirtualImage not found")
		return &reconcile.Result{}, nil
	}

	if vi.Status.Target.PersistentVolumeClaim == "" {
		vd.Status.Phase = v1alpha2.DiskFailed
		s.cb.
			Status(metav1.ConditionFalse).
			Reason(vdcondition.ProvisioningFailed).
			Message("The VirtualImage does not have the target pvc")
		return &reconcile.Result{}, nil
	}

	vs, err := object.FetchObject(ctx, types.NamespacedName{
		Namespace: vi.Namespace,
		Name:      vi.Status.Target.PersistentVolumeClaim,
	}, s.client, &vsv1.VolumeSnapshot{})
	if err != nil {
		vd.Status.Phase = v1alpha2.DiskFailed
		s.cb.
			Status(metav1.ConditionFalse).
			Reason(vdcondition.ProvisioningFailed).
			Message("The VolumeSnapshot not found")
		return &reconcile.Result{}, nil
	}

	if vs.Status == nil || !(*vs.Status.ReadyToUse) {
		vd.Status.Phase = v1alpha2.DiskFailed
		s.cb.
			Status(metav1.ConditionFalse).
			Reason(vdcondition.ProvisioningFailed).
			Message("The VolumeSnapshot is not ready to use")
		return &reconcile.Result{}, nil
	}

	pvc, err := s.buildPVC(vd, vs, vi)
	if err != nil {
		return nil, fmt.Errorf("failed to build pvc: %w", err)
	}

	err = s.client.Create(ctx, pvc)
	if err != nil && !k8serrors.IsAlreadyExists(err) {
		return nil, fmt.Errorf("create pvc: %w", err)
	}

	vd.Status.Phase = v1alpha2.DiskProvisioning
	s.cb.
		Status(metav1.ConditionFalse).
		Reason(vdcondition.Provisioning).
		Message("The PersistentVolumeClaim has been created: waiting for it to be Bound.")

	vd.Status.Progress = "0%"
	vd.Status.SourceUID = pointer.GetPointer(vi.UID)
	vdsupplements.SetPVCName(vd, pvc.Name)

	s.addOriginalMetadata(vd, vs)
	return nil, nil
}

func (s CreatePVCFromVSStep) buildPVC(vd *v1alpha2.VirtualDisk, vs *vsv1.VolumeSnapshot, vi *v1alpha2.VirtualImage) (*corev1.PersistentVolumeClaim, error) {
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
			Name:     vi.Status.Target.PersistentVolumeClaim,
		},
	}

	if storageClassName != "" {
		spec.StorageClassName = &storageClassName
		vd.Status.StorageClassName = storageClassName
	}

	if volumeMode != "" {
		spec.VolumeMode = ptr.To(corev1.PersistentVolumeMode(volumeMode))
	}

	pvcSize, err := s.getPVCSize(vd, vs)
	if err != nil {
		return nil, err
	}

	spec.Resources = corev1.VolumeResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceStorage: pvcSize,
		},
	}

	pvcKey := vdsupplements.NewGenerator(vd).PersistentVolumeClaim()

	return &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pvcKey.Name,
			Namespace: pvcKey.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				service.MakeOwnerReference(vd),
			},
		},
		Spec: spec,
	}, nil
}

func (s CreatePVCFromVSStep) getPVCSize(vd *v1alpha2.VirtualDisk, vs *vsv1.VolumeSnapshot) (resource.Quantity, error) {
	if vs.Status == nil || vs.Status.RestoreSize == nil || vs.Status.RestoreSize.IsZero() {
		return resource.Quantity{}, errors.New("vs has zero size")
	}

	if vd.Spec.PersistentVolumeClaim.Size == nil || vd.Spec.PersistentVolumeClaim.Size.IsZero() {
		return *vs.Status.RestoreSize, nil
	}

	if vd.Spec.PersistentVolumeClaim.Size.Cmp(*vs.Status.RestoreSize) == 1 {
		return *vd.Spec.PersistentVolumeClaim.Size, nil
	}

	return *vs.Status.RestoreSize, nil
}

// AddOriginalMetadata adds original annotations and labels from VolumeSnapshot to VirtualDisk,
// without overwriting existing values
func (s CreatePVCFromVSStep) addOriginalMetadata(vd *v1alpha2.VirtualDisk, vs *vsv1.VolumeSnapshot) {
	if vd.Annotations == nil {
		vd.Annotations = make(map[string]string)
	}
	if vd.Labels == nil {
		vd.Labels = make(map[string]string)
	}

	if annotationsJSON := vs.Annotations[annotations.AnnVirtualDiskOriginalAnnotations]; annotationsJSON != "" {
		var originalAnnotations map[string]string
		if err := json.Unmarshal([]byte(annotationsJSON), &originalAnnotations); err == nil {
			for key, value := range originalAnnotations {
				if _, exists := vd.Annotations[key]; !exists {
					vd.Annotations[key] = value
				}
			}
		}
	}

	if labelsJSON := vs.Annotations[annotations.AnnVirtualDiskOriginalLabels]; labelsJSON != "" {
		var originalLabels map[string]string
		if err := json.Unmarshal([]byte(labelsJSON), &originalLabels); err == nil {
			for key, value := range originalLabels {
				if _, exists := vd.Labels[key]; !exists {
					vd.Labels[key] = value
				}
			}
		}
	}
}
