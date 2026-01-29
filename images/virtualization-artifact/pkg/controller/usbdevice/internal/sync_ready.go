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

package internal

import (
	"context"
	"reflect"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/usbdevice/internal/state"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/nodeusbdevicecondition"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/usbdevicecondition"
)

const (
	nameSyncReadyHandler = "SyncReadyHandler"
)

func NewSyncReadyHandler(recorder eventrecord.EventRecorderLogger) *SyncReadyHandler {
	return &SyncReadyHandler{
		recorder: recorder,
	}
}

type SyncReadyHandler struct {
	recorder eventrecord.EventRecorderLogger
}

func (h *SyncReadyHandler) Handle(ctx context.Context, s state.USBDeviceState) (reconcile.Result, error) {
	usbDevice := s.USBDevice()

	if usbDevice.IsEmpty() {
		return reconcile.Result{}, nil
	}

	current := usbDevice.Current()
	changed := usbDevice.Changed()

	// Get corresponding NodeUSBDevice once
	nodeUSBDevice, err := s.NodeUSBDevice(ctx)
	if err != nil {
		return reconcile.Result{}, err
	}

	if nodeUSBDevice == nil {
		// NodeUSBDevice not found - mark as NotFound
		cb := conditions.NewConditionBuilder(usbdevicecondition.ReadyType).
			Generation(current.GetGeneration()).
			Status(metav1.ConditionFalse).
			Reason(usbdevicecondition.NotFound).
			Message("Corresponding NodeUSBDevice not found")

		conditions.SetCondition(cb, &changed.Status.Conditions)
		return reconcile.Result{}, nil
	}

	// Sync attributes and nodeName from NodeUSBDevice (only if changed)
	needsSync := !reflect.DeepEqual(changed.Status.Attributes, nodeUSBDevice.Status.Attributes) ||
		changed.Status.NodeName != nodeUSBDevice.Status.NodeName

	if needsSync {
		changed.Status.Attributes = nodeUSBDevice.Status.Attributes
		changed.Status.NodeName = nodeUSBDevice.Status.NodeName
	}

	// Sync Ready condition from NodeUSBDevice
	var readyCondition *metav1.Condition
	for i := range nodeUSBDevice.Status.Conditions {
		if nodeUSBDevice.Status.Conditions[i].Type == string(nodeusbdevicecondition.ReadyType) {
			readyCondition = &nodeUSBDevice.Status.Conditions[i]
			break
		}
	}

	if readyCondition == nil {
		// No Ready condition in NodeUSBDevice - mark as NotReady
		cb := conditions.NewConditionBuilder(usbdevicecondition.ReadyType).
			Generation(current.GetGeneration()).
			Status(metav1.ConditionFalse).
			Reason(usbdevicecondition.NotReady).
			Message("Ready condition not found in NodeUSBDevice")

		conditions.SetCondition(cb, &changed.Status.Conditions)
		return reconcile.Result{}, nil
	}

	// Translate Ready condition from NodeUSBDevice
	var reason usbdevicecondition.ReadyReason
	var status metav1.ConditionStatus

	switch readyCondition.Reason {
	case string(nodeusbdevicecondition.Ready):
		reason = usbdevicecondition.Ready
		status = metav1.ConditionTrue
	case string(nodeusbdevicecondition.NotReady):
		reason = usbdevicecondition.NotReady
		status = metav1.ConditionFalse
	case string(nodeusbdevicecondition.NotFound):
		reason = usbdevicecondition.NotFound
		status = metav1.ConditionFalse
	default:
		reason = usbdevicecondition.NotReady
		status = metav1.ConditionFalse
	}

	cb := conditions.NewConditionBuilder(usbdevicecondition.ReadyType).
		Generation(current.GetGeneration()).
		Status(status).
		Reason(reason).
		Message(readyCondition.Message).
		LastTransitionTime(readyCondition.LastTransitionTime.Time)

	conditions.SetCondition(cb, &changed.Status.Conditions)

	return reconcile.Result{}, nil
}

func (h *SyncReadyHandler) Name() string {
	return nameSyncReadyHandler
}
