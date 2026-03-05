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
	// TypeEvictionRequired indicates that the VirtualMachine should be evicting from node.
	TypeEvictionRequired Type = "EvictionRequired"
	// TypeNetworkReady indicates the state of additional network interfaces inside the virtual machine pod
	TypeNetworkReady Type = "NetworkReady"

	// TypeMaintenance indicates that the VirtualMachine is in maintenance mode.
	// During this condition, the VM remains stopped and no changes are allowed.
	TypeMaintenance Type = "Maintenance"
)

type AgentReadyReason string

func (r AgentReadyReason) String() string {
	return string(r)
}

const (
	ReasonAgentReady    AgentReadyReason = "AgentReady"
	ReasonAgentNotReady AgentReadyReason = "AgentNotReady"
)

type AgentVersionNotSupportedReason string

func (r AgentVersionNotSupportedReason) String() string {
	return string(r)
}

const (
	ReasonAgentSupported    AgentVersionNotSupportedReason = "AgentVersionSupported"
	ReasonAgentNotSupported AgentVersionNotSupportedReason = "AgentVersionNotSupported"
)

type ClassReadyReason string

func (r ClassReadyReason) String() string {
	return string(r)
}

const (
	ReasonClassReady    ClassReadyReason = "VirtualMachineClassReady"
	ReasonClassNotReady ClassReadyReason = "VirtualMachineClassNotReady"
)

type IpAddressReadyReason string

func (r IpAddressReadyReason) String() string {
	return string(r)
}

const (
	ReasonIPAddressReady        IpAddressReadyReason = "VirtualMachineIPAddressReady"
	ReasonIPAddressNotReady     IpAddressReadyReason = "VirtualMachineIPAddressNotReady"
	ReasonIPAddressNotAssigned  IpAddressReadyReason = "VirtualMachineIPAddressNotAssigned"
	ReasonIPAddressNotAvailable IpAddressReadyReason = "VirtualMachineIPAddressNotAvailable"
)

type MacAddressReadyReason string

func (r MacAddressReadyReason) String() string {
	return string(r)
}

const (
	ReasonMACAddressReady        MacAddressReadyReason = "VirtualMachineMACAddressReady"
	ReasonMACAddressNotReady     MacAddressReadyReason = "VirtualMachineMACAddressNotReady"
	ReasonMACAddressNotAvailable MacAddressReadyReason = "VirtualMachineMACAddressNotAvailable"
)

type BlockDevicesReadyReason string

func (r BlockDevicesReadyReason) String() string {
	return string(r)
}

const (
	ReasonBlockDevicesReady                                   BlockDevicesReadyReason = "BlockDevicesReady"
	ReasonWaitingForWaitForFirstConsumerBlockDevicesToBeReady BlockDevicesReadyReason = "WaitingForWaitForFirstConsumerBlockDevicesToBeReady"
	ReasonBlockDevicesNotReady                                BlockDevicesReadyReason = "BlockDevicesNotReady"
	// ReasonBlockDeviceLimitExceeded indicates that the limit for attaching block devices has been exceeded
	ReasonBlockDeviceLimitExceeded BlockDevicesReadyReason = "BlockDeviceLimitExceeded"
)

type ProvisioningReadyReason string

func (r ProvisioningReadyReason) String() string {
	return string(r)
}

const (
	ReasonProvisioningReady    ProvisioningReadyReason = "ProvisioningReady"
	ReasonProvisioningNotReady ProvisioningReadyReason = "ProvisioningNotReady"
)

type ConfigurationAppliedReason string

func (r ConfigurationAppliedReason) String() string {
	return string(r)
}

const (
	ReasonConfigurationApplied    ConfigurationAppliedReason = "ConfigurationApplied"
	ReasonConfigurationNotApplied ConfigurationAppliedReason = "ConfigurationNotApplied"
)

type AwaitingRestartToApplyConfigurationReason string

func (r AwaitingRestartToApplyConfigurationReason) String() string {
	return string(r)
}

const (
	ReasonUnexpectedState       AwaitingRestartToApplyConfigurationReason = "UnexpectedState"
	ReasonChangesPendingRestart AwaitingRestartToApplyConfigurationReason = "ChangesPendingRestart"
	ReasonNoRestartRequired     AwaitingRestartToApplyConfigurationReason = "NoRestartRequired"
)

