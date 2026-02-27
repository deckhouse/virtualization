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
	"fmt"
	"sort"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

const BlockDevicesPath = "blockDeviceRefs"

// compareBlockDevices returns changes between current and desired blockDevices lists.
func compareBlockDevices(current, desired *v1alpha2.VirtualMachineSpec) []FieldChange {
	if len(current.BlockDeviceRefs) == 0 && len(desired.BlockDeviceRefs) == 0 {
		return nil
	}

	fullChanges := compareEmpty(
		BlockDevicesPath,
		NewValue(current.BlockDeviceRefs, len(current.BlockDeviceRefs) == 0, false),
		NewValue(desired.BlockDeviceRefs, len(desired.BlockDeviceRefs) == 0, false),
		ActionRestart,
	)

	if len(fullChanges) > 0 {
		return fullChanges
	}

	// Detect new and retired devices.
	added := make(map[int]struct{})
	removed := make(map[int]struct{})

	deviceAccessors := []func(vm *v1alpha2.VirtualMachineSpec) map[string]int{
		cvmiIndexedNames,
		vmiIndexedNames,
		vmdIndexedNames,
	}
	for _, devAccessor := range deviceAccessors {
		currentIndexes := devAccessor(current)
		desiredIndexes := devAccessor(desired)
		updateIndexesForAddedDevices(added, currentIndexes, desiredIndexes)
		updateIndexesForRemovedDevices(removed, currentIndexes, desiredIndexes)
	}

	// Detect swapped devices if there are no add/remove changes.
	// TODO(future): Cleanup added/removed from current/desired and detect swapped.
	swapped := make(map[int]struct{})
	if len(added) == 0 && len(removed) == 0 {
		for _, devAccessor := range deviceAccessors {
			currentIndexes := devAccessor(current)
			desiredIndexes := devAccessor(desired)
			updateIndexesForSwappedDevices(swapped, currentIndexes, desiredIndexes)
		}
	}

	// Combine cvmi, vmi and vmd changes into final list.
	indexes := getUniqueIndexes(added, removed, swapped)
	changes := make([]FieldChange, 0, len(indexes))
	for _, idx := range indexes {
		_, isAdded := added[idx]
		_, isRemoved := removed[idx]
		_, isSwapped := swapped[idx]
		itemPath := blockDevicesItemPath(idx)

		switch {
		case isAdded && isRemoved:
			// Compact add+remove for the same index into one replace.
			// A different device at the same index requires restart.
			changes = append(changes, FieldChange{
				Operation:      ChangeReplace,
				Path:           itemPath,
				CurrentValue:   current.BlockDeviceRefs[idx],
				DesiredValue:   desired.BlockDeviceRefs[idx],
				ActionRequired: ActionRestart,
			})
		case isAdded:
			changes = append(changes, FieldChange{
				Operation:      ChangeAdd,
				Path:           itemPath,
				DesiredValue:   desired.BlockDeviceRefs[idx],
				ActionRequired: ActionApplyImmediate,
			})
		case isRemoved:
			changes = append(changes, FieldChange{
				Operation:      ChangeRemove,
				Path:           itemPath,
				CurrentValue:   current.BlockDeviceRefs[idx],
				ActionRequired: ActionApplyImmediate,
			})
		case isSwapped:
			changes = append(changes, FieldChange{
				Operation:      ChangeReplace,
				Path:           itemPath,
				CurrentValue:   current.BlockDeviceRefs[idx],
				DesiredValue:   desired.BlockDeviceRefs[idx],
				ActionRequired: ActionApplyImmediate,
			})
		}
	}

	return changes
}

func cvmiIndexedNames(vm *v1alpha2.VirtualMachineSpec) map[string]int {
	res := make(map[string]int)
	for idx, dev := range vm.BlockDeviceRefs {
		if dev.Kind == v1alpha2.ClusterImageDevice {
			res[dev.Name] = idx
		}
	}
	return res
}

func vmiIndexedNames(vm *v1alpha2.VirtualMachineSpec) map[string]int {
	res := make(map[string]int)
	for idx, dev := range vm.BlockDeviceRefs {
		if dev.Kind == v1alpha2.VirtualImageKind {
			res[dev.Name] = idx
		}
	}
	return res
}

func vmdIndexedNames(vm *v1alpha2.VirtualMachineSpec) map[string]int {
	res := make(map[string]int)
	for idx, dev := range vm.BlockDeviceRefs {
		if dev.Kind == v1alpha2.VirtualDiskKind {
			res[dev.Name] = idx
		}
	}
	return res
}

func blockDevicesItemPath(idx int) string {
	return fmt.Sprintf("%s.%d", BlockDevicesPath, idx)
}

func updateIndexesForAddedDevices(added map[int]struct{}, currentDevices, desiredDevices map[string]int) {
	for name, idx := range desiredDevices {
		if _, has := currentDevices[name]; !has {
			added[idx] = struct{}{}
		}
	}
}

func updateIndexesForRemovedDevices(removed map[int]struct{}, currentDevices, desiredDevices map[string]int) {
	for name, idx := range currentDevices {
		if _, has := desiredDevices[name]; !has {
			removed[idx] = struct{}{}
		}
	}
}

func updateIndexesForSwappedDevices(swapped map[int]struct{}, currentDevices, desiredDevices map[string]int) {
	for name, curIdx := range currentDevices {
		if desIdx, has := desiredDevices[name]; has {
			if curIdx != desIdx {
				swapped[curIdx] = struct{}{}
			}
		}
	}
}

func getUniqueIndexes(maps ...map[int]struct{}) []int {
	// Unique.
	acc := make(map[int]struct{})
	for _, m := range maps {
		for idx := range m {
			acc[idx] = struct{}{}
		}
	}
	// Map to array.
	res := make([]int, 0, len(acc))
	for idx := range acc {
		res = append(res, idx)
	}

	sort.Ints(res)
	return res
}
