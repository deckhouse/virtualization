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

	resourcev1beta1 "k8s.io/api/resource/v1beta1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vm/internal/state"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	virtualizationv1alpha2 "github.com/deckhouse/virtualization/api/client/generated/clientset/versioned/typed/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/usbdevicecondition"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
	subv1alpha2 "github.com/deckhouse/virtualization/api/subresources/v1alpha2"
)

// VirtClient is an interface for accessing VirtualMachine resources with subresource operations.
type VirtClient interface {
	VirtualMachines(namespace string) virtualizationv1alpha2.VirtualMachineInterface
}

const nameUSBDeviceHandler = "USBDeviceHandler"

func NewUSBDeviceHandler(cl client.Client, virtClient VirtClient) *USBDeviceHandler {
	return &USBDeviceHandler{
		client:     cl,
		virtClient: virtClient,
	}
}

type USBDeviceHandler struct {
	client     client.Client
	virtClient VirtClient
}

func (h *USBDeviceHandler) Name() string {
	return nameUSBDeviceHandler
}

func (h *USBDeviceHandler) Handle(ctx context.Context, s state.VirtualMachineState) (reconcile.Result, error) {
	log := logger.FromContext(ctx).With(logger.SlogHandler(nameUSBDeviceHandler))

	if s.VirtualMachine().IsEmpty() {
		return reconcile.Result{}, nil
	}

	vm := s.VirtualMachine().Current()
	changed := s.VirtualMachine().Changed()

	// Get all USB devices from spec
	usbDevicesByName, err := s.USBDevicesByName(ctx)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("failed to get USB devices: %w", err)
	}

	// Build current status map
	currentStatusMap := make(map[string]*v1alpha2.USBDeviceStatusRef)
	for i := range changed.Status.USBDevices {
		ref := &changed.Status.USBDevices[i]
		currentStatusMap[ref.Name] = ref
	}

	// Process each USB device in spec
	var statusRefs []v1alpha2.USBDeviceStatusRef
	for _, usbDeviceRef := range vm.Spec.USBDevices {
		usbDevice, exists := usbDevicesByName[usbDeviceRef.Name]
		if !exists {
			// USB device not found (e.g. absent on node), unplug if still attached and delete ResourceClaimTemplate
			if existingStatus := currentStatusMap[usbDeviceRef.Name]; existingStatus != nil && existingStatus.Attached {
				err := h.detachUSBDevice(ctx, vm, usbDeviceRef.Name)
				if err != nil && !apierrors.IsNotFound(err) {
					log.Error("failed to detach USB device (device not found)", "error", err, "usbDevice", usbDeviceRef.Name)
				} else {
					// Device unplugged, clean up ResourceClaimTemplate created for it
					if delErr := h.deleteResourceClaimTemplate(ctx, vm, usbDeviceRef.Name); delErr != nil && !apierrors.IsNotFound(delErr) {
						log.Error("failed to delete ResourceClaimTemplate after unplug", "error", delErr, "usbDevice", usbDeviceRef.Name)
					}
				}
			}
			statusRef := v1alpha2.USBDeviceStatusRef{
				Name:     usbDeviceRef.Name,
				Attached: false,
				Ready:    false,
			}
			statusRefs = append(statusRefs, statusRef)
			continue
		}

		// Get device ready status and conditions
		isReady := h.isUSBDeviceReady(usbDevice)
		deviceConditions := h.getDeviceConditions(usbDevice)

		// Get or create ResourceClaimTemplate
		templateName := h.getResourceClaimTemplateName(vm, usbDeviceRef.Name)
		_, err := h.getOrCreateResourceClaimTemplate(ctx, vm, usbDevice, templateName)
		if err != nil {
			log.Error("failed to get or create ResourceClaimTemplate", "error", err, "usbDevice", usbDeviceRef.Name)
			// Continue with other devices
			statusRef := v1alpha2.USBDeviceStatusRef{
				Name:       usbDeviceRef.Name,
				Attached:   false,
				Ready:      isReady,
				Conditions: deviceConditions,
			}
			statusRefs = append(statusRefs, statusRef)
			continue
		}

		// Check if device is ready
		if !isReady {
			log.Info("USB device not ready", "usbDevice", usbDeviceRef.Name)
			existingStatus := currentStatusMap[usbDeviceRef.Name]
			// If device was attached but is now absent (e.g. physically unplugged), perform unplug and delete ResourceClaimTemplate
			if existingStatus != nil && existingStatus.Attached {
				err := h.detachUSBDevice(ctx, vm, usbDeviceRef.Name)
				if err != nil && !apierrors.IsNotFound(err) {
					log.Error("failed to detach USB device (absent on device)", "error", err, "usbDevice", usbDeviceRef.Name)
				} else {
					if delErr := h.deleteResourceClaimTemplate(ctx, vm, usbDeviceRef.Name); delErr != nil && !apierrors.IsNotFound(delErr) {
						log.Error("failed to delete ResourceClaimTemplate after unplug", "error", delErr, "usbDevice", usbDeviceRef.Name)
					}
				}
			}
			// Keep existing status if available, but update ready, attached and conditions
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

		// Check if already attached
		existingStatus, alreadyAttached := currentStatusMap[usbDeviceRef.Name]
		if alreadyAttached && existingStatus.Attached {
			// Device already attached, keep status but update ready and conditions
			existingStatus.Ready = isReady
			existingStatus.Conditions = deviceConditions
			statusRefs = append(statusRefs, *existingStatus)
			continue
		}

		// Try to attach via addResourceClaim API
		requestName := fmt.Sprintf("req-%s", usbDeviceRef.Name)
		err = h.attachUSBDevice(ctx, vm, usbDeviceRef.Name, templateName, requestName)
		if err != nil && !apierrors.IsAlreadyExists(err) {
			log.Error("failed to attach USB device", "error", err, "usbDevice", usbDeviceRef.Name)
			// Keep existing status or create new one, but update ready and conditions
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

		// Device attached successfully
		// Determine if it's hotplugged (VM is running)
		isHotplugged := vm.Status.Phase == v1alpha2.MachineRunning

		// Get or assign USB address
		address := h.getOrAssignUSBAddress(existingStatus, isHotplugged, vm)

		statusRef := v1alpha2.USBDeviceStatusRef{
			Name:       usbDeviceRef.Name,
			Attached:   true,
			Ready:      isReady,
			Address:    address,
			Hotplugged: isHotplugged,
			Conditions: deviceConditions,
		}
		statusRefs = append(statusRefs, statusRef)
	}

	// Remove devices that are no longer in spec
	specDeviceNames := make(map[string]bool)
	for _, usbDeviceRef := range vm.Spec.USBDevices {
		specDeviceNames[usbDeviceRef.Name] = true
	}

	for _, existingStatus := range currentStatusMap {
		if !specDeviceNames[existingStatus.Name] {
			// Device was removed from spec - always try unplug (status may not have Attached yet or may be stale)
			err := h.detachUSBDevice(ctx, vm, existingStatus.Name)
			if err != nil && !apierrors.IsNotFound(err) {
				log.Error("failed to detach USB device", "error", err, "usbDevice", existingStatus.Name)
				// Keep in status with Attached: false so we retry unplug on next reconciliation
				existingStatus.Attached = false
				statusRefs = append(statusRefs, *existingStatus)
			} else {
				// Detach succeeded or already not attached, clean up ResourceClaimTemplate
				if delErr := h.deleteResourceClaimTemplate(ctx, vm, existingStatus.Name); delErr != nil && !apierrors.IsNotFound(delErr) {
					log.Error("failed to delete ResourceClaimTemplate after unplug", "error", delErr, "usbDevice", existingStatus.Name)
				}
			}
			// On success device is removed from status (not added to statusRefs)
		}
	}

	changed.Status.USBDevices = statusRefs

	// Update USBDeviceReady condition
	h.updateUSBDeviceReadyCondition(vm, changed, statusRefs)

	return reconcile.Result{}, nil
}

func (h *USBDeviceHandler) getResourceClaimTemplateName(vm *v1alpha2.VirtualMachine, usbDeviceName string) string {
	return fmt.Sprintf("%s-usb-%s-template", vm.Name, usbDeviceName)
}

func (h *USBDeviceHandler) getOrCreateResourceClaimTemplate(
	ctx context.Context,
	vm *v1alpha2.VirtualMachine,
	usbDevice *v1alpha2.USBDevice,
	templateName string,
) (*resourcev1beta1.ResourceClaimTemplate, error) {
	// Try to get existing template
	template := &resourcev1beta1.ResourceClaimTemplate{}
	key := types.NamespacedName{
		Name:      templateName,
		Namespace: vm.Namespace,
	}

	err := h.client.Get(ctx, key, template)
	if err == nil {
		// Template exists
		return template, nil
	}

	if client.IgnoreNotFound(err) != nil {
		return nil, fmt.Errorf("failed to get ResourceClaimTemplate: %w", err)
	}

	// Template doesn't exist, create it
	attributes := usbDevice.Status.Attributes
	if attributes.Name == "" {
		return nil, fmt.Errorf("USB device %s missing name", usbDevice.Name)
	}

	template = &resourcev1beta1.ResourceClaimTemplate{
		ObjectMeta: metav1.ObjectMeta{
			Name:      templateName,
			Namespace: vm.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: v1alpha2.SchemeGroupVersion.String(),
					Kind:       v1alpha2.VirtualMachineKind,
					Name:       vm.Name,
					UID:        vm.UID,
					Controller: ptr.To(true),
				},
			},
		},
		Spec: resourcev1beta1.ResourceClaimTemplateSpec{
			Spec: resourcev1beta1.ResourceClaimSpec{
				Devices: resourcev1beta1.DeviceClaim{
					Requests: []resourcev1beta1.DeviceRequest{
						{
							Name:            "req-0",
							AllocationMode:  resourcev1beta1.DeviceAllocationModeExactCount,
							Count:           1,
							DeviceClassName: "usb-devices.virtualization.deckhouse.io",
							Selectors: []resourcev1beta1.DeviceSelector{
								{
									CEL: &resourcev1beta1.CELDeviceSelector{
										Expression: fmt.Sprintf(`device.attributes["virtualization-dra"].name == "%s"`, attributes.Name),
									},
								},
							},
						},
					},
				},
			},
		},
	}

	if err := h.client.Create(ctx, template); err != nil {
		return nil, fmt.Errorf("failed to create ResourceClaimTemplate: %w", err)
	}

	return template, nil
}

