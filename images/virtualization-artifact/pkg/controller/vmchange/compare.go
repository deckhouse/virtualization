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
	"k8s.io/component-base/featuregate"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type SpecFieldsComparator func(prev, next *v1alpha2.VirtualMachineSpec) []FieldChange

type VMSpecFieldComparator interface {
	Compare(prev, next *v1alpha2.VirtualMachineSpec) []FieldChange
}

type vmSpecFieldsComparatorWithFn struct {
	fn func(prev, next *v1alpha2.VirtualMachineSpec) []FieldChange
}

func (v *vmSpecFieldsComparatorWithFn) Compare(prev, next *v1alpha2.VirtualMachineSpec) []FieldChange {
	if v.fn == nil {
		return nil
	}
	return v.fn(prev, next)
}

func vmSpecFieldComparator(fn SpecFieldsComparator) VMSpecFieldComparator {
	return &vmSpecFieldsComparatorWithFn{fn: fn}
}

type VMClassSpecFieldsComparator func(prev, next *v1alpha2.VirtualMachineClassSpec) []FieldChange

var vmclassSpecComparators = []VMClassSpecFieldsComparator{
	compareVMClassNodeSelector,
	compareVMClassTolerations,
}

type VMSpecComparator struct {
	featureGate featuregate.FeatureGate
}

func NewVMSpecComparator(featureGate featuregate.FeatureGate) *VMSpecComparator {
	return &VMSpecComparator{
		featureGate: featureGate,
	}
}

func (v *VMSpecComparator) comparators() []VMSpecFieldComparator {
	return []VMSpecFieldComparator{
		vmSpecFieldComparator(compareVirtualMachineClass),
		vmSpecFieldComparator(compareRunPolicy),
		vmSpecFieldComparator(compareVirtualMachineIPAddress),
		vmSpecFieldComparator(compareTopologySpreadConstraints),
		vmSpecFieldComparator(compareAffinity),
		vmSpecFieldComparator(compareNodeSelector),
		vmSpecFieldComparator(comparePriorityClassName),
		vmSpecFieldComparator(compareTolerations),
		vmSpecFieldComparator(compareDisruptions),
		vmSpecFieldComparator(compareTerminationGracePeriodSeconds),
		vmSpecFieldComparator(compareEnableParavirtualization),
		vmSpecFieldComparator(compareOSType),
		vmSpecFieldComparator(compareBootloader),
		NewComparatorCPU(v.featureGate),
		NewComparatorMemory(v.featureGate),
		vmSpecFieldComparator(compareBlockDevices),
		vmSpecFieldComparator(compareProvisioning),
		vmSpecFieldComparator(compareNetworks),
		vmSpecFieldComparator(compareUSBDevices),
	}
}

func (v *VMSpecComparator) Compare(prev, next *v1alpha2.VirtualMachineSpec) SpecChanges {
	specChanges := SpecChanges{}

	for _, comparator := range v.comparators() {
		changes := comparator.Compare(prev, next)
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
