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

package service

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/common/patch"
	commonvd "github.com/deckhouse/virtualization-controller/pkg/common/vd"
	commonvm "github.com/deckhouse/virtualization-controller/pkg/common/vm"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/kvbuilder"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vm/internal/state"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
)

type MigrationVolumesService struct {
	client           client.Client
	makeKVVMFromSpec func(ctx context.Context, s state.VirtualMachineState) (*virtv1.VirtualMachine, error)
	delay            map[types.UID]time.Time
	delayDuration    time.Duration
}

func NewMigrationVolumesService(client client.Client, makeKVVMFromSpec func(ctx context.Context, s state.VirtualMachineState) (*virtv1.VirtualMachine, error), delayDuration time.Duration) *MigrationVolumesService {
	return &MigrationVolumesService{
		client:           client,
		makeKVVMFromSpec: makeKVVMFromSpec,
		delay:            make(map[types.UID]time.Time),
		delayDuration:    delayDuration,
	}
}

func (s MigrationVolumesService) SyncVolumes(ctx context.Context, vmState state.VirtualMachineState) (reconcile.Result, error) {
	log := logger.FromContext(ctx).With("func", "SyncVolumes")
	log.Debug("Start")
	defer log.Debug("End")

	vm := vmState.VirtualMachine().Changed()

	// TODO: refactor syncKVVM and allow migration
	if commonvm.RestartRequired(vm) {
		log.Info("Virtualmachine is restart required, skip volume migration.")
		return reconcile.Result{}, nil
	}

	// not syncing if migrating
	migrating, _ := conditions.GetCondition(vmcondition.TypeMigrating, vm.Status.Conditions)
	if migrating.Status == metav1.ConditionTrue {
		log.Info("Virtualmachine is migrating, skip volume migration.")
		return reconcile.Result{}, nil
	}

	kvvmInCluster, builtKVVM, builtKVVMWithMigrationVolumes, kvvmiInCluster, err := s.getMachines(ctx, vmState)
	if err != nil {
		return reconcile.Result{}, err
	}

	if kvvmInCluster == nil || kvvmiInCluster == nil {
		log.Info("Virtualmachine or kvvmi is nil, skip volume migration.")
		return reconcile.Result{}, nil
	}

	kvvmiSynced := equality.Semantic.DeepEqual(kvvmInCluster.Spec.Template.Spec.Volumes, kvvmiInCluster.Spec.Volumes)
	if !kvvmiSynced {
		// kubevirt does not sync volumes with kvvmi yet
		log.Info("kvvmi volumes are not synced yet, skip volume migration.")
		return reconcile.Result{}, nil
	}

	readWriteOnceDisks, storageClassChangedDisks, err := s.getDisks(ctx, vmState)
	if err != nil {
		return reconcile.Result{}, err
	}

	// Check disks in generated KVVM before running kvvmSynced check: detect non-migratable disks and disks with changed storage class.
	if !s.areDisksSynced(builtKVVMWithMigrationVolumes, readWriteOnceDisks) {
		log.Info("ReadWriteOnce disks are not synced yet, skip volume migration.")
		return reconcile.Result{}, nil
	}
	if !s.areDisksSynced(builtKVVMWithMigrationVolumes, storageClassChangedDisks) {
		log.Info("Storage class changed disks are not synced yet, skip volume migration.")
		return reconcile.Result{}, nil
	}

	kvvmSynced := equality.Semantic.DeepEqual(builtKVVMWithMigrationVolumes.Spec.Template.Spec.Volumes, kvvmInCluster.Spec.Template.Spec.Volumes)
	if kvvmSynced {
		// we already synced our vm with kvvm
		log.Info("kvvm volumes are already synced, skip volume migration.")
		return reconcile.Result{}, nil
	}

	migrationRequested := builtKVVMWithMigrationVolumes.Spec.UpdateVolumesStrategy != nil && *builtKVVMWithMigrationVolumes.Spec.UpdateVolumesStrategy == virtv1.UpdateVolumesStrategyMigration
	migrationInProgress := len(kvvmiInCluster.Status.MigratedVolumes) > 0

	if !migrationRequested && !migrationInProgress {
		log.Info("Migration is not requested and not in progress, skip volume migration.")
		return reconcile.Result{}, nil
	}

	if migrationRequested && !migrationInProgress {
		// We should wait 10 seconds. This delay allows user to change storage class on other volumes
		if len(storageClassChangedDisks) > 0 {
			delay, exists := s.delay[vm.UID]
			if !exists {
				log.Info("Delay is not set, set delay and requeue after delay duration.")
				s.delay[vm.UID] = time.Now().Add(s.delayDuration)
				return reconcile.Result{RequeueAfter: s.delayDuration}, nil
			}
			if time.Now().Before(delay) {
				log.Debug("Delay is not expired, requeue after delay duration.")
				return reconcile.Result{RequeueAfter: time.Until(delay)}, nil
			}
		}

		notReadyDisks, err := s.GetVirtualDiskNamesWithUnreadyTarget(ctx, vmState)
		if err != nil {
			return reconcile.Result{}, err
		}

		if len(notReadyDisks) > 0 {
			log.Info("Some disks are not ready, wait for disks to be ready.")
			return reconcile.Result{}, nil
		}

		log.Info("All disks are ready, patch kvvm with migration volumes.")
		err = s.patchVolumes(ctx, builtKVVMWithMigrationVolumes)
		if err != nil {
			return reconcile.Result{}, err
		}
		log.Debug("kvvm volumes are patched.")

		// Clean up the delay after it's passed
		delete(s.delay, vm.UID)

		return reconcile.Result{}, nil
	}

	// migration in progress
	// if some volumes is different, we should revert all and sync again in next reconcile

	migratedPVCNames := make(map[string]struct{})

	for _, vd := range readWriteOnceDisks {
		migratedPVCNames[vd.Status.MigrationState.TargetPVC] = struct{}{}
	}
	for _, vd := range storageClassChangedDisks {
		migratedPVCNames[vd.Status.MigrationState.TargetPVC] = struct{}{}
	}

	shouldRevert := false
	for _, v := range kvvmiInCluster.Status.MigratedVolumes {
		if v.DestinationPVCInfo != nil {
			if _, ok := migratedPVCNames[v.DestinationPVCInfo.ClaimName]; !ok {
				shouldRevert = true
				break
			}
		}
	}

	if shouldRevert {
		return reconcile.Result{}, s.patchVolumes(ctx, builtKVVM)
	}

	return reconcile.Result{}, nil
}

