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
	"fmt"

	resourcev1beta1 "k8s.io/api/resource/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/vm/internal/state"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	"github.com/deckhouse/virtualization/api/client/generated/clientset/versioned"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	subv1alpha2 "github.com/deckhouse/virtualization/api/subresources/v1alpha2"
)

const nameUSBDeviceHandler = "USBDeviceHandler"

func NewUSBDeviceHandler(cl client.Client, virtClient versioned.Interface) *USBDeviceHandler {
	return &USBDeviceHandler{
		client:     cl,
		virtClient: virtClient,
	}
}

type USBDeviceHandler struct {
	client     client.Client
	virtClient versioned.Interface
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
			// USB device not found, but we still track it in status
			statusRef := v1alpha2.USBDeviceStatusRef{
				Name:     usbDeviceRef.Name,
				Attached: false,
			}
			statusRefs = append(statusRefs, statusRef)
			continue
		}

		// Get or create ResourceClaimTemplate
		templateName := h.getResourceClaimTemplateName(vm, usbDeviceRef.Name)
		_, err := h.getOrCreateResourceClaimTemplate(ctx, vm, usbDevice, templateName)
		if err != nil {
			log.Error("failed to get or create ResourceClaimTemplate", "error", err, "usbDevice", usbDeviceRef.Name)
			// Continue with other devices
			statusRef := v1alpha2.USBDeviceStatusRef{
				Name:     usbDeviceRef.Name,
				Attached: false,
			}
			statusRefs = append(statusRefs, statusRef)
			continue
		}

		// Check if device is ready
		if !h.isUSBDeviceReady(usbDevice) {
			log.Info("USB device not ready", "usbDevice", usbDeviceRef.Name)
			// Keep existing status if available
			if existingStatus, ok := currentStatusMap[usbDeviceRef.Name]; ok {
				statusRefs = append(statusRefs, *existingStatus)
			} else {
				statusRefs = append(statusRefs, v1alpha2.USBDeviceStatusRef{
					Name:     usbDeviceRef.Name,
					Attached: false,
				})
			}
			continue
		}

		// Check if already attached
		existingStatus, alreadyAttached := currentStatusMap[usbDeviceRef.Name]
		if alreadyAttached && existingStatus.Attached {
			// Device already attached, keep status
			statusRefs = append(statusRefs, *existingStatus)
			continue
		}

		// Try to attach via addResourceClaim API
		requestName := fmt.Sprintf("req-%s", usbDeviceRef.Name)
		err = h.attachUSBDevice(ctx, vm, usbDeviceRef.Name, templateName, requestName)
		if err != nil {
			log.Error("failed to attach USB device", "error", err, "usbDevice", usbDeviceRef.Name)
			// Keep existing status or create new one
			if existingStatus != nil {
				statusRefs = append(statusRefs, *existingStatus)
			} else {
				statusRefs = append(statusRefs, v1alpha2.USBDeviceStatusRef{
					Name:     usbDeviceRef.Name,
					Attached: false,
				})
			}
			continue
		}

		// Device attached successfully
		// Determine if it's hotplugged (VM is running)
		isHotplugged := vm.Status.Phase == v1alpha2.MachineRunning

		// Get or assign USB address
		address := h.getOrAssignUSBAddress(existingStatus, isHotplugged)

		statusRef := v1alpha2.USBDeviceStatusRef{
			Name:       usbDeviceRef.Name,
			Attached:   true,
			Address:    address,
			Hotplugged: isHotplugged,
		}
		statusRefs = append(statusRefs, statusRef)
	}

	// Remove devices that are no longer in spec
	specDeviceNames := make(map[string]bool)
	for _, usbDeviceRef := range vm.Spec.USBDevices {
		specDeviceNames[usbDeviceRef.Name] = true
	}

	for _, existingStatus := range currentStatusMap {
		if !specDeviceNames[existingStatus.Name] && existingStatus.Attached {
			// Device was removed from spec but is still attached, need to detach
			err := h.detachUSBDevice(ctx, vm, existingStatus.Name)
			if err != nil {
				log.Error("failed to detach USB device", "error", err, "usbDevice", existingStatus.Name)
				// Keep status but mark as not attached
				existingStatus.Attached = false
				statusRefs = append(statusRefs, *existingStatus)
			}
			// If detach succeeded, device is removed from status (not added to statusRefs)
		}
	}

	changed.Status.USBDevices = statusRefs

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
	if attributes.VendorID == "" || attributes.ProductID == "" {
		return nil, fmt.Errorf("USB device %s missing vendorID or productID", usbDevice.Name)
	}

	// Build CEL expression to match this specific USB device
	celExpression := fmt.Sprintf(
		`device.attributes["virtualization-dra"].productID == "%s" && device.attributes["virtualization-dra"].vendorID == "%s"`,
		attributes.ProductID,
		attributes.VendorID,
	)

	// Add serial number if available for more precise matching
	if attributes.Serial != "" {
		celExpression = fmt.Sprintf(`%s && device.attributes["virtualization-dra"].serial == "%s"`, celExpression, attributes.Serial)
	}

	template = &resourcev1beta1.ResourceClaimTemplate{
		ObjectMeta: metav1.ObjectMeta{
			Name:      templateName,
			Namespace: vm.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: vm.APIVersion,
					Kind:       vm.Kind,
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
										Expression: celExpression,
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

func (h *USBDeviceHandler) isUSBDeviceReady(usbDevice *v1alpha2.USBDevice) bool {
	// Check if USB device has required attributes
	if usbDevice.Status.Attributes.VendorID == "" || usbDevice.Status.Attributes.ProductID == "" {
		return false
	}

	// Check if device has node assigned
	if usbDevice.Status.NodeName == "" {
		return false
	}

	// TODO: Check conditions if needed
	return true
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

	return h.virtClient.VirtualizationV1alpha2().VirtualMachines(vm.Namespace).AddResourceClaim(ctx, vm.Name, opts)
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

	return h.virtClient.VirtualizationV1alpha2().VirtualMachines(vm.Namespace).RemoveResourceClaim(ctx, vm.Name, opts)
}

func (h *USBDeviceHandler) getOrAssignUSBAddress(
	existingStatus *v1alpha2.USBDeviceStatusRef,
	isHotplugged bool,
) *v1alpha2.USBAddress {
	// If device was already attached, keep the same address
	if existingStatus != nil && existingStatus.Address != nil {
		return existingStatus.Address
	}

	// Assign new address
	// Bus is always 0 for main USB controller
	// Port should be assigned based on available ports
	// For simplicity, we'll use a sequential port number starting from 1
	// In a real implementation, you'd need to track used ports
	port := 1 // TODO: Implement proper port allocation

	return &v1alpha2.USBAddress{
		Bus:  0,
		Port: port,
	}
}
