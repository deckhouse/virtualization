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

// Package vdsnapshot drives VirtualDiskSnapshot capture through the state-snapshotter SDK
// (github.com/deckhouse/state-snapshotter/pkg/snapshotsdk). It is a data-bearing leaf: its data leg is
// the disk's backing PVC, captured via EnsureVolumeCapture (a storage-foundation VolumeCaptureRequest)
// instead of a directly-created CSI VolumeSnapshot.
//
// The snapshot does not freeze the guest itself. A snapshot taken on its own asks the virtual machine access
// coordinator for access to the virtual machine its disk is attached to. A child of a VirtualMachineSnapshot
// is captured under the access of its parent and takes its consistency from the parent.
package vdsnapshot

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/deckhouse/pkg/log"
	"github.com/deckhouse/state-snapshotter/pkg/snapshotsdk"
	foundationv1alpha1 "github.com/deckhouse/storage-foundation/api/v1alpha1"
	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	commonvd "github.com/deckhouse/virtualization-controller/pkg/common/vd"
	"github.com/deckhouse/virtualization-controller/pkg/controller/unified-snapshotter/internal/accessclaim"
	"github.com/deckhouse/virtualization-controller/pkg/controller/unified-snapshotter/internal/adapter"
	"github.com/deckhouse/virtualization-controller/pkg/controller/unified-snapshotter/internal/annotation"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	"github.com/deckhouse/virtualization-controller/pkg/unifiedsnapshotter/statuspatch"
	coordinationv1alpha1 "github.com/deckhouse/virtualization/api/coordination/v1alpha1"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdscondition"
)

const (
	requeueAfter   = 2 * time.Second
	ControllerName = "virtualdisk-snapshot-controller"

	// reasonInvalidSource is the terminal domain reason for a snapshot whose spec does not resolve to a
	// capturable source object.
	reasonInvalidSource = "InvalidSource"

	// reasonDataCaptureRetrying is the non-terminal domain reason that carries the storage error of a data
	// capture the storage keeps retrying; it is cleared as soon as the error clears.
	reasonDataCaptureRetrying = "DataCaptureRetrying"

	// reasonSnapshotsNotSupported is the terminal domain reason for a disk whose storage driver cannot take
	// snapshots at all.
	reasonSnapshotsNotSupported = "SnapshotsNotSupported"
)

// snapshotsNotSupportedMarker is the fragment of the storage error that names a driver without snapshot
// support. It is matched without the driver name, which the storage quotes in a form that varies with the
// layer that wraps the error.
// TODO remove detection harness when storage level implements stopping retries on terminal errors.
const snapshotsNotSupportedMarker = "does not support snapshots"

// Reconciler drives VirtualDiskSnapshot capture through the state-snapshotter SDK.
type Reconciler struct {
	Client    client.Client
	APIReader client.Reader
	Log       *log.Logger
}

// SetupWithManager registers the reconciler, gated to the objects the unified mechanism owns — see
// annotation.DrivenByUnified. It also watches its access claim and the parent VirtualMachineSnapshot, whose
// terminal phase settles the consistency of a child.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		Named(ControllerName).
		For(&v1alpha2.VirtualDiskSnapshot{}, builder.WithPredicates(annotation.ShouldHandle())).
		Owns(&coordinationv1alpha1.VirtualMachineAccessClaim{}).
		Watches(&v1alpha2.VirtualMachineSnapshot{}, handler.EnqueueRequestsFromMapFunc(childrenOf),
			builder.WithPredicates(predicate.Funcs{
				UpdateFunc: func(e event.UpdateEvent) bool {
					oldVMS, oldOK := e.ObjectOld.(*v1alpha2.VirtualMachineSnapshot)
					newVMS, newOK := e.ObjectNew.(*v1alpha2.VirtualMachineSnapshot)

					return oldOK && newOK && oldVMS.Status.Phase != newVMS.Status.Phase
				},
			})).
		WithLogConstructor(logger.NewConstructor(r.Log)).
		Complete(r)
}

