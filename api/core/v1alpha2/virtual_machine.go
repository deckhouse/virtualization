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

// VirtualMachine describes the configuration and status of a virtual machine (VM).
// For a running VM, parameter changes can only be applied after the VM is rebooted, except for the following parameters (they are applied on the fly):
// - `.metadata.labels`.
// - `.metadata.annotations`.
// - `.spec.disruptions.restartApprovalMode`.
// - `.spec.disruptions.runPolicy`.
//
// +kubebuilder:object:root=true
// +kubebuilder:metadata:labels={heritage=deckhouse,module=virtualization}
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories={all,virtualization},scope=Namespaced,shortName={vm},singular=virtualmachine
// +kubebuilder:printcolumn:name="Phase",type="string",JSONPath=".status.phase",description="The phase of the virtual machine."
// +kubebuilder:printcolumn:name="Cores",priority=1,type="string",JSONPath=".spec.cpu.cores",description="The number of cores of the virtual machine."
// +kubebuilder:printcolumn:name="CoreFraction",priority=1,type="string",JSONPath=".spec.cpu.coreFraction",description="Virtual machine core fraction. The range of available values is set in the `sizePolicy` parameter of the VirtualMachineClass; if it is not set, use values within the 1–100% range."
// +kubebuilder:printcolumn:name="Memory",priority=1,type="string",JSONPath=".spec.memory.size",description="The amount of memory of the virtual machine."
// +kubebuilder:printcolumn:name="Need restart",priority=1,type="string",JSONPath=".status.conditions[?(@.type=='AwaitingRestartToApplyConfiguration')].status",description="A restart of the virtual machine is required."
// +kubebuilder:printcolumn:name="Agent",priority=1,type="string",JSONPath=".status.conditions[?(@.type=='AgentReady')].status",description="Agent status."
// +kubebuilder:printcolumn:name="Migratable",priority=1,type="string",JSONPath=".status.conditions[?(@.type=='Migratable')].status",description="Is it possible to migrate a virtual machine."
// +kubebuilder:printcolumn:name="Node",type="string",JSONPath=".status.nodeName",description="The node where the virtual machine is running."
// +kubebuilder:printcolumn:name="IPAddress",type="string",JSONPath=".status.ipAddress",description="The IP address of the virtual machine."
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp",description="Time of creation resource."
// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type VirtualMachine struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   VirtualMachineSpec   `json:"spec"`
	Status VirtualMachineStatus `json:"status,omitempty"`
}

