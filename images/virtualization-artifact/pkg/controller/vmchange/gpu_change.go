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

	"github.com/deckhouse/virtualization-controller/pkg/controller/kvbuilder"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

func compareGPUDevices(current, desired *v1alpha2.VirtualMachineSpec) []FieldChange {
	currentGPUDevices := kvbuilder.SortGPUDevices(current.GPUDevices)
	desiredGPUDevices := kvbuilder.SortGPUDevices(desired.GPUDevices)
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