// childrenOf maps a VirtualMachineSnapshot to its child VirtualDiskSnapshots.
func childrenOf(_ context.Context, obj client.Object) []reconcile.Request {
	vms, ok := obj.(*v1alpha2.VirtualMachineSnapshot)
	if !ok {
		return nil
	}

	var requests []reconcile.Request

	for _, ref := range vms.Status.ChildrenSnapshotRefs {
		if ref.Kind == v1alpha2.VirtualDiskSnapshotKind {
			requests = append(requests, reconcile.Request{NamespacedName: types.NamespacedName{Namespace: vms.Namespace, Name: ref.Name}})
		}
	}

	return requests
}

func (r *Reconciler) sdk() snapshotsdk.CaptureSDK {
	return snapshotsdk.New(r.Client, r.APIReader, snapshotsdk.NewStorageFoundationProvider(r.Client))
}

// Reconcile drives one VirtualDiskSnapshot, stopping quietly if it is deleted while the work is in
// flight. See the VirtualMachineSnapshot controller's Reconcile for why the first read's guard is not
// enough on its own.
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	result, err := r.reconcile(ctx, req)
	if object.IsGone(err, v1alpha2.SchemeGroupVersion.WithResource(v1alpha2.VirtualDiskSnapshotResource).GroupResource(), req.Name) {
		logger.FromContext(ctx).Debug("the snapshot was deleted while it was being reconciled; nothing left to do")
		return ctrl.Result{}, nil
	}

	return result, err
}

