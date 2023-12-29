package vmchange

import (
	"reflect"

	"github.com/deckhouse/virtualization-controller/api/v2alpha1"
)

func compareTopologySpreadConstraints(current, desired *v2alpha1.VirtualMachineSpec) []FieldChange {
	currentValue := NewValue(current.TopologySpreadConstraints, len(current.TopologySpreadConstraints) == 0, false)
	desiredValue := NewValue(desired.TopologySpreadConstraints, len(desired.TopologySpreadConstraints) == 0, false)

	return compareValues(
		"topologySpreadConstraints",
		currentValue,
		desiredValue,
		reflect.DeepEqual(current.TopologySpreadConstraints, desired.TopologySpreadConstraints),
		ActionRestart,
	)
}

func compareAffinity(current, desired *v2alpha1.VirtualMachineSpec) []FieldChange {
	currentValue := NewValue(current.Affinity, current.Affinity == nil, false)
	desiredValue := NewValue(desired.Affinity, desired.Affinity == nil, false)

	return compareValues(
		"affinity",
		currentValue,
		desiredValue,
		reflect.DeepEqual(current.Affinity, desired.Affinity),
		ActionRestart,
	)
}

func compareNodeSelector(current, desired *v2alpha1.VirtualMachineSpec) []FieldChange {
	currentValue := NewValue(current.NodeSelector, len(current.NodeSelector) == 0, false)
	desiredValue := NewValue(desired.NodeSelector, len(desired.NodeSelector) == 0, false)

	return compareValues(
		"nodeSelector",
		currentValue,
		desiredValue,
		reflect.DeepEqual(current.NodeSelector, desired.NodeSelector),
		ActionRestart,
	)
}

func comparePriorityClassName(current, desired *v2alpha1.VirtualMachineSpec) []FieldChange {
	return compareStrings(
		"priorityClassName",
		current.PriorityClassName,
		desired.PriorityClassName,
		"",
		ActionRestart,
	)
}

func compareTolerations(current, desired *v2alpha1.VirtualMachineSpec) []FieldChange {
	currentValue := NewValue(current.Tolerations, len(current.Tolerations) == 0, false)
	desiredValue := NewValue(desired.Tolerations, len(desired.Tolerations) == 0, false)

	return compareValues(
		"tolerations",
		currentValue,
		desiredValue,
		reflect.DeepEqual(current.Tolerations, desired.Tolerations),
		ActionRestart,
	)
}