func (s MigrationVolumesService) patchVolumes(ctx context.Context, kvvm *virtv1.VirtualMachine) error {
	patchBytes, err := patch.NewJSONPatch(
		patch.WithReplace("/spec/updateVolumesStrategy", kvvm.Spec.UpdateVolumesStrategy),
		patch.WithReplace("/spec/template/spec/volumes", kvvm.Spec.Template.Spec.Volumes),
	).Bytes()
	if err != nil {
		return err
	}

	logger.FromContext(ctx).Debug("Patch kvvm with migration volumes.", slog.String("patch", string(patchBytes)))

	err = s.client.Patch(ctx, kvvm, client.RawPatch(types.JSONPatchType, patchBytes))
	return err
}

func (s MigrationVolumesService) VolumesSynced(ctx context.Context, vmState state.VirtualMachineState) (bool, error) {
	log := logger.FromContext(ctx).With("func", "VolumesSynced")

	kvvmInCluster, _, builtKVVMWithMigrationVolumes, kvvmiInCluster, err := s.getMachines(ctx, vmState)
	if err != nil {
		return false, err
	}

	if kvvmInCluster == nil || kvvmiInCluster == nil {
		return false, fmt.Errorf("kvvm or kvvmi is nil")
	}

	migratable, _ := conditions.GetKVVMICondition(virtv1.VirtualMachineInstanceIsMigratable, kvvmiInCluster.Status.Conditions)
	if migratable.Status != corev1.ConditionTrue {
		log.Info("VirtualMachine is not migratable, volumes are not synced yet.")
		return false, nil
	}

	kvvmSynced := equality.Semantic.DeepEqual(builtKVVMWithMigrationVolumes.Spec.Template.Spec.Volumes, kvvmInCluster.Spec.Template.Spec.Volumes)
	if !kvvmSynced {
		log.Info("kvvm volumes are not synced yet")
		log.Debug("", slog.Any("builtKVVM", builtKVVMWithMigrationVolumes.Spec.Template.Spec.Volumes), slog.Any("kvvm", kvvmInCluster.Spec.Template.Spec.Volumes))
		return false, nil
	}

	kvvmiSynced := equality.Semantic.DeepEqual(kvvmInCluster.Spec.Template.Spec.Volumes, kvvmiInCluster.Spec.Volumes)
	if !kvvmiSynced {
		log.Info("kvvmi volumes are not synced yet")
		log.Debug("", slog.Any("kvvmi", kvvmInCluster.Spec.Template.Spec.Volumes), slog.Any("kvvmi", kvvmiInCluster.Spec.Volumes))
		return false, nil
	}

	readWriteOnceDisks, storageClassChangedDisks, err := s.getDisks(ctx, vmState)
	if err != nil {
		return false, err
	}

	readWriteOnceDisksSynced := s.areDisksSynced(builtKVVMWithMigrationVolumes, readWriteOnceDisks)
	if !readWriteOnceDisksSynced {
		log.Info("ReadWriteOnce disks are not synced yet")
		log.Debug("", slog.Any("readWriteOnceDisks", readWriteOnceDisks), slog.Any("builtKVVM", builtKVVMWithMigrationVolumes.Spec.Template.Spec.Volumes))
		return false, nil
	}

	storageClassChangedDisksSynced := s.areDisksSynced(builtKVVMWithMigrationVolumes, storageClassChangedDisks)
	if !storageClassChangedDisksSynced {
		log.Info("Storage class changed disks are not synced yet")
		log.Debug("", slog.Any("storageClassChangedDisks", storageClassChangedDisks), slog.Any("builtKVVM", builtKVVMWithMigrationVolumes.Spec.Template.Spec.Volumes))
		return false, nil
	}

	return true, nil
}

