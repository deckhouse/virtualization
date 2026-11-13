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

// Package vmsnapshot drives VirtualMachineSnapshot capture through the state-snapshotter SDK
// (github.com/deckhouse/state-snapshotter/pkg/snapshotsdk). It is a manifest-only aggregator: its only
// child kind is VirtualDiskSnapshot (one per disk of the captured VirtualMachine), and it carries no data
// leg of its own.
//
// Consistency is a single whole-VM guest filesystem freeze held across all disks. The snapshot does not
// freeze the guest itself: it requests access to the virtual machine through a VirtualMachineAccessClaim,
// and the virtual machine access coordinator freezes the guest, watches the session and unfreezes it.
//
// The manifest leg (see manifest_targets.go) captures the same resource set as the old controller's
// Secret snapshot: the VirtualMachine itself, its VirtualMachineIPAddress, secondary-network
// VirtualMachineMACAddresses, the provisioner Secret, and hotplugged VirtualMachineBlockDeviceAttachments.
package vmsnapshot

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"k8s.io/utils/ptr"
	virtv1 "kubevirt.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/deckhouse/deckhouse/pkg/log"
	storagev1alpha1 "github.com/deckhouse/state-snapshotter/api/storage/v1alpha1"
	"github.com/deckhouse/state-snapshotter/pkg/snapshotsdk"
	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	"github.com/deckhouse/virtualization-controller/pkg/controller/accessclaim"
	"github.com/deckhouse/virtualization-controller/pkg/controller/unified-snapshotter/internal/adapter"
	"github.com/deckhouse/virtualization-controller/pkg/controller/unified-snapshotter/internal/annotation"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	"github.com/deckhouse/virtualization-controller/pkg/unifiedsnapshotter/statuspatch"
	coordinationv1alpha1 "github.com/deckhouse/virtualization/api/coordination/v1alpha1"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmscondition"
)

const (
	requeueAfter   = 2 * time.Second
	ControllerName = "virtualmachine-snapshot-controller"

	childrenSettleDeadline = 10 * time.Minute
	planningDeadline       = 2 * time.Minute

	// reasonInvalidSource is the terminal domain reason for a snapshot whose spec does not resolve to a
	// capturable source object.
	reasonInvalidSource = "InvalidSource"
)

// Reconciler drives VirtualMachineSnapshot capture through the state-snapshotter SDK.
type Reconciler struct {
	Client    client.Client
	APIReader client.Reader
	// Unfreezer thaws the guest a snapshot started before the module update may have left frozen.
	Unfreezer accessclaim.Unfreezer
	Recorder  eventrecord.EventRecorderLogger
	Log       *log.Logger
}

// SetupWithManager registers the reconciler, gated to the objects the unified mechanism owns — see
// annotation.DrivenByUnified. It also watches the access claims the snapshots own, so a decision of the
// coordinator wakes the snapshot up.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		Named(ControllerName).
		For(&v1alpha2.VirtualMachineSnapshot{}, builder.WithPredicates(annotation.ShouldHandle())).
		Owns(&coordinationv1alpha1.VirtualMachineAccessClaim{}).
		WithLogConstructor(logger.NewConstructor(r.Log)).
		Complete(r)
}

func (r *Reconciler) sdk() snapshotsdk.CaptureSDK {
	return snapshotsdk.New(r.Client, r.APIReader, snapshotsdk.NewStorageFoundationProvider(r.Client))
}

// Reconcile drives one VirtualMachineSnapshot, stopping quietly if it is deleted while the work is in
// flight.
//
// Only the first read is guarded by IgnoreNotFound; everything after it — the status patches, the
// finalizer write, the SDK calls that re-read the object — keeps touching a snapshot that a user, or an
// e2e teardown, may remove at any moment. Each of those then returns NotFound, which as a reconcile
// error is logged as a failure and retried with backoff against an object that is never coming back.
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	result, err := r.reconcile(ctx, req)
	if object.IsGone(err, v1alpha2.SchemeGroupVersion.WithResource(v1alpha2.VirtualMachineSnapshotResource).GroupResource(), req.Name) {
		logger.FromContext(ctx).Debug("the snapshot was deleted while it was being reconciled; nothing left to do")
		return ctrl.Result{}, nil
	}

	return result, err
}

