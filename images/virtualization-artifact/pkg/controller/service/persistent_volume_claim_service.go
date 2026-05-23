/*
Copyright 2026 Flant JSC

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

package service

import (
	"context"
	"fmt"

	vsv1 "github.com/kubernetes-csi/external-snapshotter/client/v6/apis/volumesnapshot/v1"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	cdiv1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	"github.com/deckhouse/virtualization-controller/pkg/common/provisioner"
	"github.com/deckhouse/virtualization-controller/pkg/controller/supplements"
	"github.com/deckhouse/virtualization-controller/pkg/dvcr"
)

const (
	cloneStrategySnapshot = "snapshot"
	cloneStrategyCSI      = "csi-clone"
	cloneStrategyHost     = "host-assisted"
)

// PersistentVolumeClaimService is the single entry point for filling a target
// PersistentVolumeClaim with data. Callers describe the desired target PVC
// (Name, Namespace, OwnerReferences, Finalizers, base Spec) together with a
// PVCImportSource (DVCR registry image, another PVC, or nothing for a blank
// target) and let the service decide *how* to populate the PVC: a smart clone
// via VolumeSnapshot, a CSI clone via dataSource, a host-assisted copy via
// the cdi-importer pod, or any other strategy that may be added in the
// future. The service also creates and cleans up every helper resource the
// chosen strategy needs (scratch PVC, cdi-importer pod, secret/configmap
// copies of DVCR auth/CA, VolumeSnapshot, etc.).
//
// PersistentVolumeClaimService is intentionally agnostic of VirtualDisk and
// VirtualImage: the caller passes the owning object as a client.Object so the
// service can attach it as an OwnerReference where appropriate.
type PersistentVolumeClaimService struct {
	client     client.Client
	importer   *PVCImporterService
	protection *ProtectionService
}

// NewPersistentVolumeClaimService constructs a PersistentVolumeClaimService
// configured with the cdi-importer pod settings and the DVCR settings used to
// derive auth/CA supplements.
func NewPersistentVolumeClaimService(
	c client.Client,
	dvcrSettings *dvcr.Settings,
	protection *ProtectionService,
	cfg DiskImporterConfig,
) *PersistentVolumeClaimService {
	return &PersistentVolumeClaimService{
		client:     c,
		importer:   NewPVCImporterService(c, dvcrSettings, cfg.Image, cfg.ResourceRequirements, cfg.PullPolicy, cfg.Verbose),
		protection: protection,
	}
}

// Finalizers returns the finalizer slice that should be applied to target
// PVCs at creation time so the controller can perform explicit cleanup before
// garbage collection. Callers building target PVC descriptors should populate
// ObjectMeta.Finalizers with this slice.
func (s *PersistentVolumeClaimService) Finalizers() []string {
	if s.protection == nil {
		return nil
	}
	finalizer := s.protection.GetFinalizer()
	if finalizer == "" {
		return nil
	}
	return []string{finalizer}
}

// Import drives one reconciliation step toward populating target with data
// from source. On the first call (target does not yet exist in the cluster)
// it picks the most appropriate strategy, augments target.Spec with the
// strategy-specific dataSource, ensures any required helper resources, and
// creates the target PVC. Subsequent calls are idempotent: they ensure the
// helper resources are still in place and report the current import phase.
//
// The returned phase mirrors the cdi-importer pod phase or, for smart clones,
// is derived from the target PVC binding state. Once it is corev1.PodSucceeded
// the helper resources are torn down automatically.
//
// The caller does not need to know which strategy is in use; that decision is
// fully encapsulated.
func (s *PersistentVolumeClaimService) Import(ctx context.Context, target *corev1.PersistentVolumeClaim, source *PVCImportSource, owner client.Object, sup supplements.Generator, nodePlacement *provisioner.NodePlacement) (corev1.PodPhase, error) {
	existing, err := object.FetchObject(ctx, types.NamespacedName{Name: target.Name, Namespace: target.Namespace}, s.client, &corev1.PersistentVolumeClaim{})
	if err != nil {
		return "", fmt.Errorf("fetch target pvc: %w", err)
	}
	if existing == nil {
		if err := s.createTarget(ctx, target, source, owner, nodePlacement); err != nil {
			return "", err
		}
		return corev1.PodPending, nil
	}

	return s.drive(ctx, existing, source, owner, sup, nodePlacement)
}

// Cleanup removes every helper resource the import has used (cdi-importer
// pod, scratch PVC, clone VolumeSnapshot). It is idempotent and safe to call
// multiple times.
func (s *PersistentVolumeClaimService) Cleanup(ctx context.Context, sup supplements.Generator, target *corev1.PersistentVolumeClaim) (bool, error) {
	deleted, err := s.importer.CleanUp(ctx, sup, target)
	if err != nil {
		return false, err
	}
	if err := s.cleanupCloneSnapshot(ctx, target); err != nil {
		return false, err
	}
	return deleted, nil
}

// createTarget chooses the import strategy, prepares target.Spec accordingly,
// applies import annotations, applies node-placement tolerations, ensures any
// pre-creation helper resources (e.g. clone VolumeSnapshot) and finally
// creates the target PVC.
func (s *PersistentVolumeClaimService) createTarget(ctx context.Context, target *corev1.PersistentVolumeClaim, source *PVCImportSource, owner client.Object, nodePlacement *provisioner.NodePlacement) error {
	if target.Annotations == nil {
		target.Annotations = map[string]string{}
	}

	size := target.Spec.Resources.Requests[corev1.ResourceStorage]
	for k, v := range s.importer.PVCImportAnnotations(source, size) {
		target.Annotations[k] = v
	}

	if source != nil && source.PVC != nil {
		if err := s.prepareCloneTarget(ctx, target, source.PVC, owner); err != nil {
			return err
		}
	}

	if nodePlacement != nil {
		if err := provisioner.KeepNodePlacementTolerations(nodePlacement, target); err != nil {
			return fmt.Errorf("keep node placement: %w", err)
		}
	}

	if err := s.client.Create(ctx, target); err != nil && !k8serrors.IsAlreadyExists(err) {
		return fmt.Errorf("create target pvc: %w", err)
	}
	return nil
}

// prepareCloneTarget resolves the source PVC, picks the clone strategy and,
// when the strategy is smart-clone-via-snapshot, makes sure the underlying
// VolumeSnapshot exists. It mutates target.Spec / target.Annotations so the
// PVC is created with the correct dataSource and clone metadata.
func (s *PersistentVolumeClaimService) prepareCloneTarget(ctx context.Context, target *corev1.PersistentVolumeClaim, sourcePVC *PVCImportSourcePVC, owner client.Object) error {
	sourceClaim, err := object.FetchObject(ctx, types.NamespacedName{Name: sourcePVC.Name, Namespace: sourcePVC.Namespace}, s.client, &corev1.PersistentVolumeClaim{})
	if err != nil {
		return fmt.Errorf("fetch source pvc: %w", err)
	}
	if sourceClaim == nil {
		return fmt.Errorf("source pvc %s/%s not found", sourcePVC.Namespace, sourcePVC.Name)
	}

	targetSC, err := object.FetchObject(ctx, types.NamespacedName{Name: ptr.Deref(target.Spec.StorageClassName, "")}, s.client, &storagev1.StorageClass{})
	if err != nil {
		return fmt.Errorf("fetch target storage class: %w", err)
	}
	if targetSC == nil {
		return fmt.Errorf("target storage class %q not found", ptr.Deref(target.Spec.StorageClassName, ""))
	}
	targetVolumeMode := corev1.PersistentVolumeFilesystem
	if target.Spec.VolumeMode != nil {
		targetVolumeMode = *target.Spec.VolumeMode
	}

	strategy := s.choosePVCCloneStrategy(ctx, sourceClaim, targetSC, targetVolumeMode)
	target.Spec.Resources.Requests[corev1.ResourceStorage] = pvcCloneTargetSize(target.Spec.Resources.Requests[corev1.ResourceStorage], sourceClaim)

	target.Annotations[annotations.AnnPVCImportCloneStrategy] = strategy

	switch strategy {
	case cloneStrategySnapshot:
		snapshotName := target.Name + "-clone-snapshot"
		target.Annotations[annotations.AnnPVCImportCloneSnapshot] = snapshotName
		target.Spec.DataSource = &corev1.TypedLocalObjectReference{
			APIGroup: ptr.To("snapshot.storage.k8s.io"),
			Kind:     "VolumeSnapshot",
			Name:     snapshotName,
		}
		target.Spec.DataSourceRef = &corev1.TypedObjectReference{
			APIGroup: ptr.To("snapshot.storage.k8s.io"),
			Kind:     "VolumeSnapshot",
			Name:     snapshotName,
		}
		if err := s.ensureCloneSnapshot(ctx, sourceClaim, target, owner); err != nil {
			return err
		}
	case cloneStrategyCSI:
		target.Spec.DataSource = &corev1.TypedLocalObjectReference{
			Kind: "PersistentVolumeClaim",
			Name: sourceClaim.Name,
		}
		target.Spec.DataSourceRef = &corev1.TypedObjectReference{
			Kind: "PersistentVolumeClaim",
			Name: sourceClaim.Name,
		}
	}
	return nil
}

// drive moves an already-created target PVC import forward. For smart clone
// strategies the PVC is populated by Kubernetes itself, so drive only watches
// the binding state and tears down the helper VolumeSnapshot once the target
// becomes Bound. For everything else drive delegates to PVCImporterService,
// which owns the cdi-importer pod path.
func (s *PersistentVolumeClaimService) drive(ctx context.Context, target *corev1.PersistentVolumeClaim, source *PVCImportSource, owner client.Object, sup supplements.Generator, nodePlacement *provisioner.NodePlacement) (corev1.PodPhase, error) {
	if source != nil && source.PVC != nil && isSmartCloneStrategy(target.Annotations[annotations.AnnPVCImportCloneStrategy]) {
		if target.Status.Phase == corev1.ClaimBound {
			if err := s.importer.patchTargetImportPhase(ctx, target, corev1.PodSucceeded); err != nil {
				return "", err
			}
			return corev1.PodSucceeded, s.cleanupCloneSnapshot(ctx, target)
		}
		return corev1.PodPending, nil
	}

	return s.importer.Ensure(ctx, target, source, owner, sup, nodePlacement)
}

func (s *PersistentVolumeClaimService) choosePVCCloneStrategy(ctx context.Context, sourceClaim *corev1.PersistentVolumeClaim, targetSC *storagev1.StorageClass, targetVolumeMode corev1.PersistentVolumeMode) string {
	sourceSC, err := s.fetchSourceStorageClass(ctx, sourceClaim)
	if err != nil || sourceSC == nil {
		return cloneStrategyHost
	}

	preferred := cloneStrategySnapshot
	if sp, err := object.FetchObject(ctx, types.NamespacedName{Name: targetSC.Name}, s.client, &cdiv1.StorageProfile{}); err == nil && sp != nil && sp.Status.CloneStrategy != nil {
		switch *sp.Status.CloneStrategy {
		case cdiv1.CloneStrategyCsiClone:
			preferred = cloneStrategyCSI
		case cdiv1.CloneStrategyHostAssisted:
			preferred = cloneStrategyHost
		case cdiv1.CloneStrategySnapshot:
			preferred = cloneStrategySnapshot
		}
	}

	if preferred == cloneStrategySnapshot && s.canSnapshotClone(ctx, sourceClaim, sourceSC, targetSC, targetVolumeMode) {
		return cloneStrategySnapshot
	}
	if preferred != cloneStrategyHost && canCSIClone(sourceClaim, sourceSC, targetSC, targetVolumeMode) {
		return cloneStrategyCSI
	}
	if preferred == cloneStrategyCSI && s.canSnapshotClone(ctx, sourceClaim, sourceSC, targetSC, targetVolumeMode) {
		return cloneStrategySnapshot
	}
	return cloneStrategyHost
}

func (s *PersistentVolumeClaimService) canSnapshotClone(ctx context.Context, sourceClaim *corev1.PersistentVolumeClaim, sourceSC, targetSC *storagev1.StorageClass, targetVolumeMode corev1.PersistentVolumeMode) bool {
	return sourceSC.Provisioner == targetSC.Provisioner &&
		volumeModesEqual(sourceClaim, targetVolumeMode) &&
		s.snapshotClassForProvisioner(ctx, sourceSC.Provisioner) != ""
}

func canCSIClone(sourceClaim *corev1.PersistentVolumeClaim, sourceSC, targetSC *storagev1.StorageClass, targetVolumeMode corev1.PersistentVolumeMode) bool {
	return sourceClaim.Namespace != "" &&
		sourceSC.Provisioner == targetSC.Provisioner &&
		volumeModesEqual(sourceClaim, targetVolumeMode)
}

func (s *PersistentVolumeClaimService) fetchSourceStorageClass(ctx context.Context, claim *corev1.PersistentVolumeClaim) (*storagev1.StorageClass, error) {
	if claim.Spec.StorageClassName == nil || *claim.Spec.StorageClassName == "" {
		return nil, fmt.Errorf("source pvc %s/%s has no storageClassName", claim.Namespace, claim.Name)
	}
	sc, err := object.FetchObject(ctx, types.NamespacedName{Name: *claim.Spec.StorageClassName}, s.client, &storagev1.StorageClass{})
	if err != nil {
		return nil, fmt.Errorf("fetch source storage class: %w", err)
	}
	if sc == nil {
		return nil, fmt.Errorf("source storage class %q not found", *claim.Spec.StorageClassName)
	}
	return sc, nil
}

func (s *PersistentVolumeClaimService) snapshotClassForProvisioner(ctx context.Context, provisionerName string) string {
	var list vsv1.VolumeSnapshotClassList
	if err := s.client.List(ctx, &list); err != nil {
		return ""
	}
	for _, item := range list.Items {
		if item.Driver == provisionerName {
			return item.Name
		}
	}
	return ""
}

func (s *PersistentVolumeClaimService) ensureCloneSnapshot(ctx context.Context, sourceClaim, target *corev1.PersistentVolumeClaim, owner client.Object) error {
	snapshotName := target.Annotations[annotations.AnnPVCImportCloneSnapshot]
	if snapshotName == "" {
		return fmt.Errorf("clone snapshot annotation is empty")
	}
	existing, err := object.FetchObject(ctx, types.NamespacedName{Name: snapshotName, Namespace: target.Namespace}, s.client, &vsv1.VolumeSnapshot{})
	if err != nil {
		return fmt.Errorf("fetch clone snapshot: %w", err)
	}
	if existing != nil {
		return nil
	}

	sourceSC, err := s.fetchSourceStorageClass(ctx, sourceClaim)
	if err != nil {
		return err
	}
	snapshotClass := s.snapshotClassForProvisioner(ctx, sourceSC.Provisioner)
	if snapshotClass == "" {
		return fmt.Errorf("no compatible VolumeSnapshotClass found for provisioner %q", sourceSC.Provisioner)
	}

	ownerRef := ownerReferenceForObject(owner)
	ownerRef.Controller = ptr.To(false)

	vs := &vsv1.VolumeSnapshot{
		TypeMeta: metav1.TypeMeta{Kind: "VolumeSnapshot", APIVersion: "snapshot.storage.k8s.io/v1"},
		ObjectMeta: metav1.ObjectMeta{
			Name:            snapshotName,
			Namespace:       target.Namespace,
			OwnerReferences: []metav1.OwnerReference{ownerRef},
		},
		Spec: vsv1.VolumeSnapshotSpec{
			Source: vsv1.VolumeSnapshotSource{
				PersistentVolumeClaimName: ptr.To(sourceClaim.Name),
			},
			VolumeSnapshotClassName: ptr.To(snapshotClass),
		},
	}
	if err := s.client.Create(ctx, vs); err != nil && !k8serrors.IsAlreadyExists(err) {
		return fmt.Errorf("create clone snapshot: %w", err)
	}
	return nil
}

func (s *PersistentVolumeClaimService) cleanupCloneSnapshot(ctx context.Context, target *corev1.PersistentVolumeClaim) error {
	if target.Annotations[annotations.AnnPVCImportCloneStrategy] != cloneStrategySnapshot {
		return nil
	}
	snapshotName := target.Annotations[annotations.AnnPVCImportCloneSnapshot]
	if snapshotName == "" {
		return nil
	}
	err := s.client.Delete(ctx, &vsv1.VolumeSnapshot{ObjectMeta: metav1.ObjectMeta{Name: snapshotName, Namespace: target.Namespace}})
	if err != nil && !k8serrors.IsNotFound(err) {
		return err
	}
	return nil
}

func pvcCloneTargetSize(requested resource.Quantity, sourceClaim *corev1.PersistentVolumeClaim) resource.Quantity {
	size := requested.DeepCopy()
	for _, candidate := range []resource.Quantity{
		sourceClaim.Spec.Resources.Requests[corev1.ResourceStorage],
		sourceClaim.Status.Capacity[corev1.ResourceStorage],
	} {
		if !candidate.IsZero() && size.Cmp(candidate) < 0 {
			size = candidate.DeepCopy()
		}
	}
	return size
}

func volumeModesEqual(sourceClaim *corev1.PersistentVolumeClaim, targetVolumeMode corev1.PersistentVolumeMode) bool {
	sourceMode := corev1.PersistentVolumeFilesystem
	if sourceClaim.Spec.VolumeMode != nil {
		sourceMode = *sourceClaim.Spec.VolumeMode
	}
	return sourceMode == targetVolumeMode
}

func isSmartCloneStrategy(strategy string) bool {
	return strategy == cloneStrategySnapshot || strategy == cloneStrategyCSI
}
