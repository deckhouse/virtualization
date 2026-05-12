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
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/component-base/featuregate"

	"github.com/deckhouse/virtualization-controller/pkg/controller/kvbuilder"
	"github.com/deckhouse/virtualization-controller/pkg/featuregates"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type comparatorMemory struct {
	featureGate featuregate.FeatureGate
}

func NewComparatorMemory(featureGate featuregate.FeatureGate) VMSpecFieldComparator {
	return &comparatorMemory{
		featureGate: featureGate,
	}
}

// Compare detects changes in memory size.
// It is aware of hotplug mechanism. If hotplug is disabled it requires
// restart if memory.size is changed. If hotplug is enabled, it allows
// changing "on the fly". Also, it requires restart if hotplug boundary
// is crossed.
// Note: memory hotplug is enabled if VM has more than 1Gi of RAM.
func (c *comparatorMemory) Compare(current, desired *v1alpha2.VirtualMachineSpec) []FieldChange {
	hotplugThreshold := resource.NewQuantity(kvbuilder.EnableMemoryHotplugThreshold, resource.BinarySI)
	isHotpluggable := current.Memory.Size.Cmp(*hotplugThreshold) >= 0
	isHotpluggableDesired := desired.Memory.Size.Cmp(*hotplugThreshold) >= 0

	actionType := ActionRestart
	if isHotpluggable && isHotpluggableDesired {
		actionType = ActionApplyImmediate
	}

	// Restart required to decrease memory size. (current > desired)
	if current.Memory.Size.Cmp(desired.Memory.Size) == 1 {
		actionType = ActionRestart
	}

	// Require reboot if memory hotplug is not enabled.
	if !c.featureGate.Enabled(featuregates.HotplugMemoryWithLiveMigration) {
		actionType = ActionRestart
	}

	return compareQuantity("memory.size", current.Memory.Size, desired.Memory.Size, resource.Quantity{}, actionType)
}
