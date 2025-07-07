/*
Copyright 2025 Flant JSC

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

package livemigration

import (
	"fmt"

	"k8s.io/utils/ptr"

	"github.com/deckhouse/virtualization-controller/pkg/config"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

func AutoConvergeForPolicy(policy v1alpha2.LiveMigrationPolicy) (autoConverge *bool) {
	switch policy {
	case v1alpha2.AlwaysSafeMigrationPolicy, v1alpha2.PreferSafeMigrationPolicy:
		return ptr.To(false)
	case v1alpha2.AlwaysForcedMigrationPolicy, v1alpha2.PreferForcedMigrationPolicy:
		return ptr.To(true)
	}
	return nil
}

// CalculateEffectivePolicy merges live migration policy from default value and from VM.
// Also, autoConverge value may be overridden from VMOP.
func CalculateEffectivePolicy(vm v1alpha2.VirtualMachine, vmop *v1alpha2.VirtualMachineOperation) (effectivePolicy v1alpha2.LiveMigrationPolicy, autoConverge bool, err error) {
	effectivePolicy = config.DefaultLiveMigrationPolicy

	if vm.Spec.LiveMigrationPolicy != "" {
		effectivePolicy = vm.Spec.LiveMigrationPolicy
	}

	autoConvergePtr := AutoConvergeForPolicy(effectivePolicy)

	// Override autoConverge value.
	if vmop != nil {
		switch effectivePolicy {
		case v1alpha2.PreferSafeMigrationPolicy,
			v1alpha2.PreferForcedMigrationPolicy:
			if vmop.Spec.Force != nil {
				autoConvergePtr = vmop.Spec.Force
			}
		case v1alpha2.AlwaysSafeMigrationPolicy:
			if vmop.Spec.Force != nil && *vmop.Spec.Force {
				return effectivePolicy, *autoConvergePtr, fmt.Errorf("force=true is not applicable for VM liveMigrationPolicy %s", effectivePolicy)
			}
		case v1alpha2.AlwaysForcedMigrationPolicy:
			if vmop.Spec.Force != nil && !*vmop.Spec.Force {
				return effectivePolicy, *autoConvergePtr, fmt.Errorf("force=false is not applicable for VM liveMigrationPolicy %s", effectivePolicy)
			}
		}
	}

	if autoConvergePtr == nil {
		// Should not be reached. Disable autoConverge is safe.
		autoConverge = false
	} else {
		autoConverge = *autoConvergePtr
	}

	return
}
