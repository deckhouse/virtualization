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

package vd

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdcondition"
)

// readyProgress is the value of VirtualDisk.Status.Progress when the disk
// has finished provisioning.
const readyProgress = "100%"

// BeFailed reports an invariant violation when the VirtualDisk has reached
// the terminal Failed phase or its Ready condition reports the
// ProvisioningFailed reason. It is intended to be used with [Observer.Never].
func BeFailed() Predicate {
	return func(d *v1alpha2.VirtualDisk) (bool, error) {
		if d.Status.Phase == v1alpha2.DiskFailed {
			return true, fmt.Errorf("VirtualDisk entered Failed phase")
		}
		if cond := findCondition(d.Status.Conditions, vdcondition.ReadyType.String()); cond != nil {
			if isConditionFresh(cond, d) && cond.Reason == vdcondition.ProvisioningFailed.String() {
				return true, fmt.Errorf("ready condition reports ProvisioningFailed: %s", cond.Message)
			}
		}
		return false, nil
	}
}

// BeStorageClassReady reports the StorageClassReady condition is healthy.
//
// The condition is treated as healthy when:
//   - it is absent (the controller has not yet computed it);
//   - it is stale, i.e. its observedGeneration does not match the resource
//     generation (the test should wait for the controller to refresh it);
//   - it has Status=True with Reason=StorageClassReady.
//
// Any other state is reported as a definite invariant violation. Intended
// for use with [Observer.Always].
func BeStorageClassReady() Predicate {
	return func(d *v1alpha2.VirtualDisk) (bool, error) {
		cond := findCondition(d.Status.Conditions, vdcondition.StorageClassReadyType.String())
		if cond == nil || !isConditionFresh(cond, d) {
			return true, nil
		}
		if cond.Status != metav1.ConditionTrue {
			return false, fmt.Errorf(
				"StorageClassReady condition is %s/%s (message: %q), expected True/%s",
				cond.Status, cond.Reason, cond.Message, vdcondition.StorageClassReady,
			)
		}
		if cond.Reason != vdcondition.StorageClassReady.String() {
			return false, fmt.Errorf(
				"StorageClassReady reason is %q, expected %q",
				cond.Reason, vdcondition.StorageClassReady,
			)
		}
		return true, nil
	}
}

// BeDataSourceReady reports the DatasourceReady condition is healthy.
//
// The condition is treated as healthy under the same rules as for
// [BeStorageClassReady] (absent, stale, or True/DatasourceReady). The
// controller legitimately removes this condition once the disk has reached
// the Ready phase, so an absent condition is always treated as healthy.
// Intended for use with [Observer.Always].
func BeDataSourceReady() Predicate {
	return func(d *v1alpha2.VirtualDisk) (bool, error) {
		cond := findCondition(d.Status.Conditions, vdcondition.DatasourceReadyType.String())
		if cond == nil || !isConditionFresh(cond, d) {
			return true, nil
		}
		if cond.Status != metav1.ConditionTrue {
			return false, fmt.Errorf(
				"DatasourceReady condition is %s/%s (message: %q), expected True/%s",
				cond.Status, cond.Reason, cond.Message, vdcondition.DatasourceReady,
			)
		}
		if cond.Reason != vdcondition.DatasourceReady.String() {
			return false, fmt.Errorf(
				"DatasourceReady reason is %q, expected %q",
				cond.Reason, vdcondition.DatasourceReady,
			)
		}
		return true, nil
	}
}