func (r *Reconciler) reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	vds := &v1alpha2.VirtualDiskSnapshot{}
	if err := r.Client.Get(ctx, req.NamespacedName, vds); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if vds.DeletionTimestamp != nil {
		return ctrl.Result{}, nil
	}

	driven, err := annotation.DrivenByUnifiedVirtualDiskSnapshot(ctx, r.APIReader, vds)
	if err != nil {
		return ctrl.Result{}, err
	}

	if !driven {
		return ctrl.Result{}, nil
	}

	if vds.IsImport() {
		return r.reconcileImport(ctx, vds)
	}

	if vds.Status.Phase == "" {
		vds.Status.Phase = v1alpha2.VirtualDiskSnapshotPhasePending
		if err := r.patchStatus(ctx, vds); err != nil {
			return ctrl.Result{}, err
		}

		return ctrl.Result{RequeueAfter: time.Second}, nil
	}

	parent := accessclaim.ParentRef(vds)

	switch vds.Status.Phase {
	case v1alpha2.VirtualDiskSnapshotPhaseReady, v1alpha2.VirtualDiskSnapshotPhaseFailed:
		if parent != nil {
			return r.settleChildConsistency(ctx, vds, parent)
		}

		return ctrl.Result{}, nil
	}

	a := &adapter.VirtualDiskSnapshotAdapter{VDS: vds}
	sdk := r.sdk()
	domainPhase := a.GetDomainCaptureState().Phase
	planning := domainPhase != snapshotsdk.PhasePlanned && domainPhase != snapshotsdk.PhaseFinished

	var claim *coordinationv1alpha1.VirtualMachineAccessClaim
	if parent == nil {
		if claim, err = accessclaim.Get(ctx, r.Client, r.APIReader, vds.Namespace, accessclaim.Name(vds)); err != nil {
			return ctrl.Result{}, err
		}
	}

	switch domainPhase {
	case snapshotsdk.PhaseFinished:
		return r.finishAsReady(ctx, vds, parent, claim)
	case snapshotsdk.PhaseFailed:
		return r.reconcileCaptureFailed(ctx, vds, claim)
	}

	startedBeforeUpdate, err := r.startedBeforeUpdate(ctx, vds, parent, claim, planning)
	if err != nil {
		return ctrl.Result{}, err
	}

	if startedBeforeUpdate {
		return r.failCapture(ctx, a, vds, nil, string(vdscondition.AbortedByModuleUpdate),
			"The snapshot was aborted by the update of the virtualization module. Take the snapshot again.")
	}

	// A decided claim settles the snapshot even when its disk is gone. Only the claim of a captured disk is
	// released while the snapshot is in progress, so a completed claim means a captured disk.
	switch {
	case accessclaim.IsRefused(claim):
		reason, message := accessclaim.GrantedCondition(claim)
		return r.failCapture(ctx, a, vds, claim, reason, message)
	case accessclaim.IsCompleted(claim):
		if err := sdk.DomainCaptureStatus(a).Phase(snapshotsdk.PhaseFinished).Apply(ctx); err != nil {
			return ctrl.Result{}, err
		}

		return r.finishAsReady(ctx, vds, parent, claim)
	}

	// An unresolvable source is failed only after the Pending bootstrap above.
	vdName := vds.SourceVirtualDiskName()
	if vdName == "" {
		return r.failCapture(ctx, a, vds, claim, reasonInvalidSource, fmt.Sprintf(
			"spec.sourceRef must reference a %s %s", v1alpha2.SchemeGroupVersion.String(), v1alpha2.VirtualDiskKind))
	}

	// A snapshot holding its access does not wait for its disk: the session of a stopped virtual machine has
	// no deadline, so the wait would hold the machine with no end.
	holdsAccess := claim != nil && !claim.Spec.Release

	vd := &v1alpha2.VirtualDisk{}
	if err := r.Client.Get(ctx, types.NamespacedName{Namespace: vds.Namespace, Name: vdName}, vd); err != nil {
		if apierrors.IsNotFound(err) {
			if holdsAccess {
				return r.failOnLostDisk(ctx, a, vds, claim)
			}
			// The wait has no deadline, as no access is requested yet and the guest is not frozen. It covers a
			// snapshot applied together with its disk.
			if perr := sdk.DomainCaptureStatus(a).
				Phase(snapshotsdk.PhasePlanning).
				Reason(snapshotsdk.Reason(vdscondition.WaitingForTheVirtualDisk)).
				Message(fmt.Sprintf("The VirtualDisk %q does not exist. Waiting for it to appear; snapshotting will continue on its own.", vdName)).
				Apply(ctx); perr != nil {
				return ctrl.Result{}, perr
			}

			return ctrl.Result{RequeueAfter: requeueAfter}, nil
		}

		return ctrl.Result{}, err
	}

	pvcName := vd.Status.Target.PersistentVolumeClaim
	if pvcName == "" {
		if holdsAccess {
			return r.failOnLostDisk(ctx, a, vds, claim)
		}

		if perr := sdk.DomainCaptureStatus(a).
			Phase(snapshotsdk.PhasePlanning).
			Message("waiting for the virtual disk to provision its backing PersistentVolumeClaim").
			Apply(ctx); perr != nil {
			return ctrl.Result{}, perr
		}

		return ctrl.Result{RequeueAfter: requeueAfter}, nil
	}
	// APIReader (uncached, direct): r.Client is the manager's cached client, and Get() on a GVK it
	// hasn't seen yet lazily starts a cluster-wide List+Watch informer for that whole type — we only
	// ever need this one PVC, not a standing cache of every PersistentVolumeClaim in the cluster.
	pvc := &corev1.PersistentVolumeClaim{}
	if err := r.APIReader.Get(ctx, types.NamespacedName{Namespace: vds.Namespace, Name: pvcName}, pvc); err != nil {
		if apierrors.IsNotFound(err) {
			if holdsAccess {
				return r.failOnLostDisk(ctx, a, vds, claim)
			}

			if perr := sdk.DomainCaptureStatus(a).
				Phase(snapshotsdk.PhasePlanning).
				Message(fmt.Sprintf("backing PersistentVolumeClaim %q not found; waiting", pvcName)).
				Apply(ctx); perr != nil {
				return ctrl.Result{}, perr
			}

			return ctrl.Result{RequeueAfter: requeueAfter}, nil
		}

		return ctrl.Result{}, err
	}

	if pvc.Status.Phase != corev1.ClaimBound {
		if holdsAccess {
			return r.failOnLostDisk(ctx, a, vds, claim)
		}

		if err := sdk.DomainCaptureStatus(a).
			Phase(snapshotsdk.PhasePlanning).
			Message(fmt.Sprintf("backing PersistentVolumeClaim %q is not bound yet (%s); waiting", pvcName, pvc.Status.Phase)).
			Apply(ctx); err != nil {
			return ctrl.Result{}, err
		}

		return ctrl.Result{RequeueAfter: requeueAfter}, nil
	}

	if planning && claim == nil {
		if err := sdk.DomainCaptureStatus(a).Phase(snapshotsdk.PhasePlanning).Message("").Apply(ctx); err != nil {
			return ctrl.Result{}, err
		}
	}

	var mirrored bool

	if sc := pvc.Spec.StorageClassName; sc != nil && *sc != "" && vds.Status.StorageClassName != *sc {
		vds.Status.StorageClassName = *sc
		mirrored = true
	}
	// Mirror what the disk declares, not what its claim happens to request: the two diverge once the
	// claim was grown past the declaration (a restore rounded up to the driver floor, a clone sized to
	// the source's capacity), and the restore path fails a disk asking for less than this value.
	if requestedSize := commonvd.RequestedSize(vd, pvc); requestedSize != "" && vds.Status.PersistentVolumeClaimSize != requestedSize {
		vds.Status.PersistentVolumeClaimSize = requestedSize
		mirrored = true
	}

	if mirrored {
		// Persist right away so it's not silently dropped
		if err := r.patchStatus(ctx, vds); err != nil {
			return ctrl.Result{}, err
		}
	}

	if err := sdk.PublishSnapshotSource(ctx, a, snapshotsdk.SnapshotSource{
		APIVersion: v1alpha2.SchemeGroupVersion.String(),
		Kind:       v1alpha2.VirtualDiskKind,
		Name:       vd.Name,
		Namespace:  vd.Namespace,
		UID:        vd.UID,
	}); err != nil {
		return ctrl.Result{}, err
	}

	// A snapshot taken on its own captures its disk under its own access. A child is captured under its
	// parent's, granted before the parent planned it.
	if parent == nil && claim == nil {
		claim, err = r.claimAccess(ctx, vds, vd)

		var refusal *captureRefusedError
		if errors.As(err, &refusal) {
			return r.failCapture(ctx, a, vds, nil, refusal.reason, refusal.message)
		}

		if err != nil {
			return ctrl.Result{}, err
		}
	}

	if planning && claim != nil && !accessclaim.IsGranted(claim) {
		return r.waitForAccess(ctx, sdk, a, claim)
	}

	if err := sdk.EnsureVolumeCapture(ctx, a, snapshotsdk.VolumeCaptureSpec{
		DataRef: &snapshotsdk.Target{
			UID:        string(pvc.UID),
			APIVersion: "v1",
			Kind:       "PersistentVolumeClaim",
			Name:       pvc.Name,
			Namespace:  pvc.Namespace,
		},
	}); err != nil {
		return ctrl.Result{}, err
	}

	if err := sdk.EnsureManifestCapture(ctx, a, snapshotsdk.ManifestCaptureSpec{
		Targets: []snapshotsdk.ManifestTarget{{
			APIVersion: v1alpha2.SchemeGroupVersion.String(),
			Kind:       v1alpha2.VirtualDiskKind,
			Name:       vd.Name,
		}},
	}); err != nil {
		return ctrl.Result{}, err
	}

	// Fails the capture on a storage error known to be terminal (e.g. a driver without snapshot support);
	// otherwise relays the storage's retry error through the Planned barrier write, because every
	// same-phase Planned write overwrites reason and message.
	retryReason, retryMessage, isTerminalError, err := r.dataCaptureRetry(ctx, a, vds)
	if err != nil {
		return ctrl.Result{}, err
	}

	if isTerminalError {
		return r.failCapture(ctx, a, vds, claim, retryReason, retryMessage)
	}

	if err := sdk.DomainCaptureStatus(a).
		Phase(snapshotsdk.PhasePlanned).
		Reason(snapshotsdk.Reason(retryReason)).
		Message(retryMessage).
		Apply(ctx); err != nil {
		return ctrl.Result{}, err
	}

	// Barrier 2 (Finished): switch on the SDK-derived capture outcome. The snapshotter core flips latches in the
	// Status.CommonController as capture progresses.
	switch outcome := snapshotsdk.CoreCaptureOutcome(a); outcome.Outcome {
	case snapshotsdk.CaptureOutcomeFailed:
		// A failed manifest/data leg is declared terminal by the CORE, not by us: the core marks the
		// bound SnapshotContent itself terminal (e.g. reason VolumeCaptureFailed) and that is what
		// CoreCaptureOutcome/ReadyStatus read here. Failed here rather than left in Planned, where the
		// completed claim would read as a successful capture.
		return r.failCapture(ctx, a, vds, claim, outcome.Reason, outcome.Message)
	case snapshotsdk.CaptureOutcomeCapturing:
		// Capturing: wait for the core to finish. The status watch wakes us on each leg latch flip;
		// use requeue as a fallback in case a signal is missed.
		vds.Status.Phase = v1alpha2.VirtualDiskSnapshotPhaseInProgress
		return ctrl.Result{RequeueAfter: requeueAfter}, r.patchStatus(ctx, vds)
	}

	if vds.Status.Data == nil {
		// The core mirrors the bound SnapshotContent's data binding onto status.data, and does it on its
		// own schedule. We don't watch SnapshotContent, so nothing else will wake this object up once that
		// lands — requeue explicitly instead of settling into Ready without a data binding to restore from.
		return ctrl.Result{RequeueAfter: requeueAfter}, nil
	}

	// The disk is captured: release the access and wait for the verdict, which decides the consistency.
	if claim != nil {
		if err := accessclaim.Release(ctx, r.Client, claim); err != nil {
			return ctrl.Result{}, err
		}

		switch {
		case accessclaim.IsRefused(claim):
			reason, message := accessclaim.GrantedCondition(claim)
			return r.failCapture(ctx, a, vds, claim, reason, message)
		case !accessclaim.IsCompleted(claim):
			vds.Status.Phase = v1alpha2.VirtualDiskSnapshotPhaseInProgress
			return ctrl.Result{}, r.patchStatus(ctx, vds)
		}
	}

	if err := sdk.DomainCaptureStatus(a).Phase(snapshotsdk.PhaseFinished).Apply(ctx); err != nil {
		return ctrl.Result{}, err
	}

	return r.finishAsReady(ctx, vds, parent, claim)
}

