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
	"encoding/json"
	"fmt"
	"strings"
	"time"

	vsv1 "github.com/kubernetes-csi/external-snapshotter/client/v6/apis/volumesnapshot/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	commonvd "github.com/deckhouse/virtualization-controller/pkg/common/vd"
	"github.com/deckhouse/virtualization-controller/pkg/controller/accessclaim"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/reconciler"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/supplements"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	coordinationv1alpha1 "github.com/deckhouse/virtualization/api/coordination/v1alpha1"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdscondition"
)

// releaseRetryInterval is the requeue interval while ending the access of a deleted snapshot keeps failing.
// It matches the requeue interval of a snapshot.
const releaseRetryInterval = 2 * time.Second

// statusWriteRecheck is the requeue interval of a pass that waits for its own status write before it
// releases the access, as the snapshot watch skips updates that keep the generation.
const statusWriteRecheck = time.Second

type LifeCycleHandler struct {
	recorder    eventrecord.EventRecorderLogger
	snapshotter LifeCycleSnapshotter
	client      client.Client
	// apiReader is uncached: a claim created moments ago may not be in the cache yet, and reading it as
	// absent would take the snapshot for one started before the module update.
	apiReader                  client.Reader
	unfreezer                  accessclaim.Unfreezer
	volumeSnapshotAPIAvailable bool
}

func NewLifeCycleHandler(recorder eventrecord.EventRecorderLogger, snapshotter LifeCycleSnapshotter, client client.Client, apiReader client.Reader, unfreezer accessclaim.Unfreezer, volumeSnapshotAPIAvailable bool) *LifeCycleHandler {
	return &LifeCycleHandler{
		recorder:                   recorder,
		snapshotter:                snapshotter,
		client:                     client,
		apiReader:                  apiReader,
		unfreezer:                  unfreezer,
		volumeSnapshotAPIAvailable: volumeSnapshotAPIAvailable,
	}
}

