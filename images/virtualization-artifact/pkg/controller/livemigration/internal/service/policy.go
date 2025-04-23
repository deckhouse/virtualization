package service

import (
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
func CalculateEffectivePolicy(vm v1alpha2.VirtualMachine, vmop *v1alpha2.VirtualMachineOperation) (effectivePolicy v1alpha2.LiveMigrationPolicy, autoConverge bool) {
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
