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
	"errors"
	"fmt"
	"log/slog"

	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/component-base/featuregate"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	pvcspec "github.com/deckhouse/virtualization-controller/pkg/common/pvc"
	commonvd "github.com/deckhouse/virtualization-controller/pkg/common/vd"
	commonvmop "github.com/deckhouse/virtualization-controller/pkg/common/vmop"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	vdsupplements "github.com/deckhouse/virtualization-controller/pkg/controller/vd/internal/supplements"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdcondition"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
)

const migrationHandlerName = "MigrationHandler"

type storageClassValidator interface {
	IsStorageClassAllowed(scName string) bool
	IsStorageClassDeprecated(sc *storagev1.StorageClass) bool
}

type volumeAndAccessModesGetter interface {
	GetVolumeAndAccessModes(ctx context.Context, obj client.Object, sc *storagev1.StorageClass) (corev1.PersistentVolumeMode, corev1.PersistentVolumeAccessMode, error)
}

type MigrationHandler struct {
	client      client.Client
	scValidator storageClassValidator
	modeGetter  volumeAndAccessModesGetter
	gate        featuregate.FeatureGate
}

func NewMigrationHandler(client client.Client, storageClassValidator storageClassValidator, modeGetter volumeAndAccessModesGetter, gate featuregate.FeatureGate) *MigrationHandler {
	return &MigrationHandler{
		client:      client,
		scValidator: storageClassValidator,
		modeGetter:  modeGetter,
		gate:        gate,
	}
}

func (h MigrationHandler) Name() string {
	return migrationHandlerName
}

func (h MigrationHandler) Handle(ctx context.Context, vd *v1alpha2.VirtualDisk) (reconcile.Result, error) {
	if vd == nil || !vd.GetDeletionTimestamp().IsZero() {
		return reconcile.Result{}, nil
	}

	if !commonvd.VolumeMigrationEnabled(h.gate, vd) {
		return reconcile.Result{}, nil
	}

	log, ctx := logger.GetHandlerContext(ctx, migrationHandlerName)

	expectedAction, err := h.getAction(ctx, vd, log)
	if err != nil {
		return reconcile.Result{}, err
	}

	log = log.With(slog.String("action", expectedAction.String()))
	ctx = logger.ToContext(ctx, log)

	if expectedAction == none {
		log.Debug("Migration action")
	} else {
		log.Info("Migration action")
	}

	switch expectedAction {
	case none:
		h.handleNone(ctx, vd)
		return reconcile.Result{}, nil
	case migratePrepareTarget:
		return reconcile.Result{}, h.handleMigratePrepareTarget(ctx, vd)
	case migrateSync:
		return reconcile.Result{}, h.handleMigrateSync(ctx, vd)
	case revert:
		return reconcile.Result{}, h.handleRevert(ctx, vd)
	case complete:
		return reconcile.Result{}, h.handleComplete(ctx, vd)
	}

	return reconcile.Result{}, nil
}

type action int

func (a action) String() string {
	switch a {
	case none:
		return "none"
	case migratePrepareTarget:
		return "migratePrepareTarget"
	case migrateSync:
		return "migrateSync"
	case revert:
		return "revert"
	case complete:
		return "complete"
	default:
		return "unknown"
	}
}

const (
	none action = iota + 1
	migratePrepareTarget
	migrateSync
	revert
	complete
)

