package vmchange

import (
	"github.com/deckhouse/virtualization-controller/api/v1alpha2"
)

type SpecFieldsComparator func(prev, next *v1alpha2.VirtualMachineSpec) []FieldChange

var specComparators = []SpecFieldsComparator{
	compareRunPolicy,
	compareVirtualMachineIPAddressClaimName,
	compareTopologySpreadConstraints,
	compareAffinity,
	compareNodeSelector,
	comparePriorityClassName,
	compareTolerations,
	compareDisruptions,
	compareTerminationGracePeriodSeconds,
	compareEnableParavirtualization,
	compareOSType,
	compareBootloader,
	compareCPU,
	compareMemory,
	compareBlockDevices,
	compareProvisioning,
}

func CompareSpecs(prev, next *v1alpha2.VirtualMachineSpec) SpecChanges {
	specChanges := SpecChanges{}

	for _, comparator := range specComparators {
		changes := comparator(prev, next)
		if HasChanges(changes) {
			specChanges.Add(changes...)
		}
	}

	return specChanges
}
