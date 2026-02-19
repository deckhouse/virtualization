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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vm/internal/state"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
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

	_, isMigrating := conditions.GetCondition(vmcondition.TypeMigrating, changed.Status.Conditions)
	if isMigrating {
		return reconcile.Result{}, nil
	}

	hasPendingMigration, err := h.hasPendingMigrationOp(ctx, s)
	if err != nil {
		return reconcile.Result{}, err
	}

	if hasPendingMigration {
		return reconcile.Result{}, nil
	}

	usbDevicesByName, err := s.USBDevicesByName(ctx)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("failed to get USB devices: %w", err)
	}

	statusByName := make(map[string]*v1alpha2.USBDeviceStatusRef)
	for i := range changed.Status.USBDevices {
		device := &changed.Status.USBDevices[i]
		statusByName[device.Name] = device
	}

	var kvvmiLoaded bool
	var kvvmi *virtv1.VirtualMachineInstance

	var nextStatusRefs []v1alpha2.USBDeviceStatusRef
	for _, usbDeviceRef := range vm.Spec.USBDevices {
		deviceName := usbDeviceRef.Name
		existingStatus := statusByName[deviceName]

		// 1) Resolve source USBDevice object.
		usbDevice, exists := usbDevicesByName[deviceName]
		if !exists {
			nextStatusRefs = append(nextStatusRefs, h.buildDetachedStatus(nil, deviceName, false, nil))
			continue
		}

		isReady := h.isUSBDeviceReady(usbDevice)
		deviceConditions := h.getDeviceConditions(usbDevice)

		// 2) Pre-attach gates: deleting/template/ready checks.
		if !usbDevice.GetDeletionTimestamp().IsZero() {
			nextStatusRefs = append(nextStatusRefs, h.buildDetachedStatus(existingStatus, deviceName, false, deviceConditions))
			continue
		}

		templateName := h.getResourceClaimTemplateName(deviceName)
		if _, err := h.getResourceClaimTemplate(ctx, vm.Namespace, templateName); err != nil {
			if !apierrors.IsNotFound(err) {
				return reconcile.Result{}, err
			}

			log.Error("failed to get ResourceClaimTemplate", "error", err, "usbDevice", deviceName)
			nextStatusRefs = append(nextStatusRefs, h.buildDetachedStatus(nil, deviceName, isReady, deviceConditions))
			continue
		}

		if !isReady {
			nextStatusRefs = append(nextStatusRefs, h.buildDetachedStatus(existingStatus, deviceName, isReady, deviceConditions))
			continue
		}

		// 3) Runtime evidence from KVVMI and attach action.
		if !kvvmiLoaded {
			fetchedKVVMI, err := s.KVVMI(ctx)
			if err != nil {
				return reconcile.Result{}, fmt.Errorf("failed to get KVVMI: %w", err)
			}
			kvvmi = fetchedKVVMI
			kvvmiLoaded = true
		}

		inKVVMI := h.resourceClaimExistsInKVVMI(kvvmi, deviceName)
		if inKVVMI {
			address := h.getUSBAddressFromKVVMI(deviceName, kvvmi)
			isHotplugged := vm.Status.Phase == v1alpha2.MachineRunning

			if existingStatus != nil && existingStatus.Attached {
				status := *existingStatus
				status.Ready = isReady
				status.Conditions = deviceConditions
				status.Address = address
				status.Hotplugged = isHotplugged
				nextStatusRefs = append(nextStatusRefs, status)
			} else {
				nextStatusRefs = append(nextStatusRefs, h.buildAttachedStatus(deviceName, isReady, deviceConditions, address, isHotplugged))
			}
			continue
		}

		requestName := h.getResourceClaimRequestName(deviceName)
		err := h.attachUSBDevice(ctx, vm, deviceName, templateName, requestName)
		if err != nil && !apierrors.IsAlreadyExists(err) {
			log.Error("failed to attach USB device", "error", err, "usbDevice", deviceName)
		}

		nextStatusRefs = append(nextStatusRefs, h.buildDetachedStatus(existingStatus, deviceName, isReady, deviceConditions))
	}

	changed.Status.USBDevices = nextStatusRefs
	h.updateUSBDeviceReadyCondition(changed, nextStatusRefs)

	return reconcile.Result{}, nil
}

func (h *USBDeviceAttachHandler) buildAttachedStatus(
	deviceName string,
	ready bool,
	conditions []metav1.Condition,
	address *v1alpha2.USBAddress,
	hotplugged bool,
) v1alpha2.USBDeviceStatusRef {
	return v1alpha2.USBDeviceStatusRef{
		Name:       deviceName,
		Attached:   true,
		Ready:      ready,
		Address:    address,
		Hotplugged: hotplugged,
		Conditions: conditions,
	}
}

func (h *USBDeviceAttachHandler) buildDetachedStatus(
	existing *v1alpha2.USBDeviceStatusRef,
	deviceName string,
	ready bool,
	conditions []metav1.Condition,
) v1alpha2.USBDeviceStatusRef {
	status := v1alpha2.USBDeviceStatusRef{Name: deviceName}
	if existing != nil {
		status = *existing
	}

	status.Name = deviceName
	status.Attached = false
	status.Ready = ready
	status.Address = nil
	status.Hotplugged = false
	status.Conditions = conditions

	return status
}