func (h MigrationHandler) getAction(ctx context.Context, vd *v1alpha2.VirtualDisk, log *slog.Logger) (action, error) {
	// We should check ready disk only before start migration.
	inUse, _ := conditions.GetCondition(vdcondition.InUseType, vd.Status.Conditions)
	if inUse.Reason != vdcondition.AttachedToVirtualMachine.String() && conditions.IsLastUpdated(inUse, vd) {
		return none, nil
	}

	currentlyMountedVM := commonvd.GetCurrentlyMountedVMName(vd)
	if currentlyMountedVM == "" {
		log.Info("VirtualDisk is not attached to any VirtualMachine. Skip...")
		return none, nil
	}

	vm := &v1alpha2.VirtualMachine{}
	err := h.client.Get(ctx, types.NamespacedName{Name: currentlyMountedVM, Namespace: vd.Namespace}, vm)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			if commonvd.IsMigrating(vd) {
				log.Info("VirtualMachine is not found, but the VirtualDisk is migrating. Will be reverted.")
				return revert, nil
			}
		}
		return none, err
	}

	if commonvd.IsMigrating(vd) {
		return h.getActionIfMigrationInProgress(ctx, vd, vm, log)
	}

	vmMigrating, _ := conditions.GetCondition(vmcondition.TypeMigrating, vm.Status.Conditions)
	migratingPending := vmMigrating.Reason == vmcondition.ReasonMigratingPending.String()

	if migratingPending {
		ready, _ := conditions.GetCondition(vdcondition.ReadyType, vd.Status.Conditions)
		if ready.Status != metav1.ConditionTrue && conditions.IsLastUpdated(ready, vd) {
			log.Info("VirtualDisk is not ready. Cannot be migrated now. Skip...")
			return none, nil
		}

		// Check StorageClass before local disks
		if commonvd.StorageClassChanged(vd) {
			log.Info("StorageClass Changed. VirtualDisk should be migrated.")
			return migratePrepareTarget, nil
		}

		vmMigratable, _ := conditions.GetCondition(vmcondition.TypeMigratable, vm.Status.Conditions)
		disksShouldBeMigrating := vmMigratable.Reason == vmcondition.ReasonDisksShouldBeMigrating.String()

		if disksShouldBeMigrating {
			return h.getActionIfDisksShouldBeMigrating(ctx, vd, log)
		}
	}

	return none, nil
}

func (h MigrationHandler) getActionIfMigrationInProgress(ctx context.Context, vd *v1alpha2.VirtualDisk, vm *v1alpha2.VirtualMachine, log *slog.Logger) (action, error) {
	// If VirtualMachine is not running, we can't migrate it. Should be reverted.
	running, _ := conditions.GetCondition(vmcondition.TypeRunning, vm.Status.Conditions)
	if running.Status != metav1.ConditionTrue {
		log.Info("VirtualMachine is not running. Will be reverted.", slog.String("vm.name", vm.Name), slog.String("vm.namespace", vm.Namespace))
		return revert, nil
	}

	if isMigrationsMatched(vm, vd) {
		if vm.Status.MigrationState == nil {
			log.Error("VirtualMachine migration state is empty. Please report a bug.", slog.String("vm.name", vm.Name), slog.String("vm.namespace", vm.Namespace))
			return none, nil
		}
		switch vm.Status.MigrationState.Result {
		case v1alpha2.MigrationResultFailed:
			return revert, nil
		case v1alpha2.MigrationResultSucceeded:
			return complete, nil
		}
	}

	// If migration is in progress. VirtualMachine must have the migrating condition.
	migrating, migratingFound := conditions.GetCondition(vmcondition.TypeMigrating, vm.Status.Conditions)
	if !migratingFound {
		log.Info("VirtualMachine is not migrating. Will be reverted.", slog.String("vm.name", vm.Name), slog.String("vm.namespace", vm.Namespace))
		return revert, nil
	}
	if migrating.Reason == vmcondition.ReasonLastMigrationFinishedWithError.String() {
		log.Info("Last VirtualMachine migration failed. Will be reverted.", slog.String("vm.name", vm.Name), slog.String("vm.namespace", vm.Namespace))
		return revert, nil
	}

	// If not found InProgress migrating vmop, that means some wrong migration happened. Revert.
	vmop, err := h.getInProgressMigratingVMOP(ctx, vm)
	if err != nil {
		return none, err
	}
	if vmop == nil {
		log.Info("VirtualMachine is not migrating. Will be reverted.", slog.String("vm.name", vm.Name), slog.String("vm.namespace", vm.Namespace))
		return revert, nil
	}

	return migrateSync, nil
}

