package vmchange

import (
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

const (
	DefaultCPUCoreFraction               = "100%"
	DefaultDisruptionsApprovalMode       = v1alpha2.Manual
	DefaultOSType                        = v1alpha2.GenericOs
	DefaultBootloader                    = v1alpha2.BIOS
	DefaultEnableParavirtualization      = true
	DefaultTerminationGracePeriodSeconds = int64(60)
)

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

func compareVirtualMachineIPAddressClaimName(current, desired *v1alpha2.VirtualMachineSpec) []FieldChange {
	return compareStrings(
		"virtualMachineIPAddressClaimName",
		current.VirtualMachineIPAddressClaimName,
		desired.VirtualMachineIPAddressClaimName,
		"",
		ActionNone,
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
		"disruptions.approvalMode",
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

	modelChanges := compareStrings("cpu.model", current.CPU.Model, desired.CPU.Model, "", ActionRestart)
	if HasChanges(modelChanges) {
		return modelChanges
	}

	return nil
}

// compareMemory returns changes in the memory section.
func compareMemory(current, desired *v1alpha2.VirtualMachineSpec) []FieldChange {
	return compareStrings("memory.size", current.Memory.Size, desired.Memory.Size, "", ActionRestart)
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

	if current.Provisioning.Type == v1alpha2.ProvisioningTypeUserData {
		return compareStrings(
			"provisioning.userData",
			current.Provisioning.UserData,
			desired.Provisioning.UserData,
			"",
			ActionRestart,
		)
	}

	if current.Provisioning.Type == v1alpha2.ProvisioningTypeUserDataSecret {
		currentSecret := current.Provisioning.UserDataSecretRef
		desiredSecret := desired.Provisioning.UserDataSecretRef
		changes = compareEmpty(
			"provisioning.userDataSecretRef",
			NewPtrValue(currentSecret, currentSecret == nil),
			NewPtrValue(desiredSecret, desiredSecret == nil),
			ActionRestart,
		)
		if len(changes) > 0 {
			return changes
		}

		// UserDataSecretRef is not nil, compare names.
		return compareStrings(
			"provisioning.userDataSecretRef.name",
			currentSecret.Name,
			desiredSecret.Name,
			"",
			ActionRestart,
		)
	}

	return nil
}
