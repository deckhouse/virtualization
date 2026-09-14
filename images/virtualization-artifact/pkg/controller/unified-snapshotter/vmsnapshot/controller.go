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
// child kind is VirtualDiskSnapshot (one per disk of the captured VirtualMachine), consistency is a
// single whole-VM guest-agent filesystem freeze held across all of them, and it carries no data leg of
// its own.
//
// The manifest leg (see manifest_targets.go) captures the same resource set as the old controller's
// Secret snapshot: the VirtualMachine itself, its VirtualMachineIPAddress, secondary-network
// VirtualMachineMACAddresses, the provisioner Secret, and hotplugged VirtualMachineBlockDeviceAttachments.
package vmsnapshot

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	virtv1 "kubevirt.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/deckhouse/deckhouse/pkg/log"
	storagev1alpha1 "github.com/deckhouse/state-snapshotter/api/storage/v1alpha1"
	"github.com/deckhouse/state-snapshotter/pkg/snapshotsdk"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/unified-snapshotter/internal/adapter"
	"github.com/deckhouse/virtualization-controller/pkg/controller/unified-snapshotter/internal/annotation"
	"github.com/deckhouse/virtualization-controller/pkg/controller/unified-snapshotter/internal/statuspatch"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmscondition"
)

const (
	requeueAfter   = 2 * time.Second
	ControllerName = "virtualmachine-snapshot-controller"

	childrenSettleDeadline = 10 * time.Minute
	planningDeadline       = 2 * time.Minute
	// freezeConfirmDeadline bounds the wait for the guest to confirm a filesystem freeze request. It must
	// stay BELOW planningDeadline: both are measured from planningStartedAt, and the freeze happens first,
	// so a freeze allowed to spend the whole planning budget would leave planning none. A confirmation is
	// a QMP round-trip through the guest agent — it lands in seconds or it will not land.
	freezeConfirmDeadline = time.Minute

	// reasonInvalidSource is the terminal domain reason for a snapshot whose spec does not resolve to a
	// capturable source object.
	reasonInvalidSource = "InvalidSource"
)

// Reconciler drives VirtualMachineSnapshot capture through the state-snapshotter SDK.
type Reconciler struct {
	Client    client.Client
	APIReader client.Reader
	// Freezer is the same *service.SnapshotService the built-in vmsnapshot/vdsnapshot controllers already
	// use for guest-agent filesystem freeze/unfreeze.
	Freezer  *service.SnapshotService
	Recorder eventrecord.EventRecorderLogger
	Log      *log.Logger
}

// SetupWithManager registers the reconciler, gated to the objects the unified mechanism owns — see
// annotation.DrivenByUnified.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		Named(ControllerName).
		For(&v1alpha2.VirtualMachineSnapshot{}).
		WithLogConstructor(logger.NewConstructor(r.Log)).
		WithEventFilter(annotation.ShouldHandle()).
		Complete(r)
}

func (r *Reconciler) sdk() snapshotsdk.CaptureSDK {
	return snapshotsdk.New(r.Client, r.APIReader, snapshotsdk.NewStorageFoundationProvider(r.Client))
}