func (r *Reconciler) reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	vms := &v1alpha2.VirtualMachineSnapshot{}
	if err := r.Client.Get(ctx, req.NamespacedName, vms); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if vms.DeletionTimestamp != nil {
		return r.reconcileDeletion(ctx, vms)
	}

	if !annotation.DrivenByUnifiedVirtualMachineSnapshot(vms) {
		// Defensive: the manager-level predicate already filters this, but Reconcile may be invoked
		// directly (e.g. by an owned-object watch) so re-check before touching this object.
		return ctrl.Result{}, nil
	}

	if vms.IsImport() {
		return r.reconcileImport(ctx, vms)
	}

	if !controllerutil.ContainsFinalizer(vms, v1alpha2.FinalizerVMSnapshotCleanup) {
		if err := r.patchFinalizer(ctx, vms, controllerutil.AddFinalizer); err != nil {
			return ctrl.Result{}, err
		}
	}

	if vms.Status.Phase == "" {
		vms.Status.Phase = v1alpha2.VirtualMachineSnapshotPhasePending

		logger.FromContext(ctx).Info("patch phase to VirtualMachineSnapshotPhasePending")

		if err := r.patchStatus(ctx, vms); err != nil {
			return ctrl.Result{}, err
		}

		return ctrl.Result{RequeueAfter: time.Second}, nil
	}

	a := &adapter.VirtualMachineSnapshotAdapter{VMS: vms}
	sdk := r.sdk()

	claim, err := accessclaim.Get(ctx, r.Client, r.APIReader, vms.Namespace, accessclaim.Name(vms))
	if err != nil {
		return ctrl.Result{}, err
	}

	switch a.GetDomainCaptureState().Phase {
	case snapshotsdk.PhaseFinished:
		return r.reconcileCaptured(ctx, vms, claim)
	case snapshotsdk.PhaseFailed:
		return r.reconcileCaptureFailed(ctx, vms, claim)
	}

	// A snapshot started before the module update has no claim, so it fails once the guest it may have left
	// frozen is unfrozen.
	if startedBeforeUpdate(vms, claim) {
		if err := r.unfreezeStartedBeforeUpdate(ctx, vms); err != nil {
			return ctrl.Result{}, err
		}

		return r.failCapture(ctx, a, vms, nil, string(vmscondition.AbortedByModuleUpdate), accessclaim.MsgAbortedByModuleUpdate)
	}

	// A decided claim settles the snapshot even when its virtual machine is gone.
	switch {
	case accessclaim.IsRefused(claim):
		reason, message := accessclaim.GrantedCondition(claim)
		return r.failCapture(ctx, a, vms, claim, reason, message)
	case accessclaim.IsCompleted(claim) && accessclaim.IsUnfreezing(claim):
		// The guest may still be frozen: the snapshot turns Ready once the coordinator has unfrozen it.
		vms.Status.Phase = v1alpha2.VirtualMachineSnapshotPhaseInProgress
		return ctrl.Result{}, r.patchStatus(ctx, vms)
	case accessclaim.IsCompleted(claim):
		return r.finishCapture(ctx, sdk, a, vms, claim)
	}

	// An unresolvable source is failed only after the Pending bootstrap above.
	vmName := vms.SourceVirtualMachineName()
	if vmName == "" {
		return r.failCapture(ctx, a, vms, claim, reasonInvalidSource, fmt.Sprintf(
			"spec.sourceRef must reference a %s %s", v1alpha2.SchemeGroupVersion.String(), v1alpha2.VirtualMachineKind))
	}

	vm := &v1alpha2.VirtualMachine{}
	if err := r.Client.Get(ctx, types.NamespacedName{Namespace: vms.Namespace, Name: vmName}, vm); err != nil {
		if apierrors.IsNotFound(err) {
			// The wait has no deadline, as no access is requested yet and the guest is not frozen. It covers a
			// snapshot applied together with its VirtualMachine.
			if perr := sdk.DomainCaptureStatus(a).
				Phase(snapshotsdk.PhasePlanning).
				Reason(snapshotsdk.Reason(vmscondition.WaitingForTheVirtualMachine)).
				Message(fmt.Sprintf("The VirtualMachine %q does not exist. Waiting for it to appear; snapshotting will continue on its own.", vmName)).
				Apply(ctx); perr != nil {
				return ctrl.Result{}, perr
			}

			return ctrl.Result{RequeueAfter: requeueAfter}, nil
		}

		return ctrl.Result{}, err
	}
	// Stamp Planning once, and afterwards only to clean up the "waiting for the VirtualMachine"
	// message after the machine was found.
	//
	// The first stamp is what makes this snapshot's children route to the unified mechanism: a child
	// with no capture state of its own adopts the parent's, so the parent must carry captureState
	// before EnsureChildren runs (see common/snapshotter.UseUnifiedForVirtualDiskSnapshot).
	//
	// Strictly check the reason as unconditional cleanup leads to a write-storm as the next branches
	// will fill the message again and re-enqueues the next reconcile immediately, ignoring RequeueAfter.
	if st := a.GetDomainCaptureState(); st.Phase == "" || st.Reason == string(vmscondition.WaitingForTheVirtualMachine) {
		if err := sdk.DomainCaptureStatus(a).Phase(snapshotsdk.PhasePlanning).Message("").Apply(ctx); err != nil {
			return ctrl.Result{}, err
		}
	}

	if err := sdk.PublishSnapshotSource(ctx, a, snapshotsdk.SnapshotSource{
		APIVersion: v1alpha2.SchemeGroupVersion.String(),
		Kind:       v1alpha2.VirtualMachineKind,
		Name:       vm.Name,
		Namespace:  vm.Namespace,
		UID:        vm.UID,
	}); err != nil {
		return ctrl.Result{}, err
	}

	// The children are planned only once the coordinator grants the access, so no disk is captured
	// before it.
	if claim == nil {
		claim, err = accessclaim.Create(ctx, r.Client, r.APIReader, vms, vm, vms.Spec.RequiredConsistency)
		if err != nil {
			return ctrl.Result{}, err
		}
	}

	if !accessclaim.IsGranted(claim) {
		return r.waitForAccess(ctx, sdk, a, claim)
	}

	domainState := a.GetDomainCaptureState()

	planningNotFrozen := domainState.Phase != snapshotsdk.PhasePlanned &&
		domainState.Phase != snapshotsdk.PhaseFinished &&
		domainState.Phase != snapshotsdk.PhaseFailed

	// The child set is planned only while it can still change. Once the node has declared barrier 1 the set
	// is frozen and re-deriving it buys nothing: with an unchanged set EnsureChildren is a pure no-op (its
	// pre-check passes and the status closure short-circuits without a write), while with a set that has
	// drifted — a disk vetoed, removed or recreated with a fresh UID since the barrier — it returns
	// ErrChildrenSetFrozen and turns an already-captured snapshot into a terminal failure. It cannot repair
	// a refs write the SDK's frozen-phase belt dropped either: that pre-check sees the published set as
	// smaller and fails closed for exactly the same reason. What the gates further down need after the
	// barrier comes from childVirtualDiskSnapshots, which answers from the cluster.
	//
	// Skipping it is also what keeps planChildren's uncached reads affordable: the children wait requeues
	// every couple of seconds for up to childrenSettleDeadline, and re-planning through it would mean a
	// direct API GET per disk per tick for no effect.
	if planningNotFrozen {
		children, excluded, err := r.planChildren(ctx, vms, vm)
		switch {
		case err == nil:
			if err := sdk.EnsureChildren(ctx, a, children, excluded); err != nil {
				switch {
				case apierrors.IsConflict(err):
					return ctrl.Result{RequeueAfter: requeueAfter}, nil
				case errors.Is(err, snapshotsdk.ErrChildrenSetFrozen):
					// Growth of a frozen point-in-time set. Terminal, and the reaction the SDK prescribes.
					return r.failCapture(ctx, a, vms, claim, storagev1alpha1.ReasonGraphPlanningFailed, err.Error())
				default:
					// Transport or API trouble. Retry with the workqueue's backoff rather than burning the snapshot.
					return ctrl.Result{}, err
				}
			}
		case errors.Is(err, errSourceNotReady):
			return r.waitForSource(ctx, sdk, a, vms, vm, claim, err)
		default:
			return ctrl.Result{}, err
		}
	}

	// The manifest leg is planned exactly once, gated by the SDK's own predicate for it: the MCR name is
	// published on the first successful EnsureManifestCapture and its Targets are immutable afterwards
	// (CEL-enforced at the apiserver), so re-deriving the set every reconcile can only produce spurious
	// failures — a VMBDA or Secret deleted after the plan was committed would fail an established capture.
	if snapshotsdk.ManifestCaptureNeeded(a) {
		targets, tErr := r.planManifestTargets(ctx, vms, vm)
		switch {
		case tErr == nil:
		case apierrors.IsConflict(tErr):
			return ctrl.Result{RequeueAfter: requeueAfter}, nil
		case errors.Is(tErr, errSourceNotReady):
			return r.waitForSource(ctx, sdk, a, vms, vm, claim, tErr)
		case errors.Is(tErr, errInvalidSourceSpec):
			return r.failCapture(ctx, a, vms, claim, storagev1alpha1.ReasonGraphPlanningFailed, tErr.Error())
		default:
			return ctrl.Result{}, tErr
		}

		if err := sdk.EnsureManifestCapture(ctx, a, snapshotsdk.ManifestCaptureSpec{Targets: targets}); err != nil {
			switch {
			case apierrors.IsConflict(err):
				return ctrl.Result{RequeueAfter: requeueAfter}, nil
			case errors.Is(err, snapshotsdk.ErrManifestTargetsDrift):
				// The freshly declared set diverges from the frozen MCR. Terminal, per the SDK's guidance.
				return r.failCapture(ctx, a, vms, claim, storagev1alpha1.ReasonGraphPlanningFailed, err.Error())
			default:
				return ctrl.Result{}, err
			}
		}
	}

	if err := sdk.DomainCaptureStatus(a).Phase(snapshotsdk.PhasePlanned).Apply(ctx); err != nil {
		return ctrl.Result{}, err
	}

	outcome := snapshotsdk.CoreCaptureOutcome(a)
	switch outcome.Outcome {
	case snapshotsdk.CaptureOutcomeFailed:
		// Failed here rather than left in Planned, where the completed claim would read as a successful
		// capture.
		return r.failCapture(ctx, a, vms, claim, outcome.Reason, outcome.Message)
	case snapshotsdk.CaptureOutcomeCapturing:
		vms.Status.Phase = v1alpha2.VirtualMachineSnapshotPhaseInProgress

		logger.FromContext(ctx).Info("patch VMS phase to VirtualMachineSnapshotPhaseInProgress")

		return ctrl.Result{RequeueAfter: requeueAfter}, r.patchStatus(ctx, vms)
	}

	// Own manifest leg captured; wait for every child VirtualDiskSnapshot to go terminal before releasing
	// the access, so one freeze covers all disks. Gated on the core-computed
	// ChildrenSettled latch, not by inspecting each child's own legs via ChildrenCaptureStates: per the
	// SDK's own doc comment on ChildCaptureState, that per-child view is diagnostics-only and must not
	// gate a consistency action — it hangs forever on a child that fails with a domain-specific reason
	// outside the SDK's terminal-reason list. ChildrenSettled is the intended completeness signal
	// (true once every direct child is terminal, success or failure).
	//
	// The locally planned set is not used: it is nil when this pass skipped or failed re-planning.
	childSnapshots, err := r.childVirtualDiskSnapshots(ctx, vms)
	if err != nil {
		return ctrl.Result{}, err
	}

	if !childrenSettled(childSnapshots, a.CoreCaptureState()) {
		// A VolumeCaptureRequest has no guaranteed final state: storage-foundation keeps retrying a CSI
		// driver without a cap, so a child can stay non-terminal forever and ChildrenSettled never flips.
		// The guest stays frozen during this wait, so it needs an end.
		waited, pending := childrenWaitProgress(vms, childSnapshots)
		if waited > childrenSettleDeadline {
			return r.failCapture(ctx, a, vms, claim, string(vmscondition.VirtualMachineSnapshotFailed), fmt.Sprintf(
				"giving up after %s so the guest filesystem is not held frozen indefinitely: "+
					"VirtualDiskSnapshots %v have not finished capturing data%s",
				childrenSettleDeadline, pending, pendingChildrenDetails(childSnapshots)))
		}

		vms.Status.Phase = v1alpha2.VirtualMachineSnapshotPhaseInProgress

		logger.FromContext(ctx).Info("patch VMS phase to VirtualMachineSnapshotPhaseInProgress, children not settled")

		return ctrl.Result{RequeueAfter: requeueAfter}, r.patchStatus(ctx, vms)
	}

	// Fails on a failed child read directly from the children: ChildrenSettled also counts failed children,
	// and the core propagates a child failure into Ready on its own schedule, so this snapshot could otherwise
	// turn Ready with a failed disk.
	if failed, details := failedChildren(childSnapshots); len(failed) > 0 {
		return r.failCapture(ctx, a, vms, claim, string(vmscondition.VirtualMachineSnapshotFailed), fmt.Sprintf(
			"VirtualDiskSnapshots %v have failed%s", failed, details))
	}

	// Every disk is captured: release the access. The coordinator unfreezes the guest and decides whether
	// the session went through without violations; the claim watch brings the verdict back here.
	if err := accessclaim.Release(ctx, r.Client, claim); err != nil {
		return ctrl.Result{}, err
	}

	vms.Status.Phase = v1alpha2.VirtualMachineSnapshotPhaseInProgress

	return ctrl.Result{}, r.patchStatus(ctx, vms)
}

