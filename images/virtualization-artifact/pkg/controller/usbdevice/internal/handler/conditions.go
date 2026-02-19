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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/usbdevicecondition"
)

func setReadyCondition(
	usbDevice *v1alpha2.USBDevice,
	target *[]metav1.Condition,
	status metav1.ConditionStatus,
	reason usbdevicecondition.ReadyReason,
	message string,
	lastTransitionTime *metav1.Time,
) {
	cb := conditions.NewConditionBuilder(usbdevicecondition.ReadyType).
		Generation(usbDevice.GetGeneration()).
		Status(status).
		Reason(reason).
		Message(message)

	if lastTransitionTime != nil {
		cb = cb.LastTransitionTime(lastTransitionTime.Time)
	}

	conditions.SetCondition(cb, target)
}

func setAttachedCondition(
	usbDevice *v1alpha2.USBDevice,
	target *[]metav1.Condition,
	status metav1.ConditionStatus,
	reason usbdevicecondition.AttachedReason,
	message string,
) {
	cb := conditions.NewConditionBuilder(usbdevicecondition.AttachedType).
		Generation(usbDevice.GetGeneration()).
		Status(status).
		Reason(reason).
		Message(message)

	conditions.SetCondition(cb, target)
}
