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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	"github.com/deckhouse/virtualization-controller/pkg/common/patch"
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
	GetVolumeAndAccessModes(ctx context.Context, sc *storev1.StorageClass) (corev1.PersistentVolumeMode, corev1.PersistentVolumeAccessMode, error)
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
	if vd == nil || !vd.GetDeletionTimestamp().IsZero() {
		return reconcile.Result{}, nil
	}

	if !featuregates.Default().Enabled(featuregates.VolumeMigration) {
		return reconcile.Result{}, nil
	}

	action, err := h.getAction(ctx, vd)
	if err != nil {
		return reconcile.Result{}, err
	}
	switch action {
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
	currentlyMountedVM := commonvd.CurrentlyMountedVM(vd)
	if currentlyMountedVM == "" {
		return none, nil
	}

	vm := virtv2.VirtualMachine{}
	err := h.client.Get(ctx, types.NamespacedName{Name: currentlyMountedVM, Namespace: vd.Namespace}, &vm)
	if err != nil {
		return none, client.IgnoreNotFound(err)
	}

	if migrationInProgress(vd) {
		state := vm.Status.MigrationState
		if state == nil {
			return none, nil
		}
		vdStart := vd.Status.MigrationInfo.StartTimestamp
		matchWindow := state.StartTimestamp != nil && state.StartTimestamp.After(vdStart.Time) && !state.EndTimestamp.IsZero()

		if matchWindow {
			switch state.Result {
			case virtv2.MigrationResultFailed:
				return revert, nil
			case virtv2.MigrationResultSucceeded:
				return complete, nil
			}
		}

		return none, nil
	}

	migratingCondition, _ := conditions.GetCondition(vmcondition.TypeMigrating, vm.Status.Conditions)
	migratableCondition, _ := conditions.GetCondition(vmcondition.TypeMigratable, vm.Status.Conditions)
	migratingPending := migratingCondition.Reason == vmcondition.ReasonMigratingPending.String()
	disksNotMigratable := migratableCondition.Reason == vmcondition.ReasonDisksNotMigratable.String()

	if migratingPending && disksNotMigratable {
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

	if sc := vd.Spec.PersistentVolumeClaim.StorageClass; sc != nil {
		if *sc != vd.Status.StorageClassName {
			return migrate, nil
		}
	}

	return none, nil
}

func (h MigrationHandler) handleMigrate(ctx context.Context, vd *virtv2.VirtualDisk) error {
	log := logger.FromContext(ctx).With(logger.SlogHandler("migration"))

	if migrationInProgress(vd) {
		log.Debug("Migration already in progress")
		return nil
	}

	// Reset migration info
	vd.Status.MigrationInfo = virtv2.VirtualDiskMigrationInfo{}

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
				vd.Status.MigrationInfo = virtv2.VirtualDiskMigrationInfo{
					Result:         virtv2.VirtualDiskMigrationResultFailed,
					Message:        fmt.Sprintf("StorageClass %s is not allowed for use.", targetStorageClass.Name),
					StartTimestamp: metav1.Now(),
					EndTimestamp:   metav1.Now(),
				}
				return nil
			}
			if h.scValidator.IsStorageClassDeprecated(targetStorageClass) {
				vd.Status.MigrationInfo = virtv2.VirtualDiskMigrationInfo{
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

	cb := conditions.NewConditionBuilder(vdcondition.MigrationType).Generation(vd.Generation)

	if targetStorageClass == nil {
		cb.Status(metav1.ConditionFalse).
			Reason(vdcondition.PendingMigratingReason).
			Message("StorageClass not found, waiting for creation.")
		conditions.SetCondition(cb, &vd.Status.Conditions)
		return nil
	}

	if targetStorageClass.GetDeletionTimestamp() != nil {
		vd.Status.MigrationInfo = virtv2.VirtualDiskMigrationInfo{
			Result:         virtv2.VirtualDiskMigrationResultFailed,
			Message:        fmt.Sprintf("StorageClass %s is terminating and cannot be used.", targetStorageClass.Name),
			StartTimestamp: metav1.Now(),
			EndTimestamp:   metav1.Now(),
		}
	}

	if targetStorageClass.VolumeBindingMode == nil || *targetStorageClass.VolumeBindingMode != storev1.VolumeBindingWaitForFirstConsumer {
		vd.Status.MigrationInfo = virtv2.VirtualDiskMigrationInfo{
			Result:         virtv2.VirtualDiskMigrationResultFailed,
			Message:        fmt.Sprintf("StorageClass %s does not support migration, VolumeBindingMode must be WaitForFirstConsumer.", targetStorageClass.Name),
			StartTimestamp: metav1.Now(),
			EndTimestamp:   metav1.Now(),
		}
		return nil
	}

	pvc, err := h.createMigrationPersistentVolumeClaim(ctx, vd, targetStorageClass)
	if err != nil {
		return err
	}

	vd.Status.MigrationInfo = virtv2.VirtualDiskMigrationInfo{
		SourcePVC:      vd.Status.Target.PersistentVolumeClaim,
		TargetPVC:      pvc.Name,
		StartTimestamp: metav1.Now(),
	}

	cb.Status(metav1.ConditionTrue).
		Reason(vdcondition.MigratingReason).
		Message("Migration started.")
	conditions.SetCondition(cb, &vd.Status.Conditions)

	return nil
}

func (h MigrationHandler) handleRevert(ctx context.Context, vd *virtv2.VirtualDisk) error {
	err := h.deleteTargetPersistentVolumeClaim(ctx, vd)
	if err != nil {
		return err
	}
	vd.Status.MigrationInfo.EndTimestamp = metav1.Now()
	vd.Status.MigrationInfo.Result = virtv2.VirtualDiskMigrationResultFailed
	vd.Status.MigrationInfo.Message = "Migration reverted."

	conditions.RemoveCondition(vdcondition.MigrationType, &vd.Status.Conditions)
	return nil
}

func (h MigrationHandler) handleComplete(ctx context.Context, vd *virtv2.VirtualDisk) error {
	err := h.deleteSourcePersistentVolumeClaim(ctx, vd)
	if err != nil {
		return err
	}

	vd.Status.MigrationInfo.EndTimestamp = metav1.Now()
	vd.Status.MigrationInfo.Result = virtv2.VirtualDiskMigrationResultSucceeded
	vd.Status.MigrationInfo.Message = "Migration completed."

	vdsupplements.SetPVCName(vd, vd.Status.MigrationInfo.TargetPVC)

	conditions.RemoveCondition(vdcondition.MigrationType, &vd.Status.Conditions)
	return nil
}

func (h MigrationHandler) createMigrationPersistentVolumeClaim(ctx context.Context, vd *virtv2.VirtualDisk, sc *storev1.StorageClass) (*corev1.PersistentVolumeClaim, error) {
	pvcs, err := listPersistentVolumeClaims(ctx, vd, h.client)
	if err != nil {
		return nil, err
	}
	switch len(pvcs) {
	case 1:
	case 2:
		for _, pvc := range pvcs {
			if pvc.Name == vd.Status.MigrationInfo.TargetPVC {
				return &pvc, nil
			}
		}
	default:
		return nil, fmt.Errorf("unexpected number of pvcs: %d, please report a bug", len(pvcs))
	}

	volumeMode, accessMode, err := h.modeGetter.GetVolumeAndAccessModes(ctx, sc)
	if err != nil {
		return nil, fmt.Errorf("get volume and access modes: %w", err)
	}

	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: fmt.Sprintf("vd-%s-", vd.UID),
			Namespace:    vd.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				service.MakeOwnerReference(vd),
			},
		},
		Spec: ptr.Deref(
			pvcspec.CreateSpec(&sc.Name, *vd.Spec.PersistentVolumeClaim.Size, accessMode, volumeMode),
			corev1.PersistentVolumeClaimSpec{},
		),
	}

	err = h.client.Create(ctx, pvc)
	return pvc, err
}