// BeReady reports the VirtualDisk is fully provisioned.
//
// The predicate is satisfied only when the phase, the Ready condition, the
// progress, the capacity, the target PVC name and the storage class name are
// all populated and consistent with each other. Intended for use with
// [Observer.WaitFor].
//
// Returned values:
//   - (true, nil)  - the disk is ready and every status field is populated;
//   - (false, nil) - the disk is still being provisioned or the Ready
//     condition is stale;
//   - (false, err) - the disk reports an internally inconsistent ready state
//     (phase Ready without a matching Ready condition, or with a missing
//     status field). The error fails the WaitFor immediately.
func BeReady() Predicate {
	return func(d *v1alpha2.VirtualDisk) (bool, error) {
		readyCond := findCondition(d.Status.Conditions, vdcondition.ReadyType.String())

		condStale := readyCond != nil && !isConditionFresh(readyCond, d)
		condIsReady := readyCond != nil &&
			!condStale &&
			readyCond.Status == metav1.ConditionTrue &&
			readyCond.Reason == vdcondition.Ready.String()
		phaseIsReady := d.Status.Phase == v1alpha2.DiskReady

		switch {
		case phaseIsReady && condStale:
			// Wait for the controller to refresh the Ready condition.
			return false, nil
		case phaseIsReady && !condIsReady:
			return false, fmt.Errorf(
				"phase is Ready but Ready condition is %s/%s (message: %q), expected True/%s",
				condStatus(readyCond), condReason(readyCond), condMessage(readyCond), vdcondition.Ready,
			)
		case condIsReady && !phaseIsReady:
			return false, fmt.Errorf(
				"ready condition is True/%s but phase is %q, expected %q",
				vdcondition.Ready, d.Status.Phase, v1alpha2.DiskReady,
			)
		case !phaseIsReady:
			return false, nil
		}

		if d.Status.Progress != readyProgress {
			return false, fmt.Errorf(
				"phase is Ready but progress is %q, expected %q",
				d.Status.Progress, readyProgress,
			)
		}
		if d.Status.Capacity == "" {
			return false, errors.New("phase is Ready but capacity is empty")
		}
		if d.Status.Target.PersistentVolumeClaim == "" {
			return false, errors.New("phase is Ready but target.persistentVolumeClaimName is empty")
		}
		if d.Status.StorageClassName == "" {
			return false, errors.New("phase is Ready but storageClassName is empty")
		}

		return true, nil
	}
}

// BeDetached reports the VirtualDisk is not attached to any VirtualMachine.
func BeDetached() Predicate {
	return func(d *v1alpha2.VirtualDisk) (bool, error) {
		return len(d.Status.AttachedToVirtualMachines) == 0, nil
	}
}

// BeWaitForFirstConsumer reports the VirtualDisk has parked in the
// WaitForFirstConsumer phase, waiting for a consumer (a VirtualMachine) to be
// scheduled before it can provision its volume. It is used to synchronize a disk
// on a WaitForFirstConsumer storage class before creating the VirtualMachine that
// consumes it. Intended for use with [Observer.WaitFor].
func BeWaitForFirstConsumer() Predicate {
	return func(d *v1alpha2.VirtualDisk) (bool, error) {
		return d.Status.Phase == v1alpha2.DiskWaitForFirstConsumer, nil
	}
}

// BeQuotaExceeded reports the VirtualDisk has been parked in a
// quota-exhausted state.
//
// The predicate is satisfied when the Ready condition is fresh,
// reports Status=False with Reason=QuotaExceeded, and the phase is
// either Failed (importer/uploader Pod creation rejected by the
// project quota) or Pending (PVC creation rejected by the project
// quota). Any other phase together with a fresh Reason=QuotaExceeded
// is reported as a definite invariant violation.
//
// Returned values:
//   - (true, nil)  - the VirtualDisk reports a fresh quota-exceeded
//     Ready condition together with a matching phase;
//   - (false, nil) - the controller has not yet reported a fresh
//     quota-exceeded Ready condition;
//   - (false, err) - Reason=QuotaExceeded is reported with an
//     unexpected phase or Status, which is a controller bug.
//
// Intended for use with [Observer.WaitFor].
func BeQuotaExceeded() Predicate {
	return func(d *v1alpha2.VirtualDisk) (bool, error) {
		cond := findCondition(d.Status.Conditions, vdcondition.ReadyType.String())
		if cond == nil || !isConditionFresh(cond, d) {
			return false, nil
		}
		if cond.Reason != vdcondition.QuotaExceeded.String() {
			return false, nil
		}
		if cond.Status != metav1.ConditionFalse {
			return false, fmt.Errorf(
				"ready condition reason is %q but status is %s, expected %s",
				cond.Reason, cond.Status, metav1.ConditionFalse,
			)
		}
		switch d.Status.Phase {
		case v1alpha2.DiskFailed, v1alpha2.DiskPending:
			return true, nil
		default:
			return false, fmt.Errorf(
				"ready condition reason is %q but phase is %q, expected %q or %q",
				cond.Reason, d.Status.Phase, v1alpha2.DiskFailed, v1alpha2.DiskPending,
			)
		}
	}
}

