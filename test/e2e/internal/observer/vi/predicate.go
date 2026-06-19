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

package vi

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vicondition"
)

// BeFailed reports an invariant violation when the VirtualImage has reached
// the terminal Failed phase or its Ready condition reports the
// ProvisioningFailed reason. It is intended to be used with [Observer.Never].
func BeFailed() Predicate {
	return func(i *v1alpha2.VirtualImage) (bool, error) {
		if i.Status.Phase == v1alpha2.ImageFailed {
			return true, fmt.Errorf("VirtualImage entered Failed phase")
		}
		if cond := findCondition(i.Status.Conditions, vicondition.ReadyType.String()); cond != nil {
			if isConditionFresh(cond, i) && cond.Reason == vicondition.ProvisioningFailed.String() {
				return true, fmt.Errorf("ready condition reports ProvisioningFailed: %s", cond.Message)
			}
		}
		return false, nil
	}
}

// BeReady reports the VirtualImage has finished provisioning.
//
// The predicate is satisfied only when the phase is Ready and the Ready
// condition is True/Ready and is not stale. Inconsistencies (phase Ready
// without a fresh Ready condition matching it) produce a (false, error)
// pair so that any WaitFor caller fails immediately. Intended for use with
// [Observer.WaitFor].
func BeReady() Predicate {
	return func(i *v1alpha2.VirtualImage) (bool, error) {
		readyCond := findCondition(i.Status.Conditions, vicondition.ReadyType.String())

		condStale := readyCond != nil && !isConditionFresh(readyCond, i)
		condIsReady := readyCond != nil &&
			!condStale &&
			readyCond.Status == metav1.ConditionTrue &&
			readyCond.Reason == vicondition.Ready.String()
		phaseIsReady := i.Status.Phase == v1alpha2.ImageReady

		switch {
		case phaseIsReady && condStale:
			return false, nil
		case phaseIsReady && !condIsReady:
			return false, fmt.Errorf(
				"phase is Ready but Ready condition is %s/%s (message: %q), expected True/%s",
				condStatus(readyCond), condReason(readyCond), condMessage(readyCond), vicondition.Ready,
			)
		case condIsReady && !phaseIsReady:
			return false, fmt.Errorf(
				"ready condition is True/%s but phase is %q, expected %q",
				vicondition.Ready, i.Status.Phase, v1alpha2.ImageReady,
			)
		case !phaseIsReady:
			return false, nil
		}

		return true, nil
	}
}

// quotaExceededMessagePrefix is the prefix the controller prepends to
// the Ready condition message when the project quota is exhausted.
const quotaExceededMessagePrefix = "Quota exceeded"

// BeQuotaExceeded reports the VirtualImage has been parked in a
// quota-exhausted state.
//
// The predicate is satisfied when the Ready condition is fresh,
// reports Status=False with Reason=ProvisioningFailed, the message is
// prefixed with "Quota exceeded" (the controller wraps the upstream
// "exceeded quota:" Kubernetes error into a "Quota exceeded:" message),
// and the phase is Failed.
//
// Returned values:
//   - (true, nil)  - the VirtualImage reports a fresh quota-exceeded
//     Ready condition together with the Failed phase;
//   - (false, nil) - the controller has not yet reported a fresh
//     quota-exceeded Ready condition;
//   - (false, err) - the quota-exceeded message is reported with an
//     unexpected phase or Status, which is a controller bug.
//
// Intended for use with [Observer.WaitFor].
func BeQuotaExceeded() Predicate {
	return func(i *v1alpha2.VirtualImage) (bool, error) {
		cond := findCondition(i.Status.Conditions, vicondition.ReadyType.String())
		if cond == nil || !isConditionFresh(cond, i) {
			return false, nil
		}
		if cond.Reason != vicondition.ProvisioningFailed.String() {
			return false, nil
		}
		if !strings.HasPrefix(cond.Message, quotaExceededMessagePrefix) {
			return false, nil
		}
		if cond.Status != metav1.ConditionFalse {
			return false, fmt.Errorf(
				"ready condition reports a quota-exceeded ProvisioningFailed but status is %s, expected %s",
				cond.Status, metav1.ConditionFalse,
			)
		}
		if i.Status.Phase != v1alpha2.ImageFailed {
			return false, fmt.Errorf(
				"ready condition reports a quota-exceeded ProvisioningFailed but phase is %q, expected %q",
				i.Status.Phase, v1alpha2.ImageFailed,
			)
		}
		return true, nil
	}
}