func (s MigrationVolumesService) getMachines(ctx context.Context, vmState state.VirtualMachineState) (*virtv1.VirtualMachine, *virtv1.VirtualMachine, *virtv1.VirtualMachine, *virtv1.VirtualMachineInstance, error) {
	kvvmInCluster, err := vmState.KVVM(ctx)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	if kvvmInCluster == nil {
		return nil, nil, nil, nil, err
	}

	kvvmiInCluster, err := vmState.KVVMI(ctx)
	if err != nil {
		return nil, nil, nil, nil, err
	}

	builtKVVM, builtKVVMWithMigrationVolumes, err := s.makeKVVMFromVirtualMachineSpec(ctx, vmState)
	if err != nil {
		return nil, nil, nil, nil, err
	}

	return kvvmInCluster, builtKVVM, builtKVVMWithMigrationVolumes, kvvmiInCluster, nil
}

func (s MigrationVolumesService) getDisks(ctx context.Context, vmState state.VirtualMachineState) (map[string]*v1alpha2.VirtualDisk, map[string]*v1alpha2.VirtualDisk, error) {
	allDisks, err := vmState.VirtualDisksByName(ctx)
	if err != nil {
		return nil, nil, err
	}
	readWriteOnceDisks, err := s.getReadWriteOnceDisksByName(ctx, vmState)
	if err != nil {
		return nil, nil, err
	}
	storageClassChangedDisks := s.getStorageClassChangedDisksByName(allDisks, readWriteOnceDisks)

	return readWriteOnceDisks, storageClassChangedDisks, nil
}

func (s MigrationVolumesService) getReadWriteOnceDisksByName(ctx context.Context, vmState state.VirtualMachineState) (map[string]*v1alpha2.VirtualDisk, error) {
	readWriteOnceDisks, err := vmState.ReadWriteOnceVirtualDisks(ctx)
	if err != nil {
		return nil, err
	}

	readWriteOnceDisksMap := make(map[string]*v1alpha2.VirtualDisk, len(readWriteOnceDisks))
	for _, vd := range readWriteOnceDisks {
		readWriteOnceDisksMap[vd.Name] = vd
	}

	return readWriteOnceDisksMap, nil
}

func (s MigrationVolumesService) getStorageClassChangedDisksByName(all, readWriteOnceDisks map[string]*v1alpha2.VirtualDisk) map[string]*v1alpha2.VirtualDisk {
	storageClassChangedDisks := make(map[string]*v1alpha2.VirtualDisk)

	for _, vd := range all {
		if _, ok := readWriteOnceDisks[vd.Name]; ok {
			continue
		}

		if commonvd.StorageClassChanged(vd) {
			storageClassChangedDisks[vd.Name] = vd
		}
	}

	return storageClassChangedDisks
}

