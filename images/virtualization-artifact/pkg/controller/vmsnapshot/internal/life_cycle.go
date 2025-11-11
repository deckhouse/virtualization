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
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdcondition"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdscondition"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmscondition"
)

type LifeCycleHandler struct {
	recorder    eventrecord.EventRecorderLogger
	snapshotter Snapshotter
	storer      Storer
	client      client.Client
}

func NewLifeCycleHandler(recorder eventrecord.EventRecorderLogger, snapshotter Snapshotter, storer Storer, client client.Client) *LifeCycleHandler {
	return &LifeCycleHandler{
		recorder:    recorder,
		snapshotter: snapshotter,
		storer:      storer,
		client:      client,
	}
}

func (h LifeCycleHandler) Handle(ctx context.Context, vmSnapshot *v1alpha2.VirtualMachineSnapshot) (reconcile.Result, error) {
	log := logger.FromContext(ctx).With(logger.SlogHandler("lifecycle"))

	cb := conditions.NewConditionBuilder(vmscondition.VirtualMachineSnapshotReadyType)
	defer func() { conditions.SetCondition(cb.Generation(vmSnapshot.Generation), &vmSnapshot.Status.Conditions) }()

	if !conditions.HasCondition(cb.GetType(), vmSnapshot.Status.Conditions) {
		cb.Status(metav1.ConditionUnknown).Reason(conditions.ReasonUnknown)
	}

	vm, err := h.snapshotter.GetVirtualMachine(ctx, vmSnapshot.Spec.VirtualMachineName, vmSnapshot.Namespace)
	if err != nil {
		h.setPhaseConditionToFailed(cb, vmSnapshot, err)
		return reconcile.Result{}, err
	}

	kvvmi, err := h.snapshotter.GetKubeVirtVirtualMachineInstance(ctx, vm)
	if err != nil {
		h.setPhaseConditionToFailed(cb, vmSnapshot, err)
		return reconcile.Result{}, err
	}

	if vmSnapshot.DeletionTimestamp != nil {
		vmSnapshot.Status.Phase = v1alpha2.VirtualMachineSnapshotPhaseTerminating
		cb.
			Status(metav1.ConditionUnknown).
			Reason(conditions.ReasonUnknown).
			Message("")

		_, err = h.unfreezeVirtualMachineIfCan(ctx, vmSnapshot, vm, kvvmi)
		if err != nil {
			if errors.Is(err, service.ErrUntrustedFilesystemFrozenCondition) {
				return reconcile.Result{}, nil
			}
			h.setPhaseConditionToFailed(cb, vmSnapshot, err)
			return reconcile.Result{}, err
		}

		return reconcile.Result{}, nil
	}

	switch vmSnapshot.Status.Phase {
	case "":
		vmSnapshot.Status.Phase = v1alpha2.VirtualMachineSnapshotPhasePending
	case v1alpha2.VirtualMachineSnapshotPhaseFailed:
		readyCondition, _ := conditions.GetCondition(vmscondition.VirtualMachineSnapshotReadyType, vmSnapshot.Status.Conditions)
		cb.
			Status(readyCondition.Status).
			Reason(conditions.CommonReason(readyCondition.Reason)).
			Message(readyCondition.Message)
		return reconcile.Result{}, nil
	case v1alpha2.VirtualMachineSnapshotPhaseReady:
		// Ensure vd snapshots aren't lost.
		var lostVDSnapshots []string
		for _, vdSnapshotName := range vmSnapshot.Status.VirtualDiskSnapshotNames {
			vdSnapshot, err := h.snapshotter.GetVirtualDiskSnapshot(ctx, vdSnapshotName, vmSnapshot.Namespace)
			if err != nil {
				h.setPhaseConditionToFailed(cb, vmSnapshot, err)
				return reconcile.Result{}, err
			}

			switch {
			case vdSnapshot == nil:
				lostVDSnapshots = append(lostVDSnapshots, vdSnapshotName)
			case vdSnapshot.Status.Phase != v1alpha2.VirtualDiskSnapshotPhaseReady:
				log.Error("expected virtual disk snapshot to be ready, please report a bug", "vdSnapshotPhase", vdSnapshot.Status.Phase)
			}
		}

		if len(lostVDSnapshots) > 0 {
			vmSnapshot.Status.Phase = v1alpha2.VirtualMachineSnapshotPhaseFailed
			cb.Status(metav1.ConditionFalse).Reason(vmscondition.VirtualDiskSnapshotLost)
			if len(lostVDSnapshots) == 1 {
				msg := fmt.Sprintf("The underlying virtual disk snapshot (%s) is lost.", lostVDSnapshots[0])
				h.recorder.Event(
					vmSnapshot,
					corev1.EventTypeWarning,
					v1alpha2.ReasonVMSnapshottingFailed,
					msg,
				)
				cb.Message(msg)
			} else {
				msg := fmt.Sprintf("The underlying virtual disk snapshots (%s) are lost.", strings.Join(lostVDSnapshots, ", "))
				h.recorder.Event(
					vmSnapshot,
					corev1.EventTypeWarning,
					v1alpha2.ReasonVMSnapshottingFailed,
					msg,
				)
				cb.Message(msg)
			}
			return reconcile.Result{}, nil
		}

		vmSnapshot.Status.Phase = v1alpha2.VirtualMachineSnapshotPhaseReady
		cb.
			Status(metav1.ConditionTrue).
			Reason(vmscondition.VirtualMachineSnapshotReady).
			Message("")
		return reconcile.Result{}, nil
	}

	virtualMachineReadyCondition, _ := conditions.GetCondition(vmscondition.VirtualMachineReadyType, vmSnapshot.Status.Conditions)
	if vm == nil || virtualMachineReadyCondition.Status != metav1.ConditionTrue {
		vmSnapshot.Status.Phase = v1alpha2.VirtualMachineSnapshotPhasePending
		msg := fmt.Sprintf("Waiting for the virtual machine %q to be ready for snapshotting.", vmSnapshot.Spec.VirtualMachineName)
		h.recorder.Event(
			vmSnapshot,
			corev1.EventTypeNormal,
			v1alpha2.ReasonVMSnapshottingPending,
			msg,
		)
		cb.
			Status(metav1.ConditionFalse).
			Reason(vmscondition.WaitingForTheVirtualMachine).
			Message(msg)
		return reconcile.Result{}, nil
	}

	// 1. Ensure the block devices are Ready for snapshotting.
	err = h.ensureBlockDeviceConsistency(ctx, vm)
	switch {
	case err == nil:
	case errors.Is(err, ErrBlockDevicesNotReady), errors.Is(err, ErrVirtualDiskNotReady), errors.Is(err, ErrVirtualDiskResizing):
		vmSnapshot.Status.Phase = v1alpha2.VirtualMachineSnapshotPhaseFailed
		msg := service.CapitalizeFirstLetter(err.Error() + ".")
		h.recorder.Event(
			vmSnapshot,
			corev1.EventTypeNormal,
			v1alpha2.ReasonVMSnapshottingFailed,
			msg,
		)
		cb.
			Status(metav1.ConditionFalse).
			Reason(vmscondition.BlockDevicesNotReady).
			Message(msg)
		return reconcile.Result{}, nil
	default:
		h.setPhaseConditionToFailed(cb, vmSnapshot, err)
		return reconcile.Result{}, err
	}

	needToFreeze, err := h.needToFreeze(ctx, vm, kvvmi, vmSnapshot.Spec.RequiredConsistency)
	if err != nil {
		if errors.Is(err, service.ErrUntrustedFilesystemFrozenCondition) {
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}

	canFreeze, err := h.snapshotter.CanFreeze(ctx, kvvmi)
	if err != nil {
		if errors.Is(err, service.ErrUntrustedFilesystemFrozenCondition) {
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}
	isAwaitingConsistency := needToFreeze && !canFreeze && vmSnapshot.Spec.RequiredConsistency
	if isAwaitingConsistency {
		vmSnapshot.Status.Phase = v1alpha2.VirtualMachineSnapshotPhasePending
		msg := fmt.Sprintf(
			"The snapshotting of virtual machine %q might result in an inconsistent snapshot: "+
				"waiting for the virtual machine to be %s",
			vm.Name, v1alpha2.MachineStopped,
		)

		agentReadyCondition, _ := conditions.GetCondition(vmcondition.TypeAgentReady, vm.Status.Conditions)
		if agentReadyCondition.Status != metav1.ConditionTrue {
			msg = fmt.Sprintf(
				"The snapshotting of virtual machine %q might result in an inconsistent snapshot: "+
					"virtual machine agent is not ready and virtual machine cannot be frozen: "+
					"waiting for virtual machine agent to be ready or virtual machine will stop",
				vm.Name,
			)
		}

		h.recorder.Event(
			vmSnapshot,
			corev1.EventTypeNormal,
			v1alpha2.ReasonVMSnapshottingPending,
			msg,
		)
		cb.
			Status(metav1.ConditionFalse).
			Reason(vmscondition.PotentiallyInconsistent).
			Message(msg)
		return reconcile.Result{}, nil
	}

	if vmSnapshot.Status.Phase == v1alpha2.VirtualMachineSnapshotPhasePending {
		vmSnapshot.Status.Phase = v1alpha2.VirtualMachineSnapshotPhaseInProgress
		h.recorder.Event(
			vmSnapshot,
			corev1.EventTypeNormal,
			v1alpha2.ReasonVMSnapshottingStarted,
			"Virtual machine snapshotting process is started.",
		)
		cb.
			Status(metav1.ConditionFalse).
			Reason(vmscondition.FileSystemFreezing).
			Message("The snapshotting process has started.")
		return reconcile.Result{Requeue: true}, nil
	}

	var hasFrozen bool

	// 2. Ensure the virtual machine is consistent for snapshotting.
	if needToFreeze {
		hasFrozen, err = h.freezeVirtualMachine(ctx, kvvmi, vmSnapshot)
		if err != nil {
			h.setPhaseConditionToFailed(cb, vmSnapshot, err)
			return reconcile.Result{}, err
		}
	}

	if hasFrozen {
		vmSnapshot.Status.Phase = v1alpha2.VirtualMachineSnapshotPhaseInProgress
		cb.
			Status(metav1.ConditionFalse).
			Reason(vmscondition.FileSystemFreezing).
			Message(fmt.Sprintf("The virtual machine %q is in the process of being frozen for taking a snapshot.", vm.Name))
		return reconcile.Result{}, nil
	}

	// 3. Create secret.
	err = h.ensureSecret(ctx, vm, vmSnapshot)
	if err != nil {
		h.setPhaseConditionToFailed(cb, vmSnapshot, err)
		return reconcile.Result{}, err
	}

	// 4. Fill status.VirtualDiskSnapshotNames.
	h.fillStatusVirtualDiskSnapshotNames(vmSnapshot, vm)

	// 5. Get or Create VirtualDiskSnapshots.
	vdSnapshots, err := h.ensureVirtualDiskSnapshots(ctx, vmSnapshot)
	switch {
	case err == nil:
	case errors.Is(err, ErrCannotTakeSnapshot):
		vmSnapshot.Status.Phase = v1alpha2.VirtualMachineSnapshotPhaseFailed
		msg := service.CapitalizeFirstLetter(err.Error())
		h.recorder.Event(
			vmSnapshot,
			corev1.EventTypeWarning,
			v1alpha2.ReasonVMSnapshottingFailed,
			msg,
		)
		if !strings.HasSuffix(msg, ".") {
			msg += "."
		}
		cb.
			Status(metav1.ConditionFalse).
			Reason(vmscondition.VirtualMachineSnapshotFailed).
			Message(msg)
		return reconcile.Result{}, nil
	default:
		h.setPhaseConditionToFailed(cb, vmSnapshot, err)
		return reconcile.Result{}, err
	}

	// 6. Wait for VirtualDiskSnapshots to be Ready.
	readyCount := h.countReadyVirtualDiskSnapshots(vdSnapshots)
	msg := fmt.Sprintf(
		"Waiting for the virtual disk snapshots to be taken for "+
			"the block devices of the virtual machine %q (%d/%d).",
		vm.Name, readyCount, len(vdSnapshots),
	)

	if readyCount != len(vdSnapshots) {
		log.Debug("Waiting for the virtual disk snapshots to be taken for the block devices of the virtual machine")

		vmSnapshot.Status.Phase = v1alpha2.VirtualMachineSnapshotPhaseInProgress
		h.recorder.Event(
			vmSnapshot,
			corev1.EventTypeNormal,
			v1alpha2.ReasonVMSnapshottingInProgress,
			msg,
		)
		cb.
			Status(metav1.ConditionFalse).
			Reason(vmscondition.Snapshotting).
			Message(msg)
		return reconcile.Result{}, nil
	} else {
		h.recorder.Event(
			vmSnapshot,
			corev1.EventTypeNormal,
			v1alpha2.ReasonVMSnapshottingInProgress,
			msg,
		)
	}

	vmSnapshot.Status.Consistent = ptr.To(true)
	if !h.areVirtualDiskSnapshotsConsistent(vdSnapshots) {
		vmSnapshot.Status.Consistent = nil
	}

	// 7. Unfreeze VirtualMachine if can.
	unfrozen, err := h.unfreezeVirtualMachineIfCan(ctx, vmSnapshot, vm, kvvmi)
	if err != nil {
		if errors.Is(err, service.ErrUntrustedFilesystemFrozenCondition) {
			return reconcile.Result{}, nil
		}
		h.setPhaseConditionToFailed(cb, vmSnapshot, err)
		return reconcile.Result{}, err
	}

	// 8. Fill status resources.
	err = h.fillStatusResources(ctx, vmSnapshot, vm)
	if err != nil {
		h.setPhaseConditionToFailed(cb, vmSnapshot, err)
		return reconcile.Result{}, err
	}

	// 9. Move to Ready phase.
	log.Debug("The virtual disk snapshots are taken: the virtual machine snapshot is Ready now", "unfrozen", unfrozen)

	vmSnapshot.Status.Phase = v1alpha2.VirtualMachineSnapshotPhaseReady
	h.recorder.Event(
		vmSnapshot,
		corev1.EventTypeNormal,
		v1alpha2.ReasonVMSnapshottingCompleted,
		"Virtual machine snapshotting process is completed.",
	)
	cb.
		Status(metav1.ConditionTrue).
		Reason(vmscondition.VirtualMachineReady).
		Message("")

	return reconcile.Result{}, nil
}

func (h LifeCycleHandler) setPhaseConditionToFailed(cb *conditions.ConditionBuilder, vmSnapshot *v1alpha2.VirtualMachineSnapshot, err error) {
	vmSnapshot.Status.Phase = v1alpha2.VirtualMachineSnapshotPhaseFailed
	h.recorder.Event(
		vmSnapshot,
		corev1.EventTypeWarning,
		v1alpha2.ReasonVMSnapshottingFailed,
		err.Error()+".",
	)
	cb.
		Status(metav1.ConditionFalse).
		Reason(vmscondition.VirtualMachineSnapshotFailed).
		Message(service.CapitalizeFirstLetter(err.Error()) + ".")
}

func (h LifeCycleHandler) fillStatusVirtualDiskSnapshotNames(vmSnapshot *v1alpha2.VirtualMachineSnapshot, vm *v1alpha2.VirtualMachine) {
	vmSnapshot.Status.VirtualDiskSnapshotNames = nil

	for _, bdr := range vm.Status.BlockDeviceRefs {
		if bdr.Kind != v1alpha2.DiskDevice {
			continue
		}

		vmSnapshot.Status.VirtualDiskSnapshotNames = append(
			vmSnapshot.Status.VirtualDiskSnapshotNames,
			getVDSnapshotName(bdr.Name, vmSnapshot),
		)
	}
}

var ErrCannotTakeSnapshot = errors.New("cannot take snapshot")

func (h LifeCycleHandler) ensureVirtualDiskSnapshots(ctx context.Context, vmSnapshot *v1alpha2.VirtualMachineSnapshot) ([]*v1alpha2.VirtualDiskSnapshot, error) {
	vdSnapshots := make([]*v1alpha2.VirtualDiskSnapshot, 0, len(vmSnapshot.Status.VirtualDiskSnapshotNames))

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

			var vd *v1alpha2.VirtualDisk
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

			vdSnapshot = &v1alpha2.VirtualDiskSnapshot{
				TypeMeta: metav1.TypeMeta{
					Kind:       v1alpha2.VirtualDiskSnapshotKind,
					APIVersion: v1alpha2.Version,
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      vdSnapshotName,
					Namespace: vmSnapshot.Namespace,
					OwnerReferences: []metav1.OwnerReference{
						service.MakeOwnerReference(vmSnapshot),
					},
				},
				Spec: v1alpha2.VirtualDiskSnapshotSpec{
					VirtualDiskName:     vdName,
					RequiredConsistency: vmSnapshot.Spec.RequiredConsistency,
				},
			}

			vdSnapshot, err = h.snapshotter.CreateVirtualDiskSnapshot(ctx, vdSnapshot)
			if err != nil {
				return nil, err
			}
		}

		vdSnapshotReady, _ := conditions.GetCondition(vdscondition.VirtualDiskSnapshotReadyType, vdSnapshot.Status.Conditions)
		if vdSnapshotReady.Reason == vdscondition.VirtualDiskSnapshotFailed.String() || vdSnapshot.Status.Phase == v1alpha2.VirtualDiskSnapshotPhaseFailed {
			return nil, fmt.Errorf("the virtual disk snapshot %q is failed: %w. %s", vdSnapshot.Name, ErrCannotTakeSnapshot, vdSnapshotReady.Message)
		}

		vdSnapshots = append(vdSnapshots, vdSnapshot)
	}

	return vdSnapshots, nil
}