func (h *USBDeviceHandler) deleteResourceClaimTemplate(
	ctx context.Context,
	vm *v1alpha2.VirtualMachine,
	usbDeviceName string,
) error {
	templateName := h.getResourceClaimTemplateName(vm, usbDeviceName)
	template := &resourcev1beta1.ResourceClaimTemplate{}
	key := types.NamespacedName{
		Name:      templateName,
		Namespace: vm.Namespace,
	}
	if err := h.client.Get(ctx, key, template); err != nil {
		return err
	}
	return h.client.Delete(ctx, template)
}

func (h *USBDeviceHandler) isUSBDeviceReady(usbDevice *v1alpha2.USBDevice) bool {
	// Check if USB device has required attributes
	if usbDevice.Status.Attributes.VendorID == "" || usbDevice.Status.Attributes.ProductID == "" {
		return false
	}

	// Check if device has node assigned
	if usbDevice.Status.NodeName == "" {
		return false
	}

	// Check Ready condition
	readyCondition := meta.FindStatusCondition(usbDevice.Status.Conditions, string(usbdevicecondition.ReadyType))
	return readyCondition != nil && readyCondition.Status == metav1.ConditionTrue
}

func (h *USBDeviceHandler) attachUSBDevice(
	ctx context.Context,
	vm *v1alpha2.VirtualMachine,
	usbDeviceName string,
	templateName string,
	requestName string,
) error {
	// Call addResourceClaim API
	opts := subv1alpha2.VirtualMachineAddResourceClaim{
		Name:                      usbDeviceName,
		ResourceClaimTemplateName: templateName,
		RequestName:               requestName,
	}

	return h.virtClient.VirtualMachines(vm.Namespace).AddResourceClaim(ctx, vm.Name, opts)
}

