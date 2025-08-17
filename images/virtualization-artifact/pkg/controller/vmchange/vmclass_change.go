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
	"reflect"

	"github.com/deckhouse/virtualization-controller/pkg/featuregates"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

func makePathWithClass(path string) string {
	return fmt.Sprintf("VirtualMachineClass:%s", path)
}

func compareVMClassNodeSelector(current, desired *v1alpha2.VirtualMachineClassSpec) []FieldChange {
	isEmpty := func(nodeSelector v1alpha2.NodeSelector) bool {
		return len(nodeSelector.MatchExpressions) == 0 && len(nodeSelector.MatchLabels) == 0
	}

	currentValue := NewValue(current.NodeSelector, isEmpty(current.NodeSelector), false)
	desiredValue := NewValue(desired.NodeSelector, isEmpty(desired.NodeSelector), false)

	return compareValues(
		makePathWithClass("spec.nodeSelector"),
		currentValue,
		desiredValue,
		reflect.DeepEqual(current.NodeSelector, desired.NodeSelector),
		placementAction(),
	)
}

func compareVMClassTolerations(current, desired *v1alpha2.VirtualMachineClassSpec) []FieldChange {
	currentValue := NewValue(current.Tolerations, len(current.Tolerations) == 0, false)
	desiredValue := NewValue(desired.Tolerations, len(desired.Tolerations) == 0, false)

	return compareValues(
		makePathWithClass("spec.tolerations"),
		currentValue,
		desiredValue,
		reflect.DeepEqual(current.Tolerations, desired.Tolerations),
		placementAction(),
	)
}

func placementAction() ActionType {
	if featuregates.Default().Enabled(featuregates.AutoMigrationIfNodePlacementChanged) {
		return ActionApplyImmediate
	}
	return ActionRestart
}
