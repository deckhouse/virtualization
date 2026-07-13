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

	sizingpolicy "github.com/deckhouse/virtualization-controller/pkg/common/sizing_policy"
	"github.com/deckhouse/virtualization-controller/pkg/common/vm"
	"github.com/deckhouse/virtualization-controller/pkg/featuregates"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type comparatorCPU struct {
	featureGate featuregate.FeatureGate
}

const cpuPath = "cpu"

func NewComparatorCPU(featureGate featuregate.FeatureGate) VMSpecFieldComparator {
	return &comparatorCPU{
		featureGate: featureGate,
	}
}

// Compare returns changes in the cpu section.
// It returns "apply immediate" when cpu core fraction is changed or
// CPU cores change is compatible with hotplug.
func (c *comparatorCPU) Compare(current, desired *v1alpha2.VirtualMachineSpec) []FieldChange {
	coresRestartMsg := ""

	// Cores can be changed "on the fly" using CPU Hotplug ...
	coresChangedAction := ActionApplyImmediate
	// ... but sockets count change requires a reboot.
	currentSockets, _, _ := vm.CalculateCoresAndSockets(current.CPU.Cores)
	desiredSockets, _, _ := vm.CalculateCoresAndSockets(desired.CPU.Cores)
	if currentSockets != desiredSockets {
		coresRestartMsg = "Changing the number of CPU cores requires changing the CPU topology (number of sockets)."
		coresChangedAction = ActionRestart
	}

	fractionChangedAction := ActionApplyImmediate
	fractionRestartMsg := ""

	// Require reboot if CPU hotplug is not enabled.
	if !c.featureGate.Enabled(featuregates.HotplugCPUWithLiveMigration) && !c.featureGate.Enabled(featuregates.HotplugCPUAndMemoryWithInPlaceResize) {
		coresChangedAction = ActionRestart
		fractionChangedAction = ActionRestart
	}

	// A coreFraction of 100% makes the launcher pod's CPU requests equal its limits,
	// so the pod is Guaranteed; any lower value is Burstable. Kubernetes forbids an
	// in-place resize from changing a pod's QoS class, so crossing that boundary must
	// go through a restart even when CPU hotplug is on.
	if isCPUGuaranteed(current.CPU.CoreFraction) != isCPUGuaranteed(desired.CPU.CoreFraction) {
		fractionChangedAction = ActionRestart
		fractionRestartMsg = "Changing the CPU core fraction to or from 100% changes the pod QoS class, which cannot be applied in-place."
	}

	coresChanges := compareInts("cpu.cores", current.CPU.Cores, desired.CPU.Cores, 0, coresChangedAction)
	if HasChanges(coresChanges) && coresRestartMsg != "" {
		coresChanges[0].RestartMessage = coresRestartMsg
	}
	fractionChanges := compareStrings("cpu.coreFraction", current.CPU.CoreFraction, desired.CPU.CoreFraction, DefaultCPUCoreFraction, fractionChangedAction)
	if HasChanges(fractionChanges) && fractionRestartMsg != "" {
		fractionChanges[0].RestartMessage = fractionRestartMsg
	}

	// Yield a full replace for cpu section if both fields are changed.
	if HasChanges(coresChanges) && HasChanges(fractionChanges) {
		restartMsg := coresRestartMsg
		if restartMsg == "" {
			restartMsg = fractionRestartMsg
		}
		return []FieldChange{
			{
				Operation:      ChangeReplace,
				Path:           "cpu",
				CurrentValue:   current.CPU,
				DesiredValue:   desired.CPU,
				ActionRequired: MostDisruptiveAction(coresChangedAction, fractionChangedAction),
				RestartMessage: restartMsg,
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

// isCPUGuaranteed reports whether a coreFraction yields a Guaranteed pod, i.e. CPU
// requests equal limits. That happens only at 100%; an empty value defaults to
// DefaultCPUCoreFraction (100%). An unparsable value is treated as not Guaranteed so
// a malformed field never forces a restart on its own.
func isCPUGuaranteed(coreFraction string) bool {
	if coreFraction == "" {
		coreFraction = DefaultCPUCoreFraction
	}
	fraction, err := sizingpolicy.ParsePercent(coreFraction)
	if err != nil {
		return false
	}
	return fraction >= sizingpolicy.MaxCoreFraction
}