// BeReadyForUserUpload reports the VirtualImage has reached the
// WaitForUserUpload phase and exposes a usable external upload URL.
func BeReadyForUserUpload() Predicate {
	return func(i *v1alpha2.VirtualImage) (bool, error) {
		if i.Status.Phase != v1alpha2.ImageWaitForUserUpload {
			return false, nil
		}
		if i.Status.ImageUploadURLs == nil {
			return false, errors.New("phase is WaitForUserUpload but ImageUploadURLs is nil")
		}
		if i.Status.ImageUploadURLs.External == "" {
			return false, errors.New("phase is WaitForUserUpload but external upload URL is empty")
		}
		return true, nil
	}
}

// HaveValidPhaseTransitions reports an invariant violation when
// VirtualImage.Status.Phase regresses to an earlier point of the
// provisioning lifecycle.
//
// The phases observed during provisioning are organized into ordered
// milestones:
//
//	0: ""                     (the controller has not yet computed a phase)
//	1: Pending
//	2: Provisioning, WaitForUserUpload
//	3: Ready
//
// Rank-2 phases are considered equivalent: Provisioning may flip to
// WaitForUserUpload (and back) while the controller waits for the user
// upload. Once a higher milestone has been observed, the phase must not
// regress to a lower one. For example, observing "" or Pending after
// Provisioning, or any of the rank-0..2 phases after Ready, is a
// violation.
//
// Phases that are not part of the provisioning happy path (Failed,
// Terminating, ImageLost) are skipped: they are handled by other
// invariants (for example [BeFailed]) and may legally follow Ready in
// unrelated lifecycle scenarios.
//
// Intended for use with [Observer.Always].
func HaveValidPhaseTransitions() Predicate {
	var (
		maxRank     int
		maxPhase    v1alpha2.ImagePhase
		hasObserved bool
	)

	return func(i *v1alpha2.VirtualImage) (bool, error) {
		rank, known := imagePhaseRank(i.Status.Phase)
		if !known {
			return true, nil
		}

		if hasObserved && rank < maxRank {
			return false, fmt.Errorf(
				"phase regressed from %s to %s",
				displayPhase(maxPhase), displayPhase(i.Status.Phase),
			)
		}

		if !hasObserved || rank > maxRank {
			maxRank = rank
			maxPhase = i.Status.Phase
		}
		hasObserved = true
		return true, nil
	}
}

// imagePhaseRank returns the milestone rank of a VirtualImage phase along
// the provisioning happy path. Phases outside that path are reported as
// unknown (false) so that callers can skip them.
func imagePhaseRank(phase v1alpha2.ImagePhase) (int, bool) {
	switch phase {
	case "":
		return 0, true
	case v1alpha2.ImagePending:
		return 1, true
	case v1alpha2.ImageProvisioning,
		v1alpha2.ImageWaitForUserUpload:
		return 2, true
	case v1alpha2.ImageReady:
		return 3, true
	default:
		return 0, false
	}
}

func displayPhase(phase v1alpha2.ImagePhase) string {
	if phase == "" {
		return `""`
	}
	return fmt.Sprintf("%q", string(phase))
}

// HaveNonDecreasingProgress reports an invariant violation when
// VirtualImage.Status.Progress moves backwards between observed states.
func HaveNonDecreasingProgress() Predicate {
	var previous *float64

	return func(i *v1alpha2.VirtualImage) (bool, error) {
		if i.Status.Progress == "" {
			return true, nil
		}

		current, err := parseProgress(i.Status.Progress)
		if err != nil {
			return false, err
		}

		if previous != nil && current < *previous {
			return false, fmt.Errorf("progress decreased from %.2f%% to %.2f%%", *previous, current)
		}

		previous = &current
		return true, nil
	}
}

