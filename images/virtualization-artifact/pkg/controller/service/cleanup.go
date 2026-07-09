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

package service

import (
	"fmt"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
)

// DefaultCleanUpReason is the reason reported while a resource cleanup is still
// in progress but no more specific reason was produced.
const DefaultCleanUpReason = "Waiting for cleanup to finish"

func CleanUpReasonForObject(action string, obj client.Object) string {
	if obj == nil {
		return ""
	}

	return fmt.Sprintf("%s %s/%s", action, obj.GetNamespace(), obj.GetName())
}

func MergeCleanUpReasons(reasons ...string) string {
	var merged []string
	seen := make(map[string]struct{}, len(reasons))

	for _, reason := range reasons {
		if reason == "" {
			continue
		}

		if _, ok := seen[reason]; ok {
			continue
		}

		seen[reason] = struct{}{}
		merged = append(merged, reason)
	}

	return strings.Join(merged, "; ")
}

// DeletionBlockedByProtectionMessage builds the human-readable message reported on
// the Deleting condition when a resource is protected from deletion because it is
// attached to one or more VirtualMachines.
func DeletionBlockedByProtectionMessage(resourceKind string, vmNames []string) string {
	switch len(vmNames) {
	case 0:
		return fmt.Sprintf("The %s is protected from deletion by the protection finalizer", resourceKind)
	case 1:
		return fmt.Sprintf("The %s is protected from deletion because it is attached to VirtualMachine %s", resourceKind, vmNames[0])
	default:
		return fmt.Sprintf("The %s is protected from deletion because it is attached to VirtualMachines: %s", resourceKind, strings.Join(vmNames, ", "))
	}
}

// SetDeletingCondition sets a Deleting condition (Status=False) with the provided
// reason and message. The message is capitalized and terminated with a period.
func SetDeletingCondition(conds *[]metav1.Condition, conditionType, reason conditions.Stringer, generation int64, message string) {
	conditions.SetCondition(
		conditions.NewConditionBuilder(conditionType).
			Generation(generation).
			Status(metav1.ConditionFalse).
			Reason(reason).
			Message(CapitalizeFirstLetter(message)+"."),
		conds,
	)
}
