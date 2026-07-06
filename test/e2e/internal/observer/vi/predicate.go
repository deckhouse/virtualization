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

// HaveFormat reports an invariant violation when a Ready VirtualImage reports a
// status.format different from the expected on-disk format.
func HaveFormat(expected string) Predicate {
	return func(i *v1alpha2.VirtualImage) (bool, error) {
		if i.Status.Phase != v1alpha2.ImageReady {
			return true, nil
		}
		if i.Status.Format != expected {
			return false, fmt.Errorf("status.format is %q, expected %q", i.Status.Format, expected)
		}
		return true, nil
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

// ProgressExpectations describes which progress values a scenario must observe
// before the VirtualImage reaches Ready.
type ProgressExpectations struct {
	RequireZero                    bool
	RequireIntermediateExceptFifty bool
	RequireHundred                 bool
}

// HaveValidProgress enforces the common VirtualImage progress contract and the
// scenario-specific coverage expectations.
func HaveValidProgress(expect ProgressExpectations) Predicate {
	var (
		previous *float64
		observed progressObservations
	)

	return func(i *v1alpha2.VirtualImage) (bool, error) {
		if i.Status.Phase == v1alpha2.ImagePending && i.Status.Progress != "" {
			return false, fmt.Errorf("phase is Pending but progress is %q, expected empty progress", i.Status.Progress)
		}
		if i.Status.Phase == v1alpha2.ImageReady && i.Status.Progress == "" {
			return false, errors.New("phase is Ready but progress is empty, expected 100%")
		}
		if i.Status.Progress == "" {
			return true, nil
		}

		current, err := parseProgress(i.Status.Progress)
		if err != nil {
			return false, err
		}

		if current == 100 && i.Status.Phase != v1alpha2.ImageReady {
			return false, fmt.Errorf("progress is 100%% but phase is %s, expected Ready", displayPhase(i.Status.Phase))
		}
		if i.Status.Phase == v1alpha2.ImageReady && current != 100 {
			return false, fmt.Errorf("phase is Ready but progress is %q, expected 100%%", i.Status.Progress)
		}
		if previous != nil && current < *previous {
			return false, fmt.Errorf("progress decreased from %s to %s", formatProgressValue(*previous), formatProgressValue(current))
		}
		if previous != nil && current == *previous && current == 100 && i.Status.Phase == v1alpha2.ImageReady {
			return observed.satisfies(expect)
		}

		// TODO: LINSTOR thin pool lock contention can stall all storage writes on a
		// node for over a minute without surfacing any error, making progress-rate
		// checks unreliable on this cluster. The time-budget enforcement is disabled
		// until the underlying storage issue is resolved.
		//
		// if previous != nil {
		// 	budget := updateInterval
		// 	if isProgressLongPauseValue(*previous) {
		// 		budget = boundaryBudget
		// 	}
		// 	if gap := time.Since(lastAdvance); gap > budget {
		// 		return false, fmt.Errorf(
		// 			"progress stayed at %s for %s before %s; it must grow at least every %s (0%%, 50%% and 100%% may stay up to %s)",
		// 			formatProgressValue(*previous), gap.Round(time.Second), formatProgressValue(current), updateInterval, boundaryBudget,
		// 		)
		// 	}
		// }
		if previous != nil && current == *previous && i.Status.Phase != v1alpha2.ImageReady {
			return true, nil
		}

		observed.record(current)
		previous = &current

		if i.Status.Phase != v1alpha2.ImageReady {
			return true, nil
		}
		return observed.satisfies(expect)
	}
}

type progressObservations struct {
	hasZero                    bool
	hasIntermediateExceptFifty bool
	hasHundred                 bool
}

func (o *progressObservations) record(p float64) {
	switch {
	case p == 0:
		o.hasZero = true
	case p > 0 && p < 50:
		o.hasIntermediateExceptFifty = true
	case p > 50 && p < 100:
		o.hasIntermediateExceptFifty = true
	case p > 0 && p < 100 && p != 50:
		o.hasIntermediateExceptFifty = true
	case p == 100:
		o.hasHundred = true
	}
}

func (o progressObservations) satisfies(expect ProgressExpectations) (bool, error) {
	switch {
	case expect.RequireZero && !o.hasZero:
		return false, errors.New("progress reached Ready without observing 0%")
	case expect.RequireIntermediateExceptFifty && !o.hasIntermediateExceptFifty:
		return false, errors.New("progress reached Ready without observing a value in (0%;100%) different from 50%")
	case expect.RequireHundred && !o.hasHundred:
		return false, errors.New("progress reached Ready without observing 100%")
	default:
		return true, nil
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