// dataCaptureRetry returns the domain reason and message for the storage error found in the Ready
// condition of the VolumeCaptureRequest behind the data leg; both are empty when there is no request or
// no error. isTerminalError is true for an error recognized as permanent, and the message is then the
// user-facing replacement for the storage text.
//
// The VolumeCaptureRequest never turns terminal on its own, so its error is relayed to the snapshot to
// be visible to the user.
func (r *Reconciler) dataCaptureRetry(ctx context.Context, a *adapter.VirtualDiskSnapshotAdapter, vds *v1alpha2.VirtualDiskSnapshot) (reason, message string, isTerminalError bool, err error) {
	vcrName := a.GetDomainCaptureState().VolumeCaptureRequestName
	if vcrName == "" || snapshotsdk.CoreCaptureOutcome(a).Outcome != snapshotsdk.CaptureOutcomeCapturing {
		return "", "", false, nil
	}

	// APIReader (uncached): the controller does not watch VolumeCaptureRequests, so a cached Get would
	// start a cluster-wide informer for the whole type to read a single object.
	vcr := &foundationv1alpha1.VolumeCaptureRequest{}
	if err := r.APIReader.Get(ctx, types.NamespacedName{Namespace: vds.Namespace, Name: vcrName}, vcr); err != nil {
		return "", "", false, client.IgnoreNotFound(err)
	}

	ready := meta.FindStatusCondition(vcr.Status.Conditions, foundationv1alpha1.ConditionTypeReady)
	if ready == nil || ready.Status != metav1.ConditionFalse || ready.Message == "" {
		return "", "", false, nil
	}

	if strings.Contains(ready.Message, snapshotsNotSupportedMarker) {
		return reasonSnapshotsNotSupported, snapshotsNotSupportedMessage(vds), true, nil
	}

	return reasonDataCaptureRetrying, fmt.Sprintf(
		"The storage failed to capture the data of VirtualDisk %q and keeps retrying. Reason: %s. %s",
		vds.SourceVirtualDiskName(), ready.Reason, ready.Message), false, nil
}

