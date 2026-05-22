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
				return true, fmt.Errorf("Ready condition reports ProvisioningFailed: %s", cond.Message)
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
				"Ready condition is True/%s but phase is %q, expected %q",
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

// HaveNonDecreasingProgress reports an invariant violation when
// VirtualDisk.Status.Progress moves backwards between observed states.
func HaveNonDecreasingProgress() Predicate {
	var previous *float64

	return func(d *v1alpha2.VirtualDisk) (bool, error) {
		if d.Status.Progress == "" {
			return true, nil
		}

		current, err := parseProgress(d.Status.Progress)
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
