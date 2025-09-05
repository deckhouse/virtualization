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

package internal

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	storev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	pvcspec "github.com/deckhouse/virtualization-controller/pkg/common/pvc"
	commonvd "github.com/deckhouse/virtualization-controller/pkg/common/vd"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	vdsupplements "github.com/deckhouse/virtualization-controller/pkg/controller/vd/internal/supplements"
	"github.com/deckhouse/virtualization-controller/pkg/featuregates"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdcondition"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
)

type storageClassValidator interface {
	IsStorageClassAllowed(scName string) bool
	IsStorageClassDeprecated(sc *storev1.StorageClass) bool
}

type volumeAndAccessModesGetter interface {
	GetVolumeAndAccessModes(ctx context.Context, obj client.Object, sc *storev1.StorageClass) (corev1.PersistentVolumeMode, corev1.PersistentVolumeAccessMode, error)
}

type MigrationHandler struct {
	client      client.Client
	scValidator storageClassValidator
	modeGetter  volumeAndAccessModesGetter
}

func NewMigrationHandler(client client.Client, storageClassValidator storageClassValidator, modeGetter volumeAndAccessModesGetter) *MigrationHandler {
	return &MigrationHandler{
		client:      client,
		scValidator: storageClassValidator,
		modeGetter:  modeGetter,
	}
}

func (h MigrationHandler) Handle(ctx context.Context, vd *virtv2.VirtualDisk) (reconcile.Result, error) {
	if !featuregates.Default().Enabled(featuregates.VolumeMigration) {
		return reconcile.Result{}, nil
	}

	if vd == nil || !vd.GetDeletionTimestamp().IsZero() {
		return reconcile.Result{}, nil
	}

	// TODO: check vm migration

	expectedAction, err := h.getAction(ctx, vd)
	if err != nil {
		return reconcile.Result{}, err
	}
	switch expectedAction {
	case none:
		return reconcile.Result{}, nil
	case migrate:
		return reconcile.Result{}, h.handleMigrate(ctx, vd)
	case revert:
		return reconcile.Result{}, h.handleRevert(ctx, vd)
	case complete:
		return reconcile.Result{}, h.handleComplete(ctx, vd)
	}

	return reconcile.Result{}, nil
}

type action int

const (
	none action = iota
	migrate
	revert
	complete
)

func (h MigrationHandler) getAction(ctx context.Context, vd *virtv2.VirtualDisk) (action, error) {
	// We should not check ready condition, because if disk in use and attached to vm, it is already ready.
	inUse, _ := conditions.GetCondition(vdcondition.InUseType, vd.Status.Conditions)
	if inUse.Reason != vdcondition.AttachedToVirtualMachine.String() && conditions.IsLastUpdated(inUse, vd) {
		return none, nil
	}

	currentlyMountedVM := commonvd.GetCurrentlyMountedVMName(vd)
	if currentlyMountedVM == "" {
		return none, nil
	}

	vm := virtv2.VirtualMachine{}
	err := h.client.Get(ctx, types.NamespacedName{Name: currentlyMountedVM, Namespace: vd.Namespace}, &vm)
	if err != nil {
		return none, client.IgnoreNotFound(err)
	}

	if isMigrationInProgress(vd) {
		return h.getActionIfMigrationInProgress(vd, vm), nil
	}

	vmMigrating, _ := conditions.GetCondition(vmcondition.TypeMigrating, vm.Status.Conditions)
	vmMigratable, _ := conditions.GetCondition(vmcondition.TypeMigratable, vm.Status.Conditions)
	migratingPending := vmMigrating.Reason == vmcondition.ReasonMigratingPending.String()
	disksNotMigratable := vmMigratable.Reason == vmcondition.ReasonDisksNotMigratable.String()

	if migratingPending && disksNotMigratable {
		return h.getActionIfDisksNotMigratable(ctx, vd)
	}

	return h.getActionIfStorageClassChanged(vd), nil
}

