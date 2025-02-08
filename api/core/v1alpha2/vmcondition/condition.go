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
	TypeClassReady                          Type = "VirtualMachineClassReady"
	TypeBlockDevicesReady                   Type = "BlockDevicesReady"
	TypeRunning                             Type = "Running"
	TypeMigrating                           Type = "Migrating"
	TypeMigratable                          Type = "Migratable"
	TypePodStarted                          Type = "PodStarted"
	TypeProvisioningReady                   Type = "ProvisioningReady"
	TypeAgentReady                          Type = "AgentReady"
	TypeAgentVersionNotSupported            Type = "AgentVersionNotSupported"
	TypeConfigurationApplied                Type = "ConfigurationApplied"
	TypeAwaitingRestartToApplyConfiguration Type = "AwaitingRestartToApplyConfiguration"
	TypeFilesystemFrozen                    Type = "FilesystemFrozen"
	TypeSizingPolicyMatched                 Type = "SizingPolicyMatched"
	TypeSnapshotting                        Type = "Snapshotting"
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

	ReasonPodStarted    Reason = "PodStarted"
	ReasonPodNotFound   Reason = "PodNotFound"
	ReasonPodNotStarted Reason = "PodNotStarted"

	ReasonMigratable    Reason = "VirtualMachineMigratable"
	ReasonNotMigratable Reason = "VirtualMachineNotMigratable"

	ReasonVmIsMigrating                  Reason = "VirtualMachineMigrating"
	ReasonVmIsNotMigrating               Reason = "VirtualMachineNotMigrating"
	ReasonLastMigrationFinishedWithError Reason = "LastMigrationFinishedWithError"
	ReasonVmIsNotRunning                 Reason = "VirtualMachineNotRunning"
	ReasonVmIsRunning                    Reason = "VirtualMachineRunning"
	ReasonInternalVirtualMachineError    Reason = "InternalVirtualMachineError"

	ReasonFilesystemFrozen    Reason = "Frozen"
	ReasonFilesystemNotFrozen Reason = "NotFrozen"
	ReasonFilesystemNotReady  Reason = "NotReady"

	WaitingForTheSnapshotToStart Reason = "WaitingForTheSnapshotToStart"
	ReasonSnapshottingInProgress Reason = "SnapshottingInProgress"

	ReasonSizingPolicyMatched            Reason = "SizingPolicyMatched"
	ReasonSizingPolicyNotMatched         Reason = "SizingPolicyNotMatched"
	ReasonVirtualMachineClassTerminating Reason = "VirtualMachineClassTerminating"
	ReasonVirtualMachineClassNotExists   Reason = "VirtalMachineClassNotExists"

	// ReasonBlockDeviceLimitExceeded indicates that the limit for attaching block devices has been exceeded
	ReasonBlockDeviceLimitExceeded Reason = "BlockDeviceLimitExceeded"

	ReasonPodTerminatingReason      Reason = "PodTerminating"
	ReasonPodNotExistsReason        Reason = "PodNotExists"
	ReasonPodConditionMissingReason Reason = "PodConditionMissing"
	ReasonGuestNotRunningReason     Reason = "GuestNotRunning"
)
