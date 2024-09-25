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

package vmchange

import (
	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

const (
	DefaultCPUCoreFraction               = "100%"
	DefaultCPUModelName                  = "generic-v1"
	DefaultDisruptionsApprovalMode       = v1alpha2.Manual
	DefaultOSType                        = v1alpha2.GenericOs
	DefaultBootloader                    = v1alpha2.BIOS
	DefaultEnableParavirtualization      = true
	DefaultTerminationGracePeriodSeconds = int64(60)
)

func compareVirtualMachineClass(current, desired *v1alpha2.VirtualMachineSpec) []FieldChange {
	return compareStrings(
		"virtualMachineClassName",
		current.VirtualMachineClassName,
		desired.VirtualMachineClassName,
		"",
		ActionRestart)
}

// compareRunPolicy
func compareRunPolicy(current, desired *v1alpha2.VirtualMachineSpec) []FieldChange {
	return compareStrings(
		"runPolicy",
		string(current.RunPolicy),
		string(desired.RunPolicy),
		"",
		ActionApplyImmediate,
	)
}

func compareVirtualMachineIPAddress(current, desired *v1alpha2.VirtualMachineSpec) []FieldChange {
	return compareStrings(
		"virtualMachineIPAddress",
		current.VirtualMachineIPAddress,
		desired.VirtualMachineIPAddress,
		"",
		ActionRestart,
	)
}

// compareDisruptions returns changes in disruptions section.
func compareDisruptions(current, desired *v1alpha2.VirtualMachineSpec) []FieldChange {
	changes := compareEmpty(
		"disruptions",
		NewPtrValue(current.Disruptions, current.Disruptions == nil),
		NewPtrValue(desired.Disruptions, desired.Disruptions == nil),
		ActionNone,
	)
	if len(changes) > 0 {
		return changes
	}

	// Disruptions are not nils, compare approvalMode fields using "Manual" as default.
	return compareStrings(
		"disruptions.restartApprovalMode",
		string(current.Disruptions.RestartApprovalMode),
		string(desired.Disruptions.RestartApprovalMode),
		string(DefaultDisruptionsApprovalMode),
		ActionNone,
	)
}

func compareTerminationGracePeriodSeconds(current, desired *v1alpha2.VirtualMachineSpec) []FieldChange {
	return comparePtrInt64(
		"terminationGracePeriodSeconds",
		current.TerminationGracePeriodSeconds,
		desired.TerminationGracePeriodSeconds,
		DefaultTerminationGracePeriodSeconds,
		ActionRestart,
	)
}

func compareEnableParavirtualization(current, desired *v1alpha2.VirtualMachineSpec) []FieldChange {
	return compareBools(
		"enableParavirtualization",
		current.EnableParavirtualization,
		desired.EnableParavirtualization,
		DefaultEnableParavirtualization,
		ActionRestart,
	)
}

func compareOSType(current, desired *v1alpha2.VirtualMachineSpec) []FieldChange {
	return compareStrings(
		"osType",
		string(current.OsType),
		string(desired.OsType),
		string(DefaultOSType),
		ActionRestart,
	)
}

func compareBootloader(current, desired *v1alpha2.VirtualMachineSpec) []FieldChange {
	return compareStrings(
		"bootloader",
		string(current.Bootloader),
		string(desired.Bootloader),
		string(DefaultBootloader),
		ActionRestart,
	)
}

// compareCPU returns changes in the cpu section.
func compareCPU(current, desired *v1alpha2.VirtualMachineSpec) []FieldChange {
	coresChanges := compareInts("cpu.cores", current.CPU.Cores, desired.CPU.Cores, 0, ActionRestart)
	fractionChanges := compareStrings("cpu.coreFraction", current.CPU.CoreFraction, desired.CPU.CoreFraction, DefaultCPUCoreFraction, ActionRestart)

	// Yield full replace if both fields changed.
	if HasChanges(coresChanges) && HasChanges(fractionChanges) {
		return []FieldChange{
			{
				Operation:      ChangeReplace,
				Path:           "cpu",
				CurrentValue:   current.CPU,
				DesiredValue:   desired.CPU,
				ActionRequired: ActionRestart,
			},
		}
	}

	if HasChanges(coresChanges) {
		return coresChanges
	}

	if HasChanges(fractionChanges) {
		return fractionChanges
	}

	return nil
}

// compareMemory returns changes in the memory section.
func compareMemory(current, desired *v1alpha2.VirtualMachineSpec) []FieldChange {
	return compareQuantity("memory.size", current.Memory.Size, desired.Memory.Size, resource.Quantity{}, ActionRestart)
}

func compareProvisioning(current, desired *v1alpha2.VirtualMachineSpec) []FieldChange {
	changes := compareEmpty(
		"provisioning",
		NewPtrValue(current.Provisioning, current.Provisioning == nil),
		NewPtrValue(desired.Provisioning, desired.Provisioning == nil),
		ActionRestart,
	)
	if len(changes) > 0 {
		return changes
	}

	// Consider full replace if type is changed.
	if current.Provisioning.Type != desired.Provisioning.Type {
		return []FieldChange{
			{
				Operation:      ChangeReplace,
				Path:           "provisioning",
				CurrentValue:   current.Provisioning.Type,
				DesiredValue:   desired.Provisioning.Type,
				ActionRequired: ActionRestart,
			},
		}
	}

	if current.Provisioning.Type == v1alpha2.ProvisioningTypeSysprepRef {
		currentSecret := current.Provisioning.SysprepRef
		desiredSecret := desired.Provisioning.SysprepRef
		changes = compareEmpty(
			"provisioning.sysprepRef",
			NewPtrValue(currentSecret, currentSecret == nil),
			NewPtrValue(desiredSecret, desiredSecret == nil),
			ActionRestart,
		)
		if len(changes) > 0 {
			return changes
		}

		// SysprepRef is not nil, compare kinds.
		changes = compareStrings(
			"provisioning.sysprepRef.kind",
			string(currentSecret.Kind),
			string(desiredSecret.Kind),
			"",
			ActionRestart,
		)
		if len(changes) > 0 {
			return changes
		}

		// SysprepRef is not nil, compare names.
		return compareStrings(
			"provisioning.sysprepRef.name",
			currentSecret.Name,
			desiredSecret.Name,
			"",
			ActionRestart,
		)
	}

	if current.Provisioning.Type == v1alpha2.ProvisioningTypeUserData {
		return compareStrings(
			"provisioning.userData",
			current.Provisioning.UserData,
			desired.Provisioning.UserData,
			"",
			ActionRestart,
		)
	}

	if current.Provisioning.Type == v1alpha2.ProvisioningTypeUserDataRef {
		currentSecret := current.Provisioning.UserDataRef
		desiredSecret := desired.Provisioning.UserDataRef
		changes = compareEmpty(
			"provisioning.userDataRef",
			NewPtrValue(currentSecret, currentSecret == nil),
			NewPtrValue(desiredSecret, desiredSecret == nil),
			ActionRestart,
		)
		if len(changes) > 0 {
			return changes
		}

		// UserDataSecretRef is not nil, compare kinds.
		changes = compareStrings(
			"provisioning.userDataRef.kind",
			string(currentSecret.Kind),
			string(desiredSecret.Kind),
			"",
			ActionRestart,
		)
		if len(changes) > 0 {
			return changes
		}

		// UserDataSecretRef is not nil, compare names.
		return compareStrings(
			"provisioning.userDataRef.name",
			currentSecret.Name,
			desiredSecret.Name,
			"",
			ActionRestart,
		)
	}

	return nil
}