func (h LifeCycleHandler) Handle(ctx context.Context, vdSnapshot *v1alpha2.VirtualDiskSnapshot) (result reconcile.Result, retErr error) {
	log := logger.FromContext(ctx).With(logger.SlogHandler("lifecycle"))

	cb := conditions.NewConditionBuilder(vdscondition.VirtualDiskSnapshotReadyType).Generation(vdSnapshot.Generation)

	var claim *coordinationv1alpha1.VirtualMachineAccessClaim

	failedBefore := vdSnapshot.Status.Phase == v1alpha2.VirtualDiskSnapshotPhaseFailed

	defer func() {
		conditions.SetCondition(cb, &vdSnapshot.Status.Conditions)
		settleFailedConsistency(vdSnapshot)
		// A snapshot failed by this pass releases its access on the next one, once the failure is written:
		// a claim released earlier completes, and its verdict would read as a finished capture.
		if !failedBefore && vdSnapshot.Status.Phase == v1alpha2.VirtualDiskSnapshotPhaseFailed && claim != nil && retErr == nil {
			result = reconcile.Result{RequeueAfter: statusWriteRecheck}
		}
	}()

	vsName := vdSnapshot.Status.VolumeSnapshotName
	if vsName == "" {
		vsName = supplements.NewGenerator("vds", vdSnapshot.Name, vdSnapshot.Namespace, vdSnapshot.UID).CommonSupplement().Name
	}

	vs, err := h.snapshotter.GetVolumeSnapshot(ctx, vsName, vdSnapshot.Namespace)
	if err != nil {
		setPhaseConditionToFailed(cb, &vdSnapshot.Status.Phase, err)
		return reconcile.Result{}, err
	}

	// A child is captured under the access of its parent and has no claim of its own. A failed read of the
	// claim leaves the snapshot as it is. Only a snapshot in progress takes an absent claim for a start
	// before the module update, so only it reads past the cache.
	parent := accessclaim.ParentRef(vdSnapshot)
	if parent == nil {
		if vdSnapshot.Status.Phase == v1alpha2.VirtualDiskSnapshotPhaseInProgress {
			claim, err = accessclaim.Get(ctx, h.client, h.apiReader, vdSnapshot.Namespace, accessclaim.Name(vdSnapshot))
		} else {
			claim, err = accessclaim.GetCached(ctx, h.client, vdSnapshot.Namespace, accessclaim.Name(vdSnapshot))
		}

		if err != nil {
			cb.Keep(vdSnapshot.Status.Conditions)
			return reconcile.Result{}, err
		}
	}

	if vdSnapshot.DeletionTimestamp != nil {
		// The finalizer stays until the access is ended: the deletion handler runs after this one and
		// removes it otherwise.
		if err = h.releaseOnDeletion(ctx, vdSnapshot, parent, claim); err != nil {
			cb.Keep(vdSnapshot.Status.Conditions)
			logger.FromContext(ctx).Error("failed to end the access of the deleted snapshot", logger.SlogErr(err))

			return reconcile.Result{RequeueAfter: releaseRetryInterval}, fmt.Errorf("%w: %w", reconciler.ErrStopHandlerChain, err)
		}

		vdSnapshot.Status.Phase = v1alpha2.VirtualDiskSnapshotPhaseTerminating
		cb.
			Status(metav1.ConditionUnknown).
			Reason(conditions.ReasonUnknown).
			Message("")

		return reconcile.Result{}, nil
	}

	switch vdSnapshot.Status.Phase {
	case "":
		vdSnapshot.Status.Phase = v1alpha2.VirtualDiskSnapshotPhasePending
	case v1alpha2.VirtualDiskSnapshotPhaseFailed:
		readyCondition, _ := conditions.GetCondition(vdscondition.VirtualDiskSnapshotReadyType, vdSnapshot.Status.Conditions)
		cb.
			Status(metav1.ConditionFalse).
			Reason(conditions.CommonReason(readyCondition.Reason)).
			Message(readyCondition.Message)

		return reconcile.Result{}, accessclaim.Release(ctx, h.client, claim)
	case v1alpha2.VirtualDiskSnapshotPhaseReady:
		if vs == nil || vs.Status == nil || vs.Status.ReadyToUse == nil || !*vs.Status.ReadyToUse {
			vdSnapshot.Status.Phase = v1alpha2.VirtualDiskSnapshotPhaseFailed
			cb.
				Status(metav1.ConditionFalse).
				Reason(vdscondition.VolumeSnapshotLost).
				Message(fmt.Sprintf("The underlying VolumeSnapshot %q is no longer available.", vsName))

			return reconcile.Result{RequeueAfter: statusWriteRecheck}, nil
		}

		vdSnapshot.Status.Phase = v1alpha2.VirtualDiskSnapshotPhaseReady
		vdSnapshot.Status.VolumeSnapshotName = vs.Name
		// An unconfirmed unfreeze stays in the condition: the guest may still be frozen.
		message, _ := accessclaim.UnfreezeNotConfirmed(claim)
		cb.
			Status(metav1.ConditionTrue).
			Reason(vdscondition.VirtualDiskSnapshotReady).
			Message(message)

		if parent != nil {
			if err = h.settleChildConsistency(ctx, vdSnapshot, parent); err != nil {
				return reconcile.Result{}, err
			}
		}

		return reconcile.Result{}, h.backfillOriginalSize(ctx, vs)
	}

	// The claim settles the snapshot before the disk is read: a released claim means the volume snapshot was
	// ready.
	switch {
	case accessclaim.IsRefused(claim):
		// The coordinator has unfrozen the guest, or never froze it.
		reason, message := accessclaim.GrantedCondition(claim)
		vdSnapshot.Status.Phase = v1alpha2.VirtualDiskSnapshotPhaseFailed
		cb.
			Status(metav1.ConditionFalse).
			Reason(conditions.CommonReason(reason)).
			Message(message)

		return reconcile.Result{}, nil
	case claim != nil && (claim.Spec.Release || accessclaim.IsCompleted(claim)):
		return reconcile.Result{}, h.settleVerdict(ctx, cb, vdSnapshot, vs, claim)
	}

	if parent != nil {
		parentStarted, err := h.parentStartedBeforeUpdate(ctx, vdSnapshot, parent)
		if err != nil {
			cb.Keep(vdSnapshot.Status.Conditions)
			return reconcile.Result{}, err
		}

		if parentStarted {
			failAbortedByModuleUpdate(cb, vdSnapshot)
			return reconcile.Result{}, nil
		}
	}

	// Fail before requesting access: the VolumeSnapshot could never be created.
	if !h.volumeSnapshotAPIAvailable {
		vdSnapshot.Status.Phase = v1alpha2.VirtualDiskSnapshotPhaseFailed
		cb.
			Status(metav1.ConditionFalse).
			Reason(vdscondition.VirtualDiskSnapshotFailed).
			Message("Cannot take a snapshot: the storage-foundation and state-snapshotter modules must be enabled (or snapshot-controller for legacy installations).")

		return reconcile.Result{}, nil
	}

	vd, err := h.snapshotter.GetVirtualDisk(ctx, vdSnapshot.Spec.VirtualDiskName, vdSnapshot.Namespace)
	if err != nil {
		setPhaseConditionToFailed(cb, &vdSnapshot.Status.Phase, err)
		return reconcile.Result{}, err
	}

	virtualDiskReadyCondition, _ := conditions.GetCondition(vdscondition.VirtualDiskReadyType, vdSnapshot.Status.Conditions)
	if vd == nil {
		vdSnapshot.Status.Phase = v1alpha2.VirtualDiskSnapshotPhaseFailed
		msg := fmt.Sprintf("Cannot take a snapshot: the virtual disk %q does not exist.", vdSnapshot.Spec.VirtualDiskName)
		cb.
			Status(metav1.ConditionFalse).
			Reason(vdscondition.WaitingForTheVirtualDisk).
			Message(msg)

		return reconcile.Result{}, nil
	}

	if virtualDiskReadyCondition.Status != metav1.ConditionTrue {
		vdSnapshot.Status.Phase = v1alpha2.VirtualDiskSnapshotPhaseFailed
		msg := fmt.Sprintf("Cannot take a snapshot: the virtual disk %q is not ready.", vdSnapshot.Spec.VirtualDiskName)
		cb.
			Status(metav1.ConditionFalse).
			Reason(vdscondition.WaitingForTheVirtualDisk).
			Message(msg)

		return reconcile.Result{}, nil
	}

	var pvc *corev1.PersistentVolumeClaim
	if vd.Status.Target.PersistentVolumeClaim != "" {
		pvc, err = h.snapshotter.GetPersistentVolumeClaim(ctx, vd.Status.Target.PersistentVolumeClaim, vd.Namespace)
		if err != nil {
			setPhaseConditionToFailed(cb, &vdSnapshot.Status.Phase, err)
			return reconcile.Result{}, err
		}
	}

	if pvc == nil {
		vdSnapshot.Status.Phase = v1alpha2.VirtualDiskSnapshotPhaseFailed
		msg := fmt.Sprintf("Cannot take a snapshot: the persistent volume claim for virtual disk %q does not exist.", vdSnapshot.Spec.VirtualDiskName)
		cb.
			Status(metav1.ConditionFalse).
			Reason(vdscondition.WaitingForTheVirtualDisk).
			Message(msg)

		return reconcile.Result{}, nil
	}

	if pvc.Status.Phase != corev1.ClaimBound {
		vdSnapshot.Status.Phase = v1alpha2.VirtualDiskSnapshotPhaseFailed
		msg := fmt.Sprintf("Cannot take a snapshot: the persistent volume claim %q for virtual disk %q is not in phase %s.", pvc.Name, vdSnapshot.Spec.VirtualDiskName, corev1.ClaimBound)
		cb.
			Status(metav1.ConditionFalse).
			Reason(vdscondition.WaitingForTheVirtualDisk).
			Message(msg)

		return reconcile.Result{}, nil
	}

	if parent == nil {
		var proceed bool

		claim, result, proceed, err = h.requestAccess(ctx, cb, vdSnapshot, vd, claim)
		if !proceed {
			return result, err
		}
	}

	switch {
	case vs == nil:
		if vdSnapshot.Status.Phase == v1alpha2.VirtualDiskSnapshotPhasePending {
			vdSnapshot.Status.Phase = v1alpha2.VirtualDiskSnapshotPhaseInProgress
			cb.
				Status(metav1.ConditionFalse).
				Reason(vdscondition.Snapshotting).
				Message("The snapshotting process has started.")

			return reconcile.Result{RequeueAfter: statusWriteRecheck}, nil
		}

		log.Debug("The corresponding volume snapshot not found: create the new one")

		anno := make(map[string]string)
		if pvc.Spec.StorageClassName != nil && *pvc.Spec.StorageClassName != "" {
			anno[annotations.AnnStorageClassName] = *pvc.Spec.StorageClassName
		}

		if pvc.Spec.VolumeMode != nil && *pvc.Spec.VolumeMode != "" {
			anno[annotations.AnnVolumeMode] = string(*pvc.Spec.VolumeMode)
		}

		accessModes := make([]string, 0, len(pvc.Status.AccessModes))
		for _, accessMode := range pvc.Status.AccessModes {
			accessModes = append(accessModes, string(accessMode))
		}

		anno[annotations.AnnAccessModes] = strings.Join(accessModes, ",")

		if len(vd.Annotations) > 0 {
			vdAnnotationsJSON, err := json.Marshal(vd.Annotations)
			if err != nil {
				cb.Keep(vdSnapshot.Status.Conditions)
				return reconcile.Result{}, fmt.Errorf("failed to marshal VirtualDisk annotations: %w", err)
			} else {
				anno[annotations.AnnVirtualDiskOriginalAnnotations] = string(vdAnnotationsJSON)
			}
		}

		if len(vd.Labels) > 0 {
			vdLabelsJSON, err := json.Marshal(vd.Labels)
			if err != nil {
				cb.Keep(vdSnapshot.Status.Conditions)
				return reconcile.Result{}, fmt.Errorf("failed to marshal VirtualDisk labels: %w", err)
			} else {
				anno[annotations.AnnVirtualDiskOriginalLabels] = string(vdLabelsJSON)
			}
		}

		if requestedSize := commonvd.RequestedSize(vd, pvc); requestedSize != "" {
			anno[annotations.AnnVirtualDiskOriginalSize] = requestedSize
		}

		if selectedNode := pvc.Annotations[service.SelectedNodeAnnotation]; selectedNode != "" {
			anno[annotations.AnnVirtualDiskOriginalSelectedNode] = selectedNode
		}

		for _, ownerRef := range vd.OwnerReferences {
			if ownerRef.Kind == v1alpha2.VirtualMachineKind {
				anno[annotations.AnnVirtualDiskHadOwnerReference] = "true"
				break
			}
		}

		vs = &vsv1.VolumeSnapshot{
			ObjectMeta: metav1.ObjectMeta{
				Annotations: anno,
				Name:        vsName,
				Namespace:   vdSnapshot.Namespace,
				OwnerReferences: []metav1.OwnerReference{
					service.MakeOwnerReference(vdSnapshot),
				},
			},
			Spec: vsv1.VolumeSnapshotSpec{
				Source: vsv1.VolumeSnapshotSource{
					PersistentVolumeClaimName: &pvc.Name,
				},
			},
		}

		vs, err = h.snapshotter.CreateVolumeSnapshot(ctx, vs)
		if err != nil {
			setPhaseConditionToFailed(cb, &vdSnapshot.Status.Phase, err)
			return reconcile.Result{}, err
		}

		vdSnapshot.Status.Phase = v1alpha2.VirtualDiskSnapshotPhaseInProgress
		vdSnapshot.Status.VolumeSnapshotName = vs.Name
		cb.
			Status(metav1.ConditionFalse).
			Reason(vdscondition.Snapshotting).
			Message(fmt.Sprintf("The snapshotting process for virtual disk %q has started.", vdSnapshot.Spec.VirtualDiskName))

		return reconcile.Result{}, nil
	case vs.Status != nil && vs.Status.Error != nil && vs.Status.Error.Message != nil:
		log.Debug("The volume snapshot has an error")
		failOnVolumeSnapshotError(cb, vdSnapshot, vs)

		return reconcile.Result{}, nil
	case vs.Status == nil || vs.Status.ReadyToUse == nil || !*vs.Status.ReadyToUse:
		log.Debug("Waiting for the volume snapshot to be ready to use")

		vdSnapshot.Status.Phase = v1alpha2.VirtualDiskSnapshotPhaseInProgress
		vdSnapshot.Status.VolumeSnapshotName = vs.Name
		cb.
			Status(metav1.ConditionFalse).
			Reason(vdscondition.Snapshotting).
			Message(fmt.Sprintf("Waiting for the snapshot of virtual disk %q to be ready to use.", vdSnapshot.Spec.VirtualDiskName))

		return reconcile.Result{}, nil
	default:
		log.Debug("The volume snapshot is ready to use")

		if claim != nil {
			// The disk is captured: release the access; the claim watch brings the verdict.
			if err = accessclaim.Release(ctx, h.client, claim); err != nil {
				cb.Keep(vdSnapshot.Status.Conditions)
				return reconcile.Result{}, err
			}

			return reconcile.Result{}, h.settleVerdict(ctx, cb, vdSnapshot, vs, claim)
		}

		vdSnapshot.Status.VolumeSnapshotName = vs.Name
		if parent == nil {
			// Without a claim the disk stayed unattached during the capture (see requestAccess).
			vdSnapshot.Status.Consistent = ptr.To(true)
		}

		vdSnapshot.Status.Phase = v1alpha2.VirtualDiskSnapshotPhaseReady
		cb.
			Status(metav1.ConditionTrue).
			Reason(vdscondition.VirtualDiskSnapshotReady).
			Message("")

		return reconcile.Result{}, h.backfillOriginalSize(ctx, vs)
	}
}

