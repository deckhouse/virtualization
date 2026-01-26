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
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	resourcev1beta1 "k8s.io/api/resource/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/deckhouse/pkg/log"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/nodeusbdevice/internal/state"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/nodeusbdevicecondition"
)

const (
	nameDiscoveryHandler = "DiscoveryHandler"
	draDriverName        = "virtualization-dra"
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
	nodeUSBDevice := s.NodeUSBDevice()

	// Always check for new devices in ResourceSlice and create NodeUSBDevice if needed
	// This ensures we discover new devices even if reconcile was triggered for other reasons
	if _, err := h.discoverAndCreate(ctx); err != nil {
		// Log error but don't fail reconciliation
		// This is a best-effort discovery mechanism
		log.Error("failed to discover and create NodeUSBDevice", log.Err(err))
	}

	if nodeUSBDevice.IsEmpty() {
		// Resource doesn't exist - nothing to update
		return reconcile.Result{}, nil
	}

	current := nodeUSBDevice.Current()
	changed := nodeUSBDevice.Changed()

	// Update attributes from ResourceSlice if needed
	resourceSlices, err := h.getResourceSlices(ctx)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("failed to get resource slices: %w", err)
	}

	deviceInfo, found := h.findDeviceInSlices(resourceSlices, current.Status.Attributes.Hash, current.Status.NodeName)
	if !found {
		// Device not found in slices - mark as NotFound
		cb := conditions.NewConditionBuilder(nodeusbdevicecondition.ReadyType).
			Generation(current.GetGeneration()).
			Status(metav1.ConditionFalse).
			Reason(nodeusbdevicecondition.NotFound).
			Message("Device not found in ResourceSlice")
		conditions.SetCondition(cb, &changed.Status.Conditions)
		return reconcile.Result{}, nil
	}

	// Update attributes if they changed
	if !h.attributesEqual(current.Status.Attributes, deviceInfo) {
		changed.Status.Attributes = deviceInfo
	}

	return reconcile.Result{}, nil
}

func (h *DiscoveryHandler) discoverAndCreate(ctx context.Context) (reconcile.Result, error) {
	resourceSlices, err := h.getResourceSlices(ctx)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("failed to get resource slices: %w", err)
	}

	// Get all existing NodeUSBDevices to avoid duplicates
	var existingDevices v1alpha2.NodeUSBDeviceList
	if err := h.client.List(ctx, &existingDevices); err != nil {
		return reconcile.Result{}, fmt.Errorf("failed to list existing NodeUSBDevices: %w", err)
	}

	existingHashes := make(map[string]bool)
	for _, device := range existingDevices.Items {
		if device.Status.Attributes.Hash != "" {
			existingHashes[device.Status.Attributes.Hash] = true
		}
	}

	// Create NodeUSBDevice for each USB device in ResourceSlices
	// Note: resourceSlices are already filtered by draDriverName in getResourceSlices
	for _, slice := range resourceSlices {
		for _, device := range slice.Spec.Devices {
			if !strings.HasPrefix(device.Name, "usb-") {
				continue
			}

			attributes := h.convertDeviceToAttributes(device, slice.Spec.Pool.Name)
			hash := h.calculateHash(attributes)

			if existingHashes[hash] {
				continue
			}

			nodeUSBDevice := &v1alpha2.NodeUSBDevice{
				ObjectMeta: metav1.ObjectMeta{
					Name: h.generateName(hash, attributes.NodeName),
				},
				Spec: v1alpha2.NodeUSBDeviceSpec{
					AssignedNamespace: "",
				},
				Status: v1alpha2.NodeUSBDeviceStatus{
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
				},
			}

			if err := h.client.Create(ctx, nodeUSBDevice); err != nil {
				return reconcile.Result{}, fmt.Errorf("failed to create NodeUSBDevice: %w", err)
			}
		}
	}

	return reconcile.Result{}, nil
}

func (h *DiscoveryHandler) getResourceSlices(ctx context.Context) ([]resourcev1beta1.ResourceSlice, error) {
	var slices resourcev1beta1.ResourceSliceList
	if err := h.client.List(ctx, &slices, client.MatchingLabels{}); err != nil {
		return nil, err
	}

	result := make([]resourcev1beta1.ResourceSlice, 0)
	for _, slice := range slices.Items {
		if slice.Spec.Driver == draDriverName {
			result = append(result, slice)
		}
	}

	return result, nil
}

func (h *DiscoveryHandler) findDeviceInSlices(slices []resourcev1beta1.ResourceSlice, hash, nodeName string) (v1alpha2.NodeUSBDeviceAttributes, bool) {
	for _, slice := range slices {
		if slice.Spec.Pool.Name != nodeName {
			continue
		}

		for _, device := range slice.Spec.Devices {
			if !strings.HasPrefix(device.Name, "usb-") {
				continue
			}

			attributes := h.convertDeviceToAttributes(device, nodeName)
			deviceHash := h.calculateHash(attributes)

			if deviceHash == hash {
				return attributes, true
			}
		}
	}

	return v1alpha2.NodeUSBDeviceAttributes{}, false
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

	attrs.Hash = h.calculateHash(attrs)
	return attrs
}

func (h *DiscoveryHandler) calculateHash(attrs v1alpha2.NodeUSBDeviceAttributes) string {
	// Calculate hash based on main attributes
	hashInput := fmt.Sprintf("%s:%s:%s:%s:%s:%s:%s",
		attrs.NodeName,
		attrs.VendorID,
		attrs.ProductID,
		attrs.Bus,
		attrs.DeviceNumber,
		attrs.Serial,
		attrs.DevicePath,
	)

	hash := sha256.Sum256([]byte(hashInput))
	return hex.EncodeToString(hash[:])[:16] // Use first 16 characters
}

func (h *DiscoveryHandler) generateName(hash, nodeName string) string {
	// Generate name based on hash and node name
	// Format: nusb-<hash>-<nodeName>
	nodeNameSanitized := strings.ToLower(strings.ReplaceAll(nodeName, ".", "-"))
	return fmt.Sprintf("nusb-%s-%s", hash[:8], nodeNameSanitized)
}

func (h *DiscoveryHandler) attributesEqual(a, b v1alpha2.NodeUSBDeviceAttributes) bool {
	return a.Hash == b.Hash &&
		a.NodeName == b.NodeName &&
		a.VendorID == b.VendorID &&
		a.ProductID == b.ProductID &&
		a.Bus == b.Bus &&
		a.DeviceNumber == b.DeviceNumber
}