func (h LifeCycleHandler) countReadyVirtualDiskSnapshots(vdSnapshots []*v1alpha2.VirtualDiskSnapshot) int {
	var readyCount int
	for _, vdSnapshot := range vdSnapshots {
		if vdSnapshot.Status.Phase == v1alpha2.VirtualDiskSnapshotPhaseReady {
			readyCount++
		}
	}

	return readyCount
}

func (h LifeCycleHandler) areVirtualDiskSnapshotsConsistent(vdSnapshots []*v1alpha2.VirtualDiskSnapshot) bool {
	for _, vdSnapshot := range vdSnapshots {
		if vdSnapshot.Status.Consistent == nil || !*vdSnapshot.Status.Consistent {
			return false
		}
	}

	return true
}

func (h LifeCycleHandler) needToFreeze(ctx context.Context, vm *v1alpha2.VirtualMachine, kvvmi *virtv1.VirtualMachineInstance, requiredConsistency bool) (bool, error) {
	if !requiredConsistency {
		return false, nil
	}

	if vm.Status.Phase == v1alpha2.MachineStopped {
		return false, nil
	}

	isFrozen, err := h.snapshotter.IsFrozen(ctx, kvvmi)
	if err != nil {
		return false, err
	}
	if isFrozen {
		return false, nil
	}

	return true, nil
}