// HaveProgressWhileProvisioning reports an invariant violation when a
// VirtualImage in the Provisioning phase does not expose a progress
// percentage. The controller must always publish a percentage while
// provisioning (at least "0%" before the importer reports real progress), so
// an empty Progress in the Provisioning phase is a controller bug. Phases
// other than Provisioning are skipped. Intended for use with [Observer.Always].
func HaveProgressWhileProvisioning() Predicate {
	return func(i *v1alpha2.VirtualImage) (bool, error) {
		if i.Status.Phase != v1alpha2.ImageProvisioning {
			return true, nil
		}
		if i.Status.Progress == "" {
			return false, errors.New("phase is Provisioning but progress is empty, expected a percentage (at least \"0%\")")
		}
		if _, err := parseProgress(i.Status.Progress); err != nil {
			return false, fmt.Errorf("phase is Provisioning but progress is invalid: %w", err)
		}
		return true, nil
	}
}

// HaveNoProgressBeforeProvisioning reports an invariant violation when
// a VirtualImage exposes a progress percentage while it is still in the
// pre-Provisioning phases ("" or Pending). The progress percentage
// describes how much of the import has been transferred and is
// meaningful only once the import has actually started, so any
// non-empty Progress at this stage is a controller bug.
//
// Phases at and beyond Provisioning are skipped (the in-import behaviour
// is enforced by [HaveProgressWhileProvisioning] and
// [HaveTimelyProgress]).
//
// Intended for use with [Observer.Always].
func HaveNoProgressBeforeProvisioning() Predicate {
	return func(i *v1alpha2.VirtualImage) (bool, error) {
		switch i.Status.Phase {
		case "", v1alpha2.ImagePending:
			// fall through to the check below
		default:
			return true, nil
		}
		if i.Status.Progress == "" {
			return true, nil
		}
		return false, fmt.Errorf(
			"phase is %s but progress is %q, expected an empty progress until the import enters Provisioning",
			displayPhase(i.Status.Phase), i.Status.Progress,
		)
	}
}

