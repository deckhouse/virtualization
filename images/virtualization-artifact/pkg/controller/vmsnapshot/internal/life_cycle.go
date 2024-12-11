/*
Copyright 2024 Flant JSC

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
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdcondition"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdscondition"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmscondition"
)

type LifeCycleHandler struct {
	snapshotter Snapshotter
	storer      Storer
}

func NewLifeCycleHandler(snapshotter Snapshotter, storer Storer) *LifeCycleHandler {
	return &LifeCycleHandler{
		snapshotter: snapshotter,
		storer:      storer,
	}
}

func (h LifeCycleHandler) Handle(ctx context.Context, vmSnapshot *virtv2.VirtualMachineSnapshot) (reconcile.Result, error) {
	log := logger.FromContext(ctx).With(logger.SlogHandler("lifecycle"))

	cb := conditions.NewConditionBuilder(vmscondition.VirtualMachineSnapshotReadyType)
	defer func() { conditions.SetCondition(cb.Generation(vmSnapshot.Generation), &vmSnapshot.Status.Conditions) }()

	if !conditions.HasCondition(cb.GetType(), vmSnapshot.Status.Conditions) {
		cb.Status(metav1.ConditionUnknown).Reason(conditions.ReasonUnknown)
	}

	if vmSnapshot.DeletionTimestamp != nil {
		vmSnapshot.Status.Phase = virtv2.VirtualMachineSnapshotPhaseTerminating
		cb.
			Status(metav1.ConditionUnknown).
			Reason(conditions.ReasonUnknown).
			Message("")

		return reconcile.Result{}, nil
	}

	switch vmSnapshot.Status.Phase {
	case "":
		vmSnapshot.Status.Phase = virtv2.VirtualMachineSnapshotPhasePending
	case virtv2.VirtualMachineSnapshotPhaseFailed:
		vmSnapshotCondition, _ := conditions.GetCondition(vmscondition.VirtualMachineSnapshotReadyType, vmSnapshot.Status.Conditions)

		cb.
			Status(metav1.ConditionFalse).
			Reason(conditions.CommonReason(vmSnapshotCondition.Reason)).
			Message(vmSnapshotCondition.Message)

		return reconcile.Result{}, nil
	case virtv2.VirtualMachineSnapshotPhaseReady:
		// Ensure vd snapshots aren't lost.
		var lostVDSnapshots []string
		for _, vdSnapshotName := range vmSnapshot.Status.VirtualDiskSnapshotNames {
			vdSnapshot, err := h.snapshotter.GetVirtualDiskSnapshot(ctx, vdSnapshotName, vmSnapshot.Namespace)
			if err != nil {
				setPhaseConditionToFailed(cb, &vmSnapshot.Status.Phase, err)
				return reconcile.Result{}, err
			}

			switch {
			case vdSnapshot == nil:
				lostVDSnapshots = append(lostVDSnapshots, vdSnapshotName)
			case vdSnapshot.Status.Phase != virtv2.VirtualDiskSnapshotPhaseReady:
				log.Error("expected virtual disk snapshot to be ready, please report a bug", "vdSnapshotPhase", vdSnapshot.Status.Phase)
			}
		}

		if len(lostVDSnapshots) > 0 {
			vmSnapshot.Status.Phase = virtv2.VirtualMachineSnapshotPhaseFailed
			cb.Status(metav1.ConditionFalse).Reason(vmscondition.VirtualDiskSnapshotLost)
			if len(lostVDSnapshots) == 1 {
				cb.Message(fmt.Sprintf("The underlieng virtual disk snapshot (%s) is lost.", lostVDSnapshots[0]))
			} else {
				cb.Message(fmt.Sprintf("The underlieng virtual disk snapshots (%s) are lost.", strings.Join(lostVDSnapshots, ", ")))
			}
			return reconcile.Result{}, nil
		}

		vmSnapshot.Status.Phase = virtv2.VirtualMachineSnapshotPhaseReady
		cb.
			Status(metav1.ConditionTrue).
			Reason(vmscondition.VirtualMachineSnapshotReady).
			Message("")
		return reconcile.Result{}, nil
	}

	vm, err := h.snapshotter.GetVirtualMachine(ctx, vmSnapshot.Spec.VirtualMachineName, vmSnapshot.Namespace)
	if err != nil {
		setPhaseConditionToFailed(cb, &vmSnapshot.Status.Phase, err)
		return reconcile.Result{}, err
	}

	virtualMachineReadyCondition, _ := conditions.GetCondition(vmscondition.VirtualMachineReadyType, vmSnapshot.Status.Conditions)
	if vm == nil || virtualMachineReadyCondition.Status != metav1.ConditionTrue {
		vmSnapshot.Status.Phase = virtv2.VirtualMachineSnapshotPhasePending
		cb.
			Status(metav1.ConditionFalse).
			Reason(vmscondition.WaitingForTheVirtualMachine).
			Message(fmt.Sprintf("Waiting for the virtual machine %q to be ready for snapshotting.", vmSnapshot.Spec.VirtualMachineName))
		return reconcile.Result{}, nil
	}

	// 1. Ensure the block devices are Ready for snapshotting.
	err = h.ensureBlockDeviceConsistency(ctx, vm)
	switch {
	case err == nil:
	case errors.Is(err, ErrBlockDevicesNotReady), errors.Is(err, ErrVirtualDiskNotReady), errors.Is(err, ErrVirtualDiskResizing):
		vmSnapshot.Status.Phase = virtv2.VirtualMachineSnapshotPhasePending
		cb.
			Status(metav1.ConditionFalse).
			Reason(vmscondition.BlockDevicesNotReady).
			Message(service.CapitalizeFirstLetter(err.Error() + "."))
		return reconcile.Result{}, nil
	default:
		setPhaseConditionToFailed(cb, &vmSnapshot.Status.Phase, err)
		return reconcile.Result{}, err
	}

	// 2. Ensure there are no RestartAwaitingChanges.
	if len(vm.Status.RestartAwaitingChanges) > 0 {
		vmSnapshot.Status.Phase = virtv2.VirtualMachineSnapshotPhasePending
		cb.
			Status(metav1.ConditionFalse).
			Reason(vmscondition.RestartAwaitingChanges).Message(
			fmt.Sprintf(
				"Waiting for the restart and approval of changes to virtual machine %q before taking the snapshot.",
				vm.Name,
			))
		return reconcile.Result{}, nil
	}

	// 3. Ensure the virtual machine is consistent for snapshotting.
	hasFrozen, err := h.freezeVirtualMachineIfCan(ctx, vm)
	switch {
	case err == nil:
	case errors.Is(err, ErrPotentiallyInconsistent):
		if vmSnapshot.Spec.RequiredConsistency {
			vmSnapshot.Status.Phase = virtv2.VirtualMachineSnapshotPhasePending
			cb.
				Status(metav1.ConditionFalse).
				Reason(vmscondition.PotentiallyInconsistent).
				Message(fmt.Sprintf(
					"The snapshotting of virtual machine %q might result in an inconsistent snapshot: "+
						"waiting for the virtual machine to be %s",
					vm.Name, virtv2.MachineStopped,
				))
			return reconcile.Result{}, nil
		}
	default:
		setPhaseConditionToFailed(cb, &vmSnapshot.Status.Phase, err)
		return reconcile.Result{}, err
	}

	if hasFrozen {
		vmSnapshot.Status.Phase = virtv2.VirtualMachineSnapshotPhaseInProgress
		cb.
			Status(metav1.ConditionFalse).
			Reason(vmscondition.FileSystemFreezing).
			Message(fmt.Sprintf("The virtual machine %q is in the process of being frozen for taking a snapshot.", vm.Name))
		return reconcile.Result{}, nil
	}

	// 4. Create secret.
	err = h.ensureSecret(ctx, vm, vmSnapshot)
	if err != nil {
		setPhaseConditionToFailed(cb, &vmSnapshot.Status.Phase, err)
		return reconcile.Result{}, err
	}

	// 5. Fill status.VirtualDiskSnapshotNames.
	h.fillStatusVirtualDiskSnapshotNames(vmSnapshot, vm)

	// 6. Get or Create VirtualDiskSnapshots.
	vdSnapshots, err := h.ensureVirtualDiskSnapshots(ctx, vmSnapshot)
	switch {
	case err == nil:
	case errors.Is(err, ErrVolumeSnapshotClassNotFound), errors.Is(err, ErrCannotTakeSnapshot):
		vmSnapshot.Status.Phase = virtv2.VirtualMachineSnapshotPhaseFailed
		msg := service.CapitalizeFirstLetter(err.Error())
		if !strings.HasSuffix(msg, ".") {
			msg += "."
		}
		cb.
			Status(metav1.ConditionFalse).
			Reason(vmscondition.VirtualMachineSnapshotFailed).
			Message(msg)
		return reconcile.Result{}, nil
	default:
		setPhaseConditionToFailed(cb, &vmSnapshot.Status.Phase, err)
		return reconcile.Result{}, err
	}

	// 7. Wait for VirtualDiskSnapshots to be Ready.
	readyCount := h.countReadyVirtualDiskSnapshots(vdSnapshots)

	if readyCount != len(vdSnapshots) {
		log.Debug("Waiting for the virtual disk snapshots to be taken for the block devices of the virtual machine")

		vmSnapshot.Status.Phase = virtv2.VirtualMachineSnapshotPhaseInProgress
		cb.
			Status(metav1.ConditionFalse).
			Reason(vmscondition.Snapshotting).
			Message(fmt.Sprintf(
				"Waiting for the virtual disk snapshots to be taken for "+
					"the block devices of the virtual machine %q (%d/%d).",
				vm.Name, readyCount, len(vdSnapshots),
			))
		return reconcile.Result{}, nil
	}

	if vm.Status.Phase == virtv2.MachineStopped || h.snapshotter.IsFrozen(vm) {
		vmSnapshot.Status.Consistent = ptr.To(true)
	}

	// 8. Unfreeze VirtualMachine if can.
	unfrozen, err := h.unfreezeVirtualMachineIfCan(ctx, vm)
	if err != nil {
		setPhaseConditionToFailed(cb, &vmSnapshot.Status.Phase, err)
		return reconcile.Result{}, err
	}

	// 9. Move to Ready phase.
	log.Debug("The virtual disk snapshots are taken: the virtual machine snapshot is Ready now", "unfrozen", unfrozen)

	vmSnapshot.Status.Phase = virtv2.VirtualMachineSnapshotPhaseReady
	cb.
		Status(metav1.ConditionTrue).
		Reason(vmscondition.VirtualMachineReady).
		Message("")

	return reconcile.Result{}, nil
}

func setPhaseConditionToFailed(cb *conditions.ConditionBuilder, phase *virtv2.VirtualMachineSnapshotPhase, err error) {
	*phase = virtv2.VirtualMachineSnapshotPhaseFailed
	cb.
		Status(metav1.ConditionFalse).
		Reason(vmscondition.VirtualMachineSnapshotFailed).
		Message(service.CapitalizeFirstLetter(err.Error()) + ".")
}

func (h LifeCycleHandler) fillStatusVirtualDiskSnapshotNames(vmSnapshot *virtv2.VirtualMachineSnapshot, vm *virtv2.VirtualMachine) {
	vmSnapshot.Status.VirtualDiskSnapshotNames = nil

	for _, bdr := range vm.Status.BlockDeviceRefs {
		if bdr.Kind != virtv2.DiskDevice {
			continue
		}

		vmSnapshot.Status.VirtualDiskSnapshotNames = append(
			vmSnapshot.Status.VirtualDiskSnapshotNames,
			getVDSnapshotName(bdr.Name, vmSnapshot),
		)
	}
}

var ErrCannotTakeSnapshot = errors.New("cannot take snapshot")

func (h LifeCycleHandler) ensureVirtualDiskSnapshots(ctx context.Context, vmSnapshot *virtv2.VirtualMachineSnapshot) ([]*virtv2.VirtualDiskSnapshot, error) {
	vdSnapshots := make([]*virtv2.VirtualDiskSnapshot, 0, len(vmSnapshot.Status.VirtualDiskSnapshotNames))

	for _, vdSnapshotName := range vmSnapshot.Status.VirtualDiskSnapshotNames {
		vdSnapshot, err := h.snapshotter.GetVirtualDiskSnapshot(ctx, vdSnapshotName, vmSnapshot.Namespace)
		if err != nil {
			return nil, err
		}

		if vdSnapshot == nil {
			vdName, ok := getVDName(vdSnapshotName, vmSnapshot)
			if !ok {
				return nil, fmt.Errorf("failed to get VirtualDisk's name from VirtualDiskSnapshot's name %q", vdSnapshotName)
			}

			var vd *virtv2.VirtualDisk
			vd, err = h.snapshotter.GetVirtualDisk(ctx, vdName, vmSnapshot.Namespace)
			if err != nil {
				return nil, err
			}

			if vd == nil {
				return nil, fmt.Errorf("the virtual disk %q not found", vdName)
			}

			var pvc *corev1.PersistentVolumeClaim
			pvc, err = h.snapshotter.GetPersistentVolumeClaim(ctx, vd.Status.Target.PersistentVolumeClaim, vd.Namespace)
			if err != nil {
				return nil, err
			}

			if pvc == nil {
				return nil, fmt.Errorf("the persistent volume claim %q not found for the virtual disk %q", vd.Status.Target.PersistentVolumeClaim, vd.Name)
			}

			if pvc.Spec.StorageClassName == nil || *pvc.Spec.StorageClassName == "" {
				return nil, fmt.Errorf("the persistent volume claim %q doesn't have the storage class name", pvc.Name)
			}

			var vsClass string
			vsClass, err = h.getVolumeSnapshotClassByStorageClass(*pvc.Spec.StorageClassName, vmSnapshot.Spec.VolumeSnapshotClasses)
			if err != nil {
				return nil, err
			}

			vdSnapshot = &virtv2.VirtualDiskSnapshot{
				TypeMeta: metav1.TypeMeta{
					Kind:       virtv2.VirtualDiskSnapshotKind,
					APIVersion: virtv2.Version,
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      vdSnapshotName,
					Namespace: vmSnapshot.Namespace,
					OwnerReferences: []metav1.OwnerReference{
						service.MakeOwnerReference(vmSnapshot),
					},
				},
				Spec: virtv2.VirtualDiskSnapshotSpec{
					VirtualDiskName:         vdName,
					VolumeSnapshotClassName: vsClass,
					RequiredConsistency:     vmSnapshot.Spec.RequiredConsistency,
				},
			}

			vdSnapshot, err = h.snapshotter.CreateVirtualDiskSnapshot(ctx, vdSnapshot)
			if err != nil {
				return nil, err
			}
		}

		vdSnapshotReady, _ := conditions.GetCondition(vdscondition.VirtualDiskSnapshotReadyType, vdSnapshot.Status.Conditions)
		if vdSnapshotReady.Reason == vdscondition.VirtualDiskSnapshotFailed.String() || vdSnapshot.Status.Phase == virtv2.VirtualDiskSnapshotPhaseFailed {
			return nil, fmt.Errorf("the virtual disk snapshot %q is failed: %w. %s", vdSnapshot.Name, ErrCannotTakeSnapshot, vdSnapshotReady.Message)
		}

		vdSnapshots = append(vdSnapshots, vdSnapshot)
	}

	return vdSnapshots, nil
}

func (h LifeCycleHandler) countReadyVirtualDiskSnapshots(vdSnapshots []*virtv2.VirtualDiskSnapshot) int {
	var readyCount int
	for _, vdSnapshot := range vdSnapshots {
		if vdSnapshot.Status.Phase == virtv2.VirtualDiskSnapshotPhaseReady {
			readyCount++
		}
	}

	return readyCount
}

var ErrPotentiallyInconsistent = errors.New("potentially inconsistent")

func (h LifeCycleHandler) freezeVirtualMachineIfCan(ctx context.Context, vm *virtv2.VirtualMachine) (bool, error) {
	switch vm.Status.Phase {
	case virtv2.MachineStopped:
		return false, nil
	case virtv2.MachineRunning:
	default:
		return false, errors.New("cannot freeze not Running virtual machine")
	}

	if h.snapshotter.IsFrozen(vm) {
		return false, nil
	}

	if !h.snapshotter.CanFreeze(vm) {
		return false, ErrPotentiallyInconsistent
	}

	err := h.snapshotter.Freeze(ctx, vm.Name, vm.Namespace)
	if err != nil {
		return false, err
	}

	return true, nil
}

func (h LifeCycleHandler) unfreezeVirtualMachineIfCan(ctx context.Context, vm *virtv2.VirtualMachine) (bool, error) {
	if vm.Status.Phase != virtv2.MachineRunning || !h.snapshotter.IsFrozen(vm) {
		return false, nil
	}

	canUnfreeze, err := h.snapshotter.CanUnfreeze(ctx, "", vm)
	if err != nil {
		return false, err
	}

	if !canUnfreeze {
		return false, nil
	}

	err = h.snapshotter.Unfreeze(ctx, vm.Name, vm.Namespace)
	if err != nil {
		return false, err
	}

	return true, nil
}

var (
	ErrBlockDevicesNotReady = errors.New("block devices not ready")
	ErrVirtualDiskNotReady  = errors.New("virtual disk not ready")
	ErrVirtualDiskResizing  = errors.New("virtual disk is in the process of resizing")
)

func (h LifeCycleHandler) ensureBlockDeviceConsistency(ctx context.Context, vm *virtv2.VirtualMachine) error {
	bdReady, _ := conditions.GetCondition(vmcondition.TypeBlockDevicesReady, vm.Status.Conditions)
	if bdReady.Status != metav1.ConditionTrue {
		return fmt.Errorf("%w: waiting for the block devices of the virtual machine %q to be ready", ErrBlockDevicesNotReady, vm.Name)
	}

	for _, bdr := range vm.Status.BlockDeviceRefs {
		if bdr.Kind != virtv2.DiskDevice {
			continue
		}

		vd, err := h.snapshotter.GetVirtualDisk(ctx, bdr.Name, vm.Namespace)
		if err != nil {
			return err
		}

		if vd.Status.Phase != virtv2.DiskReady {
			return fmt.Errorf("%w: waiting for the virtual disk %q to be %s", ErrVirtualDiskNotReady, vd.Name, virtv2.DiskReady)
		}

		ready, _ := conditions.GetCondition(vdcondition.ReadyType, vd.Status.Conditions)
		if ready.Status != metav1.ConditionTrue {
			return fmt.Errorf("%w: waiting for the Ready condition of the virtual disk %q to be True", ErrVirtualDiskResizing, vd.Name)
		}

		resizingReady, _ := conditions.GetCondition(vdcondition.ResizingType, vd.Status.Conditions)
		if resizingReady.Reason == vdcondition.InProgress.String() {
			return fmt.Errorf("%w: waiting for the virtual disk %q to be resized", ErrVirtualDiskResizing, vd.Name)
		}
	}

	return nil
}

func (h LifeCycleHandler) ensureSecret(ctx context.Context, vm *virtv2.VirtualMachine, vmSnapshot *virtv2.VirtualMachineSnapshot) error {
	var secret *corev1.Secret
	var err error

	if vmSnapshot.Status.VirtualMachineSnapshotSecretName != "" {
		secret, err = h.snapshotter.GetSecret(ctx, vmSnapshot.Status.VirtualMachineSnapshotSecretName, vmSnapshot.Namespace)
		if err != nil {
			return err
		}
	}

	if secret == nil {
		secret, err = h.storer.Store(ctx, vm, vmSnapshot)
		if err != nil {
			return err
		}
	}

	if secret != nil {
		vmSnapshot.Status.VirtualMachineSnapshotSecretName = secret.Name
	}

	return nil
}

var ErrVolumeSnapshotClassNotFound = errors.New("the volume snapshot class not found")

func (h LifeCycleHandler) getVolumeSnapshotClassByStorageClass(storageClassName string, classes []virtv2.VolumeSnapshotClassName) (string, error) {
	for _, class := range classes {
		if class.StorageClassName == storageClassName {
			return class.VolumeSnapshotClassName, nil
		}
	}

	return "", fmt.Errorf("%w: please define the volume snapshot class for the storage class %q and recreate resource", ErrVolumeSnapshotClassNotFound, storageClassName)
}

func getVDName(vdSnapshotName string, vmSnapshot *virtv2.VirtualMachineSnapshot) (string, bool) {
	return strings.CutSuffix(vdSnapshotName, "-"+string(vmSnapshot.UID))
}

func getVDSnapshotName(vdName string, vmSnapshot *virtv2.VirtualMachineSnapshot) string {
	return fmt.Sprintf("%s-%s", vdName, vmSnapshot.UID)
}
