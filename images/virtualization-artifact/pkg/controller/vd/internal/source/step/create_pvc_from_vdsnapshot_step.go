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
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	storagefoundationv1alpha1 "github.com/deckhouse/storage-foundation/api/v1alpha1"
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
	EnsureVolumeRestoreRequest(ctx context.Context, key types.NamespacedName, storageClassName string, size *resource.Quantity, owner client.Object, sourceRef storagefoundationv1alpha1.ObjectReference, modeGetter service.VolumeAndAccessModesGetter) error
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

	if _, ok := vdSnapshot.Annotations[v1alpha2.AnnUseUnifiedSnapshotter]; ok {
		return s.takeFromUnifiedSnapshot(ctx, vd, vdSnapshot)
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

// takeFromUnifiedSnapshot clones a VirtualDisk from a VirtualDiskSnapshot captured through the unified
// state-snapshotter SDK. Unlike the CSI VolumeSnapshot path above, the destination PVC is materialized by
// storage-foundation from the snapshot's captured artifact (status.dataArtifact) via a VolumeRestoreRequest,
// rather than cloned from a namespaced CSI VolumeSnapshot: the SDK-captured artifact is a
// VolumeSnapshotContent built in a shape (spec.source.volumeHandle, no bound VolumeSnapshot) that
// external-snapshotter refuses to statically (re)bind a new VolumeSnapshot to.
func (s CreatePVCFromVDSnapshotStep) takeFromUnifiedSnapshot(ctx context.Context, vd *v1alpha2.VirtualDisk, vdSnapshot *v1alpha2.VirtualDiskSnapshot) (*reconcile.Result, error) {
	if vdSnapshot.Status.Phase != v1alpha2.VirtualDiskSnapshotPhaseReady || vdSnapshot.Status.Data == nil {
		vd.Status.Phase = v1alpha2.DiskPending
		vd.Status.Progress = ""
		s.cb.
			Status(metav1.ConditionFalse).
			Reason(vdcondition.ProvisioningNotStarted).
			Message(fmt.Sprintf("VirtualDiskSnapshot %q is not ready to use (phase %s).", vdSnapshot.Name, vdSnapshot.Status.Phase))
		return &reconcile.Result{}, nil
	}

	if err := s.validateUnifiedStorageClassCompatibility(ctx, vd, vdSnapshot); err != nil {
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

	storageClassName := vdSnapshot.Status.StorageClassName
	if vd.Spec.PersistentVolumeClaim.StorageClass != nil && *vd.Spec.PersistentVolumeClaim.StorageClass != "" {
		storageClassName = *vd.Spec.PersistentVolumeClaim.StorageClass
	}

	size, err := s.getUnifiedPVCSize(vd, vdSnapshot)
	if err != nil {
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

	if storageClassName != "" {
		vd.Status.StorageClassName = storageClassName
	}

	key := vdsupplements.NewGenerator(vd).PersistentVolumeClaim()
	sourceRef := storagefoundationv1alpha1.ObjectReference{
		APIVersion: vdSnapshot.Status.Data.ArtifactRef.APIVersion,
		Kind:       vdSnapshot.Status.Data.ArtifactRef.Kind,
		Name:       vdSnapshot.Status.Data.ArtifactRef.Name,
	}
	if err := s.pvcSvc.EnsureVolumeRestoreRequest(ctx, key, storageClassName, size, vd, sourceRef, s.disk); err != nil {
		return nil, fmt.Errorf("ensure volume restore request: %w", err)
	}

	// storage-foundation reports a restore it has given up on through the VolumeRestoreRequest and nowhere
	// else. Without this the disk would sit in Provisioning "0%" indefinitely, waiting for a PVC nobody is
	// going to create any more.
	if failure, err := s.unifiedRestoreFailure(ctx, key); err != nil {
		return nil, err
	} else if failure != "" {
		vd.Status.Phase = v1alpha2.DiskFailed
		s.cb.
			Status(metav1.ConditionFalse).
			Reason(vdcondition.ProvisioningFailed).
			Message(failure)
		s.recorder.Event(vd, corev1.EventTypeWarning, v1alpha2.ReasonDataSourceSyncFailed, failure)
		return &reconcile.Result{}, nil
	}

	vd.Status.Phase = v1alpha2.DiskProvisioning
	s.cb.
		Status(metav1.ConditionFalse).
		Reason(vdcondition.Provisioning).
		Message("The PersistentVolumeClaim restore has been requested; waiting for it to be Bound.")

	vd.Status.Progress = "0%"
	vd.Status.SourceUID = ptr.To(vdSnapshot.UID)
	vdsupplements.SetPVCName(vd, key.Name)

	return nil, nil
}

// unifiedRestoreFailure returns a human-readable reason when the VolumeRestoreRequest for key has failed
// terminally, or "" while it is still in flight.
//
// storage-foundation declares its contract in conditions.go: "Only Ready condition is used - it is set to
// True on success or False on final failure". ConditionReasonTargetsPending is the documented exception —
// the non-terminal state a retrying CSI driver parks in — so it is the one Ready=False that is not a verdict.
func (s CreatePVCFromVDSnapshotStep) unifiedRestoreFailure(ctx context.Context, key types.NamespacedName) (string, error) {
	vrr, err := object.FetchObject(ctx, key, s.client, &storagefoundationv1alpha1.VolumeRestoreRequest{})
	if err != nil {
		return "", fmt.Errorf("fetch volume restore request: %w", err)
	}
	if vrr == nil {
		return "", nil
	}

	ready := meta.FindStatusCondition(vrr.Status.Conditions, storagefoundationv1alpha1.ConditionTypeReady)
	if ready == nil || ready.Status != metav1.ConditionFalse || ready.Reason == storagefoundationv1alpha1.ConditionReasonTargetsPending {
		return "", nil
	}

	return fmt.Sprintf("Restoring the PersistentVolumeClaim failed: %s: %s.", ready.Reason, ready.Message), nil
}

func (s CreatePVCFromVDSnapshotStep) getUnifiedPVCSize(vd *v1alpha2.VirtualDisk, vdSnapshot *v1alpha2.VirtualDiskSnapshot) (*resource.Quantity, error) {
	if vd.Spec.PersistentVolumeClaim.Size != nil {
		return vd.Spec.PersistentVolumeClaim.Size, nil
	}
	if vdSnapshot.Status.PersistentVolumeClaimSize == "" {
		return nil, nil
	}
	size, err := resource.ParseQuantity(vdSnapshot.Status.PersistentVolumeClaimSize)
	if err != nil {
		return nil, fmt.Errorf("failed to parse the captured PVC size %q: %w", vdSnapshot.Status.PersistentVolumeClaimSize, err)
	}
	return &size, nil
}

func (s CreatePVCFromVDSnapshotStep) validateUnifiedStorageClassCompatibility(ctx context.Context, vd *v1alpha2.VirtualDisk, vdSnapshot *v1alpha2.VirtualDiskSnapshot) error {
	if vd.Spec.PersistentVolumeClaim.StorageClass == nil || *vd.Spec.PersistentVolumeClaim.StorageClass == "" {
		return nil
	}

	targetSCName := *vd.Spec.PersistentVolumeClaim.StorageClass
	capturedSCName := vdSnapshot.Status.StorageClassName
	if capturedSCName == "" || capturedSCName == targetSCName {
		return nil
	}

	log, _ := logger.GetDataSourceContext(ctx, "objectref")

	var targetSC storagev1.StorageClass
	if err := s.client.Get(ctx, types.NamespacedName{Name: targetSCName}, &targetSC); err != nil {
		return fmt.Errorf("cannot fetch target storage class %q: %w", targetSCName, err)
	}

	var capturedSC storagev1.StorageClass
	if err := s.client.Get(ctx, types.NamespacedName{Name: capturedSCName}, &capturedSC); err != nil {
		if k8serrors.IsNotFound(err) {
			log.With("storageClass.name", capturedSCName).Debug("Captured storage class does not exist, skipping storage class compatibility validation")
			return nil
		}
		return fmt.Errorf("cannot fetch captured storage class %q: %w", capturedSCName, err)
	}

	if targetSC.Provisioner != capturedSC.Provisioner {
		return fmt.Errorf(
			"cannot restore snapshot to storage class %q: incompatible storage providers. "+
				"Original snapshot was created by %q, target storage class uses %q. "+
				"Cross-provider snapshot restore is not supported",
			targetSCName, capturedSC.Provisioner, targetSC.Provisioner,
		)
	}

	return nil
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
