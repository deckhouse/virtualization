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

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/vm/internal/state"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

const nameUSBDeviceDetachHandler = "USBDeviceDetachHandler"

func NewUSBDeviceDetachHandler(cl client.Client, virtClient VirtClient) *USBDeviceDetachHandler {
	return &USBDeviceDetachHandler{
		usbDeviceHandlerBase: usbDeviceHandlerBase{
			client:     cl,
			virtClient: virtClient,
		},
	}
}

type USBDeviceDetachHandler struct {
	usbDeviceHandlerBase
}

func (h *USBDeviceDetachHandler) Name() string {
	return nameUSBDeviceDetachHandler
}

// Handle performs detach and cleanup for USB devices that should be unplugged:
// - devices that are no longer in spec
// - devices in spec but not found (e.g. absent on node)
// - devices in spec but not ready (e.g. physically unplugged)
func (h *USBDeviceDetachHandler) Handle(ctx context.Context, s state.VirtualMachineState) (reconcile.Result, error) {
	log := logger.FromContext(ctx).With(logger.SlogHandler(nameUSBDeviceDetachHandler))

	if s.VirtualMachine().IsEmpty() {
		return reconcile.Result{}, nil
	}

	vm := s.VirtualMachine().Current()
	changed := s.VirtualMachine().Changed()

	usbDevicesByName, err := s.USBDevicesByName(ctx)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("failed to get USB devices: %w", err)
	}

	currentStatusMap := make(map[string]*v1alpha2.USBDeviceStatusRef)
	for i := range changed.Status.USBDevices {
		ref := &changed.Status.USBDevices[i]
		currentStatusMap[ref.Name] = ref
	}

	specDeviceNames := make(map[string]bool)
	for _, usbDeviceRef := range vm.Spec.USBDevices {
		specDeviceNames[usbDeviceRef.Name] = true
	}

	// Detach devices that are no longer in spec
	for _, existingStatus := range currentStatusMap {
		if !specDeviceNames[existingStatus.Name] {
			err := h.detachUSBDevice(ctx, vm, existingStatus.Name)
			if err != nil && !apierrors.IsNotFound(err) {
				log.Error("failed to detach USB device", "error", err, "usbDevice", existingStatus.Name)
			}
		}
	}

	// Detach devices in spec that are not found or not ready but currently attached
	for _, usbDeviceRef := range vm.Spec.USBDevices {
		existingStatus := currentStatusMap[usbDeviceRef.Name]
		if existingStatus == nil || !existingStatus.Attached {
			continue
		}

		usbDevice, exists := usbDevicesByName[usbDeviceRef.Name]
		if !exists {
			err := h.detachUSBDevice(ctx, vm, usbDeviceRef.Name)
			if err != nil && !apierrors.IsNotFound(err) {
				log.Error("failed to detach USB device (device not found)", "error", err, "usbDevice", usbDeviceRef.Name)
			}
			continue
		}

		if !h.isUSBDeviceReady(usbDevice) {
			err := h.detachUSBDevice(ctx, vm, usbDeviceRef.Name)
			if err != nil && !apierrors.IsNotFound(err) {
				log.Error("failed to detach USB device (absent on device)", "error", err, "usbDevice", usbDeviceRef.Name)
			}
		}
	}

	return reconcile.Result{}, nil
}