func (h MigrationHandler) getActionIfMigrationInProgress(vd *virtv2.VirtualDisk, vm virtv2.VirtualMachine) action {
	vdStart := vd.Status.MigrationState.StartTimestamp
	state := vm.Status.MigrationState
	if state == nil {
		return none
	}

	matchWindow := state.StartTimestamp != nil && state.StartTimestamp.After(vdStart.Time) && !state.EndTimestamp.IsZero()
	if matchWindow {
		switch state.Result {
		case virtv2.MigrationResultFailed:
			return revert
		case virtv2.MigrationResultSucceeded:
			return complete
		}
	}

	migrating, _ := conditions.GetCondition(vmcondition.TypeMigrating, vm.Status.Conditions)
	if migrating.Reason == vmcondition.ReasonLastMigrationFinishedWithError.String() {
		return revert
	}

	return none
}

func (h MigrationHandler) getActionIfDisksNotMigratable(ctx context.Context, vd *virtv2.VirtualDisk) (action, error) {
	pvc := &corev1.PersistentVolumeClaim{}
	err := h.client.Get(ctx, types.NamespacedName{Name: vd.Status.Target.PersistentVolumeClaim, Namespace: vd.Namespace}, pvc)
	if err != nil {
		return none, client.IgnoreNotFound(err)
	}

	for _, mode := range pvc.Spec.AccessModes {
		if mode == corev1.ReadWriteMany {
			return none, nil
		}
	}

	return migrate, nil
}

func (h MigrationHandler) getActionIfStorageClassChanged(vd *virtv2.VirtualDisk) action {
	if sc := vd.Spec.PersistentVolumeClaim.StorageClass; sc != nil {
		if *sc != vd.Status.StorageClassName && *sc != "" && vd.Status.StorageClassName != "" {
			return migrate
		}
	}

	return none
}

