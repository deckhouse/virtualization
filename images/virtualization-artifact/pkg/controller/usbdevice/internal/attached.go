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
	"github.com/deckhouse/virtualization/api/core/v1alpha2/usbdevicecondition"
)

const (
	nameAttachedHandler = "AttachedHandler"
)

func NewAttachedHandler(client client.Client, recorder eventrecord.EventRecorderLogger) *AttachedHandler {
	return &AttachedHandler{
		client:   client,
		recorder: recorder,
	}
}

type AttachedHandler struct {
	client   client.Client
	recorder eventrecord.EventRecorderLogger
}

func (h *AttachedHandler) Handle(ctx context.Context, s state.USBDeviceState) (reconcile.Result, error) {
	usbDevice := s.USBDevice()

	if usbDevice.IsEmpty() {
		return reconcile.Result{}, nil
	}

	current := usbDevice.Current()
	changed := usbDevice.Changed()

	// TODO: Check if device is attached to a VM
	// For now, we'll mark it as Available
	// This should be implemented by checking VirtualMachine resources that reference this USBDevice

	var reason usbdevicecondition.AttachedReason
	var status metav1.ConditionStatus
	var message string

	// TODO: Implement actual attachment check
	// For now, default to Available
	reason = usbdevicecondition.Available
	status = metav1.ConditionFalse
	message = "Device is available for attachment to a virtual machine"

	cb := conditions.NewConditionBuilder(usbdevicecondition.AttachedType).
		Generation(current.GetGeneration()).
		Status(status).
		Reason(reason).
		Message(message)

	conditions.SetCondition(cb, &changed.Status.Conditions)

	return reconcile.Result{}, nil
}

func (h *AttachedHandler) Name() string {
	return nameAttachedHandler
}
