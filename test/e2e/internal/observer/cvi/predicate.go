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

package cvi

import (
	"fmt"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/cvicondition"
)

// quotaExceededMessagePrefix is the prefix the controller prepends to
// the Ready condition message when the project quota is exhausted.
const quotaExceededMessagePrefix = "Quota exceeded"

func BeFailed() Predicate {
	return func(i *v1alpha2.ClusterVirtualImage) (bool, error) {
		if i.Status.Phase == v1alpha2.ImageFailed {
			return true, fmt.Errorf("ClusterVirtualImage entered Failed phase")
		}
		if cond := findCondition(i.Status.Conditions, cvicondition.ReadyType.String()); cond != nil {
			if isConditionFresh(cond, i) && cond.Reason == cvicondition.ProvisioningFailed.String() {
				return true, fmt.Errorf("ready condition reports ProvisioningFailed: %s", cond.Message)
			}
		}
		return false, nil
	}
}

func BeReady() Predicate {
	return func(i *v1alpha2.ClusterVirtualImage) (bool, error) {
		readyCond := findCondition(i.Status.Conditions, cvicondition.ReadyType.String())

		condStale := readyCond != nil && !isConditionFresh(readyCond, i)
		condIsReady := readyCond != nil &&
			!condStale &&
			readyCond.Status == metav1.ConditionTrue &&
			readyCond.Reason == cvicondition.Ready.String()
		phaseIsReady := i.Status.Phase == v1alpha2.ImageReady

		switch {
		case phaseIsReady && condStale:
			return false, nil
		case phaseIsReady && !condIsReady:
			return false, fmt.Errorf(
				"phase is Ready but Ready condition is %s/%s (message: %q), expected True/%s",
				condStatus(readyCond), condReason(readyCond), condMessage(readyCond), cvicondition.Ready,
			)
		case condIsReady && !phaseIsReady:
			return false, fmt.Errorf(
				"ready condition is True/%s but phase is %q, expected %q",
				cvicondition.Ready, i.Status.Phase, v1alpha2.ImageReady,
			)
		case !phaseIsReady:
			return false, nil
		}

		return true, nil
	}
}

// BeQuotaExceeded reports the ClusterVirtualImage has been parked in a
// quota-exhausted state.
//
// The predicate is satisfied when the Ready condition is fresh,
// reports Status=False with Reason=ProvisioningFailed, the message is
// prefixed with "Quota exceeded" (the controller wraps the upstream
// "exceeded quota:" Kubernetes error into a "Quota exceeded:" message),
// and the phase is Failed.
//
// Returned values:
//   - (true, nil)  - the ClusterVirtualImage reports a fresh
//     quota-exceeded Ready condition together with the Failed phase;
//   - (false, nil) - the controller has not yet reported a fresh
//     quota-exceeded Ready condition;
//   - (false, err) - the quota-exceeded message is reported with an
//     unexpected phase or Status, which is a controller bug.
//
// Intended for use with [Observer.WaitFor].
func BeQuotaExceeded() Predicate {
	return func(i *v1alpha2.ClusterVirtualImage) (bool, error) {
		cond := findCondition(i.Status.Conditions, cvicondition.ReadyType.String())
		if cond == nil || !isConditionFresh(cond, i) {
			return false, nil
		}
		if cond.Reason != cvicondition.ProvisioningFailed.String() {
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

func isConditionFresh(cond *metav1.Condition, i *v1alpha2.ClusterVirtualImage) bool {
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
