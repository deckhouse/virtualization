//go:build !dev

package internal

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

var _ = Describe("Phase transitions in release build", func() {
	It("drops transition history", func() {
		transitions := []v1alpha2.VirtualMachinePhaseTransitionTimestamp{
			{Phase: v1alpha2.MachinePending, Timestamp: metav1.Now()},
		}

		Expect(NewPhaseTransitions(transitions, v1alpha2.MachinePending, v1alpha2.MachineRunning)).To(BeNil())
	})
})