func (h MigrationHandler) deleteTargetPersistentVolumeClaim(ctx context.Context, vd *virtv2.VirtualDisk) error {
	pvc := &corev1.PersistentVolumeClaim{}
	err := h.client.Get(ctx, types.NamespacedName{Name: vd.Status.MigrationInfo.TargetPVC, Namespace: vd.Namespace}, pvc)
	if err != nil {
		return client.IgnoreNotFound(err)
	}
	return deletePersistentVolumeClaim(ctx, pvc, h.client)
}

func (h MigrationHandler) deleteSourcePersistentVolumeClaim(ctx context.Context, vd *virtv2.VirtualDisk) error {
	pvc := &corev1.PersistentVolumeClaim{}
	err := h.client.Get(ctx, types.NamespacedName{Name: vd.Status.MigrationInfo.SourcePVC, Namespace: vd.Namespace}, pvc)
	if err != nil {
		return client.IgnoreNotFound(err)
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

	var indexes []int
	for i, finalizer := range pvc.Finalizers {
		switch finalizer {
		// When pod completed, we cannot remove pvc, because Kubernetes protects pvc until pod is removed.
		// https://github.com/kubernetes/kubernetes/issues/120756
		case virtv2.FinalizerVDProtection, "kubernetes.io/pvc-protection":
			indexes = append(indexes, i)
		}
	}

	if len(indexes) == 0 {
		return nil
	}

	jsonPatch := patch.NewJSONPatch()
	for _, index := range indexes {
		op := patch.WithRemove(fmt.Sprintf("/metadata/finalizers/%d", index))
		jsonPatch.Append(op)
	}
	patchBytes, err := jsonPatch.Bytes()
	if err != nil {
		return err
	}

	return c.Patch(ctx, pvc, client.RawPatch(types.JSONPatchType, patchBytes))
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

func migrationInProgress(vd *virtv2.VirtualDisk) bool {
	return vd != nil &&
		(!vd.Status.MigrationInfo.StartTimestamp.IsZero() && vd.Status.MigrationInfo.EndTimestamp.IsZero())
}
