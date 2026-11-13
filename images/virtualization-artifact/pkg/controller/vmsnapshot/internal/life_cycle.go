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
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	"github.com/deckhouse/virtualization-controller/pkg/controller/accessclaim"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	coordinationv1alpha1 "github.com/deckhouse/virtualization/api/coordination/v1alpha1"
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
	// apiReader is uncached: a claim created moments ago may not be in the cache yet, and reading it as
	// absent would take the snapshot for one started before the module update.
	apiReader client.Reader
	unfreezer accessclaim.Unfreezer
}

// statusWriteRecheck is the requeue interval of a pass that waits for its own status write before it
// releases the access, as the snapshot watch skips updates that keep the generation.
const statusWriteRecheck = time.Second

func NewLifeCycleHandler(recorder eventrecord.EventRecorderLogger, snapshotter Snapshotter, storer Storer, client client.Client, apiReader client.Reader, unfreezer accessclaim.Unfreezer) *LifeCycleHandler {
	return &LifeCycleHandler{
		recorder:    recorder,
		snapshotter: snapshotter,
		storer:      storer,
		client:      client,
		apiReader:   apiReader,
		unfreezer:   unfreezer,
	}
}

func (h LifeCycleHandler) Handle(ctx context.Context, vmSnapshot *v1alpha2.VirtualMachineSnapshot) (reconcile.Result, error) {
	log := logger.FromContext(ctx).With(logger.SlogHandler("lifecycle"))

	cb := conditions.NewConditionBuilder(vmscondition.VirtualMachineSnapshotReadyType)

	defer func() {
		conditions.SetCondition(cb.Generation(vmSnapshot.Generation), &vmSnapshot.Status.Conditions)
		settleFailedConsistency(vmSnapshot)
	}()

	if !conditions.HasCondition(cb.GetType(), vmSnapshot.Status.Conditions) {
		cb.Status(metav1.ConditionUnknown).Reason(conditions.ReasonUnknown)
	}

	vm, err := h.snapshotter.GetVirtualMachine(ctx, vmSnapshot.Spec.VirtualMachineName, vmSnapshot.Namespace)
	if err != nil {
		h.setPhaseConditionToFailed(cb, vmSnapshot, err)
		return reconcile.Result{}, err
	}

	// A failed read of the claim leaves the snapshot as it is. Only a snapshot in progress takes an absent
	// claim for a start before the module update, so only it reads past the cache.
	var claim *coordinationv1alpha1.VirtualMachineAccessClaim
	if vmSnapshot.Status.Phase == v1alpha2.VirtualMachineSnapshotPhaseInProgress {
		claim, err = accessclaim.Get(ctx, h.client, h.apiReader, vmSnapshot.Namespace, accessclaim.Name(vmSnapshot))
	} else {
		claim, err = accessclaim.GetCached(ctx, h.client, vmSnapshot.Namespace, accessclaim.Name(vmSnapshot))
	}

	if err != nil {
		cb.Keep(vmSnapshot.Status.Conditions)
		return reconcile.Result{}, err
	}

	if vmSnapshot.DeletionTimestamp != nil {
		if err = h.releaseOnDeletion(ctx, vmSnapshot, vm, claim); err != nil {
			cb.Keep(vmSnapshot.Status.Conditions)
			return reconcile.Result{}, err
		}

		vmSnapshot.Status.Phase = v1alpha2.VirtualMachineSnapshotPhaseTerminating
		cb.
			Status(metav1.ConditionUnknown).
			Reason(conditions.ReasonUnknown).
			Message("")

		return reconcile.Result{}, nil
	}

	switch vmSnapshot.Status.Phase {
	case "":
		vmSnapshot.Status.Phase = v1alpha2.VirtualMachineSnapshotPhasePending
	case v1alpha2.VirtualMachineSnapshotPhaseFailed:
		// A failed snapshot releases its access once the failure is written: a claim released earlier
		// completes, and its verdict would read as a successful capture.
		readyCondition, _ := conditions.GetCondition(vmscondition.VirtualMachineSnapshotReadyType, vmSnapshot.Status.Conditions)
		cb.
			Status(readyCondition.Status).
			Reason(conditions.CommonReason(readyCondition.Reason)).
			Message(readyCondition.Message)

		return reconcile.Result{}, accessclaim.Release(ctx, h.client, claim)
	case v1alpha2.VirtualMachineSnapshotPhaseReady:
		// Ensure vd snapshots aren't lost.
		var lostVDSnapshots []string

		for _, vdSnapshotName := range vmSnapshot.Status.VirtualDiskSnapshotNames {
			vdSnapshot, err := h.snapshotter.GetVirtualDiskSnapshot(ctx, vdSnapshotName, vmSnapshot.Namespace)
			if err != nil {
				h.setPhaseConditionToFailed(cb, vmSnapshot, err)
				return reconcile.Result{}, err
			}

			// Consider vd snapshot is lost when it is absent or in Terminating state.
			switch {
			case vdSnapshot == nil || vdSnapshot.DeletionTimestamp != nil:
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
		// An unconfirmed unfreeze stays in the condition: the guest may still be frozen.
		message, _ := accessclaim.UnfreezeNotConfirmed(claim)
		cb.
			Status(metav1.ConditionTrue).
			Reason(vmscondition.VirtualMachineSnapshotReady).
			Message(message)

		return reconcile.Result{}, nil
	}

	// A decided claim settles the snapshot even when its virtual machine is gone.
	switch {
	case accessclaim.IsRefused(claim):
		reason, message := accessclaim.GrantedCondition(claim)
		vmSnapshot.Status.Phase = v1alpha2.VirtualMachineSnapshotPhaseFailed
		h.recorder.Event(
			vmSnapshot,
			corev1.EventTypeWarning,
			v1alpha2.ReasonVMSnapshottingFailed,
			message,
		)
		cb.
			Status(metav1.ConditionFalse).
			Reason(conditions.CommonReason(reason)).
			Message(message)

		return reconcile.Result{}, nil
	case accessclaim.IsCompleted(claim) && accessclaim.IsUnfreezing(claim):
		h.setWaitingForVerdict(cb, vmSnapshot, claim)
		return reconcile.Result{}, nil
	case accessclaim.IsCompleted(claim) && len(vmSnapshot.Status.Resources) == 0:
		// The access is released only after the resources are written, so a completed claim without them
		// did not end a successful capture.
		log.Error("the access of the snapshot was released before its resources were written")

		vmSnapshot.Status.Phase = v1alpha2.VirtualMachineSnapshotPhaseFailed
		msg := fmt.Sprintf("The snapshot of the virtual machine %q could not be completed. Take a new snapshot.", vmSnapshot.Spec.VirtualMachineName)
		h.recorder.Event(
			vmSnapshot,
			corev1.EventTypeWarning,
			v1alpha2.ReasonVMSnapshottingFailed,
			msg,
		)
		cb.
			Status(metav1.ConditionFalse).
			Reason(vmscondition.VirtualMachineSnapshotFailed).
			Message(msg)

		return reconcile.Result{}, nil
	case accessclaim.IsCompleted(claim):
		log.Debug("The virtual disk snapshots are taken: the virtual machine snapshot is Ready now")

		vmSnapshot.Status.Phase = v1alpha2.VirtualMachineSnapshotPhaseReady
		vmSnapshot.Status.Consistent = ptr.To(accessclaim.Consistent(claim))
		h.recorder.Event(
			vmSnapshot,
			corev1.EventTypeNormal,
			v1alpha2.ReasonVMSnapshottingCompleted,
			"Virtual machine snapshotting process is completed.",
		)

		message, notConfirmed := accessclaim.UnfreezeNotConfirmed(claim)
		if notConfirmed {
			h.recorder.Event(vmSnapshot, corev1.EventTypeWarning, v1alpha2.ReasonFilesystemUnfreezeNotConfirmed, message)
		}

		cb.
			Status(metav1.ConditionTrue).
			Reason(vmscondition.VirtualMachineReady).
			Message(message)

		return reconcile.Result{}, nil
	}

	if vmSnapshot.Status.Phase == v1alpha2.VirtualMachineSnapshotPhasePending {
		return h.requestAccess(ctx, cb, vmSnapshot, vm, claim)
	}

	// A snapshot requests access before it leaves Pending, so one in progress without a claim was started
	// before the module update. It fails once the guest it may have left frozen is unfrozen.
	if claim == nil {
		if err = h.unfreezeStartedBeforeUpdate(ctx, vm); err != nil {
			cb.Keep(vmSnapshot.Status.Conditions)
			return reconcile.Result{}, err
		}

		vmSnapshot.Status.Phase = v1alpha2.VirtualMachineSnapshotPhaseFailed
		h.recorder.Event(
			vmSnapshot,
			corev1.EventTypeWarning,
			v1alpha2.ReasonVMSnapshottingFailed,
			accessclaim.MsgAbortedByModuleUpdate,
		)
		cb.
			Status(metav1.ConditionFalse).
			Reason(vmscondition.AbortedByModuleUpdate).
			Message(accessclaim.MsgAbortedByModuleUpdate)

		return reconcile.Result{}, nil
	}

	if claim.Spec.Release {
		// The watch on the claim brings the verdict here.
		h.setWaitingForVerdict(cb, vmSnapshot, claim)
		return reconcile.Result{}, nil
	}

	if vm == nil {
		vmSnapshot.Status.Phase = v1alpha2.VirtualMachineSnapshotPhaseFailed
		msg := fmt.Sprintf("Cannot take a snapshot: the virtual machine %q does not exist.", vmSnapshot.Spec.VirtualMachineName)
		h.recorder.Event(
			vmSnapshot,
			corev1.EventTypeWarning,
			v1alpha2.ReasonVMSnapshottingFailed,
			msg,
		)
		cb.
			Status(metav1.ConditionFalse).
			Reason(vmscondition.WaitingForTheVirtualMachine).
			Message(msg)
		// The access is released by the next pass, once the failure is written.
		return reconcile.Result{RequeueAfter: statusWriteRecheck}, nil
	}

	// 1. Create secret.
	err = h.ensureSecret(ctx, vm, vmSnapshot)
	if err != nil {
		h.setPhaseConditionToFailed(cb, vmSnapshot, err)
		return reconcile.Result{}, err
	}

	// 2. Fill status.VirtualDiskSnapshotNames.
	h.fillStatusVirtualDiskSnapshotNames(vmSnapshot, vm)

	// 3. Get or Create VirtualDiskSnapshots.
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
		// The access is released by the next pass, once the failure is written.
		return reconcile.Result{RequeueAfter: statusWriteRecheck}, nil
	default:
		h.setPhaseConditionToFailed(cb, vmSnapshot, err)
		return reconcile.Result{}, err
	}

	// 4. Wait for VirtualDiskSnapshots to be Ready.
	readyCount := h.countReadyVirtualDiskSnapshots(vdSnapshots)
	msg := fmt.Sprintf(
		"Waiting for the virtual disk snapshots to be taken for "+
			"the block devices of the virtual machine %q (%d/%d).",
		vm.Name, readyCount, len(vdSnapshots),
	)

	h.recorder.Event(
		vmSnapshot,
		corev1.EventTypeNormal,
		v1alpha2.ReasonVMSnapshottingInProgress,
		msg,
	)

	if readyCount != len(vdSnapshots) {
		log.Debug("Waiting for the virtual disk snapshots to be taken for the block devices of the virtual machine")

		vmSnapshot.Status.Phase = v1alpha2.VirtualMachineSnapshotPhaseInProgress
		cb.
			Status(metav1.ConditionFalse).
			Reason(vmscondition.Snapshotting).
			Message(msg)

		return reconcile.Result{}, nil
	}

	// 5. Fill status resources. The access is released only by a pass that finds them written, as restore
	// relies on them.
	resourcesWritten := len(vmSnapshot.Status.Resources) > 0

	err = h.fillStatusResources(ctx, vmSnapshot, vm)
	if err != nil {
		h.setPhaseConditionToFailed(cb, vmSnapshot, err)
		return reconcile.Result{}, err
	}

	if !resourcesWritten {
		vmSnapshot.Status.Phase = v1alpha2.VirtualMachineSnapshotPhaseInProgress
		cb.
			Status(metav1.ConditionFalse).
			Reason(vmscondition.Snapshotting).
			Message(msg)

		return reconcile.Result{RequeueAfter: statusWriteRecheck}, nil
	}

	// 6. Every disk is captured: release the access; the claim watch brings the verdict.
	if err = accessclaim.Release(ctx, h.client, claim); err != nil {
		cb.Keep(vmSnapshot.Status.Conditions)
		return reconcile.Result{}, err
	}

	h.setWaitingForVerdict(cb, vmSnapshot, claim)

	return reconcile.Result{}, nil
}

// requestAccess checks that the virtual machine can be snapshotted, asks the coordinator for it, and moves
// the snapshot on to capturing the disks only once the access is granted.
func (h LifeCycleHandler) requestAccess(ctx context.Context, cb *conditions.ConditionBuilder, vmSnapshot *v1alpha2.VirtualMachineSnapshot, vm *v1alpha2.VirtualMachine, claim *coordinationv1alpha1.VirtualMachineAccessClaim) (reconcile.Result, error) {
	if claim == nil {
		virtualMachineReadyCondition, _ := conditions.GetCondition(vmscondition.VirtualMachineReadyType, vmSnapshot.Status.Conditions)
		if vm == nil {
			vmSnapshot.Status.Phase = v1alpha2.VirtualMachineSnapshotPhaseFailed
			msg := fmt.Sprintf("Cannot take a snapshot: the virtual machine %q does not exist.", vmSnapshot.Spec.VirtualMachineName)
			h.recorder.Event(
				vmSnapshot,
				corev1.EventTypeWarning,
				v1alpha2.ReasonVMSnapshottingFailed,
				msg,
			)
			cb.
				Status(metav1.ConditionFalse).
				Reason(vmscondition.WaitingForTheVirtualMachine).
				Message(msg)

			return reconcile.Result{}, nil
		}

		if virtualMachineReadyCondition.Status != metav1.ConditionTrue {
			vmSnapshot.Status.Phase = v1alpha2.VirtualMachineSnapshotPhaseFailed
			msg := fmt.Sprintf("Cannot take a snapshot: the virtual machine %q is not ready.", vmSnapshot.Spec.VirtualMachineName)
			h.recorder.Event(
				vmSnapshot,
				corev1.EventTypeWarning,
				v1alpha2.ReasonVMSnapshottingFailed,
				msg,
			)
			cb.
				Status(metav1.ConditionFalse).
				Reason(vmscondition.WaitingForTheVirtualMachine).
				Message(msg)

			return reconcile.Result{}, nil
		}

		err := h.ensureBlockDeviceConsistency(ctx, vm)
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

		claim, err = accessclaim.Create(ctx, h.client, h.apiReader, vmSnapshot, vm, vmSnapshot.Spec.RequiredConsistency)
		if err != nil {
			h.setPhaseConditionToFailed(cb, vmSnapshot, err)
			return reconcile.Result{}, err
		}
	}

	if !accessclaim.IsGranted(claim) {
		reason, message := accessclaim.Waiting(claim)
		cb.
			Status(metav1.ConditionFalse).
			Reason(conditions.CommonReason(reason)).
			Message(message)
		// The watch on the claim brings its decision here.
		return reconcile.Result{}, nil
	}

	vmSnapshot.Status.Phase = v1alpha2.VirtualMachineSnapshotPhaseInProgress
	h.recorder.Event(
		vmSnapshot,
		corev1.EventTypeNormal,
		v1alpha2.ReasonVMSnapshottingStarted,
		"Virtual machine snapshotting process is started.",
	)
	cb.
		Status(metav1.ConditionFalse).
		Reason(vmscondition.Snapshotting).
		Message("The snapshotting process has started.")

	return reconcile.Result{RequeueAfter: statusWriteRecheck}, nil
}

// settleFailedConsistency sets consistent to false on a failed snapshot without a verdict, as the unified
// VirtualMachineSnapshot does.
func settleFailedConsistency(vmSnapshot *v1alpha2.VirtualMachineSnapshot) {
	if vmSnapshot.Status.Phase == v1alpha2.VirtualMachineSnapshotPhaseFailed && vmSnapshot.Status.Consistent == nil {
		vmSnapshot.Status.Consistent = ptr.To(false)
	}
}

// setWaitingForVerdict reports a snapshot whose disks are all taken and whose access is released, while
// the coordinator unfreezes the guest and decides on the session.
func (h LifeCycleHandler) setWaitingForVerdict(cb *conditions.ConditionBuilder, vmSnapshot *v1alpha2.VirtualMachineSnapshot, claim *coordinationv1alpha1.VirtualMachineAccessClaim) {
	vmSnapshot.Status.Phase = v1alpha2.VirtualMachineSnapshotPhaseInProgress
	if claim.Status.Frozen {
		cb.
			Status(metav1.ConditionFalse).
			Reason(vmscondition.FileSystemUnfreezing).
			Message(fmt.Sprintf("The virtual disk snapshots have been taken; waiting for the virtual machine %q to be unfrozen.", claim.Spec.VirtualMachine.Name))

		return
	}

	cb.
		Status(metav1.ConditionFalse).
		Reason(vmscondition.Snapshotting).
		Message("The virtual disk snapshots have been taken; the snapshot is being finalized.")
}

// releaseOnDeletion ends the access of a deleted snapshot. A snapshot in progress without a claim was
// started before the module update, and the guest it may have left frozen is unfrozen.
func (h LifeCycleHandler) releaseOnDeletion(ctx context.Context, vmSnapshot *v1alpha2.VirtualMachineSnapshot, vm *v1alpha2.VirtualMachine, claim *coordinationv1alpha1.VirtualMachineAccessClaim) error {
	if claim != nil {
		return accessclaim.Release(ctx, h.client, claim)
	}

	if vmSnapshot.Status.Phase != v1alpha2.VirtualMachineSnapshotPhaseInProgress {
		return nil
	}

	return h.unfreezeStartedBeforeUpdate(ctx, vm)
}

func (h LifeCycleHandler) unfreezeStartedBeforeUpdate(ctx context.Context, vm *v1alpha2.VirtualMachine) error {
	if vm == nil {
		return nil
	}

	kvvmi, err := h.snapshotter.GetVirtualMachineInstance(ctx, vm)
	if err != nil {
		return err
	}

	return accessclaim.UnfreezeStartedBeforeUpdate(ctx, h.apiReader, h.unfreezer, kvvmi)
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

		if vd == nil {
			return fmt.Errorf("%w: the virtual disk %q does not exist", ErrVirtualDiskNotReady, bdr.Name)
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
	var (
		secret *corev1.Secret
		err    error
	)

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

	if vmSnapshot.Spec.KeepIPAddress == v1alpha2.KeepIPAddressAlways && vm.Status.VirtualMachineIPAddress != "" {
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

	if len(vm.Spec.Networks) > 0 {
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
