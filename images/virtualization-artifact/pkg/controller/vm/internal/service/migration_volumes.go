package service

import (
	"context"
	"fmt"
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
	vm := vmState.VirtualMachine().Changed()

	// we can't migrate VM which is restarting
	if commonvm.RestartRequired(vm) {
		return reconcile.Result{}, nil
	}

	// not syncing if migrating
	migrating, _ := conditions.GetCondition(vmcondition.TypeMigrating, vm.Status.Conditions)
	if migrating.Status == metav1.ConditionTrue {
		return reconcile.Result{}, nil
	}

	kvvmInCluster, builtKVVM, builtKVVMWithMigrationVolumes, kvvmiInCluster, err := s.getMachines(ctx, vmState)
	if err != nil {
		return reconcile.Result{}, err
	}

	if kvvmInCluster == nil || kvvmiInCluster == nil {
		return reconcile.Result{}, nil
	}

	kvvmiSynced := equality.Semantic.DeepEqual(kvvmInCluster.Spec.Template.Spec.Volumes, kvvmiInCluster.Spec.Volumes)
	if !kvvmiSynced {
		// kubevirt does not sync volumes with kvvmi yet
		return reconcile.Result{}, nil
	}

	_, nonMigratableDisks, storageClassChangedDisks, err := s.getDisks(ctx, vmState)
	if err != nil {
		return reconcile.Result{}, err
	}

	// we should check that's generated kvvm has disks, which are not migratable or storage class changed, before kvvmSynced check
	if !s.isDisksSynced(builtKVVMWithMigrationVolumes, nonMigratableDisks) {
		return reconcile.Result{}, nil
	}
	if !s.isDisksSynced(builtKVVMWithMigrationVolumes, storageClassChangedDisks) {
		return reconcile.Result{}, nil
	}

	kvvmSynced := equality.Semantic.DeepEqual(builtKVVMWithMigrationVolumes.Spec.Template.Spec.Volumes, kvvmInCluster.Spec.Template.Spec.Volumes)
	if kvvmSynced {
		// we already synced our vm with kvvm
		return reconcile.Result{}, nil
	}

	migrationRequested := builtKVVMWithMigrationVolumes.Spec.UpdateVolumesStrategy != nil && *builtKVVMWithMigrationVolumes.Spec.UpdateVolumesStrategy == virtv1.UpdateVolumesStrategyMigration
	migrationInProgress := len(kvvmiInCluster.Status.MigratedVolumes) > 0

	if !migrationRequested && !migrationInProgress {
		return reconcile.Result{}, nil
	}

	if migrationRequested && !migrationInProgress {
		// We should wait 10 seconds. This delay allows user to change storage class on other volumes
		if len(storageClassChangedDisks) > 0 {
			delay, exists := s.delay[vm.UID]
			if !exists {
				s.delay[vm.UID] = time.Now().Add(s.delayDuration)
				return reconcile.Result{RequeueAfter: s.delayDuration}, nil
			}
			if time.Now().Before(delay) {
				return reconcile.Result{RequeueAfter: time.Until(delay)}, nil
			}
		}

		notReadyDisks, err := s.GetVirtualDiskNamesWithUnreadyTarget(ctx, vmState)
		if err != nil {
			return reconcile.Result{}, err
		}

		if len(notReadyDisks) > 0 {
			return reconcile.Result{}, nil
		}

		err = s.patchVolumes(ctx, builtKVVMWithMigrationVolumes)
		if err != nil {
			return reconcile.Result{}, err
		}

		// Clean up the delay after it's passed
		delete(s.delay, vm.UID)
		return reconcile.Result{}, nil
	}

	// migration in progress
	// if some volumes is different, we should revert all and sync again in next reconcile

	migratedPVCNames := make(map[string]struct{})

	for _, vd := range nonMigratableDisks {
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

	err = s.client.Patch(ctx, kvvm, client.RawPatch(types.JSONPatchType, patchBytes))
	return err
}

func (s MigrationVolumesService) VolumesSynced(ctx context.Context, vmState state.VirtualMachineState) (bool, error) {
	kvvmInCluster, _, builtKVVMWithMigrationVolumes, kvvmiInCluster, err := s.getMachines(ctx, vmState)
	if err != nil {
		return false, err
	}

	if kvvmInCluster == nil || kvvmiInCluster == nil {
		return false, fmt.Errorf("kvvm or kvvmi is nil")
	}

	migratable, _ := conditions.GetKVVMICondition(virtv1.VirtualMachineInstanceIsMigratable, kvvmiInCluster.Status.Conditions)
	if migratable.Status != corev1.ConditionTrue {
		return false, nil
	}

	kvvmSynced := equality.Semantic.DeepEqual(builtKVVMWithMigrationVolumes.Spec.Template.Spec.Volumes, kvvmInCluster.Spec.Template.Spec.Volumes)
	if !kvvmSynced {
		return false, nil
	}

	kvvmiSynced := equality.Semantic.DeepEqual(kvvmInCluster.Spec.Template.Spec.Volumes, kvvmiInCluster.Spec.Volumes)
	if !kvvmiSynced {
		return false, nil
	}

	_, nonMigratableDisks, storageClassChangedDisks, err := s.getDisks(ctx, vmState)
	if err != nil {
		return false, err
	}

	nonMigratableDisksSynced := s.isDisksSynced(builtKVVMWithMigrationVolumes, nonMigratableDisks)
	if !nonMigratableDisksSynced {
		return false, nil
	}

	storageClassChangedDisksSynced := s.isDisksSynced(builtKVVMWithMigrationVolumes, storageClassChangedDisks)
	if !storageClassChangedDisksSynced {
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

func (s MigrationVolumesService) getDisks(ctx context.Context, vmState state.VirtualMachineState) (map[string]*v1alpha2.VirtualDisk, map[string]*v1alpha2.VirtualDisk, map[string]*v1alpha2.VirtualDisk, error) {
	allDisks, err := vmState.VirtualDisksByName(ctx)
	if err != nil {
		return nil, nil, nil, err
	}
	nonMigratableDisks, err := s.getNonMigratableDisksByName(ctx, vmState)
	if err != nil {
		return nil, nil, nil, err
	}
	storageClassChangedDisks := s.getStorageClassChangedDisksByName(allDisks, nonMigratableDisks)

	return allDisks, nonMigratableDisks, storageClassChangedDisks, nil
}

func (s MigrationVolumesService) getNonMigratableDisksByName(ctx context.Context, vmState state.VirtualMachineState) (map[string]*v1alpha2.VirtualDisk, error) {
	nonMigratableDisks, err := vmState.NonMigratableVirtualDisks(ctx)
	if err != nil {
		return nil, err
	}

	nonMigratableDisksMap := make(map[string]*v1alpha2.VirtualDisk, len(nonMigratableDisks))
	for _, vd := range nonMigratableDisks {
		nonMigratableDisksMap[vd.Name] = vd
	}

	return nonMigratableDisksMap, nil
}

func (s MigrationVolumesService) getStorageClassChangedDisksByName(all, nonMigratable map[string]*v1alpha2.VirtualDisk) map[string]*v1alpha2.VirtualDisk {
	storageClassChangedDisks := make(map[string]*v1alpha2.VirtualDisk)

	for _, vd := range all {
		if _, ok := nonMigratable[vd.Name]; ok {
			continue
		}

		if commonvd.StorageClassChanged(vd) {
			storageClassChangedDisks[vd.Name] = vd
		}
	}

	return storageClassChangedDisks
}

func (s MigrationVolumesService) GetVirtualDiskNamesWithUnreadyTarget(ctx context.Context, vmState state.VirtualMachineState) ([]string, error) {
	_, nonMigratableDisks, storageClassChangedDisks, err := s.getDisks(ctx, vmState)
	if err != nil {
		return nil, err
	}

	readyNonMigratable, err := s.getReadyTargetPVCs(ctx, nonMigratableDisks)
	if err != nil {
		return nil, err
	}

	readyStorageClassChanged, err := s.getReadyTargetPVCs(ctx, storageClassChangedDisks)
	if err != nil {
		return nil, err
	}

	var notReadyDisks []string
	for _, vd := range nonMigratableDisks {
		if _, ok := readyNonMigratable[vd.Name]; !ok {
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

func (s MigrationVolumesService) isDisksSynced(kvvm *virtv1.VirtualMachine, disks map[string]*v1alpha2.VirtualDisk) bool {
	if len(disks) == 0 {
		return true
	}
	for _, v := range kvvm.Spec.Template.Spec.Volumes {
		if v.PersistentVolumeClaim != nil {
			exist := false
			for _, d := range disks {
				if d.Status.MigrationState.TargetPVC == v.PersistentVolumeClaim.ClaimName {
					exist = true
					break
				}
			}
			if !exist {
				return false
			}
		}
	}
	return true
}