// requestAccess requests the access a snapshot taken on its own captures its disk under, and reports
// whether the capture may proceed in this pass. A disk attached to no virtual machine needs no access. A
// snapshot in progress without a claim fails unless it carries the unattached-disk mark and its disk is still
// unattached.
func (h LifeCycleHandler) requestAccess(
	ctx context.Context,
	cb *conditions.ConditionBuilder,
	vdSnapshot *v1alpha2.VirtualDiskSnapshot,
	vd *v1alpha2.VirtualDisk,
	claim *coordinationv1alpha1.VirtualMachineAccessClaim,
) (*coordinationv1alpha1.VirtualMachineAccessClaim, reconcile.Result, bool, error) {
	vm, err := getVirtualMachine(ctx, vd, h.snapshotter)
	if err != nil {
		setPhaseConditionToFailed(cb, &vdSnapshot.Status.Phase, err)
		return claim, reconcile.Result{}, false, err
	}

	// The mark is written while the snapshot is still Pending, so a snapshot in progress always has it when
	// the disk turns out to be attached.
	if vm == nil && claim == nil {
		switch {
		case startedOnUnattachedDisk(vdSnapshot):
			return nil, reconcile.Result{}, true, nil
		case vdSnapshot.Status.Phase == v1alpha2.VirtualDiskSnapshotPhasePending:
			annotations.AddAnnotation(vdSnapshot, annotations.AnnStartedOnUnattachedDisk, "true")
			cb.
				Status(metav1.ConditionFalse).
				Reason(vdscondition.Snapshotting).
				Message("The snapshotting process has started.")

			return nil, reconcile.Result{RequeueAfter: statusWriteRecheck}, false, nil
		default:
			failAbortedByModuleUpdate(cb, vdSnapshot)
			return nil, reconcile.Result{}, false, nil
		}
	}

	if claim == nil {
		switch {
		case vdSnapshot.Status.Phase == v1alpha2.VirtualDiskSnapshotPhasePending:
			claim, err = accessclaim.Create(ctx, h.client, h.apiReader, vdSnapshot, vm, vdSnapshot.Spec.RequiredConsistency)
			if err != nil {
				setPhaseConditionToFailed(cb, &vdSnapshot.Status.Phase, err)
				return nil, reconcile.Result{}, false, err
			}
		case startedOnUnattachedDisk(vdSnapshot):
			vdSnapshot.Status.Phase = v1alpha2.VirtualDiskSnapshotPhaseFailed
			cb.
				Status(metav1.ConditionFalse).
				Reason(vdscondition.VirtualMachineStateChanged).
				Message(fmt.Sprintf("The virtual disk %q was attached to the virtual machine %q while the snapshot was being taken. Take the snapshot again.", vdSnapshot.Spec.VirtualDiskName, vm.Name))

			return nil, reconcile.Result{}, false, nil
		default:
			if err = h.unfreezeStartedBeforeUpdate(ctx, vm); err != nil {
				cb.Keep(vdSnapshot.Status.Conditions)
				return nil, reconcile.Result{}, false, err
			}

			failAbortedByModuleUpdate(cb, vdSnapshot)

			return nil, reconcile.Result{}, false, nil
		}
	}

	// The volume snapshot is created only once the access is granted.
	if !accessclaim.IsGranted(claim) {
		reason, message := accessclaim.Waiting(claim)
		cb.
			Status(metav1.ConditionFalse).
			Reason(conditions.CommonReason(reason)).
			Message(message)
		// The watch on the claim brings its decision here.
		return claim, reconcile.Result{}, false, nil
	}

	return claim, reconcile.Result{}, true, nil
}

