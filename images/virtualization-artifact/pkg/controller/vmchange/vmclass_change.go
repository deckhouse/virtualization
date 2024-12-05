package vmchange

import (
	"fmt"
	"reflect"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

func makePathWithClass(path string) string {
	return fmt.Sprintf("VirtualMachineClass:%s", path)
}

func compareVmClassNodeSelector(current, desired *v1alpha2.VirtualMachineClassSpec) []FieldChange {
	isEmpty := func(nodeSelector v1alpha2.NodeSelector) bool {
		return len(nodeSelector.MatchExpressions) == 0 && len(nodeSelector.MatchLabels) == 0
	}

	currentValue := NewValue(current.NodeSelector, isEmpty(current.NodeSelector), false)
	desiredValue := NewValue(desired.NodeSelector, isEmpty(desired.NodeSelector), false)

	return compareValues(
		makePathWithClass("spec.nodeSelector"),
		currentValue,
		desiredValue,
		reflect.DeepEqual(current.NodeSelector, desired.NodeSelector),
		ActionRestart,
	)
}

func compareVmClassTolerations(current, desired *v1alpha2.VirtualMachineClassSpec) []FieldChange {
	currentValue := NewValue(current.Tolerations, len(current.Tolerations) == 0, false)
	desiredValue := NewValue(desired.Tolerations, len(desired.Tolerations) == 0, false)

	return compareValues(
		makePathWithClass("spec.tolerations"),
		currentValue,
		desiredValue,
		reflect.DeepEqual(current.Tolerations, desired.Tolerations),
		ActionRestart,
	)
}
