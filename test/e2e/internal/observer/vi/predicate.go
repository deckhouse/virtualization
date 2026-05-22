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
