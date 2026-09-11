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
	NodePCIDeviceKind = "NodePCIDevice"
)

// Represents a PCI device discovered on a specific node in the cluster and suitable for passthrough.
// The module controller creates this resource from Kubernetes API data (ResourceSlice) published by the DRA driver.
// +genclient
// +genclient:nonNamespaced
// +kubebuilder:object:root=true
// +crd-enricher:deckhouse:documentation:examples={apiVersion: virtualization.deckhouse.io/v1alpha2, kind: NodePCIDevice, metadata: {name: example-pci}, spec: {assignedNamespace: workloads}}
// +kubebuilder:metadata:labels={heritage=deckhouse,module=virtualization}
// +kubebuilder:resource:categories={virtualization},scope=Cluster,shortName={npci},singular=nodepcidevice
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Node",type=string,JSONPath=`.status.nodeName`
// +kubebuilder:printcolumn:name="Address",type=string,JSONPath=`.status.attributes.pciAddress`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="Assigned",type=string,JSONPath=`.status.conditions[?(@.type=="Assigned")].status`
// +kubebuilder:printcolumn:name="Attached",type=string,JSONPath=`.status.conditions[?(@.type=="Attached")].status`
// +kubebuilder:printcolumn:name="Namespace",type=string,JSONPath=`.spec.assignedNamespace`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type NodePCIDevice struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NodePCIDeviceSpec   `json:"spec"`
	Status NodePCIDeviceStatus `json:"status,omitempty"`
}

// NodePCIDeviceList provides the needed parameters
// for requesting a list of NodePCIDevices from the system.
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type NodePCIDeviceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	// Items provides a list of NodePCIDevices.
	Items []NodePCIDevice `json:"items"`
}

type NodePCIDeviceSpec struct {
	// AssignedNamespace in which the device usage is allowed. By default, created with an empty value "".
	// When set, a corresponding PCIDevice resource is created in this namespace.
	// +kubebuilder:default:=""
	AssignedNamespace string `json:"assignedNamespace,omitempty"`
}

type NodePCIDeviceStatus struct {
	// All device attributes obtained through DRA for the device.
	Attributes NodePCIDeviceAttributes `json:"attributes,omitempty"`
	// Name of the node where the PCI device is located.
	NodeName string `json:"nodeName,omitempty"`
	// The latest available observations of an object's current state.
	Conditions []metav1.Condition `json:"conditions,omitempty"`
	// Resource generation last processed by the controller.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// NodePCIDeviceAttributes contains all attributes of a PCI device.
type NodePCIDeviceAttributes struct {
	// Device name.
	Name string `json:"name,omitempty"`
	// PCI address of the device in extended BDF notation, for example 0000:3b:00.0.
	PCIAddress string `json:"pciAddress,omitempty"`
	// PCI vendor ID in hexadecimal format.
	VendorID string `json:"vendorID,omitempty"`
	// PCI device ID in hexadecimal format.
	DeviceID string `json:"deviceID,omitempty"`
	// PCI class code in hexadecimal format.
	Class string `json:"class,omitempty"`
	// Kernel driver bound to the device on the host.
	Driver string `json:"driver,omitempty"`
	// IOMMU group number the device belongs to. The whole group is passed to the virtual machine.
	IOMMUGroup int64 `json:"iommuGroup,omitempty"`
	// NUMA node of the device; -1 when the platform reports none.
	NUMANode int64 `json:"numaNode,omitempty"`
	// Node name where the device is located.
	NodeName string `json:"nodeName,omitempty"`
}