func failOnVolumeSnapshotError(cb *conditions.ConditionBuilder, vdSnapshot *v1alpha2.VirtualDiskSnapshot, vs *vsv1.VolumeSnapshot) {
	vdSnapshot.Status.Phase = v1alpha2.VirtualDiskSnapshotPhaseFailed
	cb.
		Status(metav1.ConditionFalse).
		Reason(vdscondition.VirtualDiskSnapshotFailed).
		Message(fmt.Sprintf("VolumeSnapshot %q has an error: %s.", vs.Name, *vs.Status.Error.Message))
}

// settleVerdict settles a snapshot whose access is released: it waits for the verdict of the coordinator
// and for the guest to be unfrozen, then turns Ready with the consistency of the verdict.
func (h LifeCycleHandler) settleVerdict(
	ctx context.Context,
	cb *conditions.ConditionBuilder,
	vdSnapshot *v1alpha2.VirtualDiskSnapshot,
	vs *vsv1.VolumeSnapshot,
	claim *coordinationv1alpha1.VirtualMachineAccessClaim,
) error {
	if vs == nil {
		// The access is released only after the volume snapshot is ready, so a claim released without one did
		// not end a capture: a volume snapshot taken now would be taken from an unfrozen guest.
		logger.FromContext(ctx).Error("the access of the snapshot was released before its volume snapshot was taken")

		vdSnapshot.Status.Phase = v1alpha2.VirtualDiskSnapshotPhaseFailed
		cb.
			Status(metav1.ConditionFalse).
			Reason(vdscondition.VirtualDiskSnapshotFailed).
			Message(fmt.Sprintf("The snapshot of the virtual disk %q could not be completed. Take a new snapshot.", vdSnapshot.Spec.VirtualDiskName))

		return nil
	}

	vdSnapshot.Status.VolumeSnapshotName = vs.Name
	if vs.Status != nil && vs.Status.Error != nil && vs.Status.Error.Message != nil {
		failOnVolumeSnapshotError(cb, vdSnapshot, vs)
		return nil
	}

	if !accessclaim.IsCompleted(claim) || accessclaim.IsUnfreezing(claim) {
		vdSnapshot.Status.Phase = v1alpha2.VirtualDiskSnapshotPhaseInProgress
		if claim.Status.Frozen {
			cb.
				Status(metav1.ConditionFalse).
				Reason(vdscondition.FileSystemUnfreezing).
				Message(fmt.Sprintf("The snapshot of virtual disk %q has been taken; waiting for the virtual machine %q to be unfrozen.", vdSnapshot.Spec.VirtualDiskName, claim.Spec.VirtualMachine.Name))
		} else {
			cb.
				Status(metav1.ConditionFalse).
				Reason(vdscondition.Snapshotting).
				Message(fmt.Sprintf("The snapshot of virtual disk %q has been taken and is being finalized.", vdSnapshot.Spec.VirtualDiskName))
		}

		return nil
	}

	vdSnapshot.Status.Consistent = ptr.To(accessclaim.Consistent(claim))

	message, notConfirmed := accessclaim.UnfreezeNotConfirmed(claim)
	if notConfirmed {
		h.recorder.Event(vdSnapshot, corev1.EventTypeWarning, v1alpha2.ReasonFilesystemUnfreezeNotConfirmed, message)
	}

	vdSnapshot.Status.Phase = v1alpha2.VirtualDiskSnapshotPhaseReady
	cb.
		Status(metav1.ConditionTrue).
		Reason(vdscondition.VirtualDiskSnapshotReady).
		Message(message)

	return h.backfillOriginalSize(ctx, vs)
}

