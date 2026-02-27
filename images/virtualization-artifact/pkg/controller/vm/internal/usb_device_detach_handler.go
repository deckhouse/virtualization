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
		device := &changed.Status.USBDevices[i]
		currentStatusMap[device.Name] = device
	}

	specDeviceNames := make(map[string]struct{})
	for _, usbDeviceRef := range vm.Spec.USBDevices {
		specDeviceNames[usbDeviceRef.Name] = struct{}{}
	}

	for _, existingStatus := range currentStatusMap {
		if _, ok := specDeviceNames[existingStatus.Name]; !ok {
			err := h.detachUSBDevice(ctx, vm, existingStatus.Name)
			if err != nil && !apierrors.IsNotFound(err) {
				log.Error("failed to detach USB device", "error", err, "usbDevice", existingStatus.Name)
				return reconcile.Result{}, fmt.Errorf("failed to detach USB device %s: %w", existingStatus.Name, err)
			}
		}
	}

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
				return reconcile.Result{}, fmt.Errorf("failed to detach USB device %s (device not found): %w", usbDeviceRef.Name, err)
			}
			continue
		}

		if !usbDevice.GetDeletionTimestamp().IsZero() {
			err := h.detachUSBDevice(ctx, vm, usbDeviceRef.Name)
			if err != nil && !apierrors.IsNotFound(err) {
				log.Error("failed to detach USB device (device deleting)", "error", err, "usbDevice", usbDeviceRef.Name)
				return reconcile.Result{}, fmt.Errorf("failed to detach USB device %s (device deleting): %w", usbDeviceRef.Name, err)
			}
			continue
		}

		if !h.isUSBDeviceReady(usbDevice) {
			err := h.detachUSBDevice(ctx, vm, usbDeviceRef.Name)
			if err != nil && !apierrors.IsNotFound(err) {
				log.Error("failed to detach USB device (absent on device)", "error", err, "usbDevice", usbDeviceRef.Name)
				return reconcile.Result{}, fmt.Errorf("failed to detach USB device %s (device not ready): %w", usbDeviceRef.Name, err)
			}
		}
	}

	return reconcile.Result{}, nil
}
