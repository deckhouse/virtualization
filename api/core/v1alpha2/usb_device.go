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

const (
	USBDeviceKind     = "USBDevice"
	USBDeviceResource = "usbdevices"
)

// USBDevice represents a USB device available for attachment to virtual machines in a given namespace.
// +genclient
// +kubebuilder:object:root=true
// +kubebuilder:metadata:labels={heritage=deckhouse,module=virtualization}
// +kubebuilder:resource:categories={virtualization},scope=Namespaced,shortName={usb},singular=usbdevice
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Node",type=string,JSONPath=`.status.nodeName`
// +kubebuilder:printcolumn:name="VendorID",type=string,JSONPath=`.status.attributes.vendorID`,priority=1
// +kubebuilder:printcolumn:name="ProductID",type=string,JSONPath=`.status.attributes.productID`,priority=1
// +kubebuilder:printcolumn:name="Bus",type=string,JSONPath=`.status.attributes.bus`,priority=1
// +kubebuilder:printcolumn:name="DeviceNumber",type=string,JSONPath=`.status.attributes.deviceNumber`,priority=1
// +kubebuilder:printcolumn:name="Manufacturer",type=string,JSONPath=`.status.attributes.manufacturer`
// +kubebuilder:printcolumn:name="Product",type=string,JSONPath=`.status.attributes.product`
// +kubebuilder:printcolumn:name="Serial",type=string,JSONPath=`.status.attributes.serial`,priority=1
// +kubebuilder:printcolumn:name="Attached",type=string,JSONPath=`.status.conditions[?(@.type=="Attached")].status`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type USBDevice struct {
	metav1.TypeMeta `json:",inline"`

	metav1.ObjectMeta `json:"metadata,omitempty"`

	Status USBDeviceStatus `json:"status,omitempty"`
}

// USBDeviceList provides the needed parameters
// for requesting a list of USBDevices from the system.
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type USBDeviceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	// Items provides a list of USBDevices.
	Items []USBDevice `json:"items"`
}

// USBDeviceStatus is the observed state of `USBDevice`.
type USBDeviceStatus struct {
	// All device attributes obtained through DRA for the device.
	Attributes NodeUSBDeviceAttributes `json:"attributes,omitempty"`
	// Name of the node where the USB device is located.
	NodeName string `json:"nodeName,omitempty"`
	// The latest available observations of an object's current state.
	Conditions []metav1.Condition `json:"conditions,omitempty"`
	// Resource generation last processed by the controller.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}