// settleChildConsistency gives a ready child the consistency of its parent once the parent is terminal
// (see accessclaim.ChildConsistency).
func (h LifeCycleHandler) settleChildConsistency(ctx context.Context, vdSnapshot *v1alpha2.VirtualDiskSnapshot, parent *metav1.OwnerReference) error {
	if vdSnapshot.Status.Consistent != nil {
		return nil
	}

	consistent, settled, err := accessclaim.ChildConsistency(ctx, h.client, vdSnapshot, parent)
	if err != nil || !settled {
		// The watch on the parent brings its terminal phase here.
		return err
	}

	vdSnapshot.Status.Consistent = ptr.To(consistent)

	return nil
}

// releaseOnDeletion ends the access of a deleted snapshot. A snapshot in progress or failed without a claim
// may have been started before the module update, so it unfreezes the guest it may have left frozen. A child
// does so only when its parent has no claim either, as a VirtualMachineSnapshot has no finalizer and may
// already be gone.
func (h LifeCycleHandler) releaseOnDeletion(ctx context.Context, vdSnapshot *v1alpha2.VirtualDiskSnapshot, parent *metav1.OwnerReference, claim *coordinationv1alpha1.VirtualMachineAccessClaim) error {
	if claim != nil {
		return accessclaim.Release(ctx, h.client, claim)
	}

	switch vdSnapshot.Status.Phase {
	case v1alpha2.VirtualDiskSnapshotPhaseInProgress, v1alpha2.VirtualDiskSnapshotPhaseFailed:
	default:
		return nil
	}

	if startedOnUnattachedDisk(vdSnapshot) {
		return nil
	}

	if parent != nil {
		parentStarted, err := h.parentStartedBeforeUpdate(ctx, vdSnapshot, parent)
		if err != nil || !parentStarted {
			return err
		}
	}

	vd, err := h.snapshotter.GetVirtualDisk(ctx, vdSnapshot.Spec.VirtualDiskName, vdSnapshot.Namespace)
	if err != nil || vd == nil {
		return err
	}

	vm, err := getVirtualMachine(ctx, vd, h.snapshotter)
	if err != nil || vm == nil {
		return err
	}

	return h.unfreezeStartedBeforeUpdate(ctx, vm)
}