func (h LifeCycleHandler) freezeVirtualMachine(ctx context.Context, kvvmi *virtv1.VirtualMachineInstance, vmSnapshot *v1alpha2.VirtualMachineSnapshot) (bool, error) {
	if kvvmi.Status.Phase != virtv1.Running {
		return false, fmt.Errorf("cannot freeze not Running %s/%s virtual machine", kvvmi.Namespace, kvvmi.Name)
	}

	err := h.snapshotter.Freeze(ctx, kvvmi)
	if err != nil {
		return false, fmt.Errorf("freeze the virtual machine %s/%s: %w", kvvmi.Namespace, kvvmi.Name, err)
	}

	h.recorder.Event(
		vmSnapshot,
		corev1.EventTypeNormal,
		v1alpha2.ReasonVMSnapshottingFrozen,
		fmt.Sprintf("The file system of the virtual machine %q is frozen.", kvvmi.Name),
	)

	return true, nil
}

func (h LifeCycleHandler) unfreezeVirtualMachineIfCan(ctx context.Context, vmSnapshot *v1alpha2.VirtualMachineSnapshot, vm *v1alpha2.VirtualMachine, kvvmi *virtv1.VirtualMachineInstance) (bool, error) {
	if vm == nil || vm.Status.Phase != v1alpha2.MachineRunning {
		return false, nil
	}

	isFrozen, err := h.snapshotter.IsFrozen(ctx, kvvmi)
	if err != nil {
		return false, err
	}
	if !isFrozen {
		return false, nil
	}

	canUnfreeze, err := h.snapshotter.CanUnfreeze(ctx, vmSnapshot.Name, vm, kvvmi)
	if err != nil {
		return false, err
	}

	if !canUnfreeze {
		return false, nil
	}

	err = h.snapshotter.Unfreeze(ctx, kvvmi)
	if err != nil {
		return false, fmt.Errorf("unfreeze the virtual machine %q: %w", vm.Name, err)
	}

	h.recorder.Event(
		vmSnapshot,
		corev1.EventTypeNormal,
		v1alpha2.ReasonVMSnapshottingThawed,
		fmt.Sprintf("The file system of the virtual machine %q is thawed.", vm.Name),
	)

	return true, nil
}

