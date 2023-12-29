package v2alpha1

import (
	corev1 "k8s.io/api/core/v1"
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
	// RunPolicy is a power-on behaviour of the VM.
	RunPolicy RunPolicy `json:"runPolicy"`
	// VirtualMachineIPAddressClaimName specifies a name for the associated
	// `VirtualMahcineIPAddressClaim` resource. Defaults to `{vm name}`.
	VirtualMachineIPAddressClaimName string `json:"virtualMachineIPAddressClaimName,omitempty"`
	// TopologySpreadConstraints specifies how to spread matching pods among the given topology.
	TopologySpreadConstraints []corev1.TopologySpreadConstraint `json:"topologySpreadConstraints,omitempty"`
	// Affinity is a group of affinity scheduling rules.
	Affinity *VMAffinity `json:"affinity,omitempty"`
	// A selector which must be true for the vm to fit on a node.
	// Selector which must match a node's labels for the VM to be scheduled on that node.
	// +optional
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`
	// PriorityClassName
	PriorityClassName string `json:"priorityClassName"`
	// Tolerations define rules to tolerate node taints.
	Tolerations []corev1.Toleration `json:"tolerations,omitempty"`
	// Disruptions define an approval mode to apply disruptive (dangerous) changes.
	Disruptions                   *Disruptions      `json:"disruptions"`
	TerminationGracePeriodSeconds *int64            `json:"terminationGracePeriodSeconds,omitempty"`
	EnableParavirtualization      bool              `json:"enableParavirtualization,omitempty"`
	OsType                        OsType            `json:"osType,omitempty"`
	Bootloader                    BootloaderType    `json:"bootloader,omitempty"`
	CPU                           CPUSpec           `json:"cpu"`
	Memory                        MemorySpec        `json:"memory"`
	BlockDevices                  []BlockDeviceSpec `json:"blockDevices"`
	Provisioning                  *Provisioning     `json:"provisioning"`
	ApprovedChangeID              string            `json:"approvedChangeID,omitempty"`
}

type RunPolicy string

const (
	AlwaysOnPolicy               RunPolicy = "AlwaysOn"
	AlwaysOffPolicy              RunPolicy = "AlwaysOff"
	ManualPolicy                 RunPolicy = "Manual"
	AlwaysOnUnlessStoppedManualy RunPolicy = "AlwaysOnUnlessStoppedManualy"
)

type OsType string

const (
	Windows       OsType = "Windows"
	LegacyWindows OsType = "LegacyWindows"
	GenericOs     OsType = "Generic"
)

type BootloaderType string

const (
	BIOS              BootloaderType = "BIOS"
	EFI               BootloaderType = "EFI"
	EFIWithSecureBoot BootloaderType = "EFIWithSecureBoot"
)

type CPUSpec struct {
	Cores        int    `json:"cores"`
	CoreFraction string `json:"coreFraction"`
}

type MemorySpec struct {
	Size string `json:"size"`
}

type ApprovalMode string

const (
	Automatic ApprovalMode = "Automatic"
	Manual    ApprovalMode = "Manual"
)

type Disruptions struct {
	// Allow disruptive update mode: Manual or Automatic.
	ApprovalMode ApprovalMode `json:"approvalMode"`
}

type Provisioning struct {
	Type              ProvisioningType             `json:"type"`
	UserData          string                       `json:"userData,omitempty"`
	UserDataSecretRef *corev1.LocalObjectReference `json:"userDataSecretRef,omitempty"`
}

type VirtualMachineStatus struct {
	Phase                MachinePhase                             `json:"phase"`
	NodeName             string                                   `json:"nodeName"`
	IPAddressClaim       string                                   `json:"ipAddressClaim"`
	IPAddress            string                                   `json:"ipAddress"`
	BlockDevicesAttached []BlockDeviceStatus                      `json:"blockDevicesAttached"`
	GuestOSInfo          virtv1.VirtualMachineInstanceGuestOSInfo `json:"guestOSInfo"`
	Message              string                                   `json:"message"`
	ChangeID             string                                   `json:"changeID"`
	PendingChanges       []FieldChangeOperation                   `json:"pendingChanges,omitempty"`
}

type MachinePhase string

const (
	MachineScheduling  MachinePhase = "Scheduling"
	MachinePending     MachinePhase = "Pending"
	MachineRunning     MachinePhase = "Running"
	MachineFailed      MachinePhase = "Failed"
	MachineTerminating MachinePhase = "Terminating"
	MachineStopped     MachinePhase = "Stopped"
	MachineStopping    MachinePhase = "Stopping"
	MachineStarting    MachinePhase = "Starting"
	MachinePause       MachinePhase = "Pause"
)

// FieldChangeOperation holds one operation to be applied on the spec field.
//
// The structure of the field change operation has these fields:
//
//	operation enum(add|remove|replace)
//	path string
//	currentValue any
//	desiredValue any
//
// An 'any' type can't be described using the OpenAPI v3 schema.
// The workaround is to declare items of pendingChanges as objects with preserved fields.
type FieldChangeOperation interface{}

// VirtualMachineList contains a list of VirtualMachine
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type VirtualMachineList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`
	Items           []VirtualMachine `json:"items"`
}

type ProvisioningType string

const (
	ProvisioningTypeUserData       ProvisioningType = "UserData"
	ProvisioningTypeUserDataSecret ProvisioningType = "UserDataSecret"
)