func (h MigrationHandler) getActionIfDisksShouldBeMigrating(ctx context.Context, vd *v1alpha2.VirtualDisk, log *slog.Logger) (action, error) {
	pvc := &corev1.PersistentVolumeClaim{}
	err := h.client.Get(ctx, types.NamespacedName{Name: vd.Status.Target.PersistentVolumeClaim, Namespace: vd.Namespace}, pvc)
	if err != nil {
		return none, client.IgnoreNotFound(err)
	}

	for _, mode := range pvc.Spec.AccessModes {
		if mode == corev1.ReadWriteMany {
			log.Debug("PersistentVolumeClaim has ReadWriteMany access mode. Migrate VirtualDisk is no need. Skip...")
			return none, nil
		}
	}

	log.Info("VirtualDisk should be migrated.")
	return migratePrepareTarget, nil
}

func (h MigrationHandler) handleNone(_ context.Context, vd *v1alpha2.VirtualDisk) {
	// sync migrating conditions with pending changes
	// now, only one case possible: when storage class not found and migration was canceled
	migrating, _ := conditions.GetCondition(vdcondition.MigratingType, vd.Status.Conditions)
	if migrating.Reason == vdcondition.StorageClassNotFoundReason.String() {
		if !commonvd.StorageClassChanged(vd) {
			conditions.RemoveCondition(vdcondition.MigratingType, &vd.Status.Conditions)
		}
	}
}