func snapshotsNotSupportedMessage(vds *v1alpha2.VirtualDiskSnapshot) string {
	storageClass := "the StorageClass"
	if sc := vds.Status.StorageClassName; sc != "" {
		storageClass = fmt.Sprintf("the StorageClass %q", sc)
	}

	return fmt.Sprintf(
		"The storage driver of %s does not support snapshots, so the snapshot of the VirtualDisk %q cannot be taken. "+
			"Use a VirtualDisk with the StorageClass that supports snapshots.",
		storageClass, vds.SourceVirtualDiskName())
}

// settleConsistency sets status.consistent to false on a terminal snapshot without a verdict, so the field
// is never absent on a terminal snapshot.
func settleConsistency(vds *v1alpha2.VirtualDiskSnapshot) {
	if vds.Status.Consistent == nil {
		vds.Status.Consistent = ptr.To(false)
	}
}

// reconcileImport reports the progress the core is making on an import-mode snapshot and touches nothing
// else. See the vmsnapshot controller's reconcileImport for why there is nothing here to drive, and why
// status.consistent is left alone.
//
// status.storageClassName and status.persistentVolumeClaimSize are left alone for the same reason: they
// mirror the disk as it was at capture time, and the disk that was captured is described by the manifests
// in the archive, not by anything reachable from here. They exist to default a NEW VirtualDisk cloned
// from this snapshot; a restore does not need them, because the archive carries the VirtualDisk with its
// own size and storage class.
func (r *Reconciler) reconcileImport(ctx context.Context, vds *v1alpha2.VirtualDiskSnapshot) (ctrl.Result, error) {
	phase := v1alpha2.VirtualDiskSnapshotPhasePending

	switch {
	case meta.IsStatusConditionTrue(vds.Status.Conditions, v1alpha2.UnifiedSnapshotterConditionReady):
		phase = v1alpha2.VirtualDiskSnapshotPhaseReady
	case vds.Status.BoundSnapshotContentName != "":
		phase = v1alpha2.VirtualDiskSnapshotPhaseInProgress
	}

	if vds.Status.Phase == phase {
		return ctrl.Result{}, nil
	}

	vds.Status.Phase = phase

	return ctrl.Result{}, r.patchStatus(ctx, vds)
}

