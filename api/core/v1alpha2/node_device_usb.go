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

package v1alpha2

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NodeUSBDevice represents a USB device discovered on a specific node in the cluster.
// This resource is created automatically by the DRA (Dynamic Resource Allocation) system
// when a USB device is detected on a node.
// +genclient
// +kubebuilder:object:root=true
// +kubebuilder:metadata:labels={heritage=deckhouse,module=virtualization}
// +kubebuilder:resource:categories={virtualization},scope=Cluster,shortName={nusb},singular=nodeusbdevice
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Node",type=string,JSONPath=`.status.nodeName`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="Assigned",type=string,JSONPath=`.status.conditions[?(@.type=="Assigned")].status`
// +kubebuilder:printcolumn:name="Namespace",type=string,JSONPath=`.spec.assignedNamespace`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type NodeUSBDevice struct {
	metav1.TypeMeta `json:",inline"`

	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec NodeUSBDeviceSpec `json:"spec"`

	Status NodeUSBDeviceStatus `json:"status,omitempty"`
}

// NodeUSBDeviceList provides the needed parameters
// for requesting a list of NodeUSBDevices from the system.
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type NodeUSBDeviceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	// Items provides a list of NodeUSBDevices.
	Items []NodeUSBDevice `json:"items"`
}

type NodeUSBDeviceSpec struct {
	// Namespace in which the device usage is allowed. By default, created with an empty value "".
	// When set, a corresponding USBDevice resource is created in this namespace.
	// +kubebuilder:default:=""
	AssignedNamespace string `json:"assignedNamespace,omitempty"`
}

type NodeUSBDeviceStatus struct {
	// All device attributes obtained through DRA for the device.
	Attributes NodeUSBDeviceAttributes `json:"attributes,omitempty"`
	// Name of the node where the USB device is located.
	NodeName string `json:"nodeName,omitempty"`
	// The latest available observations of an object's current state.
	Conditions []metav1.Condition `json:"conditions,omitempty"`
	// Resource generation last processed by the controller.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// NodeUSBDeviceAttributes contains all attributes of a USB device.
type NodeUSBDeviceAttributes struct {
	// BCD (Binary Coded Decimal) device version.
	BCD string `json:"bcd,omitempty"`
	// USB bus number.
	Bus string `json:"bus,omitempty"`
	// USB device number on the bus.
	DeviceNumber string `json:"deviceNumber,omitempty"`
	// Device path in the filesystem.
	DevicePath string `json:"devicePath,omitempty"`
	// Major device number.
	Major int `json:"major,omitempty"`
	// Minor device number.
	Minor int `json:"minor,omitempty"`
	// Device name.
	Name string `json:"name,omitempty"`
	// USB vendor ID in hexadecimal format.
	VendorID string `json:"vendorID,omitempty"`
	// USB product ID in hexadecimal format.
	ProductID string `json:"productID,omitempty"`
	// Device serial number.
	Serial string `json:"serial,omitempty"`
	// Device manufacturer name.
	Manufacturer string `json:"manufacturer,omitempty"`
	// Device product name.
	Product string `json:"product,omitempty"`
	// Node name where the device is located.
	NodeName string `json:"nodeName,omitempty"`
}