func (h MigrationHandler) handleMigratePrepareTarget(ctx context.Context, vd *v1alpha2.VirtualDisk) error {
	log := logger.FromContext(ctx).With(logger.SlogHandler("migration"))

	if commonvd.IsMigrating(vd) {
		log.Error("Migration already in progress, do nothing, please report a bug.")
		return nil
	}

	cb := conditions.NewConditionBuilder(vdcondition.MigratingType).Generation(vd.Generation)

	// check resizing condition
	resizing, _ := conditions.GetCondition(vdcondition.ResizingType, vd.Status.Conditions)
	if resizing.Status == metav1.ConditionTrue {
		log.Debug("Migration is not allowed while the disk is being resized. Skip...")
		cb.
			Status(metav1.ConditionFalse).
			Reason(vdcondition.ResizingInProgressReason).
			Message("Migration is not allowed while the disk is being resized.")
		conditions.SetCondition(cb, &vd.Status.Conditions)
		return nil
	}

	// check snapshotting condition
	snapshotting, _ := conditions.GetCondition(vdcondition.SnapshottingType, vd.Status.Conditions)
	if snapshotting.Status == metav1.ConditionTrue {
		log.Debug("Migration is not allowed while the disk is being snapshotted. Skip...")
		cb.
			Status(metav1.ConditionFalse).
			Reason(vdcondition.SnapshottingInProgressReason).
			Message("Migration is not allowed while the disk is being snapshotted.")
		conditions.SetCondition(cb, &vd.Status.Conditions)
		return nil
	}

	// Reset migration info
	targetPVCName := vd.Status.MigrationState.TargetPVC
	vd.Status.MigrationState = v1alpha2.VirtualDiskMigrationState{}

	var targetStorageClass *storagev1.StorageClass
	var err error

	storageClassName := ""
	if vd.Spec.PersistentVolumeClaim.StorageClass != nil {
		storageClassName = *vd.Spec.PersistentVolumeClaim.StorageClass
	}

	switch {
	case storageClassName != "":
		targetStorageClass, err = object.FetchObject(ctx, types.NamespacedName{Name: storageClassName}, h.client, &storagev1.StorageClass{})
		if err != nil {
			return err
		}
		if targetStorageClass != nil {
			if !h.scValidator.IsStorageClassAllowed(targetStorageClass.Name) {
				log.Debug("StorageClass is not allowed for use. Skip...", slog.String("storageClass", targetStorageClass.Name))
				vd.Status.MigrationState = v1alpha2.VirtualDiskMigrationState{
					Result:         v1alpha2.VirtualDiskMigrationResultFailed,
					Message:        fmt.Sprintf("StorageClass %s is not allowed for use.", targetStorageClass.Name),
					StartTimestamp: metav1.Now(),
					EndTimestamp:   metav1.Now(),
				}
				return nil
			}
			if h.scValidator.IsStorageClassDeprecated(targetStorageClass) {
				log.Debug("StorageClass is deprecated, please use a different one. Skip...", slog.String("storageClass", targetStorageClass.Name))
				vd.Status.MigrationState = v1alpha2.VirtualDiskMigrationState{
					Result:         v1alpha2.VirtualDiskMigrationResultFailed,
					Message:        fmt.Sprintf("StorageClass %s is deprecated, please use a different one.", targetStorageClass.Name),
					StartTimestamp: metav1.Now(),
					EndTimestamp:   metav1.Now(),
				}
				return nil
			}
		}
	default:
		targetStorageClass, err = object.FetchObject(ctx, types.NamespacedName{Name: vd.Status.StorageClassName}, h.client, &storagev1.StorageClass{})
		if err != nil {
			return err
		}
	}

	if targetStorageClass == nil {
		log.Info("StorageClass not found, waiting for creation. Skip...")
		cb.Status(metav1.ConditionFalse).
			Reason(vdcondition.StorageClassNotFoundReason).
			Message("StorageClass not found, waiting for creation.")
		conditions.SetCondition(cb, &vd.Status.Conditions)
		return nil
	}

	if targetStorageClass.GetDeletionTimestamp() != nil {
		log.Info("StorageClass is terminating and cannot be used. Skip...", slog.String("storageClass", targetStorageClass.Name))
		vd.Status.MigrationState = v1alpha2.VirtualDiskMigrationState{
			Result:         v1alpha2.VirtualDiskMigrationResultFailed,
			Message:        fmt.Sprintf("StorageClass %s is terminating and cannot be used.", targetStorageClass.Name),
			StartTimestamp: metav1.Now(),
			EndTimestamp:   metav1.Now(),
		}
	}

	actualPvc, err := object.FetchObject(ctx, types.NamespacedName{Name: vd.Status.Target.PersistentVolumeClaim, Namespace: vd.Namespace}, h.client, &corev1.PersistentVolumeClaim{})
	if err != nil {
		return err
	}
	if actualPvc == nil {
		log.Info("Actual PersistentVolumeClaim is not found. Skip...")
		vd.Status.MigrationState = v1alpha2.VirtualDiskMigrationState{
			Result:         v1alpha2.VirtualDiskMigrationResultFailed,
			Message:        "Actual PersistentVolumeClaim is not found.",
			StartTimestamp: metav1.Now(),
			EndTimestamp:   metav1.Now(),
		}
		return nil
	}

	size := actualPvc.Status.Capacity[corev1.ResourceStorage]
	if size.IsZero() {
		log.Error("Failed to found capacity. Zero value. Skip...", slog.String("capacity", size.String()))
		vd.Status.MigrationState = v1alpha2.VirtualDiskMigrationState{
			Result:         v1alpha2.VirtualDiskMigrationResultFailed,
			Message:        fmt.Sprintf("Failed to parse capacity %q: zero value", vd.Status.Capacity),
			StartTimestamp: metav1.Now(),
			EndTimestamp:   metav1.Now(),
		}
		return nil
	}

	volumeMode, accessMode, err := h.modeGetter.GetVolumeAndAccessModes(ctx, vd, targetStorageClass)
	if err != nil {
		return fmt.Errorf("get volume and access modes: %w", err)
	}

	size = calculateTargetSize(size, vd.Status.ProvisionedCapacity, actualPvc.Spec.VolumeMode, volumeMode)

	log.Info("Start creating target PersistentVolumeClaim", slog.String("storageClass", targetStorageClass.Name), slog.String("capacity", size.String()))
	pvc, err := h.createTargetPersistentVolumeClaim(ctx, vd, targetStorageClass, size, targetPVCName, vd.Status.Target.PersistentVolumeClaim, volumeMode, accessMode)
	if err != nil {
		return err
	}

	log.Info(
		"The target PersistentVolumeClaim has been created or already exists",
		slog.String("state.source.pvc", vd.Status.Target.PersistentVolumeClaim),
		slog.String("state.target.pvc", pvc.Name),
	)

	if vd.Status.Target.PersistentVolumeClaim == pvc.Name {
		return errors.New("the target PersistentVolumeClaim name matched the source PersistentVolumeClaim name, please report a bug")
	}

	vd.Status.MigrationState = v1alpha2.VirtualDiskMigrationState{
		SourcePVC:      vd.Status.Target.PersistentVolumeClaim,
		TargetPVC:      pvc.Name,
		StartTimestamp: metav1.Now(),
	}

	cb.Status(metav1.ConditionFalse).
		Reason(vdcondition.MigratingWaitForTargetReadyReason).
		Message("Migration started.")
	conditions.SetCondition(cb, &vd.Status.Conditions)

	return h.handleMigrateSync(ctx, vd)
}

