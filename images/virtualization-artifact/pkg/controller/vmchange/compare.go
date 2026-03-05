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
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type SpecFieldsComparator func(prev, next *v1alpha2.VirtualMachineSpec) []FieldChange

var specComparators = []SpecFieldsComparator{
	compareVirtualMachineClass,
	compareRunPolicy,
	compareVirtualMachineIPAddress,
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
	compareNetworks,
	compareUSBDevices,
}

type VMClassSpecFieldsComparator func(prev, next *v1alpha2.VirtualMachineClassSpec) []FieldChange

var vmclassSpecComparators = []VMClassSpecFieldsComparator{
	compareVMClassNodeSelector,
	compareVMClassTolerations,
}

func CompareSpecs(prev, next *v1alpha2.VirtualMachineSpec, prevClass, nextClass *v1alpha2.VirtualMachineClassSpec) SpecChanges {
	specChanges := CompareVMSpecs(prev, next)
	specClassChanges := CompareClassSpecs(prevClass, nextClass)
	specChanges.Add(specClassChanges.GetAll()...)
	return specChanges
}

func CompareVMSpecs(prev, next *v1alpha2.VirtualMachineSpec) SpecChanges {
	specChanges := SpecChanges{}

	for _, comparator := range specComparators {
		changes := comparator(prev, next)
		if HasChanges(changes) {
			specChanges.Add(changes...)
		}
	}

	return specChanges
}

func CompareClassSpecs(prevClass, nextClass *v1alpha2.VirtualMachineClassSpec) SpecChanges {
	var specChanges SpecChanges

	for _, comparator := range vmclassSpecComparators {
		changes := comparator(prevClass, nextClass)
		if HasChanges(changes) {
			specChanges.Add(changes...)
		}
	}

	return specChanges
}
