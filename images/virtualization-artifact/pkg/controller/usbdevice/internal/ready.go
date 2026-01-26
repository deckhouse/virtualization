/*
Copyright 2025 Flant JSC

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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/usbdevice/internal/state"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/nodeusbdevicecondition"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/usbdevicecondition"
)

const (
	nameReadyHandler = "ReadyHandler"
)

func NewReadyHandler(client client.Client, recorder eventrecord.EventRecorderLogger) *ReadyHandler {
	return &ReadyHandler{
		client:   client,
		recorder: recorder,
	}
}

type ReadyHandler struct {
	client   client.Client
	recorder eventrecord.EventRecorderLogger
}

func (h *ReadyHandler) Handle(ctx context.Context, s state.USBDeviceState) (reconcile.Result, error) {
	usbDevice := s.USBDevice()

	if usbDevice.IsEmpty() {
		return reconcile.Result{}, nil
	}

	current := usbDevice.Current()
	changed := usbDevice.Changed()

	// Get corresponding NodeUSBDevice
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

	// Find Ready condition in NodeUSBDevice
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

func (h *ReadyHandler) Name() string {
	return nameReadyHandler
}