func (h *USBDeviceHandler) detachUSBDevice(
	ctx context.Context,
	vm *v1alpha2.VirtualMachine,
	usbDeviceName string,
) error {
	// Call removeResourceClaim API
	opts := subv1alpha2.VirtualMachineRemoveResourceClaim{
		Name: usbDeviceName,
	}

	return h.virtClient.VirtualMachines(vm.Namespace).RemoveResourceClaim(ctx, vm.Name, opts)
}

func (h *USBDeviceHandler) getOrAssignUSBAddress(
	existingStatus *v1alpha2.USBDeviceStatusRef,
	isHotplugged bool,
	vm *v1alpha2.VirtualMachine,
) *v1alpha2.USBAddress {
	// If device was already attached, keep the same address
	if existingStatus != nil && existingStatus.Address != nil {
		return existingStatus.Address
	}

	if isHotplugged {
		// For hotplugged devices, we don't assign a fixed address
		// The address will be assigned dynamically by the hypervisor
		return nil
	}

	// Assign new address for cold-plugged devices
	// Bus is always 0 for main USB controller
	// Port should be assigned based on available ports
	usedPorts := make(map[int]bool)
	for _, usbStatus := range vm.Status.USBDevices {
		if usbStatus.Address != nil && usbStatus.Address.Bus == 0 {
			usedPorts[usbStatus.Address.Port] = true
		}
	}

	// Find the first available port starting from 1
	// USB ports typically range from 1 to 127, but we'll use a reasonable limit
	port := 1
	for port <= 127 {
		if !usedPorts[port] {
			break
		}
		port++
	}

	if port > 127 {
		// All ports are used, fallback to port 1 (should not happen in practice)
		port = 1
	}

	return &v1alpha2.USBAddress{
		Bus:  0,
		Port: port,
	}
}

