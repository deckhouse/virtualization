package vmchange

import (
	"fmt"
	"sort"

	"github.com/deckhouse/virtualization-controller/api/core/v1alpha2"
)

const BlockDevicesPath = "blockDevices"

// compareBlockDevices returns changes between current and desired blockDevices lists.
func compareBlockDevices(current, desired *v1alpha2.VirtualMachineSpec) []FieldChange {
	if len(current.BlockDevices) == 0 && len(desired.BlockDevices) == 0 {
		return nil
	}

	fullChanges := compareEmpty(
		BlockDevicesPath,
		NewValue(current.BlockDevices, len(current.BlockDevices) == 0, false),
		NewValue(desired.BlockDevices, len(desired.BlockDevices) == 0, false),
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
			changes = append(changes, FieldChange{
				Operation:      ChangeReplace,
				Path:           itemPath,
				CurrentValue:   current.BlockDevices[idx],
				DesiredValue:   desired.BlockDevices[idx],
				ActionRequired: ActionRestart,
			})
		case isAdded:
			changes = append(changes, FieldChange{
				Operation:      ChangeAdd,
				Path:           itemPath,
				DesiredValue:   desired.BlockDevices[idx],
				ActionRequired: ActionRestart,
			})
		case isRemoved:
			changes = append(changes, FieldChange{
				Operation:      ChangeRemove,
				Path:           itemPath,
				CurrentValue:   current.BlockDevices[idx],
				ActionRequired: ActionRestart,
			})
		case isSwapped:
			changes = append(changes, FieldChange{
				Operation:      ChangeReplace,
				Path:           itemPath,
				CurrentValue:   current.BlockDevices[idx],
				DesiredValue:   desired.BlockDevices[idx],
				ActionRequired: ActionRestart,
			})
		}
	}

	return changes
}

func cvmiIndexedNames(vm *v1alpha2.VirtualMachineSpec) map[string]int {
	res := make(map[string]int)
	for idx, dev := range vm.BlockDevices {
		if dev.ClusterVirtualMachineImage != nil {
			res[dev.ClusterVirtualMachineImage.Name] = idx
		}
	}
	return res
}

func vmiIndexedNames(vm *v1alpha2.VirtualMachineSpec) map[string]int {
	res := make(map[string]int)
	for idx, dev := range vm.BlockDevices {
		if dev.VirtualMachineImage != nil {
			res[dev.VirtualMachineImage.Name] = idx
		}
	}
	return res
}

func vmdIndexedNames(vm *v1alpha2.VirtualMachineSpec) map[string]int {
	res := make(map[string]int)
	for idx, dev := range vm.BlockDevices {
		if dev.VirtualMachineDisk != nil {
			res[dev.VirtualMachineDisk.Name] = idx
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