// waitForAccess holds the node in Planning until the coordinator decides on the claim, and publishes why
// it waits (see accessclaim.Waiting).
func (r *Reconciler) waitForAccess(
	ctx context.Context,
	sdk snapshotsdk.CaptureSDK,
	a *adapter.VirtualMachineSnapshotAdapter,
	claim *coordinationv1alpha1.VirtualMachineAccessClaim,
) (ctrl.Result, error) {
	reason, message := accessclaim.Waiting(claim)

	st := a.GetDomainCaptureState()
	if st.Reason != reason || st.Message != message {
		if err := sdk.DomainCaptureStatus(a).
			Phase(snapshotsdk.PhasePlanning).
			Reason(snapshotsdk.Reason(reason)).
			Message(message).
			Apply(ctx); err != nil {
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{RequeueAfter: requeueAfter}, nil
}

// finishCapture turns a capture whose access completed into a Ready snapshot, consistent as the verdict of
// the claim says.
func (r *Reconciler) finishCapture(
	ctx context.Context,
	sdk snapshotsdk.CaptureSDK,
	a *adapter.VirtualMachineSnapshotAdapter,
	vms *v1alpha2.VirtualMachineSnapshot,
	claim *coordinationv1alpha1.VirtualMachineAccessClaim,
) (ctrl.Result, error) {
	if err := sdk.DomainCaptureStatus(a).Phase(snapshotsdk.PhaseFinished).Apply(ctx); err != nil {
		return ctrl.Result{}, err
	}

	vms.Status.Phase = v1alpha2.VirtualMachineSnapshotPhaseReady

	vms.Status.Consistent = ptr.To(accessclaim.Consistent(claim))
	if message, notConfirmed := accessclaim.UnfreezeNotConfirmed(claim); notConfirmed {
		r.Recorder.Event(vms, corev1.EventTypeWarning, v1alpha2.ReasonFilesystemUnfreezeNotConfirmed, message)
	}

	logger.FromContext(ctx).Info("patch VMS phase to VirtualMachineSnapshotPhaseReady")

	return ctrl.Result{}, r.patchStatus(ctx, vms)
}

func childrenWaitProgress(vms *v1alpha2.VirtualMachineSnapshot, children []v1alpha2.VirtualDiskSnapshot) (time.Duration, []string) {
	var (
		oldest  *metav1.Time
		pending []string
	)
	for _, vds := range children {
		if !vds.CreationTimestamp.IsZero() && (oldest == nil || vds.CreationTimestamp.Before(oldest)) {
			oldest = vds.CreationTimestamp.DeepCopy()
		}

		switch vds.Status.Phase {
		case v1alpha2.VirtualDiskSnapshotPhaseReady, v1alpha2.VirtualDiskSnapshotPhaseFailed:
		default:
			pending = append(pending, vds.Name)
		}
	}

	sort.Strings(pending)

	if oldest == nil {
		// No child carries a creation timestamp — every one of them is a placeholder for a published ref
		// whose object is gone. Nothing will ever report progress, so fall back to this node's own age
		// rather than reporting zero, which would hold the guest frozen forever.
		return time.Since(vms.CreationTimestamp.Time), pending
	}

	return time.Since(oldest.Time), pending
}

// childDetail renders the reason and message of a child VirtualDiskSnapshot, or "" when it has no message.
//
// It prefers the domain captureState, which carries the storage error relayed by the disk controller,
// and falls back to the core-owned Ready condition.
func childDetail(vds *v1alpha2.VirtualDiskSnapshot) string {
	a := &adapter.VirtualDiskSnapshotAdapter{VDS: vds}

	reason, message := a.GetDomainCaptureState().Reason, a.GetDomainCaptureState().Message
	if message == "" {
		reason, message = a.ReadyReason(), a.ReadyMessage()
	}

	if message == "" {
		return ""
	}

	return fmt.Sprintf("VirtualDiskSnapshot %q: %s: %s", vds.Name, reason, message)
}

// joinDetails renders child details as a message suffix ("" when there is none).
func joinDetails(details []string) string {
	if len(details) == 0 {
		return ""
	}

	sort.Strings(details)

	return ". " + strings.Join(details, "; ")
}

// pendingChildrenDetails renders why each non-terminal child VirtualDiskSnapshot has not finished, as a
// suffix for the timeout message ("" when no child reports a reason).
func pendingChildrenDetails(children []v1alpha2.VirtualDiskSnapshot) string {
	var details []string

	for i := range children {
		if isTerminalChild(&children[i]) {
			continue
		}

		if d := childDetail(&children[i]); d != "" {
			details = append(details, d)
		}
	}

	return joinDetails(details)
}

func isTerminalChild(vds *v1alpha2.VirtualDiskSnapshot) bool {
	return vds.Status.Phase == v1alpha2.VirtualDiskSnapshotPhaseReady || vds.Status.Phase == v1alpha2.VirtualDiskSnapshotPhaseFailed
}

// failedChildren returns the names of the children that failed and a message suffix with their reasons.
//
// A child counts as failed when either its phase or its domain capture phase says so: the core flips
// ChildrenSettled off the domain phase, which the disk controller writes before it patches the phase, so
// reading only the phase would miss a failure the settled latch has already counted.
func failedChildren(children []v1alpha2.VirtualDiskSnapshot) ([]string, string) {
	var (
		names   []string
		details []string
	)

	for i := range children {
		vds := &children[i]

		a := &adapter.VirtualDiskSnapshotAdapter{VDS: vds}
		if vds.Status.Phase != v1alpha2.VirtualDiskSnapshotPhaseFailed && a.GetDomainCaptureState().Phase != snapshotsdk.PhaseFailed {
			continue
		}

		names = append(names, vds.Name)
		if d := childDetail(vds); d != "" {
			details = append(details, d)
		}
	}

	sort.Strings(names)

	return names, joinDetails(details)
}

// failCapture fails the snapshot and releases its access, so the coordinator unfreezes the guest at once
// instead of holding it until the session timeout.
func (r *Reconciler) failCapture(
	ctx context.Context,
	a *adapter.VirtualMachineSnapshotAdapter,
	vms *v1alpha2.VirtualMachineSnapshot,
	claim *coordinationv1alpha1.VirtualMachineAccessClaim,
	reason, message string,
) (ctrl.Result, error) {
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

	vms.Status.Phase = v1alpha2.VirtualMachineSnapshotPhaseFailed
	settleConsistency(vms)
	logger.FromContext(ctx).Info("patch VMS phase to VirtualMachineSnapshotPhaseFailed", "reason", reason, "message", message)

	return ctrl.Result{}, r.patchStatus(ctx, vms)
}

// settleConsistency sets status.consistent to false on a terminal snapshot without a verdict, so the field
// is never absent on a terminal snapshot.
func settleConsistency(vms *v1alpha2.VirtualMachineSnapshot) {
	if vms.Status.Consistent == nil {
		vms.Status.Consistent = ptr.To(false)
	}
}

// childVirtualDiskSnapshots resolves the child VirtualDiskSnapshots this node must wait on.
//
// status.childrenSnapshotRefs is the published, authoritative set whenever it is non-empty. An EMPTY set
// is ambiguous: either the node is genuinely childless (a machine with no disk devices, or one whose every
// disk was vetoed), or its refs write never landed. EnsureChildren's StatusFromCurrent closure is
// fail-closed and DROPS that write when the node races into a frozen phase between its pre-check read and
// the patch retry — it returns nil while the child CRs it already created stay unpublished, which the SDK
// documents as its TOCTOU belt.
//
// Telling the two apart gates three decisions below: whether to keep waiting, whether the wait has run out
// of time, and whether the snapshot may claim consistency. Read as "childless", a dropped write makes this
// controller unfreeze the guest, declare the snapshot consistent and mark it Ready while its children are
// still capturing data. So on an empty set the answer comes from the cluster instead, by ownerRef — the
// relation children.Reconcile established before the publication it may not have reached.
//
// The fallback List is uncached on purpose: the children may have been created moments ago in this very
// reconcile, and a lagging informer would report the childless answer this function exists to rule out.
func (r *Reconciler) childVirtualDiskSnapshots(ctx context.Context, vms *v1alpha2.VirtualMachineSnapshot) ([]v1alpha2.VirtualDiskSnapshot, error) {
	if len(vms.Status.ChildrenSnapshotRefs) > 0 {
		children := make([]v1alpha2.VirtualDiskSnapshot, 0, len(vms.Status.ChildrenSnapshotRefs))
		for _, ref := range vms.Status.ChildrenSnapshotRefs {
			if ref.Kind != v1alpha2.VirtualDiskSnapshotKind {
				continue
			}

			vds := &v1alpha2.VirtualDiskSnapshot{}
			if err := r.Client.Get(ctx, types.NamespacedName{Namespace: vms.Namespace, Name: ref.Name}, vds); err != nil {
				if apierrors.IsNotFound(err) {
					// A published child that is gone still counts as declared: it cannot report itself
					// terminal or vouch for its disk, and dropping it here would silently shrink the set.
					children = append(children, v1alpha2.VirtualDiskSnapshot{
						ObjectMeta: metav1.ObjectMeta{Name: ref.Name, Namespace: vms.Namespace},
					})

					continue
				}

				return nil, err
			}

			children = append(children, *vds)
		}

		return children, nil
	}

	list := &v1alpha2.VirtualDiskSnapshotList{}
	if err := r.APIReader.List(ctx, list, client.InNamespace(vms.Namespace)); err != nil {
		return nil, err
	}

	owned := make([]v1alpha2.VirtualDiskSnapshot, 0, len(list.Items))
	for _, vds := range list.Items {
		for _, ref := range vds.OwnerReferences {
			if ref.UID == vms.UID {
				owned = append(owned, vds)
				break
			}
		}
	}

	return owned, nil
}

// childrenSettled reports whether every direct child snapshot has gone terminal (captured or failed),
// reading the core latch the way the SDK itself does: nil means "no children, or not computed yet" and
// must read as false. children comes from childVirtualDiskSnapshots, which is what makes the childless
// shortcut trustworthy.
func childrenSettled(children []v1alpha2.VirtualDiskSnapshot, cs snapshotsdk.CoreCaptureState) bool {
	if len(children) == 0 {
		return true
	}

	return cs.ChildrenSettled != nil && *cs.ChildrenSettled
}

// waitForSource holds the node in Planning while an object the virtual machine references is missing, and
// fails the capture past planningDeadline, counted from planningStartedAt: the guest may already be frozen.
func (r *Reconciler) waitForSource(
	ctx context.Context,
	sdk snapshotsdk.CaptureSDK,
	a *adapter.VirtualMachineSnapshotAdapter,
	vms *v1alpha2.VirtualMachineSnapshot,
	vm *v1alpha2.VirtualMachine,
	claim *coordinationv1alpha1.VirtualMachineAccessClaim,
	err error,
) (ctrl.Result, error) {
	if time.Since(planningStartedAt(vms, vm, claim)) > planningDeadline {
		return r.failCapture(ctx, a, vms, claim, storagev1alpha1.ReasonGraphPlanningFailed, fmt.Sprintf(
			"giving up after %s: %s", planningDeadline, err))
	}

	if perr := sdk.DomainCaptureStatus(a).
		Phase(snapshotsdk.PhasePlanning).
		Message(fmt.Sprintf("waiting for a resource referenced by the virtual machine: %s", err)).
		Apply(ctx); perr != nil {
		return ctrl.Result{}, perr
	}

	return ctrl.Result{RequeueAfter: requeueAfter}, nil
}

// planningStartedAt is the latest of the snapshot creation, the VirtualMachine creation and the grant of the
// access, so the wait for the virtual machine or for the freeze is not charged to the planning budget.
func planningStartedAt(vms *v1alpha2.VirtualMachineSnapshot, vm *v1alpha2.VirtualMachine, claim *coordinationv1alpha1.VirtualMachineAccessClaim) time.Time {
	started := vms.CreationTimestamp.Time
	if vm != nil && vm.CreationTimestamp.After(started) {
		started = vm.CreationTimestamp.Time
	}

	if claim != nil && claim.Status.GrantedAt != nil && claim.Status.GrantedAt.After(started) {
		started = claim.Status.GrantedAt.Time
	}

	return started
}

// reconcileImport is the whole of this controller's part in an import: it reports the progress the core
// is making, and touches nothing else.
//
// An import-mode snapshot has no capture to drive. Its manifests arrive through the
// manifests-and-children-refs-upload subresource, its children are recorded there too, and the core's
// binder assembles the SnapshotContent and decides readiness. There is no source VirtualMachine to read
// — spec forbids naming one — nothing to freeze, and so nothing to unfreeze either, which is why an
// import takes no cleanup finalizer.
//
// status.consistent is deliberately left alone. Consistency is a fact about the capture this archive came
// from, recorded in the manifests it carries; spec.requiredConsistency on an import marker is the CRD's
// default and says nothing about it. Reporting false there would be a claim about the original capture
// that this controller has no way to make.
func (r *Reconciler) reconcileImport(ctx context.Context, vms *v1alpha2.VirtualMachineSnapshot) (ctrl.Result, error) {
	// Pending until the binder binds a SnapshotContent, InProgress while the uploaded payload is being
	// assembled into it, Ready once the core says the content is. Failed has no mapping on purpose: the
	// core does not fail an import node, it leaves it unready, and the reason lives in the conditions it
	// owns.
	phase := v1alpha2.VirtualMachineSnapshotPhasePending

	switch {
	case meta.IsStatusConditionTrue(vms.Status.Conditions, v1alpha2.UnifiedSnapshotterConditionReady):
		phase = v1alpha2.VirtualMachineSnapshotPhaseReady
	case vms.Status.BoundSnapshotContentName != "":
		phase = v1alpha2.VirtualMachineSnapshotPhaseInProgress
	}

	// The children are compared too, not just the phase. They are recorded on status by the upload
	// subresource, which lands while the phase is already InProgress and does not move it — so a
	// phase-only comparison would return early here and never republish them under the name users read.
	if vms.Status.Phase == phase && slices.Equal(vms.Status.VirtualDiskSnapshotNames, vdSnapshotNames(vms.Status.ChildrenSnapshotRefs)) {
		return ctrl.Result{}, nil
	}

	vms.Status.Phase = phase

	return ctrl.Result{}, r.patchStatus(ctx, vms)
}

// reconcileCaptured completes the phase of a capture already finished. A snapshot finished before the module
// update has no claim and is not consistent.
func (r *Reconciler) reconcileCaptured(ctx context.Context, vms *v1alpha2.VirtualMachineSnapshot, claim *coordinationv1alpha1.VirtualMachineAccessClaim) (ctrl.Result, error) {
	if vms.Status.Phase == v1alpha2.VirtualMachineSnapshotPhaseReady {
		return ctrl.Result{}, nil
	}

	if claim != nil {
		vms.Status.Consistent = ptr.To(accessclaim.Consistent(claim))
	}

	settleConsistency(vms)

	vms.Status.Phase = v1alpha2.VirtualMachineSnapshotPhaseReady

	logger.FromContext(ctx).Info("patch VMS phase to VirtualMachineSnapshotPhaseReady, capture already finished")

	return ctrl.Result{}, r.patchStatus(ctx, vms)
}

// reconcileCaptureFailed completes the phase of a capture already failed, and releases its access. A capture
// failed before the module update unfreezes the guest first, as the controller before the update wrote the
// failure before the unfreeze.
func (r *Reconciler) reconcileCaptureFailed(ctx context.Context, vms *v1alpha2.VirtualMachineSnapshot, claim *coordinationv1alpha1.VirtualMachineAccessClaim) (ctrl.Result, error) {
	if vms.Status.Phase == v1alpha2.VirtualMachineSnapshotPhaseFailed {
		return ctrl.Result{}, nil
	}

	if startedBeforeUpdate(vms, claim) {
		if err := r.unfreezeStartedBeforeUpdate(ctx, vms); err != nil {
			return ctrl.Result{}, err
		}
	}

	if err := accessclaim.Release(ctx, r.Client, claim); err != nil {
		return ctrl.Result{}, err
	}

	vms.Status.Phase = v1alpha2.VirtualMachineSnapshotPhaseFailed
	settleConsistency(vms)
	logger.FromContext(ctx).Info("patch VMS phase to VirtualMachineSnapshotPhaseFailed, capture already failed")

	return ctrl.Result{}, r.patchStatus(ctx, vms)
}

// reconcileDeletion releases the access of the snapshot, then removes its finalizer; the claim's own
// finalizer keeps the claim until the guest is unfrozen. A snapshot in progress or failed without a claim may
// have been started before the module update, so it unfreezes the guest itself.
func (r *Reconciler) reconcileDeletion(ctx context.Context, vms *v1alpha2.VirtualMachineSnapshot) (ctrl.Result, error) {
	if !controllerutil.ContainsFinalizer(vms, v1alpha2.FinalizerVMSnapshotCleanup) {
		return ctrl.Result{}, nil
	}

	claim, err := accessclaim.Get(ctx, r.Client, r.APIReader, vms.Namespace, accessclaim.Name(vms))
	if err != nil {
		return ctrl.Result{}, err
	}

	if startedBeforeUpdate(vms, claim) || (claim == nil && vms.Status.Phase == v1alpha2.VirtualMachineSnapshotPhaseFailed) {
		if err := r.unfreezeStartedBeforeUpdate(ctx, vms); err != nil {
			return ctrl.Result{}, err
		}
	}

	if err := accessclaim.Release(ctx, r.Client, claim); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, r.patchFinalizer(ctx, vms, controllerutil.RemoveFinalizer)
}

// annFSFreezeRequestedAt is put on a snapshot by the controller before the module update once it asks the
// guest to freeze, which it does while the snapshot is still Pending.
const annFSFreezeRequestedAt = annotations.AnnAPIGroupV + "/fs-freeze-requested-at"

// startedBeforeUpdate reports a snapshot in progress, without an access claim, started before the module
// update: it asked for a freeze, or got past the point where a snapshot creates its claim (leaving Pending
// or planning a child). A snapshot without a phase is new, as that controller set the phase before freezing.
func startedBeforeUpdate(vms *v1alpha2.VirtualMachineSnapshot, claim *coordinationv1alpha1.VirtualMachineAccessClaim) bool {
	switch {
	case claim != nil,
		vms.Status.Phase == v1alpha2.VirtualMachineSnapshotPhaseReady,
		vms.Status.Phase == v1alpha2.VirtualMachineSnapshotPhaseFailed:
		return false
	}

	if _, ok := vms.Annotations[annFSFreezeRequestedAt]; ok {
		return true
	}

	switch vms.Status.Phase {
	case "":
		return false
	case v1alpha2.VirtualMachineSnapshotPhasePending:
		return len(vms.Status.ChildrenSnapshotRefs) > 0
	}

	return true
}

// unfreezeStartedBeforeUpdate unfreezes the guest of the virtual machine a snapshot started before the module
// update may have left frozen. That freeze has no safety unfreeze.
func (r *Reconciler) unfreezeStartedBeforeUpdate(ctx context.Context, vms *v1alpha2.VirtualMachineSnapshot) error {
	name := vms.SourceVirtualMachineName()
	if name == "" {
		return nil
	}

	kvvmi := &virtv1.VirtualMachineInstance{}
	if err := r.Client.Get(ctx, types.NamespacedName{Namespace: vms.Namespace, Name: name}, kvvmi); err != nil {
		return client.IgnoreNotFound(err)
	}

	return accessclaim.UnfreezeStartedBeforeUpdate(ctx, r.APIReader, r.Unfreezer, kvvmi)
}

// patchFinalizer adds or removes this controller's cleanup finalizer, refreshing the object from the API
// server on each attempt.
//
// Unlike an annotation, a finalizer list cannot be merged: a JSON merge patch replaces it wholesale, so a
// patch built from a stale read would silently drop a finalizer added meanwhile — the API server's own
// foregroundDeletion among them, which would break cascading deletion. The list is therefore written
// under an optimistic lock, which turns that silent loss back into a conflict, and the conflict is
// retried against a fresh uncached read instead of being returned to the caller. The uncached read
// matters: the object arrives here through the informer cache, whose copy can already be behind by the
// time a snapshot is first reconciled, which is exactly when this finalizer is added.
func (r *Reconciler) patchFinalizer(
	ctx context.Context,
	vms *v1alpha2.VirtualMachineSnapshot,
	mutate func(client.Object, string) bool,
) error {
	key := client.ObjectKeyFromObject(vms)

	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		if err := r.APIReader.Get(ctx, key, vms); err != nil {
			return err
		}

		base := vms.DeepCopy()
		if !mutate(vms, v1alpha2.FinalizerVMSnapshotCleanup) {
			return nil
		}

		return r.Client.Patch(ctx, vms, client.MergeFromWithOptions(base, client.MergeFromWithOptimisticLock{}))
	})
}

