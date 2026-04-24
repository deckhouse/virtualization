//go:build dev

package internal

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

var _ = Describe("Phase transitions in dev build", func() {
	It("appends and keeps the latest five transitions", func() {
		transitions := []v1alpha2.VirtualMachinePhaseTransitionTimestamp{
			{Phase: v1alpha2.MachinePending, Timestamp: metav1.Now()},
			{Phase: v1alpha2.MachineStarting, Timestamp: metav1.Now()},
			{Phase: v1alpha2.MachineRunning, Timestamp: metav1.Now()},
			{Phase: v1alpha2.MachineStopping, Timestamp: metav1.Now()},
			{Phase: v1alpha2.MachineStopped, Timestamp: metav1.Now()},
		}

		updated := NewPhaseTransitions(transitions, v1alpha2.MachineStopped, v1alpha2.MachinePending)

		Expect(updated).To(HaveLen(5))
		Expect(updated[0].Phase).To(Equal(v1alpha2.MachineStarting))
		Expect(updated[4].Phase).To(Equal(v1alpha2.MachinePending))
		Expect(updated[4].Timestamp.IsZero()).To(BeFalse())
	})
})