func (h MigrationHandler) handleMigrate(ctx context.Context, vd *virtv2.VirtualDisk) error {
	log := logger.FromContext(ctx).With(logger.SlogHandler("migration"))

	if isMigrationInProgress(vd) {
		log.Error("Migration already in progress, do nothing, please report a bug.")
		return nil
	}

	cb := conditions.NewConditionBuilder(vdcondition.MigratingType).Generation(vd.Generation)

	// check resizing condition
	resizing, _ := conditions.GetCondition(vdcondition.ResizingType, vd.Status.Conditions)
	if resizing.Status == metav1.ConditionTrue {
		cb.
			Status(metav1.ConditionFalse).
			Reason(vdcondition.PendingMigratingReason).
			Message("Migration is not allowed while the disk is being resized.")
		conditions.SetCondition(cb, &vd.Status.Conditions)
		return nil
	}

	// check snapshotting condition
	snapshotting, _ := conditions.GetCondition(vdcondition.SnapshottingType, vd.Status.Conditions)
	if snapshotting.Status == metav1.ConditionTrue {
		cb.
			Status(metav1.ConditionFalse).
			Reason(vdcondition.PendingMigratingReason).
			Message("Migration is not allowed while the disk is being snapshotted.")
		conditions.SetCondition(cb, &vd.Status.Conditions)
		return nil
	}

	// Reset migration info
	vd.Status.MigrationState = virtv2.VirtualDiskMigrationState{}

	var targetStorageClass *storev1.StorageClass
	var err error

	storageClassName := ""
	if vd.Spec.PersistentVolumeClaim.StorageClass != nil {
		storageClassName = *vd.Spec.PersistentVolumeClaim.StorageClass
	}

	switch {
	case storageClassName != "":
		targetStorageClass, err = object.FetchObject(ctx, types.NamespacedName{Name: storageClassName}, h.client, &storev1.StorageClass{})
		if err != nil {
			return err
		}
		if targetStorageClass != nil {
			if !h.scValidator.IsStorageClassAllowed(targetStorageClass.Name) {
				vd.Status.MigrationState = virtv2.VirtualDiskMigrationState{
					Result:         virtv2.VirtualDiskMigrationResultFailed,
					Message:        fmt.Sprintf("StorageClass %s is not allowed for use.", targetStorageClass.Name),
					StartTimestamp: metav1.Now(),
					EndTimestamp:   metav1.Now(),
				}
				return nil
			}
			if h.scValidator.IsStorageClassDeprecated(targetStorageClass) {
				vd.Status.MigrationState = virtv2.VirtualDiskMigrationState{
					Result:         virtv2.VirtualDiskMigrationResultFailed,
					Message:        fmt.Sprintf("StorageClass %s is deprecated, please use a different one.", targetStorageClass.Name),
					StartTimestamp: metav1.Now(),
					EndTimestamp:   metav1.Now(),
				}
				return nil
			}
		}
	default:
		targetStorageClass, err = object.FetchObject(ctx, types.NamespacedName{Name: vd.Status.StorageClassName}, h.client, &storev1.StorageClass{})
		if err != nil {
			return err
		}
	}

	if targetStorageClass == nil {
		cb.Status(metav1.ConditionFalse).
			Reason(vdcondition.PendingMigratingReason).
			Message("StorageClass not found, waiting for creation.")
		conditions.SetCondition(cb, &vd.Status.Conditions)
		return nil
	}

	if targetStorageClass.GetDeletionTimestamp() != nil {
		vd.Status.MigrationState = virtv2.VirtualDiskMigrationState{
			Result:         virtv2.VirtualDiskMigrationResultFailed,
			Message:        fmt.Sprintf("StorageClass %s is terminating and cannot be used.", targetStorageClass.Name),
			StartTimestamp: metav1.Now(),
			EndTimestamp:   metav1.Now(),
		}
	}

	if targetStorageClass.VolumeBindingMode == nil || *targetStorageClass.VolumeBindingMode != storev1.VolumeBindingWaitForFirstConsumer {
		vd.Status.MigrationState = virtv2.VirtualDiskMigrationState{
			Result:         virtv2.VirtualDiskMigrationResultFailed,
			Message:        fmt.Sprintf("StorageClass %s does not support migration, VolumeBindingMode must be WaitForFirstConsumer.", targetStorageClass.Name),
			StartTimestamp: metav1.Now(),
			EndTimestamp:   metav1.Now(),
		}
		return nil
	}

	size, err := resource.ParseQuantity(vd.Status.Capacity)
	if err != nil {
		vd.Status.MigrationState = virtv2.VirtualDiskMigrationState{
			Result:         virtv2.VirtualDiskMigrationResultFailed,
			Message:        fmt.Sprintf("Failed to parse capacity %q: %v", vd.Status.Capacity, err),
			StartTimestamp: metav1.Now(),
			EndTimestamp:   metav1.Now(),
		}
		return nil
	}
	if size.IsZero() {
		vd.Status.MigrationState = virtv2.VirtualDiskMigrationState{
			Result:         virtv2.VirtualDiskMigrationResultFailed,
			Message:        fmt.Sprintf("Failed to parse capacity %q: zero value", vd.Status.Capacity),
			StartTimestamp: metav1.Now(),
			EndTimestamp:   metav1.Now(),
		}
		return nil
	}

	pvc, err := h.createTargetPersistentVolumeClaim(ctx, vd, targetStorageClass, size)
	if err != nil {
		return err
	}

	vd.Status.MigrationState = virtv2.VirtualDiskMigrationState{
		SourcePVC:      vd.Status.Target.PersistentVolumeClaim,
		TargetPVC:      pvc.Name,
		StartTimestamp: metav1.Now(),
	}

	cb.Status(metav1.ConditionTrue).
		Reason(vdcondition.MigratingInProgressReason).
		Message("Migration started.")
	conditions.SetCondition(cb, &vd.Status.Conditions)

	return nil
}

