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
	"strings"

	resourcev1beta1 "k8s.io/api/resource/v1beta1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/deckhouse/pkg/log"
	"github.com/deckhouse/virtualization-controller/pkg/controller/nodeusbdevice/internal/hash"
	"github.com/deckhouse/virtualization-controller/pkg/controller/nodeusbdevice/internal/state"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/nodeusbdevicecondition"
)

const (
	nameDiscoveryHandler = "DiscoveryHandler"
)

func NewDiscoveryHandler(client client.Client, recorder eventrecord.EventRecorderLogger) *DiscoveryHandler {
	return &DiscoveryHandler{
		client:   client,
		recorder: recorder,
	}
}

type DiscoveryHandler struct {
	client   client.Client
	recorder eventrecord.EventRecorderLogger
}

func (h *DiscoveryHandler) Name() string {
	return nameDiscoveryHandler
}

func (h *DiscoveryHandler) Handle(ctx context.Context, s state.NodeUSBDeviceState) (reconcile.Result, error) {
	// Get ResourceSlices
	resourceSlices, err := s.ResourceSlices(ctx)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("failed to get resource slices: %w", err)
	}

	// Check for new devices in ResourceSlice and create NodeUSBDevice if needed
	// This ensures we discover new devices even if reconcile was triggered for other reasons
	if err := h.discoverAndCreate(ctx, s, resourceSlices); err != nil {
		// Log error but don't fail reconciliation
		// This is a best-effort discovery mechanism
		log.Error("failed to discover and create NodeUSBDevice", log.Err(err))
	}

	return reconcile.Result{}, nil
}

func (h *DiscoveryHandler) discoverAndCreate(ctx context.Context, s state.NodeUSBDeviceState, resourceSlices []resourcev1beta1.ResourceSlice) error {
	// Check if current device exists - if it does, we only need to check for new devices
	// This avoids unnecessary List when reconciling existing devices
	currentDevice := s.NodeUSBDevice()
	hasCurrentDevice := !currentDevice.IsEmpty()

	// Collect all hashes from ResourceSlices first
	deviceHashesInSlices := make(map[string]bool)
	for _, slice := range resourceSlices {
		for _, device := range slice.Spec.Devices {
			if !strings.HasPrefix(device.Name, "usb-") {
				continue
			}
			hash := hash.CalculateHashFromDevice(device, slice.Spec.Pool.Name)
			deviceHashesInSlices[hash] = true
		}
	}

	// If we have a current device and its hash is in slices, we can skip List
	// Only do List if we need to check for new devices
	if hasCurrentDevice {
		current := currentDevice.Current()
		if current.Status.Attributes.Hash != "" && deviceHashesInSlices[current.Status.Attributes.Hash] {
			// Current device exists and is in slices - only check for new devices
			// We still need to List to check for duplicates, but we can optimize
			// by only checking hashes that are in slices
			var existingDevices v1alpha2.NodeUSBDeviceList
			if err := h.client.List(ctx, &existingDevices); err != nil {
				return fmt.Errorf("failed to list existing NodeUSBDevices: %w", err)
			}

			existingHashes := make(map[string]bool)
			for _, device := range existingDevices.Items {
				if device.Status.Attributes.Hash != "" {
					existingHashes[device.Status.Attributes.Hash] = true
				}
			}

			// Only create devices that are in slices but not in existing
			for _, slice := range resourceSlices {
				for _, device := range slice.Spec.Devices {
					if !strings.HasPrefix(device.Name, "usb-") {
						continue
					}

					attributes := h.convertDeviceToAttributes(device, slice.Spec.Pool.Name)
					hash := hash.CalculateHash(attributes)

					if !existingHashes[hash] {
						if err := h.createNodeUSBDevice(ctx, attributes, hash); err != nil {
							return err
						}
					}
				}
			}
			return nil
		}
	}

	// No current device or it's not in slices - need full List
	var existingDevices v1alpha2.NodeUSBDeviceList
	if err := h.client.List(ctx, &existingDevices); err != nil {
		return fmt.Errorf("failed to list existing NodeUSBDevices: %w", err)
	}

	existingHashes := make(map[string]bool)
	for _, device := range existingDevices.Items {
		if device.Status.Attributes.Hash != "" {
			existingHashes[device.Status.Attributes.Hash] = true
		}
	}

	// Create NodeUSBDevice for each USB device in ResourceSlices
	// Note: resourceSlices are already filtered by draDriverName in state.ResourceSlices
	for _, slice := range resourceSlices {
		for _, device := range slice.Spec.Devices {
			if !strings.HasPrefix(device.Name, "usb-") {
				continue
			}

			attributes := h.convertDeviceToAttributes(device, slice.Spec.Pool.Name)
			hash := hash.CalculateHash(attributes)

			if existingHashes[hash] {
				continue
			}

			if err := h.createNodeUSBDevice(ctx, attributes, hash); err != nil {
				return err
			}
		}
	}

	return nil
}

