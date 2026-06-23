/*
Copyright 2026 Flant JSC

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
	"slices"
	"strings"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

func compareGPUDevices(current, desired *v1alpha2.VirtualMachineSpec) []FieldChange {
	currentGPUDevices := sortedGPUDevicesForCompare(current.GPUDevices)
	desiredGPUDevices := sortedGPUDevicesForCompare(desired.GPUDevices)
	currentValue := NewValue(currentGPUDevices, current.GPUDevices == nil, false)
	desiredValue := NewValue(desiredGPUDevices, desired.GPUDevices == nil, false)

	return compareValues(
		"gpuDevices",
		currentValue,
		desiredValue,
		reflect.DeepEqual(currentGPUDevices, desiredGPUDevices),
		ActionRestart,
	)
}

func sortedGPUDevicesForCompare(devices []v1alpha2.GPUDeviceSpec) []v1alpha2.GPUDeviceSpec {
	if devices == nil {
		return nil
	}

	sorted := slices.Clone(devices)
	slices.SortFunc(sorted, func(a, b v1alpha2.GPUDeviceSpec) int {
		return strings.Compare(a.Name, b.Name)
	})
	return sorted
}
