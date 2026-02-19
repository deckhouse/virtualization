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
	"reflect"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

func compareTopologySpreadConstraints(current, desired *v1alpha2.VirtualMachineSpec) []FieldChange {
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

func compareAffinity(current, desired *v1alpha2.VirtualMachineSpec) []FieldChange {
	currentValue := NewValue(current.Affinity, current.Affinity == nil, false)
	desiredValue := NewValue(desired.Affinity, desired.Affinity == nil, false)

	return compareValues(
		"affinity",
		currentValue,
		desiredValue,
		reflect.DeepEqual(current.Affinity, desired.Affinity),
		placementAction(),
	)
}

func compareNodeSelector(current, desired *v1alpha2.VirtualMachineSpec) []FieldChange {
	currentValue := NewValue(current.NodeSelector, len(current.NodeSelector) == 0, false)
	desiredValue := NewValue(desired.NodeSelector, len(desired.NodeSelector) == 0, false)

	return compareValues(
		"nodeSelector",
		currentValue,
		desiredValue,
		reflect.DeepEqual(current.NodeSelector, desired.NodeSelector),
		placementAction(),
	)
}

func comparePriorityClassName(current, desired *v1alpha2.VirtualMachineSpec) []FieldChange {
	return compareStrings(
		"priorityClassName",
		current.PriorityClassName,
		desired.PriorityClassName,
		"",
		ActionRestart,
	)
}

func compareTolerations(current, desired *v1alpha2.VirtualMachineSpec) []FieldChange {
	currentValue := NewValue(current.Tolerations, len(current.Tolerations) == 0, false)
	desiredValue := NewValue(desired.Tolerations, len(desired.Tolerations) == 0, false)

	return compareValues(
		"tolerations",
		currentValue,
		desiredValue,
		reflect.DeepEqual(current.Tolerations, desired.Tolerations),
		placementAction(),
	)
}

func compareNetworks(current, desired *v1alpha2.VirtualMachineSpec) []FieldChange {
	currentValue := NewValue(current.Networks, current.Networks == nil, false)
	desiredValue := NewValue(desired.Networks, desired.Networks == nil, false)

	return compareValues(
		"networks",
		currentValue,
		desiredValue,
		reflect.DeepEqual(current.Networks, desired.Networks),
		ActionRestart,
	)
}