// parentStartedBeforeUpdate reports a child whose parent has no access claim, as a parent started before the
// module update has none.
func (h LifeCycleHandler) parentStartedBeforeUpdate(ctx context.Context, vdSnapshot *v1alpha2.VirtualDiskSnapshot, parent *metav1.OwnerReference) (bool, error) {
	parentSnapshot := &v1alpha2.VirtualMachineSnapshot{ObjectMeta: metav1.ObjectMeta{UID: parent.UID}}
	parentClaim, err := accessclaim.Get(ctx, h.client, h.apiReader, vdSnapshot.Namespace, accessclaim.Name(parentSnapshot))

	return err == nil && parentClaim == nil, err
}

func failAbortedByModuleUpdate(cb *conditions.ConditionBuilder, vdSnapshot *v1alpha2.VirtualDiskSnapshot) {
	vdSnapshot.Status.Phase = v1alpha2.VirtualDiskSnapshotPhaseFailed
	cb.
		Status(metav1.ConditionFalse).
		Reason(vdscondition.AbortedByModuleUpdate).
		Message(accessclaim.MsgAbortedByModuleUpdate)
}

// settleFailedConsistency sets consistent to false on a failed snapshot without a verdict, as the unified
// VirtualDiskSnapshot does.
func settleFailedConsistency(vdSnapshot *v1alpha2.VirtualDiskSnapshot) {
	if vdSnapshot.Status.Phase == v1alpha2.VirtualDiskSnapshotPhaseFailed && vdSnapshot.Status.Consistent == nil {
		vdSnapshot.Status.Consistent = ptr.To(false)
	}
}

