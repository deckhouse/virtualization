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

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/nodeusbdevicecondition"
)

func setAssignedAvailableCondition(nodeUSBDevice *v1alpha2.NodeUSBDevice, target *[]metav1.Condition, message string) {
	setAssignedCondition(nodeUSBDevice, target, metav1.ConditionFalse, nodeusbdevicecondition.Available, message)
}

func setAssignedInProgressCondition(nodeUSBDevice *v1alpha2.NodeUSBDevice, target *[]metav1.Condition, message string) {
	setAssignedCondition(nodeUSBDevice, target, metav1.ConditionFalse, nodeusbdevicecondition.InProgress, message)
}

func setAssignedReadyCondition(nodeUSBDevice *v1alpha2.NodeUSBDevice, target *[]metav1.Condition, assignedNamespace string) {
	message := fmt.Sprintf("The device is assigned to namespace %q, and the corresponding USBDevice has been created.", assignedNamespace)
	setAssignedCondition(nodeUSBDevice, target, metav1.ConditionTrue, nodeusbdevicecondition.Assigned, message)
}

func setAssignedCondition(
	nodeUSBDevice *v1alpha2.NodeUSBDevice,
	target *[]metav1.Condition,
	status metav1.ConditionStatus,
	reason nodeusbdevicecondition.AssignedReason,
	message string,
) {
	cb := conditions.NewConditionBuilder(nodeusbdevicecondition.AssignedType).
		Generation(nodeUSBDevice.GetGeneration()).
		Status(status).
		Reason(reason).
		Message(message)

	conditions.SetCondition(cb, target)
}

func isDeviceAbsentOnHost(conditions []metav1.Condition) bool {
	readyCondition := meta.FindStatusCondition(conditions, string(nodeusbdevicecondition.ReadyType))
	if readyCondition == nil {
		return false
	}

	return readyCondition.Reason == string(nodeusbdevicecondition.NotFound)
}