func calculateTargetSize(size resource.Quantity, realSize *resource.Quantity, oldVolumeMode *corev1.PersistentVolumeMode, newVolumeMode corev1.PersistentVolumeMode) resource.Quantity {
	if realSize != nil && realSize.Cmp(size) == 1 {
		return *realSize
	}

	blockToFs := oldVolumeMode != nil && *oldVolumeMode == corev1.PersistentVolumeBlock && newVolumeMode == corev1.PersistentVolumeFilesystem
	fsToBlock := oldVolumeMode != nil && *oldVolumeMode == corev1.PersistentVolumeFilesystem && newVolumeMode == corev1.PersistentVolumeBlock

	if blockToFs {
		// 1% overhead for filesystem
		fsOverhead := size.Value() / 100
		size.Add(*resource.NewQuantity(fsOverhead, resource.BinarySI))
		return size
	}

	if fsToBlock {
		// overhead no need to add
		return size
	}

	const blockOverhead = int64(8 * 1024 * 1024)
	size.Add(*resource.NewQuantity(blockOverhead, resource.BinarySI))
	return size
}

func (h MigrationHandler) handleMigrateSync(ctx context.Context, vd *v1alpha2.VirtualDisk) error {
	pvc, err := h.getTargetPersistentVolumeClaim(ctx, vd)
	if err != nil {
		return err
	}

	cb := conditions.NewConditionBuilder(vdcondition.MigratingType).
		Generation(vd.Generation).
		Status(metav1.ConditionFalse).
		Reason(vdcondition.MigratingWaitForTargetReadyReason)

	if pvc == nil {
		cb.Message("Target persistent volume claim is not found.")
		conditions.SetCondition(cb, &vd.Status.Conditions)
		return nil
	}

	if pvc.Status.Phase == corev1.ClaimBound {
		cb.Status(metav1.ConditionTrue).Reason(vdcondition.InProgress).Message("Target persistent volume claim is bound.")
		conditions.SetCondition(cb, &vd.Status.Conditions)
		return nil
	}

	if pvc.Status.Phase == corev1.ClaimPending {
		var storageClassName string
		if sc := pvc.Spec.StorageClassName; sc != nil && *sc != "" {
			storageClassName = *sc
		}
		if storageClassName == "" {
			cb.Message("Target persistent volume claim is pending.")
			conditions.SetCondition(cb, &vd.Status.Conditions)
			return nil
		}

		sc := &storagev1.StorageClass{}
		err = h.client.Get(ctx, types.NamespacedName{Name: storageClassName}, sc)
		if err != nil {
			if k8serrors.IsNotFound(err) {
				cb.Message("Target persistent volume claim is pending, StorageClass is not found.")
				conditions.SetCondition(cb, &vd.Status.Conditions)
				return nil
			}
			return err
		}

		isWaitForFistConsumer := sc.VolumeBindingMode == nil || *sc.VolumeBindingMode == storagev1.VolumeBindingWaitForFirstConsumer
		if isWaitForFistConsumer {
			cb.Status(metav1.ConditionTrue).Reason(vdcondition.InProgress).Message("Target persistent volume claim is waiting for first consumer.")
			conditions.SetCondition(cb, &vd.Status.Conditions)
			return nil
		}
	}

	cb.Message("Target persistent volume claim is not bound or not waiting for first consumer.")
	conditions.SetCondition(cb, &vd.Status.Conditions)
	return nil
}