// vmsOwnedStatus lists exactly the VirtualMachineSnapshotStatus fields this controller ever sets. Every
// other status field (captureState.commonController, boundSnapshotContentName, conditions,
// childrenSnapshotRefs, ...) is owned by the core/SDK and deliberately absent here, so patchStatus's
// merge patch never touches them — see internal/statuspatch.
type vmsOwnedStatus struct {
	Phase                    v1alpha2.VirtualMachineSnapshotPhase `json:"phase,omitempty"`
	Consistent               *bool                                `json:"consistent,omitempty"`
	VirtualDiskSnapshotNames []string                             `json:"virtualDiskSnapshotNames,omitempty"`
}

func (r *Reconciler) patchStatus(ctx context.Context, vms *v1alpha2.VirtualMachineSnapshot) error {
	patch, err := statuspatch.For(v1alpha2.SchemeGroupVersion.WithKind(v1alpha2.VirtualMachineSnapshotKind), vmsOwnedStatus{
		Phase:                    vms.Status.Phase,
		Consistent:               vms.Status.Consistent,
		VirtualDiskSnapshotNames: vdSnapshotNames(vms.Status.ChildrenSnapshotRefs),
	})
	if err != nil {
		return err
	}

	return r.Client.Status().Patch(ctx, vms, patch)
}