// finishAsReady makes the snapshot Ready. A snapshot under its own access is consistent as its verdict says,
// and one of a disk that stayed unattached is consistent; a child waits for its parent (see
// settleChildConsistency).
func (r *Reconciler) finishAsReady(ctx context.Context, vds *v1alpha2.VirtualDiskSnapshot, parent *metav1.OwnerReference, claim *coordinationv1alpha1.VirtualMachineAccessClaim) (ctrl.Result, error) {
	switch {
	case claim != nil:
		vds.Status.Consistent = ptr.To(accessclaim.Consistent(claim))
	case parent == nil && startedOnUnattachedDisk(vds):
		vds.Status.Consistent = ptr.To(true)
	case parent == nil:
		settleConsistency(vds)
	}

	vds.Status.Phase = v1alpha2.VirtualDiskSnapshotPhaseReady

	logger.FromContext(ctx).Info("patch VDS phase to VirtualDiskSnapshotPhaseReady",
		slog.Bool("consistent", vds.Status.Consistent != nil && *vds.Status.Consistent))

	return ctrl.Result{}, r.patchStatus(ctx, vds)
}

func (r *Reconciler) finishAsFailed(ctx context.Context, vds *v1alpha2.VirtualDiskSnapshot) (ctrl.Result, error) {
	settleConsistency(vds)
	vds.Status.Phase = v1alpha2.VirtualDiskSnapshotPhaseFailed

	logger.FromContext(ctx).Info("patch VDS phase to VirtualDiskSnapshotPhaseFailed")

	return ctrl.Result{}, r.patchStatus(ctx, vds)
}

// reconcileCaptureFailed completes the phase of a capture already failed, and releases its access.
func (r *Reconciler) reconcileCaptureFailed(ctx context.Context, vds *v1alpha2.VirtualDiskSnapshot, claim *coordinationv1alpha1.VirtualMachineAccessClaim) (ctrl.Result, error) {
	if err := accessclaim.Release(ctx, r.Client, claim); err != nil {
		return ctrl.Result{}, err
	}

	return r.finishAsFailed(ctx, vds)
}

// failOnLostDisk fails a snapshot whose disk or its storage became unavailable while the snapshot held its
// access.
func (r *Reconciler) failOnLostDisk(ctx context.Context, a *adapter.VirtualDiskSnapshotAdapter, vds *v1alpha2.VirtualDiskSnapshot, claim *coordinationv1alpha1.VirtualMachineAccessClaim) (ctrl.Result, error) {
	return r.failCapture(ctx, a, vds, claim, string(vdscondition.VirtualDiskSnapshotFailed), fmt.Sprintf(
		"The virtual disk %q became unavailable while the snapshot was being taken. Take the snapshot again.", vds.SourceVirtualDiskName()))
}

