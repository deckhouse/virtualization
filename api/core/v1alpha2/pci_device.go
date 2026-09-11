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
	PCIDeviceKind     = "PCIDevice"
	PCIDeviceResource = "pcidevices"
)

// Represents a PCI device available for attachment to virtual machines in a given namespace.
// +genclient
// +kubebuilder:object:root=true
// +crd-enricher:deckhouse:documentation:examples={apiVersion: virtualization.deckhouse.io/v1alpha2, kind: PCIDevice, metadata: {name: example-pci}}
// +kubebuilder:metadata:labels={heritage=deckhouse,module=virtualization}
// +kubebuilder:resource:categories={virtualization},scope=Namespaced,shortName={pci},singular=pcidevice
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Node",type=string,JSONPath=`.status.nodeName`
// +kubebuilder:printcolumn:name="Address",type=string,JSONPath=`.status.attributes.pciAddress`
// +kubebuilder:printcolumn:name="VendorID",type=string,JSONPath=`.status.attributes.vendorID`,priority=1
// +kubebuilder:printcolumn:name="DeviceID",type=string,JSONPath=`.status.attributes.deviceID`,priority=1
// +kubebuilder:printcolumn:name="Class",type=string,JSONPath=`.status.attributes.class`,priority=1
// +kubebuilder:printcolumn:name="Attached",type=string,JSONPath=`.status.conditions[?(@.type=="Attached")].status`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type PCIDevice struct {
	metav1.TypeMeta `json:",inline"`

	metav1.ObjectMeta `json:"metadata,omitempty"`

	Status PCIDeviceStatus `json:"status,omitempty"`
}

// PCIDeviceList provides the needed parameters
// for requesting a list of PCIDevices from the system.
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type PCIDeviceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	// Items provides a list of PCIDevices.
	Items []PCIDevice `json:"items"`
}

// Observed state of `PCIDevice`.
type PCIDeviceStatus struct {
	// All device attributes obtained through DRA for the device.
	Attributes NodePCIDeviceAttributes `json:"attributes,omitempty"`
	// Name of the node where the PCI device is located.
	NodeName string `json:"nodeName,omitempty"`
	// Latest available observations of an object's current state.
	Conditions []metav1.Condition `json:"conditions,omitempty"`
	// Resource generation last processed by the controller.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}
