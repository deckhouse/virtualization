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
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/usbdevice/internal/state"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/usbdevicecondition"
)

const (
	nameDeletionHandler = "DeletionHandler"
)

func NewDeletionHandler(client client.Client, recorder eventrecord.EventRecorderLogger) *DeletionHandler {
	return &DeletionHandler{
		client:   client,
		recorder: recorder,
	}
}

type DeletionHandler struct {
	client   client.Client
	recorder eventrecord.EventRecorderLogger
}

func (h *DeletionHandler) Handle(ctx context.Context, s state.USBDeviceState) (reconcile.Result, error) {
	usbDevice := s.USBDevice()

	if usbDevice.IsEmpty() {
		return reconcile.Result{}, nil
	}

	current := usbDevice.Current()
	changed := usbDevice.Changed()

	// Add finalizer if not deleting
	if current.GetDeletionTimestamp().IsZero() {
		controllerutil.AddFinalizer(changed, v1alpha2.FinalizerUSBDeviceCleanup)
		return reconcile.Result{}, nil
	}

	// Check if device is attached to a VM
	// TODO: Implement hot unplug before deletion
	// For now, we just check the Attached condition
	attached := false
	for _, condition := range current.Status.Conditions {
		if condition.Type == string(usbdevicecondition.AttachedType) {
			if condition.Status == "True" && condition.Reason == string(usbdevicecondition.AttachedToVirtualMachine) {
				attached = true
				break
			}
		}
	}

	if attached {
		// TODO: Implement hot unplug logic here
		// For now, we'll just log and continue
		h.recorder.Eventf(changed, "Normal", "Deletion", "Device is attached to VM, hot unplug will be performed")
		// Return to retry after hot unplug
		return reconcile.Result{Requeue: true}, fmt.Errorf("device is attached to VM, hot unplug required")
	}

	// Remove finalizer
	controllerutil.RemoveFinalizer(changed, v1alpha2.FinalizerUSBDeviceCleanup)

	return reconcile.Result{}, nil
}

func (h *DeletionHandler) Name() string {
	return nameDeletionHandler
}
