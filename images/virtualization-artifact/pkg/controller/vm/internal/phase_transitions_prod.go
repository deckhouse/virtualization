//go:build !dev

package internal

import "github.com/deckhouse/virtualization/api/core/v1alpha2"

func NewPhaseTransitions([]v1alpha2.VirtualMachinePhaseTransitionTimestamp, v1alpha2.MachinePhase, v1alpha2.MachinePhase) []v1alpha2.VirtualMachinePhaseTransitionTimestamp {
	return nil
}