func startedOnUnattachedDisk(vdSnapshot *v1alpha2.VirtualDiskSnapshot) bool {
	_, ok := vdSnapshot.Annotations[annotations.AnnStartedOnUnattachedDisk]
	return ok
}

func (h LifeCycleHandler) unfreezeStartedBeforeUpdate(ctx context.Context, vm *v1alpha2.VirtualMachine) error {
	kvvmi, err := h.snapshotter.GetVirtualMachineInstance(ctx, vm)
	if err != nil {
		return err
	}

	return accessclaim.UnfreezeStartedBeforeUpdate(ctx, h.apiReader, h.unfreezer, kvvmi)
}

func getVirtualMachine(ctx context.Context, vd *v1alpha2.VirtualDisk, snapshotter virtualMachineGetter) (*v1alpha2.VirtualMachine, error) {
	if vd == nil {
		return nil, nil
	}

	// TODO: ensure vd.Status.AttachedToVirtualMachines is in the actual state.
	switch len(vd.Status.AttachedToVirtualMachines) {
	case 0:
		return nil, nil
	case 1:
		vm, err := snapshotter.GetVirtualMachine(ctx, vd.Status.AttachedToVirtualMachines[0].Name, vd.Namespace)
		if err != nil {
			return nil, err
		}

		return vm, nil
	default:
		return nil, fmt.Errorf("the virtual disk %q is attached to multiple virtual machines", vd.Name)
	}
}