// BeReadyForUserUpload reports the VirtualDisk has reached the
// WaitForUserUpload phase and exposes a usable external upload URL.
//
// Returned values:
//   - (true, nil)  - the disk is in WaitForUserUpload and has both upload
//     URLs populated;
//   - (false, nil) - the disk has not yet reached WaitForUserUpload;
//   - (false, err) - the disk is in WaitForUserUpload but the upload URLs
//     are missing or empty (a controller bug).
func BeReadyForUserUpload() Predicate {
	return func(d *v1alpha2.VirtualDisk) (bool, error) {
		if d.Status.Phase != v1alpha2.DiskWaitForUserUpload {
			return false, nil
		}
		if d.Status.ImageUploadURLs == nil {
			return false, errors.New("phase is WaitForUserUpload but ImageUploadURLs is nil")
		}
		if d.Status.ImageUploadURLs.External == "" {
			return false, errors.New("phase is WaitForUserUpload but external upload URL is empty")
		}
		return true, nil
	}
}

// HaveValidPhaseTransitions reports an invariant violation when
// VirtualDisk.Status.Phase regresses to an earlier point of the
// provisioning lifecycle.
//
// The phases observed during provisioning are organized into ordered
// milestones:
//
//	0: ""                     (the controller has not yet computed a phase)
//	1: Pending
//	2: Provisioning, WaitForUserUpload, WaitForFirstConsumer
//	3: Ready
//
// Rank-2 phases are considered equivalent: Provisioning may flip to
// WaitForUserUpload or WaitForFirstConsumer (and back) while the
// controller waits for the user upload or for the first consumer. Once a
// higher milestone has been observed, the phase must not regress to a
// lower one. For example, observing "" or Pending after Provisioning,
// or any of the rank-0..2 phases after Ready, is a violation.
//
// Phases that are not part of the provisioning happy path (Failed,
// Terminating, PVCLost, Resizing, Migrating) are skipped: they are
// handled by other invariants (for example [BeFailed]) and may legally
// follow Ready in unrelated lifecycle scenarios.
//
// Intended for use with [Observer.Always].
func HaveValidPhaseTransitions() Predicate {
	var (
		maxRank     int
		maxPhase    v1alpha2.DiskPhase
		hasObserved bool
	)

	return func(d *v1alpha2.VirtualDisk) (bool, error) {
		rank, known := diskPhaseRank(d.Status.Phase)
		if !known {
			return true, nil
		}

		if hasObserved && rank < maxRank {
			return false, fmt.Errorf(
				"phase regressed from %s to %s",
				displayPhase(maxPhase), displayPhase(d.Status.Phase),
			)
		}

		if !hasObserved || rank > maxRank {
			maxRank = rank
			maxPhase = d.Status.Phase
		}
		hasObserved = true
		return true, nil
	}
}

// diskPhaseRank returns the milestone rank of a VirtualDisk phase along
// the provisioning happy path. Phases outside that path are reported as
// unknown (false) so that callers can skip them.
func diskPhaseRank(phase v1alpha2.DiskPhase) (int, bool) {
	switch phase {
	case "":
		return 0, true
	case v1alpha2.DiskPending:
		return 1, true
	case v1alpha2.DiskProvisioning,
		v1alpha2.DiskWaitForUserUpload,
		v1alpha2.DiskWaitForFirstConsumer:
		return 2, true
	case v1alpha2.DiskReady:
		return 3, true
	default:
		return 0, false
	}
}

func displayPhase(phase v1alpha2.DiskPhase) string {
	if phase == "" {
		return `""`
	}
	return fmt.Sprintf("%q", string(phase))
}

// ProgressExpectations describes which progress values a scenario must observe
// before the VirtualDisk reaches Ready.
type ProgressExpectations struct {
	RequireZero                    bool
	RequireBetweenZeroAndFifty     bool
	RequireBetweenFiftyAndHundred  bool
	RequireIntermediateExceptFifty bool
	RequireHundred                 bool
}