type RunningReason string

func (r RunningReason) String() string {
	return string(r)
}

const (
	ReasonVirtualMachineNotRunning    RunningReason = "NotRunning"
	ReasonVirtualMachineRunning       RunningReason = "Running"
	ReasonInternalVirtualMachineError RunningReason = "InternalVirtualMachineError"
	ReasonPodNotStarted               RunningReason = "PodNotStarted"
	ReasonPodTerminating              RunningReason = "PodTerminating"
	ReasonPodNotFound                 RunningReason = "PodNotFound"
	ReasonPodConditionMissing         RunningReason = "PodConditionMissing"
	ReasonGuestNotRunning             RunningReason = "GuestNotRunning"
)

type FilesystemFrozenReason string

func (r FilesystemFrozenReason) String() string {
	return string(r)
}

const (
	// ReasonFilesystemFrozen indicates that virtual machine's filesystem has been successfully frozen.
	ReasonFilesystemFrozen FilesystemFrozenReason = "Frozen"
)

type SnapshottingReason string

func (r SnapshottingReason) String() string {
	return string(r)
}

const (
	WaitingForTheSnapshotToStart SnapshottingReason = "WaitingForTheSnapshotToStart"
	ReasonSnapshottingInProgress SnapshottingReason = "SnapshottingInProgress"
)

type SizingPolicyMatchedReason string

func (r SizingPolicyMatchedReason) String() string {
	return string(r)
}

const (
	ReasonSizingPolicyNotMatched         SizingPolicyMatchedReason = "SizingPolicyNotMatched"
	ReasonVirtualMachineClassTerminating SizingPolicyMatchedReason = "VirtualMachineClassTerminating"
	ReasonVirtualMachineClassNotFound    SizingPolicyMatchedReason = "VirtualMachineClassNotFound"
)

type FirmwareUpToDateReason string

func (r FirmwareUpToDateReason) String() string {
	return string(r)
}

const (
	ReasonFirmwareUpToDate  FirmwareUpToDateReason = "FirmwareUpToDate"
	ReasonFirmwareOutOfDate FirmwareUpToDateReason = "FirmwareOutOfDate"
)

type EvictionRequiredReason string

func (r EvictionRequiredReason) String() string {
	return string(r)
}

const (
	// ReasonEvictionRequired indicates that the VirtualMachine should be evicting from node.
	ReasonEvictionRequired EvictionRequiredReason = "EvictionRequired"
)

type NetworkReadyReason string

func (r NetworkReadyReason) String() string {
	return string(r)
}

const (
	// ReasonNetworkReady indicates that the additional network interfaces in the virtual machine pod are ready.
	ReasonNetworkReady NetworkReadyReason = "NetworkReady"
	// ReasonNetworkNotReady indicates that the additional network interfaces in the virtual machine pod are not ready.
	ReasonNetworkNotReady NetworkReadyReason = "NetworkNotReady"
	// ReasonSDNModuleDisabled indicates that the SDN module is disabled, which may prevent network interfaces from becoming ready.
	ReasonSDNModuleDisabled NetworkReadyReason = "SDNModuleDisabled"
)

type MigratableReason string

func (r MigratableReason) String() string {
	return string(r)
}

const (
	ReasonMigratable               MigratableReason = "VirtualMachineMigratable"
	ReasonNonMigratable            MigratableReason = "VirtualMachineNonMigratable"
	ReasonDisksNotMigratable       MigratableReason = "VirtualMachineDisksNotMigratable"
	ReasonDisksShouldBeMigrating   MigratableReason = "VirtualMachineDisksShouldBeMigrating"
	ReasonHostDevicesNotMigratable MigratableReason = "VirtualMachineHostDevicesNotMigratable"
	ReasonUSBShouldBeMigrating     MigratableReason = "VirtualMachineUSBShouldBeMigrating"
)

type MigratingReason string

func (r MigratingReason) String() string {
	return string(r)
}

const (
	ReasonMigratingPending    MigratingReason = "Pending"
	ReasonReadyToMigrate      MigratingReason = "ReadyToMigrate"
	ReasonMigratingInProgress MigratingReason = "InProgress"
)

type MaintenanceReason string

func (r MaintenanceReason) String() string {
	return string(r)
}

const (
	ReasonMaintenanceRestore MaintenanceReason = "RestoreInProgress"
)