func (h *USBDeviceHandler) getDeviceConditions(usbDevice *v1alpha2.USBDevice) []metav1.Condition {
	// Copy conditions from USBDevice
	conditions := make([]metav1.Condition, 0, len(usbDevice.Status.Conditions))
	for _, cond := range usbDevice.Status.Conditions {
		conditions = append(conditions, *cond.DeepCopy())
	}
	return conditions
}

func (h *USBDeviceHandler) updateUSBDeviceReadyCondition(
	vm *v1alpha2.VirtualMachine,
	changed *v1alpha2.VirtualMachine,
	statusRefs []v1alpha2.USBDeviceStatusRef,
) {
	// Check if all USB devices are ready
	allReady := true
	var notReadyDevices []string

	for _, statusRef := range statusRefs {
		if !statusRef.Ready {
			allReady = false
			notReadyDevices = append(notReadyDevices, statusRef.Name)
		}
	}

	var reason vmcondition.USBDeviceReadyReason
	var status metav1.ConditionStatus
	var message string

	if len(statusRefs) == 0 {
		// No USB devices specified, remove condition
		conditions.RemoveCondition(vmcondition.TypeUSBDeviceReady, &changed.Status.Conditions)
		return
	}

	if allReady {
		reason = vmcondition.ReasonUSBDeviceReady
		status = metav1.ConditionTrue
		message = "All USB devices are ready"
	} else {
		reason = vmcondition.ReasonSomeDevicesNotReady
		status = metav1.ConditionFalse
		if len(notReadyDevices) == 1 {
			message = fmt.Sprintf("USB device '%s' is not ready", notReadyDevices[0])
		} else {
			message = fmt.Sprintf("USB devices '%v' are not ready", notReadyDevices)
		}
	}

	cb := conditions.NewConditionBuilder(vmcondition.TypeUSBDeviceReady).
		Generation(vm.GetGeneration()).
		Status(status).
		Reason(reason).
		Message(message)

	conditions.SetCondition(cb, &changed.Status.Conditions)
}
