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

const nameUSBDeviceAttachHandler = "USBDeviceAttachHandler"

func NewUSBDeviceAttachHandler(cl client.Client, virtClient VirtClient) *USBDeviceAttachHandler {
	return &USBDeviceAttachHandler{
		usbDeviceHandlerBase: usbDeviceHandlerBase{
			client:     cl,
			virtClient: virtClient,
		},
	}
}

type USBDeviceAttachHandler struct {
	usbDeviceHandlerBase
}

func (h *USBDeviceAttachHandler) Name() string {
	return nameUSBDeviceAttachHandler
}

// Handle builds USB device status, attaches devices that are ready, and updates USBDeviceReady condition.
func (h *USBDeviceAttachHandler) Handle(ctx context.Context, s state.VirtualMachineState) (reconcile.Result, error) {
	log := logger.FromContext(ctx).With(logger.SlogHandler(nameUSBDeviceAttachHandler))

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

	var statusRefs []v1alpha2.USBDeviceStatusRef
	for _, usbDeviceRef := range vm.Spec.USBDevices {
		usbDevice, exists := usbDevicesByName[usbDeviceRef.Name]
		if !exists {
			statusRefs = append(statusRefs, v1alpha2.USBDeviceStatusRef{
				Name:     usbDeviceRef.Name,
				Attached: false,
				Ready:    false,
			})
			continue
		}

		isReady := h.isUSBDeviceReady(usbDevice)
		deviceConditions := h.getDeviceConditions(usbDevice)

		templateName := h.getResourceClaimTemplateName(usbDeviceRef.Name)
		_, err := h.getResourceClaimTemplate(ctx, vm.Namespace, templateName)
		if err != nil {
			log.Error("failed to get ResourceClaimTemplate", "error", err, "usbDevice", usbDeviceRef.Name)
			statusRefs = append(statusRefs, v1alpha2.USBDeviceStatusRef{
				Name:       usbDeviceRef.Name,
				Attached:   false,
				Ready:      isReady,
				Conditions: deviceConditions,
			})
			continue
		}

		if !isReady {
			existingStatus := currentStatusMap[usbDeviceRef.Name]
			if existingStatus != nil {
				existingStatus.Ready = isReady
				existingStatus.Attached = false
				existingStatus.Conditions = deviceConditions
				statusRefs = append(statusRefs, *existingStatus)
			} else {
				statusRefs = append(statusRefs, v1alpha2.USBDeviceStatusRef{
					Name:       usbDeviceRef.Name,
					Attached:   false,
					Ready:      isReady,
					Conditions: deviceConditions,
				})
			}
			continue
		}

		existingStatus, alreadyAttached := currentStatusMap[usbDeviceRef.Name]
		if alreadyAttached && existingStatus.Attached {
			kvvmi, _ := s.KVVMI(ctx)
			existingStatus.Ready = isReady
			existingStatus.Conditions = deviceConditions
			existingStatus.Address = h.getUSBAddressFromKVVMI(usbDeviceRef.Name, kvvmi)
			statusRefs = append(statusRefs, *existingStatus)
			continue
		}

		requestName := fmt.Sprintf("req-%s", usbDeviceRef.Name)
		err = h.attachUSBDevice(ctx, vm, usbDeviceRef.Name, templateName, requestName)
		if err != nil && !apierrors.IsAlreadyExists(err) {
			log.Error("failed to attach USB device", "error", err, "usbDevice", usbDeviceRef.Name)
			if existingStatus != nil {
				existingStatus.Ready = isReady
				existingStatus.Conditions = deviceConditions
				statusRefs = append(statusRefs, *existingStatus)
			} else {
				statusRefs = append(statusRefs, v1alpha2.USBDeviceStatusRef{
					Name:       usbDeviceRef.Name,
					Attached:   false,
					Ready:      isReady,
					Conditions: deviceConditions,
				})
			}
			continue
		}

		isHotplugged := vm.Status.Phase == v1alpha2.MachineRunning
		kvvmi, _ := s.KVVMI(ctx)
		address := h.getUSBAddressFromKVVMI(usbDeviceRef.Name, kvvmi)

		statusRefs = append(statusRefs, v1alpha2.USBDeviceStatusRef{
			Name:       usbDeviceRef.Name,
			Attached:   true,
			Ready:      isReady,
			Address:    address,
			Hotplugged: isHotplugged,
			Conditions: deviceConditions,
		})
	}

	// Devices removed from spec are not in statusRefs (detached by USBDeviceDetachHandler)

	changed.Status.USBDevices = statusRefs
	h.updateUSBDeviceReadyCondition(vm, changed, statusRefs)

	return reconcile.Result{}, nil
}