func (h MigrationHandler) handleRevert(ctx context.Context, vd *v1alpha2.VirtualDisk) error {
	log := logger.FromContext(ctx)
	log.Info("Start reverting...")

	if vd.Status.MigrationState.TargetPVC == vd.Status.Target.PersistentVolumeClaim {
		return errors.New("cannot revert: the target PersistentVolumeClaim name matched the source PersistentVolumeClaim name, please report a bug")
	}

	err := h.deleteTargetPersistentVolumeClaim(ctx, vd)
	if err != nil {
		return err
	}
	log.Debug("Target PersistentVolumeClaim was deleted", slog.String("pvc.name", vd.Status.MigrationState.TargetPVC), slog.String("pvc.namespace", vd.Namespace))

	vd.Status.MigrationState.EndTimestamp = metav1.Now()
	vd.Status.MigrationState.Result = v1alpha2.VirtualDiskMigrationResultFailed
	vd.Status.MigrationState.Message = "Migration reverted."

	conditions.RemoveCondition(vdcondition.MigratingType, &vd.Status.Conditions)
	return nil
}

func (h MigrationHandler) handleComplete(ctx context.Context, vd *v1alpha2.VirtualDisk) error {
	log := logger.FromContext(ctx)
	log.Info("Start completing...")

	targetPVC, err := h.getTargetPersistentVolumeClaim(ctx, vd)
	if err != nil {
		return err
	}

	// If target PVC is not found, it means that the migration was not completed successfully.
	// revert old PVC and remove migration condition.
	if targetPVC == nil {
		log.Info("Target PersistentVolumeClaim is not found. Revert old PersistentVolumeClaim and remove migration condition.", slog.String("pvc.name", vd.Status.MigrationState.TargetPVC), slog.String("pvc.namespace", vd.Namespace))
		vd.Status.MigrationState.EndTimestamp = metav1.Now()
		vd.Status.MigrationState.Result = v1alpha2.VirtualDiskMigrationResultFailed
		vd.Status.MigrationState.Message = "Migration failed: target PVC is not found."

		vdsupplements.SetPVCName(vd, vd.Status.MigrationState.SourcePVC)
		conditions.RemoveCondition(vdcondition.MigratingType, &vd.Status.Conditions)
		return nil
	}

	// If target PVC is not bound, it means that the migration was not completed successfully.
	// revert old PVC and remove migration condition.
	if targetPVC.Status.Phase != corev1.ClaimBound {
		log.Info("Target PersistentVolumeClaim is not bound. Revert old PersistentVolumeClaim and remove migration condition.", slog.String("pvc.name", vd.Status.MigrationState.TargetPVC), slog.String("pvc.namespace", vd.Namespace))

		err = h.deleteTargetPersistentVolumeClaim(ctx, vd)
		if err != nil {
			return err
		}
		log.Debug("Target PersistentVolumeClaim was deleted", slog.String("pvc.name", vd.Status.MigrationState.TargetPVC), slog.String("pvc.namespace", vd.Namespace))

		vd.Status.MigrationState.EndTimestamp = metav1.Now()
		vd.Status.MigrationState.Result = v1alpha2.VirtualDiskMigrationResultFailed
		vd.Status.MigrationState.Message = "Migration failed: target PVC is not bound."

		vdsupplements.SetPVCName(vd, vd.Status.MigrationState.SourcePVC)
		conditions.RemoveCondition(vdcondition.MigratingType, &vd.Status.Conditions)
		return nil
	}

	log.Info("Complete migration. Delete source PersistentVolumeClaim", slog.String("pvc.name", vd.Status.MigrationState.SourcePVC), slog.String("pvc.namespace", vd.Namespace))
	err = h.deleteSourcePersistentVolumeClaim(ctx, vd)
	if err != nil {
		return err
	}
	log.Debug("Source PersistentVolumeClaim was deleted", slog.String("pvc.name", vd.Status.MigrationState.SourcePVC), slog.String("pvc.namespace", vd.Namespace))

	if sc := vd.Spec.PersistentVolumeClaim.StorageClass; sc != nil && *sc != "" {
		vd.Status.StorageClassName = *sc
	}
	vd.Status.MigrationState.EndTimestamp = metav1.Now()
	vd.Status.MigrationState.Result = v1alpha2.VirtualDiskMigrationResultSucceeded
	vd.Status.MigrationState.Message = "Migration completed."

	vdsupplements.SetPVCName(vd, vd.Status.MigrationState.TargetPVC)

	conditions.RemoveCondition(vdcondition.MigratingType, &vd.Status.Conditions)
	return nil
}