func (h MigrationHandler) handleRevert(ctx context.Context, vd *virtv2.VirtualDisk) error {
	err := h.deleteTargetPersistentVolumeClaim(ctx, vd)
	if err != nil {
		return err
	}
	vd.Status.MigrationState.EndTimestamp = metav1.Now()
	vd.Status.MigrationState.Result = virtv2.VirtualDiskMigrationResultFailed
	vd.Status.MigrationState.Message = "Migration reverted."

	conditions.RemoveCondition(vdcondition.MigratingType, &vd.Status.Conditions)
	return nil
}

func (h MigrationHandler) handleComplete(ctx context.Context, vd *virtv2.VirtualDisk) error {
	targetPVC, err := h.getTargetPersistentVolumeClaim(ctx, vd)
	if err != nil {
		return err
	}

	// If target PVC is not found, it means that the migration was not completed successfully.
	// revert old PVC and remove migration condition.
	if targetPVC == nil {
		vd.Status.MigrationState.EndTimestamp = metav1.Now()
		vd.Status.MigrationState.Result = virtv2.VirtualDiskMigrationResultFailed
		vd.Status.MigrationState.Message = "Migration failed: target PVC is not found."

		vdsupplements.SetPVCName(vd, vd.Status.MigrationState.SourcePVC)
		conditions.RemoveCondition(vdcondition.MigratingType, &vd.Status.Conditions)
		return nil
	}

	// If target PVC is not bound, it means that the migration was not completed successfully.
	// revert old PVC and remove migration condition.
	if targetPVC.Status.Phase != corev1.ClaimBound {
		err = h.deleteTargetPersistentVolumeClaim(ctx, vd)
		if err != nil {
			return err
		}

		vd.Status.MigrationState.EndTimestamp = metav1.Now()
		vd.Status.MigrationState.Result = virtv2.VirtualDiskMigrationResultFailed
		vd.Status.MigrationState.Message = "Migration failed: target PVC is not bound."

		vdsupplements.SetPVCName(vd, vd.Status.MigrationState.SourcePVC)
		conditions.RemoveCondition(vdcondition.MigratingType, &vd.Status.Conditions)
		return nil
	}

	err = h.deleteSourcePersistentVolumeClaim(ctx, vd)
	if err != nil {
		return err
	}

	vd.Status.MigrationState.EndTimestamp = metav1.Now()
	vd.Status.MigrationState.Result = virtv2.VirtualDiskMigrationResultSucceeded
	vd.Status.MigrationState.Message = "Migration completed."

	vdsupplements.SetPVCName(vd, vd.Status.MigrationState.TargetPVC)

	conditions.RemoveCondition(vdcondition.MigratingType, &vd.Status.Conditions)
	return nil
}

func (h MigrationHandler) createTargetPersistentVolumeClaim(ctx context.Context, vd *virtv2.VirtualDisk, sc *storev1.StorageClass, size resource.Quantity) (*corev1.PersistentVolumeClaim, error) {
	pvcs, err := listPersistentVolumeClaims(ctx, vd, h.client)
	if err != nil {
		return nil, err
	}
	switch len(pvcs) {
	case 1:
	case 2:
		for _, pvc := range pvcs {
			// If TargetPVC is empty, that means previous reconciliation failed and not updated TargetPVC in status.
			// So, we should use pvc, that is not equal to SourcePVC.
			if pvc.Name == vd.Status.MigrationState.TargetPVC || pvc.Name != vd.Status.MigrationState.SourcePVC {
				return &pvc, nil
			}
		}
	default:
		return nil, fmt.Errorf("unexpected number of pvcs: %d, please report a bug", len(pvcs))
	}

	volumeMode, accessMode, err := h.modeGetter.GetVolumeAndAccessModes(ctx, vd, sc)
	if err != nil {
		return nil, fmt.Errorf("get volume and access modes: %w", err)
	}

	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: fmt.Sprintf("vd-%s-", vd.UID),
			Namespace:    vd.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				service.MakeControllerOwnerReference(vd),
			},
		},
		Spec: ptr.Deref(
			pvcspec.CreateSpec(&sc.Name, size, accessMode, volumeMode),
			corev1.PersistentVolumeClaimSpec{},
		),
	}

	err = h.client.Create(ctx, pvc)
	return pvc, err
}

