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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/cvicondition"
)

func BeFailed() Predicate {
	return func(i *v1alpha2.ClusterVirtualImage) (bool, error) {
		if i.Status.Phase == v1alpha2.ImageFailed {
			return true, fmt.Errorf("ClusterVirtualImage entered Failed phase")
		}
		if cond := findCondition(i.Status.Conditions, cvicondition.ReadyType.String()); cond != nil {
			if isConditionFresh(cond, i) && cond.Reason == cvicondition.ProvisioningFailed.String() {
				return true, fmt.Errorf("Ready condition reports ProvisioningFailed: %s", cond.Message)
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
				"Ready condition is True/%s but phase is %q, expected %q",
				cvicondition.Ready, i.Status.Phase, v1alpha2.ImageReady,
			)
		case !phaseIsReady:
			return false, nil
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
