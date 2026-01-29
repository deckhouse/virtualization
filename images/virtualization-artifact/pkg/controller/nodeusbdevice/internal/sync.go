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
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/nodeusbdevice/internal/hash"
	"github.com/deckhouse/virtualization-controller/pkg/controller/nodeusbdevice/internal/state"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

const (
	nameSyncHandler = "SyncHandler"
)

func NewSyncHandler(recorder eventrecord.EventRecorderLogger) *SyncHandler {
	return &SyncHandler{
		recorder: recorder,
	}
}

type SyncHandler struct {
	recorder eventrecord.EventRecorderLogger
}

func (h *SyncHandler) Name() string {
	return nameSyncHandler
}

// Handle synchronizes NodeUSBDevice attributes from ResourceSlice.
// This handler updates dynamic attributes that may change without changing the device hash.
func (h *SyncHandler) Handle(ctx context.Context, s state.NodeUSBDeviceState) (reconcile.Result, error) {
	nodeUSBDevice := s.NodeUSBDevice()

	if nodeUSBDevice.IsEmpty() {
		return reconcile.Result{}, nil
	}

	current := nodeUSBDevice.Current()
	changed := nodeUSBDevice.Changed()

	// Get ResourceSlices
	resourceSlices, err := s.ResourceSlices(ctx)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("failed to get resource slices: %w", err)
	}

	// Find device in ResourceSlices and get updated attributes
	updatedAttrs, found := h.findDeviceInSlices(resourceSlices, current.Status.Attributes.Hash, current.Status.NodeName)
	if !found {
		// Device not found - ReadyHandler will handle this case
		return reconcile.Result{}, nil
	}

	// Check if any attributes changed and update
	if h.attributesChanged(current.Status.Attributes, updatedAttrs) {
		changed.Status.Attributes = updatedAttrs
	}

	return reconcile.Result{}, nil
}

func (h *SyncHandler) findDeviceInSlices(slices []resourcev1beta1.ResourceSlice, searchedHash, nodeName string) (v1alpha2.NodeUSBDeviceAttributes, bool) {
	for _, slice := range slices {
		if slice.Spec.Pool.Name != nodeName {
			continue
		}

		for _, device := range slice.Spec.Devices {
			if !strings.HasPrefix(device.Name, "usb-") {
				continue
			}

			attrs := h.convertDeviceToAttributes(device, nodeName)
			deviceHash := hash.CalculateHash(attrs)

			if deviceHash == searchedHash {
				// Set hash in attributes
				attrs.Hash = searchedHash
				return attrs, true
			}
		}
	}

	return v1alpha2.NodeUSBDeviceAttributes{}, false
}

func (h *SyncHandler) convertDeviceToAttributes(device resourcev1beta1.Device, nodeName string) v1alpha2.NodeUSBDeviceAttributes {
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

	return attrs
}

// attributesChanged compares attributes to check if they need updating.
// This compares all attributes, not just the ones used for hash calculation.
func (h *SyncHandler) attributesChanged(current, updated v1alpha2.NodeUSBDeviceAttributes) bool {
	return current.Name != updated.Name ||
		current.Manufacturer != updated.Manufacturer ||
		current.Product != updated.Product ||
		current.BCD != updated.BCD ||
		current.Major != updated.Major ||
		current.Minor != updated.Minor ||
		current.VendorID != updated.VendorID ||
		current.ProductID != updated.ProductID ||
		current.Bus != updated.Bus ||
		current.DeviceNumber != updated.DeviceNumber ||
		current.Serial != updated.Serial ||
		current.DevicePath != updated.DevicePath
}