type VirtualMachineSpec struct {
	// +kubebuilder:default:="AlwaysOnUnlessStoppedManually"
	RunPolicy RunPolicy `json:"runPolicy,omitempty"`

	// Name for the associated `virtualMachineIPAddress` resource.
	// Specified when it is necessary to use a previously created IP address of the VM.
	// If not explicitly specified, by default a `virtualMachineIPAddress` resource is created for the VM with a name similar to the VM resource (`.metadata.name`).
	VirtualMachineIPAddress string `json:"virtualMachineIPAddressName,omitempty"`

	TopologySpreadConstraints []corev1.TopologySpreadConstraint `json:"topologySpreadConstraints,omitempty"`

	Affinity *VMAffinity `json:"affinity,omitempty"`

	// NodeSelector must match a node's labels for the VM to be scheduled on that node.
	// [The same](https://kubernetes.io/docs/tasks/configure-pod-container/assign-pods-nodes//) as in the pods `spec.nodeSelector` parameter in Kubernetes.
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`

	// PriorityClassName [The same](https://kubernetes.io/docs/concepts/scheduling-eviction/pod-priority-preemption/)  as in the pods `spec.priorityClassName` parameter in Kubernetes.
	PriorityClassName string `json:"priorityClassName,omitempty"`

	// Tolerations define rules to tolerate node taints.
	// The same](https://kubernetes.io/docs/concepts/scheduling-eviction/taint-and-toleration/) as in the pods `spec.tolerations` parameter in Kubernetes.
	Tolerations []corev1.Toleration `json:"tolerations,omitempty"`

	// +kubebuilder:default:={"restartApprovalMode": "Manual"}
	Disruptions *Disruptions `json:"disruptions,omitempty"`

	// Grace period observed after signalling a VM to stop after which the VM is force terminated.
	// +kubebuilder:default:=60
	TerminationGracePeriodSeconds *int64 `json:"terminationGracePeriodSeconds,omitempty"`

	// Use the `virtio` bus to connect virtual devices of the VM. Set false to disable `virtio` for this VM.
	// Note: To use paravirtualization mode, some operating systems require the appropriate drivers to be installed.
	// +kubebuilder:default:=true
	EnableParavirtualization bool `json:"enableParavirtualization,omitempty"`

	// +kubebuilder:default:="Generic"
	OsType OsType `json:"osType,omitempty"`
	// +kubebuilder:default:="BIOS"
	Bootloader BootloaderType `json:"bootloader,omitempty"`
	// Name of the `VirtualMachineClass` resource describing the requirements for a virtual CPU, memory and the resource allocation policy and node placement policies for virtual machines.
	VirtualMachineClassName string     `json:"virtualMachineClassName"`
	CPU                     CPUSpec    `json:"cpu"`
	Memory                  MemorySpec `json:"memory"`
	// List of block devices that can be mounted by disks belonging to the virtual machine.
	// The order of booting is determined by the order in the list.
	// +kubebuilder:validation:MinItems:=1
	// +kubebuilder:validation:MaxItems:=16
	BlockDeviceRefs []BlockDeviceSpecRef `json:"blockDeviceRefs"`
	Provisioning    *Provisioning        `json:"provisioning,omitempty"`

	// Live migration policy type.
	LiveMigrationPolicy LiveMigrationPolicy `json:"liveMigrationPolicy"`
	Networks            []NetworksSpec      `json:"networks,omitempty"`
	// List of USB devices to attach to the virtual machine.
	// Devices are referenced by name of USBDevice resource in the same namespace.
	// +kubebuilder:validation:MaxItems:=8
	USBDevices []USBDeviceSpecRef `json:"usbDevices,omitempty"`
}

// RunPolicy parameter defines the VM startup policy
// * `AlwaysOn` - after creation the VM is always in a running state, even in case of its shutdown by OS means.
// * `AlwaysOff` - after creation the VM is always in the off state.
// * `Manual` - after creation the VM is switched off, the VM state (switching on/off) is controlled via sub-resources or OS means.
// * `AlwaysOnUnlessStoppedManually` - after creation the VM is always in a running state. The VM can be shutdown by means of the OS or use the d8 utility: `d8 v stop <vm_name>`.
//
// +kubebuilder:validation:Enum={AlwaysOn,AlwaysOff,Manual,AlwaysOnUnlessStoppedManually}
type RunPolicy string

const (
	AlwaysOnPolicy                RunPolicy = "AlwaysOn"
	AlwaysOffPolicy               RunPolicy = "AlwaysOff"
	ManualPolicy                  RunPolicy = "Manual"
	AlwaysOnUnlessStoppedManually RunPolicy = "AlwaysOnUnlessStoppedManually"
)

// The OsType parameter allows you to select the type of used OS, for which a VM with an optimal set of required virtual devices and parameters will be created.
//
// * Windows - for Microsoft Windows family operating systems.
// * Generic - for other types of OS.
// +kubebuilder:validation:Enum={Windows,Generic}
type OsType string

const (
	Windows   OsType = "Windows"
	GenericOs OsType = "Generic"
)

// The BootloaderType defines bootloader for VM.
// * BIOS - use legacy BIOS.
// * EFI - use Unified Extensible Firmware (EFI/UEFI).
// * EFIWithSecureBoot - use UEFI/EFI with SecureBoot support.
// +kubebuilder:validation:Enum={BIOS,EFI,EFIWithSecureBoot}
type BootloaderType string

const (
	BIOS              BootloaderType = "BIOS"
	EFI               BootloaderType = "EFI"
	EFIWithSecureBoot BootloaderType = "EFIWithSecureBoot"
)

// CPUSpec specifies the CPU settings for the VM.
type CPUSpec struct {
	// Specifies the number of cores inside the VM. The value must be greater or equal 1.
	// +kubebuilder:validation:Format:=int32
	// +kubebuilder:validation:Minimum=1
	Cores int `json:"cores"`

	// Guaranteed share of CPU that will be allocated to the VM. Specified as a percentage.
	// The range of available values is defined in the VirtualMachineClass sizing policy.
	// If not specified, the default value from the VirtualMachineClass will be used.
	// +kubebuilder:validation:Pattern=`^(100|[1-9][0-9]?|[1-9])%$`
	CoreFraction string `json:"coreFraction,omitempty"`
}

// MemorySpec specifies the memory settings for the VM.
type MemorySpec struct {
	Size resource.Quantity `json:"size"`
}

// RestartApprovalMode defines a restart approving mode: Manual or Automatic.
// +kubebuilder:validation:Enum={Manual,Automatic}
type RestartApprovalMode string

const (
	Automatic RestartApprovalMode = "Automatic"
	Manual    RestartApprovalMode = "Manual"
)

// Disruptions describes the policy for applying changes that require rebooting the VM
// Changes to some VM configuration settings require a reboot of the VM to apply them. This policy allows you to specify the behavior of how the VM will respond to such changes.
type Disruptions struct {
	RestartApprovalMode RestartApprovalMode `json:"restartApprovalMode,omitempty"`
}

// Provisioning is a block allows you to configure the provisioning script for the VM.
//
// +kubebuilder:validation:XValidation:rule="self.type == 'UserData' ? has(self.userData) && !has(self.userDataRef) && !has(self.sysprepRef) : true",message="UserData cannot have userDataRef or sysprepRef."
// +kubebuilder:validation:XValidation:rule="self.type == 'UserDataRef' ? has(self.userDataRef) && !has(self.userData) && !has(self.sysprepRef) : true",message="UserDataRef cannot have userData or sysprepRef."
// +kubebuilder:validation:XValidation:rule="self.type == 'SysprepRef' ? has(self.sysprepRef) && !has(self.userData) && !has(self.userDataRef) : true",message="SysprepRef cannot have userData or userDataRef."
type Provisioning struct {
	Type ProvisioningType `json:"type"`
	// Inline cloud-init userdata script.
	UserData    string       `json:"userData,omitempty"`
	UserDataRef *UserDataRef `json:"userDataRef,omitempty"`
	SysprepRef  *SysprepRef  `json:"sysprepRef,omitempty"`
}

// UserDataRef is reference to an existing resource with a cloud-init script.
// Resource structure for userDataRef type:
// * `.data.userData`.
type UserDataRef struct {
	// The kind of existing cloud-init automation resource.
	// The following options are supported:
	//   - Secret
	//
	// +kubebuilder:validation:Enum:={Secret}
	// +kubebuilder:default:="Secret"
	Kind UserDataRefKind `json:"kind,omitempty"`
	Name string          `json:"name"`
}

type UserDataRefKind string

const (
	UserDataRefKindSecret UserDataRefKind = "Secret"
)

// SysprepRef is reference to an existing Windows sysprep automation.
// Resource structure for the SysprepRef type:
// * `.data.autounattend.xml`.
// * `.data.unattend.xml`.
type SysprepRef struct {
	// The kind of existing Windows sysprep automation resource.
	// The following options are supported:
	//  - Secret
	// +kubebuilder:validation:Enum:={Secret}
	// +kubebuilder:default:="Secret"
	Kind SysprepRefKind `json:"kind,omitempty"`
	Name string         `json:"name"`
}

type SysprepRefKind string

const (
	SysprepRefKindSecret SysprepRefKind = "Secret"
)

// LiveMigrationPolicy defines policy for live migration process:
// * `Never` - This VM is not eligible for live migration.
// * `Manual` - This VM is eligible for migrations triggered by user, no automatic migrations.
// * `AlwaysSafe` - Use Safe options for automatic and VMOP migrations. Do not enable CPU throttling.
// * `PreferSafe` - Use Safe options for automatic migrations. CPU throttling can be enabled with force=true in VMOP.
// * `AlwaysForced` - Enable CPU throttling for automatic and VMOP migrations. No way to disable CPU throttling.
// * `PreferForced` - Enable CPU throttling for automatic migrations. CPU throttling can be disabled with force=false in VMOP.
//
// +kubebuilder:validation:Enum={Manual,Never,AlwaysSafe,PreferSafe,AlwaysForced,PreferForced}
type LiveMigrationPolicy string

const (
	NetworksTypeMain           = "Main"
	NetworksTypeNetwork        = "Network"
	NetworksTypeClusterNetwork = "ClusterNetwork"
)

type NetworksSpec struct {
	Type                         string `json:"type"`
	Name                         string `json:"name,omitempty"`
	VirtualMachineMACAddressName string `json:"virtualMachineMACAddressName,omitempty"`
}

const (
	ManualMigrationPolicy       LiveMigrationPolicy = "Manual"
	NeverMigrationPolicy        LiveMigrationPolicy = "Never"
	AlwaysSafeMigrationPolicy   LiveMigrationPolicy = "AlwaysSafe"
	PreferSafeMigrationPolicy   LiveMigrationPolicy = "PreferSafe"
	AlwaysForcedMigrationPolicy LiveMigrationPolicy = "AlwaysForced"
	PreferForcedMigrationPolicy LiveMigrationPolicy = "PreferForced"
)

type VirtualMachineStatus struct {
	Phase MachinePhase `json:"phase"`
	// The name of the node on which the VM is currently running.
	Node string `json:"nodeName"`
	// Name of `virtualMachineIPAddressName` holding the ip address of the VirtualMachine.
	VirtualMachineIPAddress string `json:"virtualMachineIPAddressName"`
	// IP address of VM.
	IPAddress string `json:"ipAddress"`
	// The list of attached block device attachments.
	BlockDeviceRefs []BlockDeviceStatusRef                   `json:"blockDeviceRefs,omitempty"`
	GuestOSInfo     virtv1.VirtualMachineInstanceGuestOSInfo `json:"guestOSInfo,omitempty"`
	// Detailed state of the virtual machine lifecycle.
	Conditions []metav1.Condition `json:"conditions,omitempty"`
	// VirtualMachine statistics.
	Stats *VirtualMachineStats `json:"stats,omitempty"`
	// Migration info.
	MigrationState *VirtualMachineMigrationState `json:"migrationState,omitempty"`
	// Generating a resource that was last processed by the controller.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	/*
		Change operation has these fields:
		* operation enum(add|remove|replace)
		* path string
		* currentValue any (bool|int|string|struct|array of structs)
		* desiredValue any (bool|int|string|struct|array of structs)
		Such 'any' type can't be described using the OpenAPI v3 schema.
		The workaround is to declare a whole change operation structure
		using 'type: object' and 'x-kubernetes-preserve-fields: true'.
	*/

	// RestartAwaitingChanges holds operations to be manually approved before applying to the virtual machine spec.
	RestartAwaitingChanges []apiextensionsv1.JSON `json:"restartAwaitingChanges,omitempty"`
	// List of virtual machine pods.
	VirtualMachinePods []VirtualMachinePod `json:"virtualMachinePods,omitempty"`
	// Hypervisor versions.
	Versions  Versions         `json:"versions,omitempty"`
	Resources ResourcesStatus  `json:"resources,omitempty"`
	Networks  []NetworksStatus `json:"networks,omitempty"`
	// List of USB devices attached to the virtual machine.
	USBDevices []USBDeviceStatusRef `json:"usbDevices,omitempty"`
}

type VirtualMachineStats struct {
	// The history of phases.
	PhasesTransitions []VirtualMachinePhaseTransitionTimestamp `json:"phasesTransitions,omitempty"`
	// Launch information.
	LaunchTimeDuration VirtualMachineLaunchTimeDuration `json:"launchTimeDuration,omitempty"`
}

// VirtualMachinePhaseTransitionTimestamp gives a timestamp in relation to when a phase is set on a vm.
type VirtualMachinePhaseTransitionTimestamp struct {
	Phase MachinePhase `json:"phase,omitempty"`
	// PhaseTransitionTimestamp is the timestamp of when the phase change occurred
	// +kubebuilder:validation:Format:=date-time
	// +nullable
	Timestamp metav1.Time `json:"timestamp,omitempty"`
}

type VirtualMachineLaunchTimeDuration struct {
	// The waiting time for dependent resources. pending -> starting.
	// +nullable
	WaitingForDependencies *metav1.Duration `json:"waitingForDependencies,omitempty"`
	// The waiting time for the virtual machine to start. starting -> running.
	// +nullable
	VirtualMachineStarting *metav1.Duration `json:"virtualMachineStarting,omitempty"`
	// The waiting time for the guestOsAgent to start. running -> running with guestOSAgent.
	// +nullable
	GuestOSAgentStarting *metav1.Duration `json:"guestOSAgentStarting,omitempty"`
}

type VirtualMachineMigrationState struct {
	// Migration start time.
	StartTimestamp *metav1.Time `json:"startTimestamp,omitempty"`
	// Migration end time.
	EndTimestamp *metav1.Time           `json:"endTimestamp,omitempty"`
	Target       VirtualMachineLocation `json:"target,omitempty"`
	Source       VirtualMachineLocation `json:"source,omitempty"`
	Result       MigrationResult        `json:"result,omitempty"`
}

// MigrationResult defines a migration result
// +kubebuilder:validation:Enum:={"Succeeded","Failed",""}
type MigrationResult string

const (
	MigrationResultSucceeded MigrationResult = "Succeeded"
	MigrationResultFailed    MigrationResult = "Failed"
)

type VirtualMachineLocation struct {
	// The name of the node on which the VM is currently migrating.
	Node string `json:"node,omitempty"`
	// The name of the pod where the VM is currently being migrated.
	Pod string `json:"pod,omitempty"`
}

type VirtualMachinePod struct {
	// Name of virtual machine pod.
	Name string `json:"name"`
	// Current working pod.
	Active bool `json:"active"`
}

// ResourcesStatus defines resource usage statistics.
type ResourcesStatus struct {
	CPU    CPUStatus    `json:"cpu,omitempty"`
	Memory MemoryStatus `json:"memory,omitempty"`
}

// CPUStatus defines statistics about the CPU resource usage.
type CPUStatus struct {
	// Current number of cores inside the VM.
	Cores int `json:"cores"`
	// Current CoreFraction.
	CoreFraction string `json:"coreFraction,omitempty"`
	// Requested cores.
	RequestedCores resource.Quantity `json:"requestedCores,omitempty"`
	// runtime overhead.
	RuntimeOverhead resource.Quantity `json:"runtimeOverhead,omitempty"`
	// Topology with Cores count and Sockets count.
	Topology Topology `json:"topology,omitempty"`
}

// Topology defines count of used CPU cores and sockets.
type Topology struct {
	// Current number of cores inside the VM.
	CoresPerSocket int `json:"coresPerSocket"`
	// Current number of cores inside the VM.
	Sockets int `json:"sockets"`
}

// MemoryStatus defines statistics about the Memory resource usage.
type MemoryStatus struct {
	// Current memory size.
	Size resource.Quantity `json:"size"`
	// Memory runtime overhead.
	RuntimeOverhead resource.Quantity `json:"runtimeOverhead,omitempty"`
}

// Versions defines statistics about the hypervisor versions.
type Versions struct {
	// Qemu is the version of the qemu hypervisor.
	Qemu string `json:"qemu,omitempty"`
	// Libvirt is the version of the libvirt.
	Libvirt string `json:"libvirt,omitempty"`
}

type NetworksStatus struct {
	Type                         string `json:"type"`
	Name                         string `json:"name,omitempty"`
	MAC                          string `json:"macAddress,omitempty"`
	VirtualMachineMACAddressName string `json:"virtualMachineMACAddressName,omitempty"`
}

// MachinePhase defines current phase of the virtual machine:
// * `Pending` - The process of starting the VM is in progress.
// * `Running` - VM is running.
// * `Degraded` - An error occurred during the startup process or while the VM is running.
// * `Terminating` - The VM is currently in the process of shutting down.
// * `Stopped` - The VM is stopped.
// +kubebuilder:validation:Enum:={Pending,Running,Terminating,Stopped,Stopping,Starting,Migrating,Pause,Degraded}
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
	MachineDegraded    MachinePhase = "Degraded"
)

// VirtualMachineList contains a list of VirtualMachine
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type VirtualMachineList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	// Items provides a list of VirtualMachines
	Items []VirtualMachine `json:"items"`
}

// ProvisioningType parameter defines the type of provisioning script:
//
// Parameters supported for using the provisioning script:
// * UserData - use the cloud-init in the .spec.provisioning.UserData section.
// * UserDataRef - use a cloud-init script that resides in a different resource.
// * SysprepRef - Use a Windows Automation script that resides in a different resource.
// More information: https://cloudinit.readthedocs.io/en/latest/reference/examples.html
type ProvisioningType string

const (
	ProvisioningTypeUserData    ProvisioningType = "UserData"
	ProvisioningTypeUserDataRef ProvisioningType = "UserDataRef"
	ProvisioningTypeSysprepRef  ProvisioningType = "SysprepRef"
)

const (
	SecretTypeCloudInit corev1.SecretType = "provisioning.virtualization.deckhouse.io/cloud-init"
	SecretTypeSysprep   corev1.SecretType = "provisioning.virtualization.deckhouse.io/sysprep"
)

// USBDeviceSpecRef references a USB device by name.
type USBDeviceSpecRef struct {
	// The name of USBDevice resource in the same namespace.
	Name string `json:"name"`
}

// USBDeviceStatusRef represents the status of a USB device attached to the virtual machine.
type USBDeviceStatusRef struct {
	// The name of USBDevice resource.
	Name string `json:"name"`
	// The USB device is attached to the virtual machine.
	Attached bool `json:"attached"`
	// USB device is ready to use.
	Ready bool `json:"ready"`
	// USB address inside the virtual machine.
	Address *USBAddress `json:"address,omitempty"`
	// USB device is attached via hot plug connection.
	Hotplugged bool `json:"hotplugged,omitempty"`
	// Conditions for this USB device.
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// USBAddress represents the USB bus address inside the virtual machine.
type USBAddress struct {
	// USB bus number (always 0 for the main USB controller).
	Bus int `json:"bus"`
	// USB port number on the selected bus.
	Port int `json:"port"`
}
