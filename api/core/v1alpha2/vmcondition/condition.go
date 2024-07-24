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
	TypeCPUModelReady                       Type = "CPUModelReady"
	TypeIPAddressClaimReady                 Type = "VirtualMachineIPAddressClaimReady"
	TypeBlockDevicesReady                   Type = "BlockDevicesReady"
	TypeRunning                             Type = "Running"
	TypeMigrating                           Type = "Migrating"
	TypePodStarted                          Type = "PodStarted"
	TypeProvisioningReady                   Type = "ProvisioningReady"
	TypeAgentReady                          Type = "AgentReady"
	TypeAgentVersionNotSupported            Type = "AgentVersionNotSupported"
	TypeConfigurationApplied                Type = "ConfigurationApplied"
	TypeAwaitingRestartToApplyConfiguration Type = "AwaitingRestartToApplyConfiguration"
)

type Reason string

func (r Reason) String() string {
	return string(r)
}

const (
	ReasonAgentNotReady Reason = "AgentNotReady"

	ReasonCPUModelReady    Reason = "CPUModelReady"
	ReasonCPUModelNotReady Reason = "CPUModelNotReady"

	ReasonIPAddressClaimReady        Reason = "VirtualMachineIPAddressClaimReady"
	ReasonIPAddressClaimNotReady     Reason = "VirtualMachineIPAddressClaimNotReady"
	ReasonIPAddressClaimNotAssigned  Reason = "VirtualMachineIPAddressClaimNotAssigned"
	ReasonIPAddressClaimNotAvailable Reason = "VirtualMachineIPAddressClaimNotAvailable"

	ReasonBlockDevicesReady    Reason = "BlockDevicesReady"
	ReasonBlockDevicesNotReady Reason = "BlockDevicesNotReady"

	ReasonProvisioningReady    Reason = "ProvisioningReady"
	ReasonProvisioningNotReady Reason = "ProvisioningNotReady"

	ReasonConfigurationApplied           Reason = "ConfigurationApplied"
	ReasonConfigurationNotApplied        Reason = "ConfigurationNotApplied"
	ReasonRestartAwaitingChangesExist    Reason = "RestartAwaitingChangesExist"
	ReasonRestartAwaitingChangesNotExist Reason = "RestartAwaitingChangesNotExist"
	ReasonRestartNoNeed                  Reason = "NoNeedRestart"

	ReasonPodNodFound      Reason = "PodNotFound"
	ReasonPodStarted       Reason = "PodStarted"
	ReasonVmIsMigrating    Reason = "VirtualMachineMigrating"
	ReasonVmIsNotMigrating Reason = "VirtualMachineNotMigrating"
	ReasonVmIsNotRunning   Reason = "VirtualMachineNotRunning"
	ReasonVmIsRunning      Reason = "VirtualMachineRunning"
)