var (
	ErrBlockDevicesNotReady = errors.New("block devices not ready")
	ErrVirtualDiskNotReady  = errors.New("virtual disk not ready")
	ErrVirtualDiskResizing  = errors.New("virtual disk is in the process of resizing")
)

func (h LifeCycleHandler) ensureBlockDeviceConsistency(ctx context.Context, vm *v1alpha2.VirtualMachine) error {
	bdReady, _ := conditions.GetCondition(vmcondition.TypeBlockDevicesReady, vm.Status.Conditions)
	if bdReady.Status != metav1.ConditionTrue {
		return fmt.Errorf("%w: waiting for the block devices of the virtual machine %q to be ready", ErrBlockDevicesNotReady, vm.Name)
	}

	for _, bdr := range vm.Status.BlockDeviceRefs {
		if bdr.Kind != v1alpha2.DiskDevice {
			continue
		}

		vd, err := h.snapshotter.GetVirtualDisk(ctx, bdr.Name, vm.Namespace)
		if err != nil {
			return err
		}

		if vd.Status.Phase != v1alpha2.DiskReady {
			return fmt.Errorf("%w: waiting for the virtual disk %q to be %s", ErrVirtualDiskNotReady, vd.Name, v1alpha2.DiskReady)
		}

		ready, _ := conditions.GetCondition(vdcondition.ReadyType, vd.Status.Conditions)
		if ready.Status != metav1.ConditionTrue || !conditions.IsLastUpdated(ready, vd) {
			return fmt.Errorf("%w: waiting for the Ready condition of the virtual disk %q to be True", ErrVirtualDiskNotReady, vd.Name)
		}

		resizing, _ := conditions.GetCondition(vdcondition.ResizingType, vd.Status.Conditions)
		if resizing.Status == metav1.ConditionTrue && vd.Generation == resizing.ObservedGeneration {
			return fmt.Errorf("%w: waiting for the virtual disk %q to be resized", ErrVirtualDiskResizing, vd.Name)
		}
	}

	return nil
}