func vdSnapshotNames(refs []v1alpha2.UnifiedSnapshotterChildRef) []string {
	names := make([]string, 0, len(refs))
	for _, ref := range refs {
		if ref.Kind == v1alpha2.VirtualDiskSnapshotKind {
			names = append(names, ref.Name)
		}
	}

	sort.Strings(names)

	return names
}

// planChildren builds the desired VirtualDiskSnapshot set: one per disk device attached to vm, deriving
// each a deterministic name so EnsureChildren's create-or-adopt stays idempotent across reconciles. Each
// child is driven by this same SDK-based controller family (vdsnapshot), never by the built-in one,
// because it resolves its mechanism from this parent's captureState.
//
// The disks are read through APIReader, uncached, for the same reason planManifestTargets is: this decides
// a point-in-time set the SDK freezes at barrier 1. Two things a lagging cache gets wrong here, and both
// are one-way doors — the state-snapshotter.deckhouse.io/exclude veto label, so a disk the user excluded
// is captured anyway; and the disk UID, which is the whole child name (ChildSnapshotName), so a stale UID
// after a same-name recreation makes the set look grown and EnsureChildren reject it as frozen.
func (r *Reconciler) planChildren(ctx context.Context, vms *v1alpha2.VirtualMachineSnapshot, vm *v1alpha2.VirtualMachine) ([]snapshotsdk.ChildSpec, []snapshotsdk.ExcludedObjectRef, error) {
	candidates := make([]client.Object, 0, len(vm.Status.BlockDeviceRefs))
	for _, bdr := range vm.Status.BlockDeviceRefs {
		if bdr.Kind != v1alpha2.DiskDevice {
			continue
		}

		vd := &v1alpha2.VirtualDisk{}

		err := r.APIReader.Get(ctx, types.NamespacedName{Namespace: vms.Namespace, Name: bdr.Name}, vd)
		switch {
		case apierrors.IsNotFound(err):
			return nil, nil, sourceNotReady(v1alpha2.VirtualDiskKind, bdr.Name)
		case err != nil:
			return nil, nil, fmt.Errorf("get VirtualDisk %s/%s: %w", vms.Namespace, bdr.Name, err)
		}

		candidates = append(candidates, vd)
	}

	kept, vetoed := snapshotsdk.PartitionExcluded(candidates)

	specs := make([]snapshotsdk.ChildSpec, 0, len(kept))
	for _, obj := range kept {
		child := &v1alpha2.VirtualDiskSnapshot{
			ObjectMeta: childObjectMeta(vms, obj.GetUID()),
			Spec: v1alpha2.VirtualDiskSnapshotSpec{
				SourceRef: &v1alpha2.UnifiedSnapshotterSpecSourceRef{
					APIVersion: v1alpha2.SchemeGroupVersion.String(),
					Kind:       v1alpha2.VirtualDiskKind,
					Name:       obj.GetName(),
				},
				RequiredConsistency: vms.Spec.RequiredConsistency,
			},
		}
		specs = append(specs, snapshotsdk.ChildSpec{Object: child})
	}

	excluded := make([]snapshotsdk.ExcludedObjectRef, 0, len(vetoed))
	for _, obj := range vetoed {
		excluded = append(excluded, snapshotsdk.ExcludedObjectRef{
			APIVersion: v1alpha2.SchemeGroupVersion.String(),
			Kind:       v1alpha2.VirtualDiskKind,
			Name:       obj.GetName(),
		})
	}

	return specs, excluded, nil
}

// childObjectMeta carries no mechanism annotation: a child derives its mechanism from this parent's
// status instead (see common/snapshotter.UseUnifiedForVirtualDiskSnapshot). Stamping it would pin the
// child to whatever the parent decided at creation time, and the annotations are on their way out.
func childObjectMeta(vms *v1alpha2.VirtualMachineSnapshot, sourceUID types.UID) metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:      snapshotsdk.ChildSnapshotName(vms.UID, sourceUID),
		Namespace: vms.Namespace,
	}
}
