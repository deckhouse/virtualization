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

	action := ActionRestart
	if isOnlyNetworkIDAutofillChange(current.Networks, desired.Networks) {
		action = ActionNone
	} else if isOnlyNonMainNetworksChanged(current.Networks, desired.Networks) {
		action = ActionApplyImmediate
	}

	return compareValues(
		"networks",
		currentValue,
		desiredValue,
		reflect.DeepEqual(current.Networks, desired.Networks),
		action,
	)
}

func isOnlyNonMainNetworksChanged(current, desired []v1alpha2.NetworksSpec) bool {
	currentMain := getMainNetwork(current)
	desiredMain := getMainNetwork(desired)
	return reflect.DeepEqual(currentMain, desiredMain)
}

func getMainNetwork(networks []v1alpha2.NetworksSpec) *v1alpha2.NetworksSpec {
	for i := range networks {
		if networks[i].Type == v1alpha2.NetworksTypeMain {
			return &networks[i]
		}
	}
	return nil
}

func isOnlyNetworkIDAutofillChange(current, desired []v1alpha2.NetworksSpec) bool {
	if len(current) != len(desired) {
		return false
	}

	for i := range current {
		if current[i].Type != desired[i].Type ||
			current[i].Name != desired[i].Name ||
			current[i].VirtualMachineMACAddressName != desired[i].VirtualMachineMACAddressName {
			return false
		}

		if current[i].ID == nil {
			continue
		}

		if desired[i].ID == nil || *current[i].ID != *desired[i].ID {
			return false
		}
	}

	return true
}
