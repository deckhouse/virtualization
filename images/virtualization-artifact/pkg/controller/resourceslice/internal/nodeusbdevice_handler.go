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

	"github.com/deckhouse/virtualization-controller/pkg/common/hash"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/nodeusbdevicecondition"
)

const (
	nameNodeUSBDeviceHandler = "NodeUSBDeviceHandler"
)

func NewNodeUSBDeviceHandler(client client.Client) *NodeUSBDeviceHandler {
	return &NodeUSBDeviceHandler{client: client}
}

type NodeUSBDeviceHandler struct {
	client client.Client
}

func (h *NodeUSBDeviceHandler) Name() string {
	return nameNodeUSBDeviceHandler
}

func (h *NodeUSBDeviceHandler) Handle(ctx context.Context, slice *resourcev1beta1.ResourceSlice) error {
	hasUSBDevices := false
	for _, device := range slice.Spec.Devices {
		if strings.HasPrefix(device.Name, "usb-") {
			hasUSBDevices = true
			break
		}
	}
	if !hasUSBDevices {
		return nil
	}

	nodeName := slice.Spec.Pool.Name

	var existingDevices v1alpha2.NodeUSBDeviceList
	if err := h.client.List(ctx, &existingDevices); err != nil {
		return fmt.Errorf("list NodeUSBDevices: %w", err)
	}

	existingHashes := make(map[string]bool)
	for _, device := range existingDevices.Items {
		if device.Status.Attributes.Hash != "" {
			existingHashes[device.Status.Attributes.Hash] = true
		}
	}

	for _, device := range slice.Spec.Devices {
		if !strings.HasPrefix(device.Name, "usb-") {
			continue
		}

		attributes := convertDeviceToAttributes(device, nodeName)
		hashStr := hash.CalculateHash(attributes)

		if existingHashes[hashStr] {
			continue
		}

		if err := h.createNodeUSBDevice(ctx, attributes, hashStr); err != nil {
			return err
		}

		existingHashes[hashStr] = true
	}

	return nil
}

func (h *NodeUSBDeviceHandler) createNodeUSBDevice(ctx context.Context, attributes v1alpha2.NodeUSBDeviceAttributes, hashStr string) error {
	name := generateName(hashStr, attributes.NodeName)

	existing := &v1alpha2.NodeUSBDevice{}
	err := h.client.Get(ctx, client.ObjectKey{Name: name}, existing)
	if err == nil {
		return nil
	}
	if !apierrors.IsNotFound(err) {
		return fmt.Errorf("check NodeUSBDevice %s: %w", name, err)
	}

	nodeUSBDevice := &v1alpha2.NodeUSBDevice{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: v1alpha2.NodeUSBDeviceSpec{
			AssignedNamespace: "",
		},
	}

	if err := h.client.Create(ctx, nodeUSBDevice); err != nil {
		if apierrors.IsAlreadyExists(err) {
			return nil
		}
		return fmt.Errorf("create NodeUSBDevice %s: %w", name, err)
	}

	attributes.Hash = hashStr
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
		return fmt.Errorf("update NodeUSBDevice %s status: %w", name, err)
	}

	return nil
}

func convertDeviceToAttributes(device resourcev1beta1.Device, nodeName string) v1alpha2.NodeUSBDeviceAttributes {
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

func generateName(hashStr, nodeName string) string {
	nodeNameSanitized := strings.ToLower(strings.ReplaceAll(nodeName, ".", "-"))
	return fmt.Sprintf("nusb-%s-%s", hashStr[:8], nodeNameSanitized)
}