func (s MigrationVolumesService) GetVirtualDiskNamesWithUnreadyTarget(ctx context.Context, vmState state.VirtualMachineState) ([]string, error) {
	readWriteOnceDisks, storageClassChangedDisks, err := s.getDisks(ctx, vmState)
	if err != nil {
		return nil, err
	}

	readyReadWriteOnce, err := s.getReadyTargetPVCs(ctx, readWriteOnceDisks)
	if err != nil {
		return nil, err
	}

	readyStorageClassChanged, err := s.getReadyTargetPVCs(ctx, storageClassChangedDisks)
	if err != nil {
		return nil, err
	}

	var notReadyDisks []string
	for _, vd := range readWriteOnceDisks {
		if _, ok := readyReadWriteOnce[vd.Name]; !ok {
			notReadyDisks = append(notReadyDisks, vd.Name)
		}
	}
	for _, vd := range storageClassChangedDisks {
		if _, ok := readyStorageClassChanged[vd.Name]; !ok {
			notReadyDisks = append(notReadyDisks, vd.Name)
		}
	}

	return notReadyDisks, nil
}

func (s MigrationVolumesService) getReadyTargetPVCs(ctx context.Context, disks map[string]*v1alpha2.VirtualDisk) (map[string]*corev1.PersistentVolumeClaim, error) {
	targetPVCs := make(map[string]*corev1.PersistentVolumeClaim)

	storageClassesIsWaitForFirstConsumer := make(map[string]bool)

	for _, disk := range disks {
		target := disk.Status.Target.PersistentVolumeClaim
		if target != "" && disk.Status.MigrationState.EndTimestamp.IsZero() {
			pvc := &corev1.PersistentVolumeClaim{}
			err := s.client.Get(ctx, types.NamespacedName{Name: target, Namespace: disk.Namespace}, pvc)
			if err != nil {
				if k8serrors.IsNotFound(err) {
					continue
				}
				return nil, err
			}

			switch pvc.Status.Phase {
			case corev1.ClaimBound:
				targetPVCs[disk.Name] = pvc
			case corev1.ClaimPending:
				var storageClassName string
				if sc := pvc.Spec.StorageClassName; sc != nil && *sc != "" {
					storageClassName = *sc
				} else {
					continue
				}

				isWaitForFirstConsumer, found := storageClassesIsWaitForFirstConsumer[storageClassName]
				if !found {
					sc := &storagev1.StorageClass{}
					err = s.client.Get(ctx, types.NamespacedName{Name: storageClassName}, sc)
					if err != nil {
						if k8serrors.IsNotFound(err) {
							continue
						}
						return nil, err
					}

					isWaitForFirstConsumer = sc.VolumeBindingMode == nil || *sc.VolumeBindingMode == storagev1.VolumeBindingWaitForFirstConsumer
					storageClassesIsWaitForFirstConsumer[storageClassName] = isWaitForFirstConsumer
				}

				if isWaitForFirstConsumer {
					targetPVCs[disk.Name] = pvc
				}
			}
		}
	}

	return targetPVCs, nil
}

func (s MigrationVolumesService) makeKVVMFromVirtualMachineSpec(ctx context.Context, vmState state.VirtualMachineState) (*virtv1.VirtualMachine, *virtv1.VirtualMachine, error) {
	kvvm, err := s.makeKVVMFromSpec(ctx, vmState)
	if err != nil {
		return nil, nil, err
	}
	kvvmBuilder := kvbuilder.NewKVVM(kvvm.DeepCopy(), kvbuilder.DefaultOptions(vmState.VirtualMachine().Current()))
	vdByName, err := vmState.VirtualDisksByName(ctx)
	if err != nil {
		return nil, nil, err
	}
	err = kvbuilder.ApplyMigrationVolumes(kvvmBuilder, vmState.VirtualMachine().Changed(), vdByName)
	if err != nil {
		return nil, nil, err
	}
	kvvmWithMigrationVolumes := kvvmBuilder.GetResource()
	return kvvm, kvvmWithMigrationVolumes, nil
}

// areDisksSynced checks whether all disks are synchronized with their corresponding PVCs in kvvm
// All TargetPVCs on disks must be present in kvvm
func (s MigrationVolumesService) areDisksSynced(kvvm *virtv1.VirtualMachine, disks map[string]*v1alpha2.VirtualDisk) bool {
	if len(disks) == 0 {
		return true
	}

	claims := make(map[string]struct{})
	for _, v := range kvvm.Spec.Template.Spec.Volumes {
		if v.PersistentVolumeClaim != nil {
			claims[v.PersistentVolumeClaim.ClaimName] = struct{}{}
		}
	}

	for _, d := range disks {
		if _, ok := claims[d.Status.MigrationState.TargetPVC]; !ok {
			return false
		}
	}

	return true
}
