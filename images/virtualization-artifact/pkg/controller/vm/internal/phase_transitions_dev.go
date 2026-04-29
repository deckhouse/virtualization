//go:build dev

package internal

import (
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

func NewPhaseTransitions(phaseTransitions []v1alpha2.VirtualMachinePhaseTransitionTimestamp, oldPhase, newPhase v1alpha2.MachinePhase) []v1alpha2.VirtualMachinePhaseTransitionTimestamp {
	now := metav1.NewTime(time.Now().Truncate(time.Second))

	if oldPhase != newPhase {
		phaseTransitions = append(phaseTransitions, v1alpha2.VirtualMachinePhaseTransitionTimestamp{
			Phase:     newPhase,
			Timestamp: now,
		})
	}
	if len(phaseTransitions) > 5 {
		return phaseTransitions[len(phaseTransitions)-5:]
	}
	return phaseTransitions
}
