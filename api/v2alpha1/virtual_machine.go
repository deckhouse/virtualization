package v2alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	virtv1 "kubevirt.io/api/core/v1"
)

const (
	VMKind     = "VirtualMachine"
	VMResource = "virtualmachines"
)

// VirtualMachine is a disk ready to be bound by a VM
// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type VirtualMachine struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   VirtualMachineSpec   `json:"spec"`
	Status VirtualMachineStatus `json:"status,omitempty"`
}

type VirtualMachineSpec struct {
	RunPolicy                RunPolicy         `json:"runPolicy"`
	CPU                      CPUSpec           `json:"cpu"`
	Memory                   MemorySpec        `json:"memory"`
	BlockDevices             []BlockDeviceSpec `json:"blockDevices"`
	EnableParavirtualization bool              `json:"enableParavirtualization,omitempty"`
	OsType                   OsType            `json:"osType,omitempty"`
}

type RunPolicy string

const (
	AlwaysOnPolicy RunPolicy = "AlwaysOn"
)

type OsType string

const (
	Windows       OsType = "Windows"
	LegacyWindows OsType = "LegacyWindows"
	GenericOs     OsType = "Generic"
)

type CPUSpec struct {
	Cores int `json:"cores"`
}

type MemorySpec struct {
	Size string `json:"size"`
}

type VirtualMachineStatus struct {
	Phase                MachinePhase                             `json:"phase"`
	NodeName             string                                   `json:"nodeName"`
	IPAddress            string                                   `json:"ipAddress"`
	BlockDevicesAttached []BlockDeviceStatus                      `json:"blockDevicesAttached"`
	GuestOSInfo          virtv1.VirtualMachineInstanceGuestOSInfo `json:"guestOSInfo"`
}

type MachinePhase string

const (
	MachineScheduling  MachinePhase = "Scheduling"
	MachinePending     MachinePhase = "Pending"
	MachineRunning     MachinePhase = "Running"
	MachineFailed      MachinePhase = "Failed"
	MachineTerminating MachinePhase = "Terminating"
	MachineStopped     MachinePhase = "Stopped"
)

// VirtualMachineList contains a list of VirtualMachine
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type VirtualMachineList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`
	Items           []VirtualMachine `json:"items"`
}