type virtualMachineGetter interface {
	GetVirtualMachine(ctx context.Context, name, namespace string) (*v1alpha2.VirtualMachine, error)
}

func (h LifeCycleHandler) backfillOriginalSize(ctx context.Context, vs *vsv1.VolumeSnapshot) error {
	if _, ok := vs.Annotations[annotations.AnnVirtualDiskOriginalSize]; ok {
		return nil
	}

	if vs.Spec.Source.PersistentVolumeClaimName == nil || *vs.Spec.Source.PersistentVolumeClaimName == "" {
		return nil
	}

	pvc, err := h.snapshotter.GetPersistentVolumeClaim(ctx, *vs.Spec.Source.PersistentVolumeClaimName, vs.Namespace)
	if err != nil {
		return fmt.Errorf("failed to get the source persistent volume claim of the volume snapshot %q: %w", vs.Name, err)
	}

	if pvc == nil {
		return nil
	}

	requestedSize := pvc.Spec.Resources.Requests[corev1.ResourceStorage]
	if requestedSize.IsZero() {
		return nil
	}

	annotations.AddAnnotation(vs, annotations.AnnVirtualDiskOriginalSize, requestedSize.String())

	if err = h.snapshotter.UpdateVolumeSnapshot(ctx, vs); err != nil {
		return fmt.Errorf("failed to backfill the original size onto the volume snapshot %q: %w", vs.Name, err)
	}

	return nil
}

func setPhaseConditionToFailed(cb *conditions.ConditionBuilder, phase *v1alpha2.VirtualDiskSnapshotPhase, err error) {
	*phase = v1alpha2.VirtualDiskSnapshotPhaseFailed

	cb.
		Status(metav1.ConditionFalse).
		Reason(vdscondition.VirtualDiskSnapshotFailed).
		Message(service.CapitalizeFirstLetter(err.Error() + "."))
}
