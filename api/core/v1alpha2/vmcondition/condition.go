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

package vmcondition

type Type string

func (t Type) String() string {
	return string(t)
}

const (
	TypeIPAddressReady                      Type = "VirtualMachineIPAddressReady"
	TypeMACAddressReady                     Type = "VirtualMachineMACAddressReady"
	TypeClassReady                          Type = "VirtualMachineClassReady"
	TypeBlockDevicesReady                   Type = "BlockDevicesReady"
	TypeRunning                             Type = "Running"
	TypeMigrating                           Type = "Migrating"
	TypeMigratable                          Type = "Migratable"
	TypeProvisioningReady                   Type = "ProvisioningReady"
	TypeAgentReady                          Type = "AgentReady"
	TypeAgentVersionNotSupported            Type = "AgentVersionNotSupported"
	TypeConfigurationApplied                Type = "ConfigurationApplied"
	TypeAwaitingRestartToApplyConfiguration Type = "AwaitingRestartToApplyConfiguration"
	// TypeFilesystemFrozen indicates whether the filesystem is currently frozen, a necessary condition for creating a snapshot.
	TypeFilesystemFrozen    Type = "FilesystemFrozen"
	TypeSizingPolicyMatched Type = "SizingPolicyMatched"
	TypeSnapshotting        Type = "Snapshotting"
	// TypeFirmwareUpToDate indicates whether the firmware on the virtual machine is up to date.
	// This condition is used to determine if a migration or update is required due to changes in the firmware version.
	TypeFirmwareUpToDate Type = "FirmwareUpToDate"
	// TypeNeedsEvict indicates that the VirtualMachine should be evicting from node.
	TypeNeedsEvict Type = "NeedsEvict"
	// TypeNetworkReady indicates the state of additional network interfaces inside the virtual machine pod
	TypeNetworkReady Type = "NetworkReady"

	// TypeMaintenance indicates that the VirtualMachine is in maintenance mode.
	// During this condition, the VM remains stopped and no changes are allowed.
	TypeMaintenance Type = "Maintenance"
)

type Reason string

func (r Reason) String() string {
	return string(r)
}

const (
	ReasonAgentReady    Reason = "AgentReady"
	ReasonAgentNotReady Reason = "AgentNotReady"

	ReasonAgentSupported    Reason = "AgentVersionSupported"
	ReasonAgentNotSupported Reason = "AgentVersionNotSupported"

	ReasonClassReady    Reason = "VirtualMachineClassReady"
	ReasonClassNotReady Reason = "VirtualMachineClassNotReady"

	ReasonIPAddressReady        Reason = "VirtualMachineIPAddressReady"
	ReasonIPAddressNotReady     Reason = "VirtualMachineIPAddressNotReady"
	ReasonIPAddressNotAssigned  Reason = "VirtualMachineIPAddressNotAssigned"
	ReasonIPAddressNotAvailable Reason = "VirtualMachineIPAddressNotAvailable"

	ReasonMACAddressReady        Reason = "VirtualMachineMACAddressReady"
	ReasonMACAddressNotReady     Reason = "VirtualMachineMACAddressNotReady"
	ReasonMACAddressNotAssigned  Reason = "VirtualMachineMACAddressNotAssigned"
	ReasonMACAddressNotAvailable Reason = "VirtualMachineMACAddressNotAvailable"

	ReasonBlockDevicesReady           Reason = "BlockDevicesReady"
	ReasonWaitingForProvisioningToPVC Reason = "WaitingForTheProvisioningToPersistentVolumeClaim"
	ReasonBlockDevicesNotReady        Reason = "BlockDevicesNotReady"

	ReasonProvisioningReady    Reason = "ProvisioningReady"
	ReasonProvisioningNotReady Reason = "ProvisioningNotReady"

	ReasonConfigurationApplied    Reason = "ConfigurationApplied"
	ReasonConfigurationNotApplied Reason = "ConfigurationNotApplied"

	ReasonRestartAwaitingChangesExist        Reason = "RestartAwaitingChangesExist"
	ReasonRestartAwaitingVMClassChangesExist Reason = "RestartAwaitingVMClassChangesExist"
	ReasonRestartNoNeed                      Reason = "NoNeedRestart"

	ReasonMigratable    Reason = "VirtualMachineMigratable"
	ReasonNotMigratable Reason = "VirtualMachineNotMigratable"

	ReasonVmIsMigrating                  Reason = "VirtualMachineMigrating"
	ReasonVmIsNotMigrating               Reason = "VirtualMachineNotMigrating"
	ReasonLastMigrationFinishedWithError Reason = "LastMigrationFinishedWithError"
	ReasonVmIsNotRunning                 Reason = "VirtualMachineNotRunning"
	ReasonVmIsRunning                    Reason = "VirtualMachineRunning"
	ReasonInternalVirtualMachineError    Reason = "InternalVirtualMachineError"
	ReasonPodNotStarted                  Reason = "PodNotStarted"
	ReasonMigrationIsPending             Reason = "MigrationIsPending"

	// 	ReasonFilesystemFrozen indicates that virtual machine's filesystem has been successfully frozen.
	ReasonFilesystemFrozen Reason = "Frozen"

	WaitingForTheSnapshotToStart Reason = "WaitingForTheSnapshotToStart"
	ReasonSnapshottingInProgress Reason = "SnapshottingInProgress"

	ReasonSizingPolicyNotMatched         Reason = "SizingPolicyNotMatched"
	ReasonVirtualMachineClassTerminating Reason = "VirtualMachineClassTerminating"
	ReasonVirtualMachineClassNotExists   Reason = "VirtualMachineClassNotExists"

	// ReasonBlockDeviceLimitExceeded indicates that the limit for attaching block devices has been exceeded
	ReasonBlockDeviceLimitExceeded Reason = "BlockDeviceLimitExceeded"

	ReasonPodTerminating      Reason = "PodTerminating"
	ReasonPodNotExists        Reason = "PodNotExists"
	ReasonPodConditionMissing Reason = "PodConditionMissing"
	ReasonGuestNotRunning     Reason = "GuestNotRunning"

	// ReasonFirmwareUpToDate indicates that the firmware up to date.
	ReasonFirmwareUpToDate Reason = "FirmwareUpToDate"
	// ReasonFirmwareOutOfDate indicates that the firmware out of date.
	ReasonFirmwareOutOfDate Reason = "FirmwareOutOfDate"

	// ReasonNeedsEvict indicates that the VirtualMachine should be evicting from node.
	ReasonNeedsEvict Reason = "NeedsEvict"

	// ReasonNetworkReady indicates that the additional network interfaces in the virtual machine pod are ready.
	ReasonNetworkReady Reason = "NetworkReady"
	// ReasonNetworkNotReady indicates that the additional network interfaces in the virtual machine pod are not ready.
	ReasonNetworkNotReady Reason = "NetworkNotReady"
	// ReasonSDNModuleDisable indicates that the SDN module is disabled, which may prevent network interfaces from becoming ready.
	ReasonSDNModuleDisable Reason = "SDNModuleDisable"

	// ReasonMaintenanceRestore indicates that the VirtualMachine is in maintenance mode for restore operation.
	ReasonMaintenanceRestore Reason = "RestoreInProgress"
)