func (h MigrationHandler) getTargetPersistentVolumeClaim(ctx context.Context, vd *virtv2.VirtualDisk) (*corev1.PersistentVolumeClaim, error) {
	return object.FetchObject(ctx, types.NamespacedName{Name: vd.Status.MigrationState.TargetPVC, Namespace: vd.Namespace}, h.client, &corev1.PersistentVolumeClaim{})
}

func (h MigrationHandler) getSourcePersistentVolumeClaim(ctx context.Context, vd *virtv2.VirtualDisk) (*corev1.PersistentVolumeClaim, error) {
	return object.FetchObject(ctx, types.NamespacedName{Name: vd.Status.MigrationState.SourcePVC, Namespace: vd.Namespace}, h.client, &corev1.PersistentVolumeClaim{})
}

func (h MigrationHandler) deleteTargetPersistentVolumeClaim(ctx context.Context, vd *virtv2.VirtualDisk) error {
	pvc, err := h.getTargetPersistentVolumeClaim(ctx, vd)
	if pvc == nil || err != nil {
		return err
	}

	return deletePersistentVolumeClaim(ctx, pvc, h.client)
}

func (h MigrationHandler) deleteSourcePersistentVolumeClaim(ctx context.Context, vd *virtv2.VirtualDisk) error {
	pvc, err := h.getSourcePersistentVolumeClaim(ctx, vd)
	if pvc == nil || err != nil {
		return err
	}

	return deletePersistentVolumeClaim(ctx, pvc, h.client)
}

func deletePersistentVolumeClaim(ctx context.Context, pvc *corev1.PersistentVolumeClaim, c client.Client) error {
	if pvc.DeletionTimestamp.IsZero() {
		err := c.Delete(ctx, pvc)
		if err != nil {
			return err
		}
	}

	var newFinalizers []string
	var shouldPatch bool
	for _, finalizer := range pvc.Finalizers {
		switch finalizer {
		// When pod completed, we cannot remove pvc, because Kubernetes protects pvc until pod is removed.
		// https://github.com/kubernetes/kubernetes/issues/120756
		case virtv2.FinalizerVDProtection, "kubernetes.io/pvc-protection": // remove
			shouldPatch = true
		default:
			newFinalizers = append(newFinalizers, finalizer)
		}
	}

	if shouldPatch {
		patch, err := service.GetPatchFinalizers(newFinalizers)
		if err != nil {
			return err
		}
		return client.IgnoreNotFound(c.Patch(ctx, pvc, patch))
	}

	return nil
}

func listPersistentVolumeClaims(ctx context.Context, vd *virtv2.VirtualDisk, c client.Client) ([]corev1.PersistentVolumeClaim, error) {
	pvcList := &corev1.PersistentVolumeClaimList{}
	err := c.List(ctx, pvcList, client.InNamespace(vd.Namespace))
	if err != nil {
		return nil, err
	}

	var pvcs []corev1.PersistentVolumeClaim
	for _, pvc := range pvcList.Items {
		for _, ownerRef := range pvc.OwnerReferences {
			if ownerRef.UID == vd.UID {
				pvcs = append(pvcs, pvc)
				break
			}
		}
	}

	return pvcs, nil
}

func isMigrationInProgress(vd *virtv2.VirtualDisk) bool {
	return vd != nil &&
		(!vd.Status.MigrationState.StartTimestamp.IsZero() && vd.Status.MigrationState.EndTimestamp.IsZero())
}