// failCapture fails the snapshot and releases its access, so the coordinator unfreezes the guest at once
// instead of holding it until the session timeout.
func (r *Reconciler) failCapture(ctx context.Context, a *adapter.VirtualDiskSnapshotAdapter, vds *v1alpha2.VirtualDiskSnapshot, claim *coordinationv1alpha1.VirtualMachineAccessClaim, reason, message string) (ctrl.Result, error) {
	// A failed snapshot, such as one refused for a busy virtual machine, is not a controller fault.
	logger.FromContext(ctx).Info("failing the disk capture",
		slog.String("reason", reason), slog.String("message", message))

	if err := r.sdk().DomainCaptureStatus(a).
		Phase(snapshotsdk.PhaseFailed).
		Reason(snapshotsdk.Reason(reason)).
		Message(message).
		Apply(ctx); err != nil {
		return ctrl.Result{}, err
	}

	if err := accessclaim.Release(ctx, r.Client, claim); err != nil {
		return ctrl.Result{}, err
	}

	// A failed capture is not consistent, whatever a snapshot started before the module update recorded.
	vds.Status.Consistent = ptr.To(false)

	return r.finishAsFailed(ctx, vds)
}

var errMultipleAttachedVirtualMachines = errors.New("attached to multiple virtual machines")

// getAttachedVirtualMachine returns the one VirtualMachine the virtual disk is attached to.
//
// A nil VirtualMachine means there is nothing to freeze: the disk is attached to none, or the one it
// names is already gone. Several attached VirtualMachines are reported as
// errMultipleAttachedVirtualMachines instead — which one to freeze is ambiguous, so the caller decides
// what that means for the snapshot rather than mistaking it for a disk nobody uses.
func (r *Reconciler) getAttachedVirtualMachine(ctx context.Context, vd *v1alpha2.VirtualDisk) (*v1alpha2.VirtualMachine, error) {
	attached := vd.Status.AttachedToVirtualMachines

	if len(attached) == 0 {
		return nil, nil
	}

	if len(attached) > 1 {
		names := make([]string, 0, len(attached))
		for _, vm := range attached {
			names = append(names, vm.Name)
		}

		return nil, fmt.Errorf("the virtual disk %q is %w: %s", vd.Name, errMultipleAttachedVirtualMachines, strings.Join(names, ", "))
	}

	vm := &v1alpha2.VirtualMachine{}

	err := r.Client.Get(ctx, types.NamespacedName{Namespace: vd.Namespace, Name: attached[0].Name}, vm)
	if apierrors.IsNotFound(err) {
		return nil, nil
	}

	if err != nil {
		return nil, err
	}

	return vm, nil
}

// vdsOwnedStatus lists exactly the VirtualDiskSnapshotStatus fields this controller ever sets. Every
// other status field (captureState.commonController, boundSnapshotContentName, conditions, ...) is
// owned by the core/SDK and deliberately absent here, so patchStatus's merge patch never touches them —
// see internal/statuspatch.
type vdsOwnedStatus struct {
	Phase                     v1alpha2.VirtualDiskSnapshotPhase `json:"phase,omitempty"`
	Consistent                *bool                             `json:"consistent,omitempty"`
	StorageClassName          string                            `json:"storageClassName,omitempty"`
	PersistentVolumeClaimSize string                            `json:"persistentVolumeClaimSize,omitempty"`
}

func (r *Reconciler) patchStatus(ctx context.Context, vds *v1alpha2.VirtualDiskSnapshot) error {
	patch, err := statuspatch.For(v1alpha2.SchemeGroupVersion.WithKind(v1alpha2.VirtualDiskSnapshotKind), vdsOwnedStatus{
		Phase:                     vds.Status.Phase,
		Consistent:                vds.Status.Consistent,
		StorageClassName:          vds.Status.StorageClassName,
		PersistentVolumeClaimSize: vds.Status.PersistentVolumeClaimSize,
	})
	if err != nil {
		return err
	}

	return r.Client.Status().Patch(ctx, vds, patch)
}
