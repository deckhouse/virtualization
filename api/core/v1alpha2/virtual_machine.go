/*
Copyright 2024 Flant JSC

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
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	virtv1 "kubevirt.io/api/core/v1"
)

const (
	VirtualMachineKind     = "VirtualMachine"
	VirtualMachineResource = "virtualmachines"
)

// VirtualMachine specifies configuration of the virtual machine.
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

	// VirtualMachineIPAddress specifies a name for the associated
	// `VirtualMachineIPAddress` resource. Defaults to `{vm name}`.
	VirtualMachineIPAddress string `json:"virtualMachineIPAddressName,omitempty"`

	// TopologySpreadConstraints specifies how to spread matching pods among the given topology.
	TopologySpreadConstraints []corev1.TopologySpreadConstraint `json:"topologySpreadConstraints,omitempty"`

	// Affinity is a group of affinity scheduling rules.
	Affinity *VMAffinity `json:"affinity,omitempty"`

	// NodeSelector must match a node's labels for the VM to be scheduled on that node.
	// +optional
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`

	// PriorityClassName
	PriorityClassName string `json:"priorityClassName"`

	// Tolerations define rules to tolerate node taints.
	Tolerations []corev1.Toleration `json:"tolerations,omitempty"`

	// Disruptions define an approval mode to apply disruptive (dangerous) changes.
	Disruptions *Disruptions `json:"disruptions"`

	// TerminationGracePeriodSeconds
	TerminationGracePeriodSeconds *int64 `json:"terminationGracePeriodSeconds,omitempty"`

	// EnableParavirtualization flag disables virtio for virtual machine.
	// Default value is true, so omitempty is not specified.
	EnableParavirtualization bool `json:"enableParavirtualization"`

	OsType                  OsType               `json:"osType,omitempty"`
	Bootloader              BootloaderType       `json:"bootloader,omitempty"`
	VirtualMachineClassName string               `json:"virtualMachineClassName,omitempty"`
	CPU                     CPUSpec              `json:"cpu"`
	Memory                  MemorySpec           `json:"memory"`
	BlockDeviceRefs         []BlockDeviceSpecRef `json:"blockDeviceRefs"`
	Provisioning            *Provisioning        `json:"provisioning"`
}

type RunPolicy string

const (
	AlwaysOnPolicy                RunPolicy = "AlwaysOn"
	AlwaysOffPolicy               RunPolicy = "AlwaysOff"
	ManualPolicy                  RunPolicy = "Manual"
	AlwaysOnUnlessStoppedManually RunPolicy = "AlwaysOnUnlessStoppedManually"
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
	Size resource.Quantity `json:"size"`
}

type RestartApprovalMode string

const (
	Automatic RestartApprovalMode = "Automatic"
	Manual    RestartApprovalMode = "Manual"
)

type Disruptions struct {
	// RestartApprovalMode defines a restart approving mode: Manual or Automatic.
	RestartApprovalMode RestartApprovalMode `json:"restartApprovalMode"`
}

type Provisioning struct {
	Type        ProvisioningType `json:"type"`
	UserData    string           `json:"userData,omitempty"`
	UserDataRef *UserDataRef     `json:"userDataRef,omitempty"`
	SysprepRef  *SysprepRef      `json:"sysprepRef,omitempty"`
}

type UserDataRef struct {
	Kind UserDataRefKind `json:"kind"`
	Name string          `json:"name"`
}

type UserDataRefKind string

const (
	UserDataRefKindSecret UserDataRefKind = "Secret"
)

type SysprepRef struct {
	Kind SysprepRefKind `json:"kind"`
	Name string         `json:"name"`
}

type SysprepRefKind string

const (
	SysprepRefKindSecret SysprepRefKind = "Secret"
)

type VirtualMachineStatus struct {
	Phase                   MachinePhase                             `json:"phase"`
	Node                    string                                   `json:"nodeName"`
	VirtualMachineIPAddress string                                   `json:"virtualMachineIPAddressName"`
	IPAddress               string                                   `json:"ipAddress"`
	BlockDeviceRefs         []BlockDeviceStatusRef                   `json:"blockDeviceRefs"`
	GuestOSInfo             virtv1.VirtualMachineInstanceGuestOSInfo `json:"guestOSInfo"`
	Conditions              []metav1.Condition                       `json:"conditions,omitempty"`
	Stats                   *VirtualMachineStats                     `json:"stats,omitempty"`
	MigrationState          *VirtualMachineMigrationState            `json:"migrationState,omitempty"`
	ObservedGeneration      int64                                    `json:"observedGeneration,omitempty"`
	// RestartAwaitingChanges holds operations to be manually approved
	// before applying to the virtual machine spec.
	//
	// Change operation has these fields:
	//
	//	operation enum(add|remove|replace)
	//	path string
	//	currentValue any (bool|int|string|struct|array of structs)
	//	desiredValue any (bool|int|string|struct|array of structs)
	//
	// Such 'any' type can't be described using the OpenAPI v3 schema.
	// The workaround is to declare a whole change operation structure
	// using 'type: object' and 'x-kubernetes-preserve-fields: true'.
	RestartAwaitingChanges []apiextensionsv1.JSON `json:"restartAwaitingChanges,omitempty"`
}

type VirtualMachineStats struct {
	PhasesTransitions  []VirtualMachinePhaseTransitionTimestamp `json:"phasesTransitions,omitempty"`
	LaunchTimeDuration VirtualMachineLaunchTimeDuration         `json:"launchTimeDuration,omitempty"`
}

// VirtualMachinePhaseTransitionTimestamp gives a timestamp in relation to when a phase is set on a vm.
type VirtualMachinePhaseTransitionTimestamp struct {
	Phase MachinePhase `json:"phase,omitempty"`
	// PhaseTransitionTimestamp is the timestamp of when the phase change occurred
	Timestamp metav1.Time `json:"timestamp,omitempty"`
}

type VirtualMachineLaunchTimeDuration struct {
	WaitingForDependencies *metav1.Duration `json:"waitingForDependencies,omitempty"`
	VirtualMachineStarting *metav1.Duration `json:"virtualMachineStarting,omitempty"`
	GuestOSAgentStarting   *metav1.Duration `json:"guestOSAgentStarting,omitempty"`
}

type VirtualMachineMigrationState struct {
	StartTimestamp *metav1.Time           `json:"startTimestamp,omitempty"`
	EndTimestamp   *metav1.Time           `json:"endTimestamp,omitempty"`
	Target         VirtualMachineLocation `json:"target,omitempty"`
	Source         VirtualMachineLocation `json:"source,omitempty"`
	Result         MigrationResult        `json:"result,omitempty"`
}

type MigrationResult string

const (
	MigrationResultSucceeded MigrationResult = "Succeeded"
	MigrationResultFailed    MigrationResult = "Failed"
)

type VirtualMachineLocation struct {
	Node string `json:"node,omitempty"`
	Pod  string `json:"pod,omitempty"`
}

type MachinePhase string

const (
	MachinePending     MachinePhase = "Pending"
	MachineRunning     MachinePhase = "Running"
	MachineTerminating MachinePhase = "Terminating"
	MachineStopped     MachinePhase = "Stopped"
	MachineStopping    MachinePhase = "Stopping"
	MachineStarting    MachinePhase = "Starting"
	MachineMigrating   MachinePhase = "Migrating"
	MachinePause       MachinePhase = "Pause"
)

// VirtualMachineList contains a list of VirtualMachine
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type VirtualMachineList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`
	Items           []VirtualMachine `json:"items"`
}

type ProvisioningType string

const (
	ProvisioningTypeUserData    ProvisioningType = "UserData"
	ProvisioningTypeUserDataRef ProvisioningType = "UserDataRef"
	ProvisioningTypeSysprepRef  ProvisioningType = "SysprepRef"
)
