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

package internal

import (
	"context"
	"errors"
	"log/slog"
	"slices"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmaccess/internal/state"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	coordinationv1alpha1 "github.com/deckhouse/virtualization/api/coordination/v1alpha1"
	"github.com/deckhouse/virtualization/api/coordination/v1alpha1/vmaccondition"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

const (
	// FreezeConfirmTimeout bounds the retries of a freeze request, counted from the moment the claim started
	// freezing. A guest with a 5% CPU share can take up to about 5 minutes to confirm a freeze, as the guest
	// agent gets little CPU time while it freezes the filesystems one by one.
	FreezeConfirmTimeout = 5 * time.Minute
	// UnfreezeConfirmTimeout bounds the wait for an unfreeze request to go through and for fsFreezeStatus to
	// stop reading frozen; past it the guest is left to its safety unfreeze. An unfreeze walks the same
	// filesystems as a freeze, hence the same value.
	UnfreezeConfirmTimeout = FreezeConfirmTimeout
	// LateFreezeUnfreezeWindow is how long a session whose freeze was never confirmed keeps the lock after it
	// ends and resends the unfreeze: a freeze request that failed for the coordinator may still land in the
	// guest. The window fits three unfreeze requests.
	LateFreezeUnfreezeWindow = 2*RequestResendInterval + PollInterval
	// UnfrozenTolerance is how long an empty fsFreezeStatus is tolerated during a frozen session before it
	// counts as an unexpected unfreeze, as the field can read empty for seconds while the freeze is held.
	UnfrozenTolerance = 30 * time.Second
	// SafetyUnfreezeMargin is added to FreezeConfirmTimeout and the session timeout to get the safety unfreeze
	// timeout passed to the guest, so the guest does not unfreeze itself before the session deadline.
	SafetyUnfreezeMargin = time.Minute
	// RequestResendInterval is the least time between two freeze or unfreeze requests for the same claim,
	// so a request whose API call failed is retried without flooding a guest that is merely slow.
	RequestResendInterval = 30 * time.Second
	// PollInterval is the requeue interval while a request waits for its confirmation, so resends and
	// timeouts do not wait for an event.
	PollInterval = 5 * time.Second
	// StatusWriteRecheck is the requeue interval while a pass waits for a write to its claim to show up in
	// the cache: the final status before the lock is released, and the finalizer before the lock is taken.
	StatusWriteRecheck = time.Second
	// LockLossTolerance is how long a claim that took the lock may go unnamed by it before the lock counts as
	// lost, as the cache of the internal virtual machine can lag behind the claim cache.
	LockLossTolerance = UnfrozenTolerance
	// StaleFreezeRecheck is the requeue interval while a freeze left by an older snapshot waits for the
	// snapshots of the virtual machine in progress to finish. It matches the requeue interval of a snapshot.
	StaleFreezeRecheck = 2 * time.Second
)

// LifecycleHandler drives the claims of one virtual machine: it decides on new claims, watches the
// session of the claim holding the lock, and releases the lock once the session is over.
type LifecycleHandler struct {
	freezer  Freezer
	recorder eventrecord.EventRecorderLogger
	now      func() time.Time

	// unfrozen measures how long fsFreezeStatus has read empty during a frozen session.
	unfrozen *stopwatch
	// lockLost measures how long a claim that took the lock has gone unnamed by it.
	lockLost *stopwatch
	// orphaned measures how long a lock whose claim is gone has been seen.
	orphaned *stopwatch
	// freezeSent and unfreezeSent measure the time since a request was last sent. They are kept apart, so a
	// release right after the freeze is not held back by it.
	freezeSent   *stopwatch
	unfreezeSent *stopwatch

	// staleUnfreezing and staleUnfreezeSent track the unfreeze of a freeze left by a snapshot taken before the
	// module update. They are keyed by the instance UID, as such a freeze belongs to no claim.
	staleUnfreezing   *stopwatch
	staleUnfreezeSent *stopwatch

	// unfreezeAccepted holds the claims whose unfreeze request went through while fsFreezeStatus still reads
	// frozen. It is in memory only: after a restart the unfreeze is sent again, which an unfrozen guest
	// ignores.
	unfreezeAccepted *claimSet
}

func NewLifecycleHandler(freezer Freezer, recorder eventrecord.EventRecorderLogger, now func() time.Time) *LifecycleHandler {
	return &LifecycleHandler{
		freezer:           freezer,
		recorder:          recorder,
		now:               now,
		unfrozen:          newStopwatch(),
		lockLost:          newStopwatch(),
		orphaned:          newStopwatch(),
		freezeSent:        newStopwatch(),
		unfreezeSent:      newStopwatch(),
		staleUnfreezing:   newStopwatch(),
		staleUnfreezeSent: newStopwatch(),
		unfreezeAccepted:  newClaimSet(),
	}
}

func (h *LifecycleHandler) Name() string {
	return "LifecycleHandler"
}

func (h *LifecycleHandler) Handle(ctx context.Context, s *state.State) (reconcile.Result, error) {
	var (
		result reconcile.Result
		errs   error
	)

	// A claim that took the lock before it was malformed goes to watchLostLock below.
	if s.MalformedLock {
		logger.FromContext(ctx).Warn("the access lock of the virtual machine names no claim; dropping it")

		if err := s.ReleaseLock(ctx); err != nil {
			return reconcile.Result{}, err
		}
	}

	if s.Lock != nil {
		var (
			res reconcile.Result
			err error
		)
		if holder := s.Holder(); holder != nil {
			res, err = h.handleHolder(ctx, s, holder)
		} else {
			res, err = h.dropOrphanedLock(ctx, s)
		}

		result = mergeResults(result, res)
		errs = errors.Join(errs, err)
	} else if lostLockSession(s) == nil {
		res, err := h.clearStaleFreeze(ctx, s)
		result = mergeResults(result, res)
		errs = errors.Join(errs, err)
	}

	for _, c := range s.Claims {
		if c == s.Holder() {
			continue
		}
		// A finished claim without the lock needs no in-memory tracking.
		if isTerminal(c.Changed) || c.Changed.DeletionTimestamp != nil {
			h.forget(c.Changed.UID)
			continue
		}

		var (
			res reconcile.Result
			err error
		)
		if tookLock(c.Changed) {
			res, err = h.watchLostLock(ctx, s, c)
		} else {
			res, err = h.decide(ctx, s, c)
		}

		result = mergeResults(result, res)
		errs = errors.Join(errs, err)
	}

	return result, errs
}

// clearStaleFreeze clears the freeze a snapshot taken before the module update left on the instance: it
// unfreezes the guest and removes the filesystem request, which would deny every claim while it stays.
// It runs only while no claim holds or took the lock, so a freeze it sends no claimUID for is never a session's,
// and only while no snapshot of the virtual machine is in progress, because the built-in snapshots still
// freeze the guest through that request.
//
// A failed unfreeze is sent again every RequestResendInterval and the request stays, as that freeze has no
// safety unfreeze and the request is the only record of it. Past UnfreezeConfirmTimeout the failure is
// reported as an event on the virtual machine and the request is removed, so it does not deny claims for
// good.
func (h *LifecycleHandler) clearStaleFreeze(ctx context.Context, s *state.State) (reconcile.Result, error) {
	if !s.HasFilesystemRequest() {
		if s.KVVMI != nil {
			h.forgetStaleFreeze(s.KVVMI.UID)
		}

		return reconcile.Result{}, nil
	}

	inProgress, err := s.SnapshotInProgress(ctx)
	if err != nil {
		return reconcile.Result{}, err
	}

	if inProgress {
		// The snapshot phase is not watched, so a recheck notices the snapshot finish.
		return reconcile.Result{RequeueAfter: StaleFreezeRecheck}, nil
	}

	log := logger.FromContext(ctx)
	key := s.Key()

	uid := s.KVVMI.UID
	unfrozen := false

	if isRunning(s.KVVMI) {
		if !h.staleUnfreezeSent.due(uid, h.now(), RequestResendInterval) {
			return reconcile.Result{RequeueAfter: PollInterval}, nil
		}

		log.Info("unfreezing the guest a snapshot taken before the module update may have left frozen")

		err := h.freezer.Unfreeze(ctx, key.Namespace, key.Name, "")
		switch {
		case err == nil:
			unfrozen = true
		case h.staleUnfreezing.elapsed(uid, h.now()) < UnfreezeConfirmTimeout:
			log.Warn("failed to unfreeze the guest a snapshot taken before the module update may have left frozen; retrying", logger.SlogErr(err))
			return reconcile.Result{RequeueAfter: RequestResendInterval}, nil
		default:
			log.Warn("failed to unfreeze the guest a snapshot taken before the module update may have left frozen", logger.SlogErr(err))
			h.reportToVM(s, msgStaleFreezeNotUnfrozen(key.Name, err))
		}
	}

	h.forgetStaleFreeze(uid)

	if err := s.DiscardFilesystemRequest(ctx); err != nil {
		log.Warn("failed to remove the filesystem request a snapshot taken before the module update left", logger.SlogErr(err))
		h.reportToVM(s, msgStaleFreezeRequestNotRemoved(key.Name, err))
	}

	if unfrozen {
		// The unfreeze returns once the guest is unfrozen, so the fsFreezeStatus read before it is stale.
		s.KVVMI.Status.FSFreezeStatus = ""
	}

	return reconcile.Result{}, nil
}

// reportToVM records a warning event on the virtual machine.
func (h *LifecycleHandler) reportToVM(s *state.State, msg string) {
	if s.VM == nil {
		return
	}

	h.recorder.Event(s.VM, corev1.EventTypeWarning, v1alpha2.ReasonFilesystemFreezeCleanupFailed, msg)
}

// decide grants or denies a claim that does not hold the lock.
func (h *LifecycleHandler) decide(ctx context.Context, s *state.State, c *state.Claim) (reconcile.Result, error) {
	claim := c.Changed
	vm := s.Key().Name

	if claim.Status.Phase == "" {
		claim.Status.Phase = coordinationv1alpha1.AccessClaimPhasePending
	}

	claim.Status.ObservedGeneration = claim.Generation

	// A claim released before it was granted completes without a session.
	if claim.Spec.Release {
		h.complete(claim, vm)
		return reconcile.Result{}, nil
	}

	lostLock := lostLockSession(s)
	switch {
	case s.VM == nil:
		h.deny(claim, vmaccondition.VirtualMachineNotReady, msgVMNotFound(vm))
		return reconcile.Result{}, nil
	case s.VM.UID != claim.Spec.VirtualMachine.UID:
		h.deny(claim, vmaccondition.VirtualMachineStateChanged, msgVMRecreated(vm))
		return reconcile.Result{}, nil
	case s.KVVM == nil:
		h.deny(claim, vmaccondition.VirtualMachineNotReady, msgVMNotReady(vm))
		return reconcile.Result{}, nil
	case s.Lock != nil:
		var holder *metav1.OwnerReference
		if hc := s.Holder(); hc != nil {
			holder = metav1.GetControllerOf(hc.Current)
		}

		h.deny(claim, vmaccondition.VirtualMachineBusy, msgBusy(vm, holder))

		return reconcile.Result{}, nil
	case lostLock != nil:
		h.deny(claim, vmaccondition.VirtualMachineBusy, msgBusy(vm, metav1.GetControllerOf(lostLock.Current)))
		return reconcile.Result{}, nil
	case s.Migration != nil:
		h.deny(claim, vmaccondition.VirtualMachineBusy, msgMigrating(vm, migrationOperation(s.Migration)))
		return reconcile.Result{}, nil
	case startingOrStopping(s.KVVMI):
		h.deny(claim, vmaccondition.VirtualMachineNotReady, msgVMStartingOrStopping(vm))
		return reconcile.Result{}, nil
	case frozenOutsideCoordinator(s.KVVMI):
		h.deny(claim, vmaccondition.FilesystemAlreadyFrozen, msgAlreadyFrozen(vm))
		return reconcile.Result{}, nil
	}

	running := isRunning(s.KVVMI)
	if running && wantsFreeze(claim) && !state.AgentConnected(s.KVVMI) && freezePolicy(claim) == coordinationv1alpha1.FreezePolicyRequired {
		h.deny(claim, vmaccondition.FilesystemFreezeFailed, msgAgentNotConnected(vm))
		return reconcile.Result{}, nil
	}

	// The lock is taken only once the finalizer is written, so a claim deleted during its session stays until
	// the guest is unfrozen.
	if !controllerutil.ContainsFinalizer(c.Current, v1alpha2.FinalizerVMACCleanup) {
		return reconcile.Result{RequeueAfter: StatusWriteRecheck}, nil
	}

	var vmiUID types.UID
	if running {
		vmiUID = s.KVVMI.UID
	}

	if err := s.AcquireLock(ctx, claim.UID, vmiUID); err != nil {
		return reconcile.Result{}, err
	}

	claim.Status.VirtualMachineInstanceUID = vmiUID

	return h.startSession(ctx, s, claim), nil
}

// handleHolder watches the session of the claim holding the lock.
func (h *LifecycleHandler) handleHolder(ctx context.Context, s *state.State, c *state.Claim) (reconcile.Result, error) {
	claim := c.Changed

	// The lock is written before the status, so it survives a failed status write.
	claim.Status.VirtualMachineInstanceUID = s.Lock.VMIUID
	h.lockLost.reset(claim.UID)

	if !isTerminal(claim) && claim.DeletionTimestamp == nil {
		var res reconcile.Result

		switch claim.Status.Phase {
		case coordinationv1alpha1.AccessClaimPhaseGranted:
			res = h.watchGranted(s, claim)
		default:
			res = h.watchFreezing(ctx, s, claim)
		}

		if !isTerminal(claim) {
			return res, nil
		}
	}

	return h.finish(ctx, s, c)
}

// watchLostLock ends the session of a claim that took the lock but is not named by it. It waits
// LockLossTolerance for the lock to show up in the cache, then takes the lock back if none is set, so that
// finish unfreezes the guest. With the virtual machine or its internal virtual machine gone or recreated, the
// session ends at once.
func (h *LifecycleHandler) watchLostLock(ctx context.Context, s *state.State, c *state.Claim) (reconcile.Result, error) {
	claim := c.Changed
	vm := s.Key().Name

	if s.KVVM == nil || s.VM == nil || s.VM.UID != claim.Spec.VirtualMachine.UID {
		h.endLostLock(claim, msgVMRecreated(vm))
		return reconcile.Result{}, nil
	}

	if lost := h.lockLost.elapsed(claim.UID, h.now()); lost < LockLossTolerance {
		return reconcile.Result{RequeueAfter: LockLossTolerance - lost}, nil
	}

	if s.Lock == nil {
		if err := s.AcquireLock(ctx, claim.UID, claim.Status.VirtualMachineInstanceUID); err != nil {
			return reconcile.Result{}, err
		}
	}

	h.lockLost.reset(claim.UID)

	h.endLostLock(claim, msgLockLost(vm))

	return reconcile.Result{RequeueAfter: PollInterval}, nil
}

// endLostLock aborts a granted claim, or denies one still freezing, that has lost its lock.
func (h *LifecycleHandler) endLostLock(claim *coordinationv1alpha1.VirtualMachineAccessClaim, msg string) {
	if claim.Status.Phase == coordinationv1alpha1.AccessClaimPhaseGranted {
		h.abort(claim, vmaccondition.VirtualMachineStateChanged, msg)
	} else {
		h.deny(claim, vmaccondition.VirtualMachineStateChanged, msg)
	}
}

// lostLockSession returns the claim that took the lock but is not named by it, if any. No other claim is
// granted until it ends, as its session may still be running.
func lostLockSession(s *state.State) *state.Claim {
	holder := s.Holder()
	for _, c := range s.Claims {
		if c != holder && !isTerminal(c.Changed) && c.Changed.DeletionTimestamp == nil && tookLock(c.Changed) {
			return c
		}
	}

	return nil
}

// tookLock reports whether the claim took the lock: decide marks a claim Freezing or Granted only after
// the lock is written.
func tookLock(claim *coordinationv1alpha1.VirtualMachineAccessClaim) bool {
	return claim.Status.Phase == coordinationv1alpha1.AccessClaimPhaseGranted || grantedReason(claim) == vmaccondition.Freezing
}

// watchFreezing ends the session of a claim still freezing on a release or a state change, and otherwise
// goes on freezing.
func (h *LifecycleHandler) watchFreezing(ctx context.Context, s *state.State, claim *coordinationv1alpha1.VirtualMachineAccessClaim) reconcile.Result {
	if claim.Spec.Release {
		h.complete(claim, s.Key().Name)
		return reconcile.Result{}
	}

	if reason, msg := h.stateChanged(s, claim); reason != "" {
		h.deny(claim, reason, msg)
		return reconcile.Result{}
	}

	return h.startSession(ctx, s, claim)
}

// startSession grants a claim holding the lock, freezing the guest first when the claim asks for it. A
// successful freeze request is the confirmation, as fsFreezeStatus can read empty for a frozen guest. A
// best-effort freeze is skipped without a guest agent or past FreezeConfirmTimeout; a required one fails past
// it.
func (h *LifecycleHandler) startSession(ctx context.Context, s *state.State, claim *coordinationv1alpha1.VirtualMachineAccessClaim) reconcile.Result {
	vm := s.Key().Name

	switch {
	case s.Lock.VMIUID == "":
		// The session of a stopped machine has no deadline: there is no guest to unfreeze, and starting the
		// machine ends the session.
		h.grant(claim, vmaccondition.NotRequired, msgNotRequiredStopped(vm))
		claim.Status.Deadline = nil

		return reconcile.Result{}
	case !wantsFreeze(claim):
		h.grant(claim, vmaccondition.NotRequired, msgNotRequiredNotRequested(vm))
		return h.deadlineRequeue(claim)
	case isFrozen(s.KVVMI):
		h.grant(claim, vmaccondition.Frozen, msgFrozen(vm))
		return h.deadlineRequeue(claim)
	case !state.AgentConnected(s.KVVMI) && freezePolicy(claim) == coordinationv1alpha1.FreezePolicyBestEffort:
		h.grant(claim, vmaccondition.FreezeSkipped, msgFreezeSkipped(vm))
		return h.deadlineRequeue(claim)
	}

	h.setGranted(claim, metav1.ConditionFalse, vmaccondition.Freezing, msgFreezing(vm))

	waited := h.now().Sub(conditionSince(claim, vmaccondition.GrantedType))
	if waited > FreezeConfirmTimeout {
		if freezePolicy(claim) == coordinationv1alpha1.FreezePolicyBestEffort {
			h.grant(claim, vmaccondition.FreezeSkipped, msgFreezeSkipped(vm))
			return h.deadlineRequeue(claim)
		}

		h.deny(claim, vmaccondition.FilesystemFreezeFailed, msgFreezeNotConfirmed(vm, FreezeConfirmTimeout))

		return reconcile.Result{}
	}

	if h.sendFreeze(ctx, s, claim) {
		h.grant(claim, vmaccondition.Frozen, msgFrozen(vm))
		return h.deadlineRequeue(claim)
	}

	return reconcile.Result{RequeueAfter: PollInterval}
}

// watchGranted aborts the session of a granted claim on a violation, and completes it on release.
func (h *LifecycleHandler) watchGranted(s *state.State, claim *coordinationv1alpha1.VirtualMachineAccessClaim) reconcile.Result {
	vm := s.Key().Name

	// After the release the guest stays frozen until finish, so a later violation does not affect the
	// capture. A replaced instance does: its guest may have run unfrozen during the capture.
	if instanceReplaced(s, claim) {
		h.abort(claim, vmaccondition.VirtualMachineStateChanged, msgVMInstanceChanged(vm))
		return reconcile.Result{}
	}

	if claim.Spec.Release {
		if h.releasedUnfrozen(s, claim) {
			h.abort(claim, vmaccondition.DeadlineExceeded, msgDeadlineExceeded(vm, claim.Spec.Timeout.Duration))
		} else {
			h.complete(claim, vm)
		}

		return reconcile.Result{}
	}

	if reason, msg := h.stateChanged(s, claim); reason != "" {
		h.abort(claim, reason, msg)
		return reconcile.Result{}
	}

	var requeue time.Duration

	if grantedReason(claim) == vmaccondition.Frozen {
		if isFrozen(s.KVVMI) {
			h.unfrozen.reset(claim.UID)
		} else {
			unfrozenFor := h.unfrozen.elapsed(claim.UID, h.now())
			if unfrozenFor > UnfrozenTolerance {
				h.abort(claim, vmaccondition.FilesystemUnfrozenUnexpectedly, msgUnfrozenUnexpectedly(vm))
				return reconcile.Result{}
			}

			requeue = UnfrozenTolerance - unfrozenFor
		}
	}

	if claim.Status.Deadline != nil && !h.now().Before(claim.Status.Deadline.Time) {
		h.abort(claim, vmaccondition.DeadlineExceeded, msgDeadlineExceeded(vm, claim.Spec.Timeout.Duration))
		return reconcile.Result{}
	}

	return mergeResults(reconcile.Result{RequeueAfter: requeue}, h.deadlineRequeue(claim))
}

// releasedUnfrozen reports a frozen session released past its deadline while the guest no longer reads
// frozen: the safety unfreeze in the guest may have fired before the capture finished.
func (h *LifecycleHandler) releasedUnfrozen(s *state.State, claim *coordinationv1alpha1.VirtualMachineAccessClaim) bool {
	return claim.Status.Frozen &&
		claim.Status.Deadline != nil && !h.now().Before(claim.Status.Deadline.Time) &&
		!isFrozen(s.KVVMI)
}

// instanceReplaced reports a running instance other than the one the session started with.
func instanceReplaced(s *state.State, claim *coordinationv1alpha1.VirtualMachineAccessClaim) bool {
	return isRunning(s.KVVMI) && s.KVVMI.UID != claim.Status.VirtualMachineInstanceUID
}

// stateChanged reports a change of the virtual machine that ends the session of a claim holding the lock,
// or an empty reason.
func (h *LifecycleHandler) stateChanged(s *state.State, claim *coordinationv1alpha1.VirtualMachineAccessClaim) (vmaccondition.GrantedReason, string) {
	vm := s.Key().Name

	switch {
	case s.VM == nil || s.VM.UID != claim.Spec.VirtualMachine.UID:
		return vmaccondition.VirtualMachineStateChanged, msgVMRecreated(vm)
	case runningUID(s.KVVMI) != claim.Status.VirtualMachineInstanceUID:
		return vmaccondition.VirtualMachineStateChanged, msgVMInstanceChanged(vm)
	// A stopping instance stays Running until it is gone, and a frozen guest cannot shut down gracefully,
	// so the session ends as soon as the stop begins.
	case isRunning(s.KVVMI) && s.KVVMI.DeletionTimestamp != nil:
		return vmaccondition.VirtualMachineStateChanged, msgVMInstanceChanged(vm)
	case s.Migration != nil:
		return vmaccondition.VirtualMachineStateChanged, msgMigrationStarted(vm)
	}

	return "", ""
}

// finish ends the session of a claim holding the lock: it unfreezes the guest the session may have frozen,
// and releases the lock.
//
// The unfreeze is sent whatever fsFreezeStatus reads, as the field can read empty for a frozen guest. The lock
// is held until the request goes through and fsFreezeStatus stops reading frozen, so the next claim never sees
// this session's freeze; past UnfreezeConfirmTimeout the guest is left to its safety unfreeze. A session whose
// freeze was never confirmed resends the unfreeze for LateFreezeUnfreezeWindow.
func (h *LifecycleHandler) finish(ctx context.Context, s *state.State, c *state.Claim) (reconcile.Result, error) {
	claim := c.Changed
	vm := s.Key().Name
	log := logger.FromContext(ctx).With(slog.String("claim", claim.Name))

	if !mayHoldFreeze(s, claim) {
		if claim.Status.Frozen {
			h.setUnfrozen(claim, metav1.ConditionTrue, vmaccondition.UnfreezeConfirmed, msgUnfreezeConfirmed(vm))
		}

		return h.releaseWritten(ctx, s, c)
	}

	if !claim.Status.Frozen {
		// A freeze request may still land in the guest, and only a connected agent can carry it out.
		if state.AgentConnected(s.KVVMI) {
			h.sendUnfreeze(ctx, s, claim.UID)

			if h.now().Sub(sessionEndedAt(claim)) < LateFreezeUnfreezeWindow {
				return reconcile.Result{RequeueAfter: PollInterval}, nil
			}
		}

		return h.releaseWritten(ctx, s, c)
	}

	if cond := findCondition(claim, vmaccondition.UnfrozenType); cond == nil {
		h.setUnfrozen(claim, metav1.ConditionFalse, vmaccondition.Unfreezing, msgUnfreezing(vm))
	} else if cond.Reason != vmaccondition.Unfreezing.String() {
		// The unfreeze is already decided, and only its write is left to wait for.
		return h.releaseWritten(ctx, s, c)
	}

	if h.unfreezeAccepted.has(claim.UID) || h.sendUnfreeze(ctx, s, claim.UID) {
		if !isFrozen(s.KVVMI) {
			h.setUnfrozen(claim, metav1.ConditionTrue, vmaccondition.UnfreezeConfirmed, msgUnfreezeConfirmed(vm))
			return h.releaseWritten(ctx, s, c)
		}

		h.unfreezeAccepted.add(claim.UID)
	}

	if h.now().Sub(conditionSince(claim, vmaccondition.UnfrozenType)) > UnfreezeConfirmTimeout {
		log.Error("the guest did not confirm the unfreeze; releasing the lock and leaving the guest to its safety unfreeze")
		h.setUnfrozen(claim, metav1.ConditionFalse, vmaccondition.UnfreezeNotConfirmed, msgUnfreezeNotConfirmed(vm, UnfreezeConfirmTimeout))

		return h.releaseWritten(ctx, s, c)
	}

	return reconcile.Result{RequeueAfter: PollInterval}, nil
}

// sessionEndedAt returns when the session of a claim that reached finish was over: the time of its verdict,
// or of its deletion when it was deleted before any verdict.
func sessionEndedAt(claim *coordinationv1alpha1.VirtualMachineAccessClaim) time.Time {
	if !isTerminal(claim) && claim.DeletionTimestamp != nil {
		return claim.DeletionTimestamp.Time
	}

	return conditionSince(claim, vmaccondition.GrantedType)
}

// mayHoldFreeze reports whether the session asked for a freeze and the instance it is bound to still runs.
func mayHoldFreeze(s *state.State, claim *coordinationv1alpha1.VirtualMachineAccessClaim) bool {
	return wantsFreeze(claim) && s.Lock.VMIUID != "" && isRunning(s.KVVMI) && s.KVVMI.UID == s.Lock.VMIUID
}

// dropOrphanedLock releases a lock whose claim is gone, which happens only when the claim's finalizer is
// removed by hand. A running instance the lock is bound to is unfrozen first, for up to
// UnfreezeConfirmTimeout.
func (h *LifecycleHandler) dropOrphanedLock(ctx context.Context, s *state.State) (reconcile.Result, error) {
	claimUID := s.Lock.ClaimUID

	if s.Lock.VMIUID != "" && isRunning(s.KVVMI) && s.KVVMI.UID == s.Lock.VMIUID {
		if !h.sendUnfreeze(ctx, s, claimUID) && h.orphaned.elapsed(claimUID, h.now()) < UnfreezeConfirmTimeout {
			return reconcile.Result{RequeueAfter: PollInterval}, nil
		}
	}

	logger.FromContext(ctx).Warn("the claim holding the virtual machine is gone; releasing its lock",
		slog.String("claimUID", string(claimUID)))

	return reconcile.Result{}, h.release(ctx, s, claimUID)
}

// releaseWritten releases the lock of the claim once its final status is written, and requeues until
// then, so a claim never reads as granted while another pass can take the lock.
func (h *LifecycleHandler) releaseWritten(ctx context.Context, s *state.State, c *state.Claim) (reconcile.Result, error) {
	if !equality.Semantic.DeepEqual(c.Current.Status, c.Changed.Status) {
		return reconcile.Result{RequeueAfter: StatusWriteRecheck}, nil
	}

	return reconcile.Result{}, h.release(ctx, s, c.Changed.UID)
}

func (h *LifecycleHandler) release(ctx context.Context, s *state.State, claimUID types.UID) error {
	if err := s.ReleaseLock(ctx); err != nil {
		return err
	}

	h.forget(claimUID)

	return nil
}

// sendFreeze requests the freeze, at most once per RequestResendInterval, and reports whether a request
// went through.
func (h *LifecycleHandler) sendFreeze(ctx context.Context, s *state.State, claim *coordinationv1alpha1.VirtualMachineAccessClaim) bool {
	if !h.freezeSent.due(claim.UID, h.now(), RequestResendInterval) {
		return false
	}

	unfreezeTimeout := FreezeConfirmTimeout + claim.Spec.Timeout.Duration + SafetyUnfreezeMargin

	key := s.Key()
	if err := h.freezer.Freeze(ctx, key.Namespace, key.Name, claim.UID, unfreezeTimeout); err != nil {
		logger.FromContext(ctx).Warn("failed to request the filesystem freeze, will retry", logger.SlogErr(err))
		return false
	}

	return true
}

// sendUnfreeze requests the unfreeze, at most once per RequestResendInterval, and reports whether a
// request went through.
func (h *LifecycleHandler) sendUnfreeze(ctx context.Context, s *state.State, claimUID types.UID) bool {
	if !h.unfreezeSent.due(claimUID, h.now(), RequestResendInterval) {
		return false
	}

	key := s.Key()
	if err := h.freezer.Unfreeze(ctx, key.Namespace, key.Name, claimUID); err != nil {
		logger.FromContext(ctx).Warn("failed to request the filesystem unfreeze, will retry", logger.SlogErr(err))
		return false
	}

	return true
}

func (h *LifecycleHandler) forgetStaleFreeze(vmiUID types.UID) {
	h.staleUnfreezing.reset(vmiUID)
	h.staleUnfreezeSent.reset(vmiUID)
}

func (h *LifecycleHandler) forget(claimUID types.UID) {
	h.unfrozen.reset(claimUID)
	h.lockLost.reset(claimUID)
	h.orphaned.reset(claimUID)
	h.freezeSent.reset(claimUID)
	h.unfreezeSent.reset(claimUID)
	h.unfreezeAccepted.remove(claimUID)
}

func (h *LifecycleHandler) grant(claim *coordinationv1alpha1.VirtualMachineAccessClaim, reason vmaccondition.GrantedReason, msg string) {
	now := metav1.NewTime(h.now())
	claim.Status.Phase = coordinationv1alpha1.AccessClaimPhaseGranted
	claim.Status.Frozen = reason == vmaccondition.Frozen
	claim.Status.GrantedAt = &now
	claim.Status.Deadline = &metav1.Time{Time: now.Add(claim.Spec.Timeout.Duration)}
	h.setGranted(claim, metav1.ConditionTrue, reason, msg)
}

func (h *LifecycleHandler) deny(claim *coordinationv1alpha1.VirtualMachineAccessClaim, reason vmaccondition.GrantedReason, msg string) {
	claim.Status.Phase = coordinationv1alpha1.AccessClaimPhaseDenied
	h.setGranted(claim, metav1.ConditionFalse, reason, msg)
}

func (h *LifecycleHandler) abort(claim *coordinationv1alpha1.VirtualMachineAccessClaim, reason vmaccondition.GrantedReason, msg string) {
	claim.Status.Phase = coordinationv1alpha1.AccessClaimPhaseAborted
	h.setGranted(claim, metav1.ConditionFalse, reason, msg)
}

func (h *LifecycleHandler) complete(claim *coordinationv1alpha1.VirtualMachineAccessClaim, vm string) {
	claim.Status.Phase = coordinationv1alpha1.AccessClaimPhaseCompleted
	h.setGranted(claim, metav1.ConditionFalse, vmaccondition.Released, msgReleased(vm))
}

func (h *LifecycleHandler) setGranted(claim *coordinationv1alpha1.VirtualMachineAccessClaim, status metav1.ConditionStatus, reason vmaccondition.GrantedReason, msg string) {
	h.setCondition(claim, vmaccondition.GrantedType, status, reason.String(), msg)
}

func (h *LifecycleHandler) setUnfrozen(claim *coordinationv1alpha1.VirtualMachineAccessClaim, status metav1.ConditionStatus, reason vmaccondition.UnfrozenReason, msg string) {
	h.setCondition(claim, vmaccondition.UnfrozenType, status, reason.String(), msg)
}

// setCondition sets a condition, moving its lastTransitionTime on any change of status or reason. The
// coordinator measures its waits from that time, so a new reason has to start a new wait.
func (h *LifecycleHandler) setCondition(claim *coordinationv1alpha1.VirtualMachineAccessClaim, t vmaccondition.Type, status metav1.ConditionStatus, reason, msg string) {
	claim.Status.ObservedGeneration = claim.Generation
	cond := metav1.Condition{
		Type:               t.String(),
		Status:             status,
		Reason:             reason,
		Message:            msg,
		ObservedGeneration: claim.Generation,
		LastTransitionTime: metav1.NewTime(h.now()),
	}

	for i := range claim.Status.Conditions {
		existing := &claim.Status.Conditions[i]
		if existing.Type != cond.Type {
			continue
		}

		if existing.Status == cond.Status && existing.Reason == cond.Reason {
			cond.LastTransitionTime = existing.LastTransitionTime
		}

		*existing = cond

		return
	}

	claim.Status.Conditions = append(claim.Status.Conditions, cond)
}

func (h *LifecycleHandler) deadlineRequeue(claim *coordinationv1alpha1.VirtualMachineAccessClaim) reconcile.Result {
	if claim.Status.Deadline == nil {
		return reconcile.Result{}
	}

	return reconcile.Result{RequeueAfter: max(claim.Status.Deadline.Sub(h.now()), time.Second)}
}

func findCondition(claim *coordinationv1alpha1.VirtualMachineAccessClaim, t vmaccondition.Type) *metav1.Condition {
	return meta.FindStatusCondition(claim.Status.Conditions, t.String())
}

func conditionSince(claim *coordinationv1alpha1.VirtualMachineAccessClaim, t vmaccondition.Type) time.Time {
	if c := findCondition(claim, t); c != nil {
		return c.LastTransitionTime.Time
	}

	return claim.CreationTimestamp.Time
}

func grantedReason(claim *coordinationv1alpha1.VirtualMachineAccessClaim) vmaccondition.GrantedReason {
	if c := findCondition(claim, vmaccondition.GrantedType); c != nil {
		return vmaccondition.GrantedReason(c.Reason)
	}

	return ""
}

func isTerminal(claim *coordinationv1alpha1.VirtualMachineAccessClaim) bool {
	switch claim.Status.Phase {
	case coordinationv1alpha1.AccessClaimPhaseDenied,
		coordinationv1alpha1.AccessClaimPhaseAborted,
		coordinationv1alpha1.AccessClaimPhaseCompleted:
		return true
	}

	return false
}

func wantsFreeze(claim *coordinationv1alpha1.VirtualMachineAccessClaim) bool {
	return slices.Contains(claim.Spec.Access, coordinationv1alpha1.AccessModeFreeze)
}

func freezePolicy(claim *coordinationv1alpha1.VirtualMachineAccessClaim) coordinationv1alpha1.FreezePolicy {
	if claim.Spec.FreezePolicy == nil {
		return coordinationv1alpha1.FreezePolicyRequired
	}

	return *claim.Spec.FreezePolicy
}

func isRunning(kvvmi *virtv1.VirtualMachineInstance) bool {
	return kvvmi != nil && kvvmi.Status.Phase == virtv1.Running
}

// startingOrStopping reports an instance on its way up or down: not running yet, or running with its
// deletion begun.
func startingOrStopping(kvvmi *virtv1.VirtualMachineInstance) bool {
	if kvvmi == nil {
		return false
	}

	if isRunning(kvvmi) {
		return kvvmi.DeletionTimestamp != nil
	}

	return !isFinal(kvvmi)
}

func isFinal(kvvmi *virtv1.VirtualMachineInstance) bool {
	return kvvmi != nil && (kvvmi.Status.Phase == virtv1.Succeeded || kvvmi.Status.Phase == virtv1.Failed)
}

// runningUID is the UID a session records for the instance: empty unless the instance is running, so a
// stopped virtual machine and one whose instance has finished compare equal.
func runningUID(kvvmi *virtv1.VirtualMachineInstance) types.UID {
	if isRunning(kvvmi) {
		return kvvmi.UID
	}

	return ""
}

func isFrozen(kvvmi *virtv1.VirtualMachineInstance) bool {
	return kvvmi != nil && kvvmi.Status.FSFreezeStatus == service.FSFrozen
}

// frozenOutsideCoordinator reports a guest frozen without a lock, or a pending freeze request of a snapshot
// taken before the module update. An unfreeze request does not count, as it never leaves the guest frozen.
func frozenOutsideCoordinator(kvvmi *virtv1.VirtualMachineInstance) bool {
	if kvvmi == nil {
		return false
	}

	return kvvmi.Annotations[annotations.AnnVMFilesystemRequest] == service.RequestFSFreeze || isFrozen(kvvmi)
}

// migrationOperation returns the VirtualMachineOperation that controls the migration, or nil.
func migrationOperation(m *virtv1.VirtualMachineInstanceMigration) *metav1.OwnerReference {
	owner := metav1.GetControllerOf(m)
	if owner == nil || owner.Kind != v1alpha2.VirtualMachineOperationKind {
		return nil
	}

	return owner
}

func mergeResults(a, b reconcile.Result) reconcile.Result {
	switch {
	case a.RequeueAfter == 0:
		return b
	case b.RequeueAfter == 0:
		return a
	default:
		return reconcile.Result{RequeueAfter: min(a.RequeueAfter, b.RequeueAfter)}
	}
}