func (h MigrationHandler) getInProgressMigratingVMOP(ctx context.Context, vm *v1alpha2.VirtualMachine) (*v1alpha2.VirtualMachineOperation, error) {
	vmops := &v1alpha2.VirtualMachineOperationList{}
	err := h.client.List(ctx, vmops, client.InNamespace(vm.Namespace))
	if err != nil {
		return nil, err
	}

	for _, vmop := range vmops.Items {
		if commonvmop.IsMigration(&vmop) && commonvmop.IsInProgressOrPending(&vmop) {
			return &vmop, nil
		}
	}

	return nil, nil
}

func (h MigrationHandler) createTargetPersistentVolumeClaim(ctx context.Context, vd *v1alpha2.VirtualDisk, sc *storagev1.StorageClass, size resource.Quantity, targetPVCName, sourcePVCName string, volumeMode corev1.PersistentVolumeMode, accessMode corev1.PersistentVolumeAccessMode) (*corev1.PersistentVolumeClaim, error) {
	pvcs, err := listPersistentVolumeClaims(ctx, vd, h.client)
	if err != nil {
		return nil, err
	}
	switch len(pvcs) {
	case 1: // only source pvc exists
	case 2:
		for _, pvc := range pvcs {
			// If TargetPVC is empty, that means previous reconciliation failed and not updated TargetPVC in status.
			// So, we should use pvc, that is not equal to SourcePVC.
			if pvc.Name == targetPVCName || pvc.Name != sourcePVCName {
				return &pvc, nil
			}
		}
	default:
		return nil, fmt.Errorf("unexpected number of pvcs: %d, please report a bug", len(pvcs))
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

func (h MigrationHandler) getTargetPersistentVolumeClaim(ctx context.Context, vd *v1alpha2.VirtualDisk) (*corev1.PersistentVolumeClaim, error) {
	return object.FetchObject(ctx, types.NamespacedName{Name: vd.Status.MigrationState.TargetPVC, Namespace: vd.Namespace}, h.client, &corev1.PersistentVolumeClaim{})
}

func (h MigrationHandler) getSourcePersistentVolumeClaim(ctx context.Context, vd *v1alpha2.VirtualDisk) (*corev1.PersistentVolumeClaim, error) {
	return object.FetchObject(ctx, types.NamespacedName{Name: vd.Status.MigrationState.SourcePVC, Namespace: vd.Namespace}, h.client, &corev1.PersistentVolumeClaim{})
}

func (h MigrationHandler) deleteTargetPersistentVolumeClaim(ctx context.Context, vd *v1alpha2.VirtualDisk) error {
	pvc, err := h.getTargetPersistentVolumeClaim(ctx, vd)
	if pvc == nil || err != nil {
		return err
	}

	return deletePersistentVolumeClaim(ctx, pvc, h.client)
}

func (h MigrationHandler) deleteSourcePersistentVolumeClaim(ctx context.Context, vd *v1alpha2.VirtualDisk) error {
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
		case v1alpha2.FinalizerVDProtection, "kubernetes.io/pvc-protection": // remove
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

func listPersistentVolumeClaims(ctx context.Context, vd *v1alpha2.VirtualDisk, c client.Client) ([]corev1.PersistentVolumeClaim, error) {
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

// this function returns true when virtual machine migration includes virtual disk migration
// VD-StartTimestamp -> VM-StartTimestamp -> VM-EndTimestamp -> VD-EndTimestamp
func isMigrationsMatched(vm *v1alpha2.VirtualMachine, vd *v1alpha2.VirtualDisk) bool {
	vdStart := vd.Status.MigrationState.StartTimestamp
	state := vm.Status.MigrationState

	return state != nil && state.StartTimestamp != nil && state.StartTimestamp.After(vdStart.Time) && !state.EndTimestamp.IsZero()
}