// HaveValidProgress enforces the common VirtualDisk progress contract and the
// scenario-specific coverage expectations.
func HaveValidProgress(expect ProgressExpectations, updateInterval, boundaryBudget time.Duration) Predicate {
	var (
		previous    *float64
		lastAdvance time.Time
		observed    progressObservations
	)

	return func(d *v1alpha2.VirtualDisk) (bool, error) {
		if d.Status.Phase == v1alpha2.DiskPending && d.Status.Progress != "" {
			return false, fmt.Errorf("phase is Pending but progress is %q, expected empty progress", d.Status.Progress)
		}
		if d.Status.Phase == v1alpha2.DiskReady && d.Status.Progress == "" {
			return false, errors.New("phase is Ready but progress is empty, expected 100%")
		}
		if d.Status.Progress == "" {
			return true, nil
		}

		current, err := parseProgress(d.Status.Progress)
		if err != nil {
			return false, err
		}

		if current == 100 && d.Status.Phase != v1alpha2.DiskReady {
			return false, fmt.Errorf("progress is 100%% but phase is %s, expected Ready", displayPhase(d.Status.Phase))
		}
		if d.Status.Phase == v1alpha2.DiskReady && current != 100 {
			return false, fmt.Errorf("phase is Ready but progress is %q, expected 100%%", d.Status.Progress)
		}
		if previous != nil && current < *previous {
			return false, fmt.Errorf("progress decreased from %s to %s", formatProgressValue(*previous), formatProgressValue(current))
		}
		if previous != nil && current == *previous && current == 100 && d.Status.Phase == v1alpha2.DiskReady {
			return observed.satisfies(expect)
		}

		now := time.Now()
		if previous != nil {
			budget := updateInterval
			if isProgressLongPauseValue(*previous) {
				budget = boundaryBudget
			}
			if gap := now.Sub(lastAdvance); gap > budget {
				return false, fmt.Errorf(
					"progress stayed at %s for %s before %s; it must grow at least every %s (0%%, 50%% and 100%% may stay up to %s)",
					formatProgressValue(*previous), gap.Round(time.Second), formatProgressValue(current), updateInterval, boundaryBudget,
				)
			}
			if current == *previous && d.Status.Phase != v1alpha2.DiskReady {
				return true, nil
			}
		}

		observed.record(current)
		previous = &current
		lastAdvance = now

		if d.Status.Phase != v1alpha2.DiskReady {
			return true, nil
		}
		return observed.satisfies(expect)
	}
}

type progressObservations struct {
	hasZero                    bool
	hasBetweenZeroAndFifty     bool
	hasBetweenFiftyAndHundred  bool
	hasIntermediateExceptFifty bool
	hasHundred                 bool
}

func (o *progressObservations) record(p float64) {
	switch {
	case p == 0:
		o.hasZero = true
	case p > 0 && p < 50:
		o.hasBetweenZeroAndFifty = true
		o.hasIntermediateExceptFifty = true
	case p > 50 && p < 100:
		o.hasBetweenFiftyAndHundred = true
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
	case expect.RequireBetweenZeroAndFifty && !o.hasBetweenZeroAndFifty:
		return false, errors.New("progress reached Ready without observing a value in (0%;50%)")
	case expect.RequireBetweenFiftyAndHundred && !o.hasBetweenFiftyAndHundred:
		return false, errors.New("progress reached Ready without observing a value in (50%;100%)")
	case expect.RequireIntermediateExceptFifty && !o.hasIntermediateExceptFifty:
		return false, errors.New("progress reached Ready without observing a value in (0%;100%) different from 50%")
	case expect.RequireHundred && !o.hasHundred:
		return false, errors.New("progress reached Ready without observing 100%")
	default:
		return true, nil
	}
}

// isProgressLongPauseValue reports whether p may legitimately stay unchanged
// for the longer progress budget.
func isProgressLongPauseValue(p float64) bool {
	return p == 0 || p == 50 || p == 100
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

// isConditionFresh reports whether the condition has been computed against
// the latest observed generation of the resource.
func isConditionFresh(cond *metav1.Condition, d *v1alpha2.VirtualDisk) bool {
	return cond.ObservedGeneration == d.GetGeneration()
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