func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
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

	if !controllerutil.ContainsFinalizer(vms, v1alpha2.FinalizerVMSnapshotCleanup) {
		controllerutil.AddFinalizer(vms, v1alpha2.FinalizerVMSnapshotCleanup)
		if err := r.Client.Update(ctx, vms); err != nil {
			return ctrl.Result{}, err
		}
	}

	if vms.Status.Phase == "" {
		vms.Status.Phase = v1alpha2.VirtualMachineSnapshotPhasePending
		r.Log.Info("patch phase to VirtualMachineSnapshotPhasePending")
		if err := r.patchStatus(ctx, vms); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	a := &adapter.VirtualMachineSnapshotAdapter{VMS: vms}
	sdk := r.sdk()

	switch a.GetDomainCaptureState().Phase {
	case snapshotsdk.PhaseFinished:
		return r.reconcileCaptured(ctx, vms)
	case snapshotsdk.PhaseFailed:
		return r.reconcileCaptureFailed(ctx, vms)
	}

	// An unresolvable source is failed only here, AFTER the phase switch above. Failing it earlier put it
	// past the reach of reconcileCaptureFailed, so nothing was left to release a freeze this snapshot may
	// still hold; and it pre-empted the Pending bootstrap, so an object the cluster default hands to this
	// controller never got a phase at all.
	vmName := vms.SourceVirtualMachineName()
	if vmName == "" {
		vm, kvvmi, err := r.cleanupSource(ctx, vms)
		if err != nil {
			return ctrl.Result{}, err
		}
		return r.failCapture(ctx, a, vms, vm, kvvmi, reasonInvalidSource, fmt.Sprintf(
			"spec.sourceRef must reference a %s %s", v1alpha2.SchemeGroupVersion.String(), v1alpha2.VirtualMachineKind))
	}

	vm := &v1alpha2.VirtualMachine{}
	if err := r.Client.Get(ctx, types.NamespacedName{Namespace: vms.Namespace, Name: vmName}, vm); err != nil {
		if apierrors.IsNotFound(err) {
			// Waited on indefinitely, and deliberately so: nothing is frozen yet at this point (the freeze
			// block is further down), so there is nothing this wait can hold hostage. It absorbs the apply
			// ordering race of a VirtualMachineSnapshot and its VirtualMachine landing in one manifest set.
			// Per the SDK, a non-terminal "waiting for X" stays in Planning and reports itself through
			// DomainCaptureStatus rather than failing — the way a Pod stays Pending with a message. The
			// reason is what makes that machine-readable; conditions here are core-owned.
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
	if err := sdk.DomainCaptureStatus(a).Phase(snapshotsdk.PhasePlanning).Message("").Apply(ctx); err != nil {
		return ctrl.Result{}, err
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

	kvvmi, err := r.getKVVMI(ctx, vm)
	if err != nil {
		return ctrl.Result{}, err
	}
	// Read the freeze state only after the sync, and never paper over an untrusted read. While a
	// freeze/unfreeze request is in flight both calls report ErrUntrustedFilesystemFrozenCondition; taking
	// that for "not frozen" sends us into the freeze block below, where CanFreeze reports false *because the
	// guest is already frozen* — indistinguishable there from "the agent cannot freeze it", and so it fails a
	// snapshot whose freeze in fact succeeded.
	//
	// That in-flight window needs an end, though. SyncFSFreezeRequest clears the request annotation only
	// once status.fsFreezeStatus catches up, so a freeze the guest never confirms leaves the annotation in
	// place and both calls keep reporting the sentinel — parking this node in Planning forever. Nothing
	// else in the system retires such a request, and a core-planned node cannot be told to skip
	// consistency (requiredConsistency is defaulted to true on it), so one stuck guest would hold a whole
	// namespace capture. Past freezeConfirmDeadline the answer is settled instead of waited on.
	var frozen bool
	if kvvmi != nil {
		what := "sync the guest filesystem freeze request"
		freezeErr := r.Freezer.SyncFSFreezeRequest(ctx, kvvmi)
		if freezeErr == nil {
			what = "read the guest filesystem freeze state"
			frozen, freezeErr = r.Freezer.IsFrozen(kvvmi)
		}

		if freezeErr != nil {
			// The budget covers ONLY the in-flight-request sentinel. A transport or API failure must be
			// retried, never spent as freeze time and then read as "the guest will not confirm".
			inFlight := errors.Is(freezeErr, service.ErrUntrustedFilesystemFrozenCondition)
			if !inFlight || time.Since(planningStartedAt(vms, vm)) <= freezeConfirmDeadline {
				return r.waitForFreeze(ctx, sdk, a, vms, vm, what, freezeErr)
			}

			// Retire the request before deciding anything: it is the abandoned annotation, not the
			// missing freeze, that would keep failing every later round-trip — releaseFreeze's included,
			// which would then hold this snapshot short of its terminal phase.
			if derr := r.Freezer.DiscardFSFreezeRequest(ctx, kvvmi); derr != nil {
				return ctrl.Result{}, derr
			}

			return r.failCapture(ctx, a, vms, vm, kvvmi, string(vmscondition.PotentiallyInconsistent), fmt.Sprintf(
				"the virtual machine %q did not confirm the guest filesystem freeze within %s",
				vm.Name, freezeConfirmDeadline))
		}
	}

	domainState := a.GetDomainCaptureState()
	planningNotFrozen := domainState.Phase != snapshotsdk.PhasePlanned &&
		domainState.Phase != snapshotsdk.PhaseFinished &&
		domainState.Phase != snapshotsdk.PhaseFailed
	if vms.Spec.RequiredConsistency && planningNotFrozen && !frozen {
		canFreeze, cErr := r.Freezer.CanFreeze(ctx, kvvmi)
		if cErr != nil {
			return ctrl.Result{}, cErr
		}
		if canFreeze {
			if err := r.Freezer.Freeze(ctx, kvvmi); err != nil {
				return ctrl.Result{}, err
			}
			return ctrl.Result{RequeueAfter: requeueAfter}, nil
		}
		if kvvmi != nil && kvvmi.Status.Phase == virtv1.Running {
			return r.failCapture(ctx, a, vms, vm, kvvmi, string(vmscondition.PotentiallyInconsistent), fmt.Sprintf(
				"cannot take a consistent snapshot of virtual machine %q: the virtual machine agent is not ready and the virtual machine cannot be frozen", vm.Name))
		}
		// Not running (or no kvvmi): trivially consistent without a freeze.
	}

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
					return r.failCapture(ctx, a, vms, vm, kvvmi, storagev1alpha1.ReasonGraphPlanningFailed, err.Error())
				default:
					// Transport or API trouble. Retry with the workqueue's backoff rather than burning the snapshot.
					return ctrl.Result{}, err
				}
			}
		case errors.Is(err, errSourceNotReady):
			return r.waitForSource(ctx, sdk, a, vms, vm, kvvmi, err)
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
			return r.waitForSource(ctx, sdk, a, vms, vm, kvvmi, tErr)
		case errors.Is(tErr, errInvalidSourceSpec):
			return r.failCapture(ctx, a, vms, vm, kvvmi, storagev1alpha1.ReasonGraphPlanningFailed, tErr.Error())
		default:
			return ctrl.Result{}, tErr
		}
		if err := sdk.EnsureManifestCapture(ctx, a, snapshotsdk.ManifestCaptureSpec{Targets: targets}); err != nil {
			switch {
			case apierrors.IsConflict(err):
				return ctrl.Result{RequeueAfter: requeueAfter}, nil
			case errors.Is(err, snapshotsdk.ErrManifestTargetsDrift):
				// The freshly declared set diverges from the frozen MCR. Terminal, per the SDK's guidance.
				return r.failCapture(ctx, a, vms, vm, kvvmi, storagev1alpha1.ReasonGraphPlanningFailed, err.Error())
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
		if !r.releaseFreeze(ctx, vms, vm, kvvmi) {
			return ctrl.Result{RequeueAfter: requeueAfter}, nil
		}
		vms.Status.Phase = v1alpha2.VirtualMachineSnapshotPhaseFailed
		r.Log.Info("patch VMS phase to VirtualMachineSnapshotPhaseFailed")
		return ctrl.Result{}, r.patchStatus(ctx, vms)
	case snapshotsdk.CaptureOutcomeCapturing:
		vms.Status.Phase = v1alpha2.VirtualMachineSnapshotPhaseInProgress
		r.Log.Info("patch VMS phase to VirtualMachineSnapshotPhaseInProgress")
		return ctrl.Result{RequeueAfter: requeueAfter}, r.patchStatus(ctx, vms)
	}

	// Own manifest leg captured; wait for every child VirtualDiskSnapshot to go terminal before
	// unfreezing (the whole point of one shared freeze across all disks). Gated on the core-computed
	// ChildrenSettled latch, not by inspecting each child's own legs via ChildrenCaptureStates: per the
	// SDK's own doc comment on ChildCaptureState, that per-child view is diagnostics-only and must not
	// gate a consistency action — it hangs forever on a child that fails with a domain-specific reason
	// outside the SDK's terminal-reason list. ChildrenSettled is the intended completeness signal
	// (true once every direct child is terminal, success or failure).

	// Resolved once here and reused by all three decisions below — whether to keep waiting, whether the
	// wait has run out of time, and whether this snapshot may claim consistency. The locally planned set is
	// deliberately NOT consulted: it is nil whenever this reconcile skipped or failed re-planning on a
	// frozen plan, and childVirtualDiskSnapshots answers from the cluster in exactly that case.
	childSnapshots, err := r.childVirtualDiskSnapshots(ctx, vms)
	if err != nil {
		return ctrl.Result{}, err
	}

	if !childrenSettled(childSnapshots, a.CoreCaptureState()) {
		// A VolumeCaptureRequest has no guaranteed final state: storage-foundation keeps retrying a CSI
		// driver without a cap, so a child can stay non-terminal forever and ChildrenSettled never flips.
		// This is the only wait left that holds the guest filesystem frozen, so it needs an end.
		waited, pending := childrenWaitProgress(vms, childSnapshots)
		if waited > childrenSettleDeadline {
			return r.failCapture(ctx, a, vms, vm, kvvmi, string(vmscondition.VirtualMachineSnapshotFailed), fmt.Sprintf(
				"giving up after %s so the guest filesystem is not held frozen indefinitely: "+
					"VirtualDiskSnapshots %v have not finished capturing data",
				childrenSettleDeadline, pending))
		}

		vms.Status.Phase = v1alpha2.VirtualMachineSnapshotPhaseInProgress
		r.Log.Info("patch VMS phase to VirtualMachineSnapshotPhaseInProgress, children not settled")
		return ctrl.Result{RequeueAfter: requeueAfter}, r.patchStatus(ctx, vms)
	}

	if !r.releaseFreeze(ctx, vms, vm, kvvmi) {
		vms.Status.Phase = v1alpha2.VirtualMachineSnapshotPhaseInProgress
		return ctrl.Result{RequeueAfter: requeueAfter}, r.patchStatus(ctx, vms)
	}
	if err := sdk.DomainCaptureStatus(a).Phase(snapshotsdk.PhaseFinished).Apply(ctx); err != nil {
		return ctrl.Result{}, err
	}
	vms.Status.Phase = v1alpha2.VirtualMachineSnapshotPhaseReady

	if childrenAreConsistent(childSnapshots) {
		vms.Status.Consistent = ptr.To(true)
	}
	settleConsistency(vms)

	r.Log.Info("patch VMS phase to VirtualMachineSnapshotPhaseReady")
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

func (r *Reconciler) failCapture(
	ctx context.Context,
	a *adapter.VirtualMachineSnapshotAdapter,
	vms *v1alpha2.VirtualMachineSnapshot,
	vm *v1alpha2.VirtualMachine,
	kvvmi *virtv1.VirtualMachineInstance,
	reason, message string,
) (ctrl.Result, error) {
	if err := r.sdk().DomainCaptureStatus(a).
		Phase(snapshotsdk.PhaseFailed).
		Reason(snapshotsdk.Reason(reason)).
		Message(message).
		Apply(ctx); err != nil {
		return ctrl.Result{}, err
	}

	if !r.releaseFreeze(ctx, vms, vm, kvvmi) {
		return ctrl.Result{RequeueAfter: requeueAfter}, nil
	}

	vms.Status.Phase = v1alpha2.VirtualMachineSnapshotPhaseFailed
	r.Log.Info("patch VMS phase to VirtualMachineSnapshotPhaseFailed", "reason", reason, "message", message)
	return ctrl.Result{}, r.patchStatus(ctx, vms)
}

// settleConsistency latches the negative answer to the consistency question once the snapshot reaches a
// terminal phase, mirroring its VirtualDiskSnapshot counterpart.
//
// It is latched this late on purpose: the checks above set it to true only after every child has vouched
// for its disk, so writing false any earlier would pin a snapshot whose children had simply not finished.
// Leaving it absent is not an option: a terminal VirtualMachineSnapshot never reconciles again, and the
// user cannot tell an absent field ("we could not make this consistent") from one that was never computed.
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
// gives up as a planning failure once planningDeadline passes. Unlike the wait for the VirtualMachine
// itself, this one is bounded: the guest filesystem may already be frozen by the time planning runs, and
// nothing else would release it.
//
// The budget is measured from whichever happened LATER, this snapshot's creation or the VirtualMachine's.
// Measuring from the snapshot alone charged it for time it could not use: the wait for a missing
// VirtualMachine above is unbounded, so a machine that appears an hour later would find the whole budget
// already spent and the first transient miss would fail an otherwise healthy capture.
func (r *Reconciler) waitForSource(
	ctx context.Context,
	sdk snapshotsdk.CaptureSDK,
	a *adapter.VirtualMachineSnapshotAdapter,
	vms *v1alpha2.VirtualMachineSnapshot,
	vm *v1alpha2.VirtualMachine,
	kvvmi *virtv1.VirtualMachineInstance,
	err error,
) (ctrl.Result, error) {
	if time.Since(planningStartedAt(vms, vm)) > planningDeadline {
		return r.failCapture(ctx, a, vms, vm, kvvmi, storagev1alpha1.ReasonGraphPlanningFailed, fmt.Sprintf(
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

// planningStartedAt is the earliest moment planning could have begun: both the snapshot and its source
// VirtualMachine must exist for that, so it is the later of the two creation timestamps. A recreated
// VirtualMachine legitimately restarts the budget — the freeze the old one held died with its instance.
func planningStartedAt(vms *v1alpha2.VirtualMachineSnapshot, vm *v1alpha2.VirtualMachine) time.Time {
	started := vms.CreationTimestamp.Time
	if vm != nil && vm.CreationTimestamp.After(started) {
		started = vm.CreationTimestamp.Time
	}
	return started
}

func (r *Reconciler) reconcileCaptured(ctx context.Context, vms *v1alpha2.VirtualMachineSnapshot) (ctrl.Result, error) {
	if vms.Status.Phase == v1alpha2.VirtualMachineSnapshotPhaseReady {
		return ctrl.Result{}, nil
	}

	childSnapshots, err := r.childVirtualDiskSnapshots(ctx, vms)
	if err != nil {
		return ctrl.Result{}, err
	}
	if childrenAreConsistent(childSnapshots) {
		vms.Status.Consistent = ptr.To(true)
	}
	settleConsistency(vms)

	vms.Status.Phase = v1alpha2.VirtualMachineSnapshotPhaseReady
	r.Log.Info("patch VMS phase to VirtualMachineSnapshotPhaseReady, capture already finished")
	return ctrl.Result{}, r.patchStatus(ctx, vms)
}

func (r *Reconciler) reconcileCaptureFailed(ctx context.Context, vms *v1alpha2.VirtualMachineSnapshot) (ctrl.Result, error) {
	if vms.Status.Phase == v1alpha2.VirtualMachineSnapshotPhaseFailed {
		return ctrl.Result{}, nil
	}

	vm, kvvmi, err := r.cleanupSource(ctx, vms)
	if err != nil {
		return ctrl.Result{}, err
	}
	if !r.releaseFreeze(ctx, vms, vm, kvvmi) {
		return ctrl.Result{RequeueAfter: requeueAfter}, nil
	}

	vms.Status.Phase = v1alpha2.VirtualMachineSnapshotPhaseFailed
	r.Log.Info("patch VMS phase to VirtualMachineSnapshotPhaseFailed, capture already failed")
	return ctrl.Result{}, r.patchStatus(ctx, vms)
}

// reconcileDeletion unfreezes the guest filesystem if this snapshot's freeze is still held, then releases the finalizer.
func (r *Reconciler) reconcileDeletion(ctx context.Context, vms *v1alpha2.VirtualMachineSnapshot) (ctrl.Result, error) {
	if !controllerutil.ContainsFinalizer(vms, v1alpha2.FinalizerVMSnapshotCleanup) {
		return ctrl.Result{}, nil
	}

	vm, kvvmi, err := r.cleanupSource(ctx, vms)
	if err != nil {
		return ctrl.Result{}, err
	}
	if !r.releaseFreeze(ctx, vms, vm, kvvmi) {
		return ctrl.Result{RequeueAfter: requeueAfter}, nil
	}

	controllerutil.RemoveFinalizer(vms, v1alpha2.FinalizerVMSnapshotCleanup)
	return ctrl.Result{}, r.Client.Update(ctx, vms)
}

// cleanupSource resolves the VirtualMachine whose guest filesystem this snapshot may still hold frozen,
// together with its VirtualMachineInstance. It reads spec first and falls back to status.sourceRef.
//
// The fallback is what keeps a wedged snapshot recoverable. spec is not immutable — the CRD only enforces
// that exactly one of virtualMachineName/sourceRef is set — so an edit pointing sourceRef at another kind
// makes SourceVirtualMachineName go empty while a freeze this snapshot took is still held. status.sourceRef
// is this snapshot's own record of what it captured, published (PublishSnapshotSource) before anything is
// frozen, so it still names the right VirtualMachine when spec no longer does.
//
// A nil VirtualMachine means there is nothing to unfreeze: no name resolves, or the object is already gone.
// releaseFreeze treats a nil VirtualMachineInstance as "nothing held", so callers need no extra branch.
func (r *Reconciler) cleanupSource(ctx context.Context, vms *v1alpha2.VirtualMachineSnapshot) (*v1alpha2.VirtualMachine, *virtv1.VirtualMachineInstance, error) {
	name := vms.SourceVirtualMachineName()
	if name == "" {
		if ref := vms.Status.SourceRef; ref != nil &&
			ref.Kind == v1alpha2.VirtualMachineKind &&
			ref.APIVersion == v1alpha2.SchemeGroupVersion.String() {
			name = ref.Name
		}
	}
	if name == "" {
		return nil, nil, nil
	}

	vm := &v1alpha2.VirtualMachine{}
	if err := r.Client.Get(ctx, types.NamespacedName{Namespace: vms.Namespace, Name: name}, vm); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil, nil
		}
		return nil, nil, err
	}

	kvvmi, err := r.getKVVMI(ctx, vm)
	if err != nil {
		return nil, nil, err
	}
	return vm, kvvmi, nil
}

// waitForFreeze holds the node in Planning while a guest filesystem freeze request is in flight and
// publishes WHY it waits. Without the published reason the only user-visible signal is the core's
// ManifestCapturePending on the Ready condition, which points at the wrong leg entirely: manifests are
// not what this is waiting for.
func (r *Reconciler) waitForFreeze(
	ctx context.Context,
	sdk snapshotsdk.CaptureSDK,
	a *adapter.VirtualMachineSnapshotAdapter,
	vms *v1alpha2.VirtualMachineSnapshot,
	vm *v1alpha2.VirtualMachine,
	what string,
	err error,
) (ctrl.Result, error) {
	r.freezeTrouble(ctx, vms, what, err)

	if perr := sdk.DomainCaptureStatus(a).
		Phase(snapshotsdk.PhasePlanning).
		Reason(snapshotsdk.Reason(vmscondition.FileSystemFreezing)).
		Message(fmt.Sprintf(
			"Waiting for the virtual machine %q to confirm the guest filesystem freeze. Snapshotting will continue once the guest agent responds.",
			vm.Name)).
		Apply(ctx); perr != nil {
		return ctrl.Result{}, perr
	}

	return ctrl.Result{RequeueAfter: requeueAfter}, nil
}

// freezeTrouble reports a guest filesystem freeze round-trip that did not go through.
//
// A request the guest agent has not confirmed yet (ErrUntrustedFilesystemFrozenCondition) is the normal
// in-flight state rather than a fault: the annotation is already set and only Status.FSFreezeStatus has
// yet to catch up, so the next reconcile settles it. It stays a debug line and never reaches the user's
// Events — the same way releaseFreeze already treats that sentinel. Escalating it made a snapshot that
// went on to succeed carry Warning events.
//
// Anything else is worth surfacing, but only the log gets the wrapped error: it names the internal
// instance and its freeze status, which the Event text must not.
func (r *Reconciler) freezeTrouble(ctx context.Context, vms *v1alpha2.VirtualMachineSnapshot, what string, err error) {
	log := logger.FromContext(ctx)

	if errors.Is(err, service.ErrUntrustedFilesystemFrozenCondition) {
		log.Debug("waiting for the guest agent to confirm the guest filesystem freeze request", "err", err.Error())
		return
	}

	log.Error("failed to "+what+", will retry", "err", err.Error())
	if r.Recorder != nil {
		r.Recorder.Eventf(vms, corev1.EventTypeWarning, v1alpha2.ReasonVMSnapshottingPending,
			"Cannot %s of the virtual machine %q. Snapshotting is on hold until the guest agent responds.",
			what, vms.SourceVirtualMachineName())
	}
}

func (r *Reconciler) releaseFreeze(ctx context.Context, vms *v1alpha2.VirtualMachineSnapshot, vm *v1alpha2.VirtualMachine, kvvmi *virtv1.VirtualMachineInstance) bool {
	if kvvmi == nil {
		return true
	}

	if err := r.Freezer.SyncFSFreezeRequest(ctx, kvvmi); err != nil {
		r.Log.Debug("failed to sync the guest filesystem freeze request, will retry", "err", err.Error())
		return false
	}

	frozen, err := r.Freezer.IsFrozen(kvvmi)
	if err != nil {
		r.Log.Debug("failed to read the guest filesystem freeze state, will retry", "err", err.Error())
		return false
	}
	if !frozen {
		return true
	}

	canUnfreeze, err := r.Freezer.CanUnfreezeWithVirtualMachineSnapshotTree(ctx, vms, vm, kvvmi)
	switch {
	case errors.Is(err, service.ErrUntrustedFilesystemFrozenCondition):
		return false
	case err != nil:
		r.Log.Error("failed to check whether the guest filesystem freeze may be released, will retry", "err", err.Error())
		return false
	}
	if !canUnfreeze {
		// Another in-flight snapshot of the same VirtualMachine still holds the freeze and will release
		// it itself. Nothing left for this snapshot to wait on.
		return true
	}

	if err := r.Freezer.Unfreeze(ctx, kvvmi); err != nil {
		r.Log.Debug("failed to request guest filesystem unfreeze, will retry", "err", err.Error())
	}

	return false
}

// childrenAreConsistent aggregates the consistency each child VirtualDiskSnapshot resolved for itself:
// the whole-VM snapshot is consistent only when every captured disk is.
func childrenAreConsistent(children []v1alpha2.VirtualDiskSnapshot) bool {
	for _, vds := range children {
		// A placeholder for a published ref whose object is gone carries no status, so it cannot vouch for
		// its disk and consistency stays unknown — the same answer the Get-based version gave on NotFound.
		if vds.Status.Consistent == nil || !*vds.Status.Consistent {
			return false
		}
	}

	return true
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

func (r *Reconciler) getKVVMI(ctx context.Context, vm *v1alpha2.VirtualMachine) (*virtv1.VirtualMachineInstance, error) {
	// APIReader (uncached, direct) is deliberate here: r.Client is the manager's cached client, and Get()
	// on a GVK it hasn't seen yet lazily starts a cluster-wide List+Watch informer for that whole type —
	// we only ever need this one VMI, not a standing cache of every VirtualMachineInstance in the cluster.
	kvvmi := &virtv1.VirtualMachineInstance{}
	if err := r.APIReader.Get(ctx, types.NamespacedName{Namespace: vm.Namespace, Name: vm.Name}, kvvmi); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	return kvvmi, nil
}