func (h *DiscoveryHandler) createNodeUSBDevice(ctx context.Context, attributes v1alpha2.NodeUSBDeviceAttributes, hash string) error {
	name := h.generateName(hash, attributes.NodeName)

	// Check if device already exists
	existing := &v1alpha2.NodeUSBDevice{}
	err := h.client.Get(ctx, client.ObjectKey{Name: name}, existing)
	if err == nil {
		// Device already exists, skip creation
		return nil
	}
	if !apierrors.IsNotFound(err) {
		// Unexpected error
		return fmt.Errorf("failed to check if NodeUSBDevice exists: %w", err)
	}

	// Create NodeUSBDevice without status (status is a subresource)
	nodeUSBDevice := &v1alpha2.NodeUSBDevice{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: v1alpha2.NodeUSBDeviceSpec{
			AssignedNamespace: "",
		},
	}

	if err := h.client.Create(ctx, nodeUSBDevice); err != nil {
		// If device was created by another process between check and create, ignore the error
		if apierrors.IsAlreadyExists(err) {
			return nil
		}
		return fmt.Errorf("failed to create NodeUSBDevice: %w", err)
	}

	// Update status separately (status is a subresource)
	// Set all attributes including Hash
	attributes.Hash = hash
	nodeUSBDevice.Status = v1alpha2.NodeUSBDeviceStatus{
		Attributes: attributes,
		NodeName:   attributes.NodeName,
		Conditions: []metav1.Condition{
			{
				Type:               string(nodeusbdevicecondition.ReadyType),
				Status:             metav1.ConditionTrue,
				Reason:             string(nodeusbdevicecondition.Ready),
				Message:            "Device is ready to use",
				LastTransitionTime: metav1.Now(),
			},
			{
				Type:               string(nodeusbdevicecondition.AssignedType),
				Status:             metav1.ConditionFalse,
				Reason:             string(nodeusbdevicecondition.Available),
				Message:            "No namespace is assigned for the device",
				LastTransitionTime: metav1.Now(),
			},
		},
	}

	if err := h.client.Status().Update(ctx, nodeUSBDevice); err != nil {
		return fmt.Errorf("failed to update NodeUSBDevice status: %w", err)
	}

	return nil
}

func (h *DiscoveryHandler) convertDeviceToAttributes(device resourcev1beta1.Device, nodeName string) v1alpha2.NodeUSBDeviceAttributes {
	attrs := v1alpha2.NodeUSBDeviceAttributes{
		NodeName: nodeName,
		Name:     device.Name,
	}

	if device.Basic == nil {
		return attrs
	}

	for key, attr := range device.Basic.Attributes {
		switch string(key) {
		case "name":
			if attr.StringValue != nil {
				attrs.Name = *attr.StringValue
			}
		case "manufacturer":
			if attr.StringValue != nil {
				attrs.Manufacturer = *attr.StringValue
			}
		case "product":
			if attr.StringValue != nil {
				attrs.Product = *attr.StringValue
			}
		case "vendorID":
			if attr.StringValue != nil {
				attrs.VendorID = *attr.StringValue
			}
		case "productID":
			if attr.StringValue != nil {
				attrs.ProductID = *attr.StringValue
			}
		case "bcd":
			if attr.StringValue != nil {
				attrs.BCD = *attr.StringValue
			}
		case "bus":
			if attr.StringValue != nil {
				attrs.Bus = *attr.StringValue
			}
		case "deviceNumber":
			if attr.StringValue != nil {
				attrs.DeviceNumber = *attr.StringValue
			}
		case "serial":
			if attr.StringValue != nil {
				attrs.Serial = *attr.StringValue
			}
		case "devicePath":
			if attr.StringValue != nil {
				attrs.DevicePath = *attr.StringValue
			}
		case "major":
			if attr.IntValue != nil {
				attrs.Major = int(*attr.IntValue)
			}
		case "minor":
			if attr.IntValue != nil {
				attrs.Minor = int(*attr.IntValue)
			}
		}
	}

	attrs.Hash = hash.CalculateHash(attrs)
	return attrs
}

func (h *DiscoveryHandler) generateName(hash, nodeName string) string {
	// Generate name based on hash and node name
	// Format: nusb-<hash>-<nodeName>
	nodeNameSanitized := strings.ToLower(strings.ReplaceAll(nodeName, ".", "-"))
	return fmt.Sprintf("nusb-%s-%s", hash[:8], nodeNameSanitized)
}
