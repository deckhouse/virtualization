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
	"k8s.io/component-base/featuregate"

	"github.com/deckhouse/virtualization-controller/pkg/common/vm"
	"github.com/deckhouse/virtualization-controller/pkg/featuregates"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type comparatorCPU struct {
	featureGate featuregate.FeatureGate
}

func NewComparatorCPU(featureGate featuregate.FeatureGate) VMSpecFieldComparator {
	return &comparatorCPU{
		featureGate: featureGate,
	}
}

// Compare returns changes in the cpu section.
// // It supports CPU hotplug mechanism for cores changes.
// // It requires reboot if cpu fraction is changed or if COU hotplug is disabled.
func (c *comparatorCPU) Compare(current, desired *v1alpha2.VirtualMachineSpec) []FieldChange {
	// Cores can be changed "on the fly" using CPU Hotplug ...
	coresChangedAction := ActionApplyImmediate
	// ... but sockets count change requires a reboot.
	currentSockets, _ := vm.CalculateCoresAndSockets(current.CPU.Cores)
	desiredSockets, _ := vm.CalculateCoresAndSockets(desired.CPU.Cores)
	if currentSockets != desiredSockets {
		coresChangedAction = ActionRestart
	}

	// Require reboot if CPU hotplug is not enabled.
	if !c.featureGate.Enabled(featuregates.HotplugCPUWithLiveMigration) {
		coresChangedAction = ActionRestart
	}

	coresChanges := compareInts("cpu.cores", current.CPU.Cores, desired.CPU.Cores, 0, coresChangedAction)
	fractionChanges := compareStrings("cpu.coreFraction", current.CPU.CoreFraction, desired.CPU.CoreFraction, DefaultCPUCoreFraction, ActionRestart)

	// Yield full replace if both fields changed.
	if HasChanges(coresChanges) && HasChanges(fractionChanges) {
		return []FieldChange{
			{
				Operation:      ChangeReplace,
				Path:           "cpu",
				CurrentValue:   current.CPU,
				DesiredValue:   desired.CPU,
				ActionRequired: ActionRestart,
			},
		}
	}

	if HasChanges(coresChanges) {
		return coresChanges
	}

	if HasChanges(fractionChanges) {
		return fractionChanges
	}

	return nil
}