// HaveTimelyProgress reports an invariant violation when, during active
// provisioning, VirtualImage.Status.Progress does not stream smoothly.
//
// The import must report distinct percentages frequently, so two checks are
// enforced while the image is in the Provisioning phase:
//
//   - Timeliness: progress must advance at least once every threshold. The only
//     exception are the two stage boundaries 0% and 50% - the points where the
//     import legitimately pauses (import-pod scheduling at 0% and the DVCR->PVC
//     hand-off at 50%); they may stay unchanged up to boundaryBudget, never
//     longer.
//   - Coverage: progress must actually stream intermediate percentages. Jumping
//     straight from one stage boundary to the next (0% -> >=50% or 50% -> 100%)
//     means the intermediate values were never reported, so progress would be
//     just 0%/50%/100%, which is rejected. Coverage is also enforced across
//     the Provisioning -> Ready boundary, where the controller would otherwise
//     leap from a still-low progress (e.g. 0%) directly to 100%.
//
// The Pending and WaitForUserUpload phases legitimately sit at 0% (the image is
// waiting for the user upload to begin) and are skipped. Intended for use with
// [Observer.Always].
func HaveTimelyProgress(threshold, boundaryBudget time.Duration) Predicate {
	var (
		tracking     bool
		lastProgress float64
		lastAdvance  time.Time
	)

	return func(i *v1alpha2.VirtualImage) (bool, error) {
		// Provisioning -> Ready boundary: the in-Provisioning jump check
		// below stops tracking the moment the phase leaves Provisioning,
		// so a controller that leaps from a still-low progress directly
		// to 100% at the very moment of transition would slip past it.
		// Re-evaluate the stage-jump rule here using the freshly-observed
		// final progress (typically 100%) and reset tracking afterwards.
		if i.Status.Phase == v1alpha2.ImageReady && tracking {
			final, err := parseProgress(i.Status.Progress)
			if err != nil {
				tracking = false
				return false, fmt.Errorf("phase is Ready but progress is invalid: %w", err)
			}
			tracking = false
			if isProgressStageJump(lastProgress, final) {
				return false, fmt.Errorf(
					"image transitioned to Ready with progress %s after %s; intermediate percentages between %s and %s were never reported",
					formatProgressValue(final), formatProgressValue(lastProgress),
					formatProgressValue(lastProgress), formatProgressValue(final),
				)
			}
			return true, nil
		}

		if i.Status.Phase != v1alpha2.ImageProvisioning {
			// Not importing right now; restart tracking for the next window.
			tracking = false
			return true, nil
		}
		if i.Status.Progress == "" {
			return true, nil
		}

		current, err := parseProgress(i.Status.Progress)
		if err != nil {
			return false, err
		}

		now := time.Now()
		if !tracking {
			tracking = true
			lastProgress = current
			lastAdvance = now
			return true, nil
		}
		if current == lastProgress {
			return true, nil
		}

		budget := threshold
		if isProgressBoundary(lastProgress) {
			budget = boundaryBudget
		}
		if gap := now.Sub(lastAdvance); gap > budget {
			return false, fmt.Errorf(
				"progress stalled at %s for %s before advancing to %s; it must update at least every %s (only 0%%/50%% may pause, up to %s)",
				formatProgressValue(lastProgress), gap.Round(time.Second), formatProgressValue(current), threshold, boundaryBudget,
			)
		}

		if isProgressStageJump(lastProgress, current) {
			return false, fmt.Errorf(
				"progress jumped from %s to %s without any intermediate values; it must stream distinct percentages, not only 0%%/50%%/100%%",
				formatProgressValue(lastProgress), formatProgressValue(current),
			)
		}

		lastProgress = current
		lastAdvance = now
		return true, nil
	}
}

// isProgressBoundary reports whether p is one of the two stage boundaries (0% or
// 50%) where the import legitimately pauses.
func isProgressBoundary(p float64) bool {
	return p == 0 || p == 50
}

// isProgressStageJump reports whether advancing from -> to skips a whole import
// stage's stream of intermediate values: 0% straight to >=50%, or 50% straight
// to 100%.
func isProgressStageJump(from, to float64) bool {
	switch {
	case from == 0 && to >= 50:
		return true
	case from == 50 && to >= 100:
		return true
	default:
		return false
	}
}

// formatProgressValue renders a parsed progress percentage the same way the
// controller does: 0%/100% without a fraction, everything else with one decimal.
func formatProgressValue(p float64) string {
	switch p {
	case 0:
		return "0%"
	case 100:
		return "100%"
	default:
		return fmt.Sprintf("%.1f%%", p)
	}
}

func parseProgress(progress string) (float64, error) {
	value := strings.TrimSuffix(progress, "%")
	if value == progress {
		return 0, fmt.Errorf("progress %q does not have %% suffix", progress)
	}

	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0, fmt.Errorf("parse progress %q: %w", progress, err)
	}
	if parsed < 0 || parsed > 100 {
		return 0, fmt.Errorf("progress %q is outside 0..100 range", progress)
	}
	return parsed, nil
}

func isConditionFresh(cond *metav1.Condition, i *v1alpha2.VirtualImage) bool {
	return cond.ObservedGeneration == i.GetGeneration()
}

func findCondition(conds []metav1.Condition, condType string) *metav1.Condition {
	for i := range conds {
		if conds[i].Type == condType {
			return &conds[i]
		}
	}
	return nil
}

func condStatus(cond *metav1.Condition) metav1.ConditionStatus {
	if cond == nil {
		return "<absent>"
	}
	return cond.Status
}

func condReason(cond *metav1.Condition) string {
	if cond == nil {
		return "<absent>"
	}
	return cond.Reason
}

func condMessage(cond *metav1.Condition) string {
	if cond == nil {
		return ""
	}
	return cond.Message
}
