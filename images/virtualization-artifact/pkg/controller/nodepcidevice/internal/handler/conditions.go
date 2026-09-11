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

package handler

import (
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/nodepcidevicecondition"
)

func setAssignedAvailableCondition(nodePCIDevice *v1alpha2.NodePCIDevice, target *[]metav1.Condition, message string) {
	setAssignedCondition(nodePCIDevice, target, metav1.ConditionFalse, nodepcidevicecondition.Available, message)
}

func setAssignedInProgressCondition(nodePCIDevice *v1alpha2.NodePCIDevice, target *[]metav1.Condition, message string) {
	setAssignedCondition(nodePCIDevice, target, metav1.ConditionFalse, nodepcidevicecondition.InProgress, message)
}

func setAssignedReadyCondition(nodePCIDevice *v1alpha2.NodePCIDevice, target *[]metav1.Condition, assignedNamespace string) {
	message := fmt.Sprintf("The device is assigned to namespace %q, and the corresponding PCIDevice has been created.", assignedNamespace)
	setAssignedCondition(nodePCIDevice, target, metav1.ConditionTrue, nodepcidevicecondition.Assigned, message)
}

func setAssignedCondition(
	nodePCIDevice *v1alpha2.NodePCIDevice,
	target *[]metav1.Condition,
	status metav1.ConditionStatus,
	reason nodepcidevicecondition.AssignedReason,
	message string,
) {
	cb := conditions.NewConditionBuilder(nodepcidevicecondition.AssignedType).
		Generation(nodePCIDevice.GetGeneration()).
		Status(status).
		Reason(reason).
		Message(message)

	conditions.SetCondition(cb, target)
}

func setAttachedCondition(
	nodePCIDevice *v1alpha2.NodePCIDevice,
	target *[]metav1.Condition,
	status metav1.ConditionStatus,
	reason nodepcidevicecondition.AttachedReason,
	message string,
) {
	cb := conditions.NewConditionBuilder(nodepcidevicecondition.AttachedType).
		Generation(nodePCIDevice.GetGeneration()).
		Status(status).
		Reason(reason).
		Message(message)

	conditions.SetCondition(cb, target)
}

// deviceAbsenceDuration reports whether the device is absent on the host and
// for how long, based on the Ready condition.
func deviceAbsenceDuration(conditions []metav1.Condition) (time.Duration, bool) {
	readyCondition := meta.FindStatusCondition(conditions, string(nodepcidevicecondition.ReadyType))
	if readyCondition == nil || readyCondition.Reason != string(nodepcidevicecondition.NotFound) || readyCondition.LastTransitionTime.IsZero() {
		return 0, false
	}
	return time.Since(readyCondition.LastTransitionTime.Time), true
}