func (h LifeCycleHandler) ensureSecret(ctx context.Context, vm *v1alpha2.VirtualMachine, vmSnapshot *v1alpha2.VirtualMachineSnapshot) error {
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

func getVDName(vdSnapshotName string, vmSnapshot *v1alpha2.VirtualMachineSnapshot) (string, bool) {
	return strings.CutSuffix(vdSnapshotName, "-"+string(vmSnapshot.UID))
}

func getVDSnapshotName(vdName string, vmSnapshot *v1alpha2.VirtualMachineSnapshot) string {
	return fmt.Sprintf("%s-%s", vdName, vmSnapshot.UID)
}

func (h LifeCycleHandler) fillStatusResources(ctx context.Context, vmSnapshot *v1alpha2.VirtualMachineSnapshot, vm *v1alpha2.VirtualMachine) error {
	vmSnapshot.Status.Resources = []v1alpha2.ResourceRef{}

	vmSnapshot.Status.Resources = append(vmSnapshot.Status.Resources, v1alpha2.ResourceRef{
		Kind:       vm.Kind,
		ApiVersion: vm.APIVersion,
		Name:       vm.Name,
	})

	if vmSnapshot.Spec.KeepIPAddress == v1alpha2.KeepIPAddressAlways {
		vmip, err := object.FetchObject(ctx, types.NamespacedName{
			Namespace: vm.Namespace,
			Name:      vm.Status.VirtualMachineIPAddress,
		}, h.client, &v1alpha2.VirtualMachineIPAddress{})
		if err != nil {
			return err
		}

		if vmip == nil {
			return fmt.Errorf("the virtual machine ip address %q not found", vm.Status.VirtualMachineIPAddress)
		}

		vmSnapshot.Status.Resources = append(vmSnapshot.Status.Resources, v1alpha2.ResourceRef{
			Kind:       vmip.Kind,
			ApiVersion: vmip.APIVersion,
			Name:       vmip.Name,
		})
	}

	if len(vm.Spec.Networks) > 1 {
		for _, ns := range vm.Status.Networks {
			if ns.Type == v1alpha2.NetworksTypeMain {
				continue
			}

			vmmac, err := object.FetchObject(ctx, types.NamespacedName{
				Namespace: vm.Namespace,
				Name:      ns.VirtualMachineMACAddressName,
			}, h.client, &v1alpha2.VirtualMachineMACAddress{})
			if err != nil {
				return err
			}
			if vmmac == nil {
				return fmt.Errorf("the virtual machine mac address %q not found", ns.VirtualMachineMACAddressName)
			}
			vmSnapshot.Status.Resources = append(vmSnapshot.Status.Resources, v1alpha2.ResourceRef{
				Kind:       vmmac.Kind,
				ApiVersion: vmmac.APIVersion,
				Name:       vmmac.Name,
			})
		}
	}

	provisioner, err := h.getProvisionerFromVM(ctx, vm)
	if err != nil {
		return err
	}
	if provisioner != nil {
		vmSnapshot.Status.Resources = append(vmSnapshot.Status.Resources, v1alpha2.ResourceRef{
			Kind:       provisioner.Kind,
			ApiVersion: provisioner.APIVersion,
			Name:       provisioner.Name,
		})
	}

	for _, bdr := range vm.Status.BlockDeviceRefs {
		if bdr.VirtualMachineBlockDeviceAttachmentName != "" {
			vmbda, err := object.FetchObject(ctx, types.NamespacedName{Name: bdr.VirtualMachineBlockDeviceAttachmentName, Namespace: vm.Namespace}, h.client, &v1alpha2.VirtualMachineBlockDeviceAttachment{})
			if err != nil {
				return err
			}
			if vmbda == nil {
				continue
			}
			vmSnapshot.Status.Resources = append(vmSnapshot.Status.Resources, v1alpha2.ResourceRef{
				Kind:       vmbda.Kind,
				ApiVersion: vmbda.APIVersion,
				Name:       vmbda.Name,
			})
		}

		if bdr.Kind != v1alpha2.DiskDevice {
			continue
		}

		vd, err := object.FetchObject(ctx, types.NamespacedName{Name: bdr.Name, Namespace: vm.Namespace}, h.client, &v1alpha2.VirtualDisk{})
		if err != nil {
			return err
		}
		if vd == nil {
			continue
		}
		vmSnapshot.Status.Resources = append(vmSnapshot.Status.Resources, v1alpha2.ResourceRef{
			Kind:       vd.Kind,
			ApiVersion: vd.APIVersion,
			Name:       vd.Name,
		})
	}

	return nil
}

func (h LifeCycleHandler) getProvisionerFromVM(ctx context.Context, vm *v1alpha2.VirtualMachine) (*corev1.Secret, error) {
	if vm.Spec.Provisioning != nil {
		var provisioningSecretName string

		switch vm.Spec.Provisioning.Type {
		case v1alpha2.ProvisioningTypeSysprepRef:
			if vm.Spec.Provisioning.SysprepRef == nil {
				return nil, nil
			}

			if vm.Spec.Provisioning.SysprepRef.Kind == v1alpha2.SysprepRefKindSecret {
				provisioningSecretName = vm.Spec.Provisioning.SysprepRef.Name
			}

		case v1alpha2.ProvisioningTypeUserDataRef:
			if vm.Spec.Provisioning.UserDataRef == nil {
				return nil, nil
			}

			if vm.Spec.Provisioning.UserDataRef.Kind == v1alpha2.UserDataRefKindSecret {
				provisioningSecretName = vm.Spec.Provisioning.UserDataRef.Name
			}
		}

		if provisioningSecretName != "" {
			secretKey := types.NamespacedName{Name: provisioningSecretName, Namespace: vm.Namespace}
			provisioner, err := object.FetchObject(ctx, secretKey, h.client, &corev1.Secret{})
			return provisioner, err
		} else {
			return nil, nil
		}
	} else {
		return nil, nil
	}
}
