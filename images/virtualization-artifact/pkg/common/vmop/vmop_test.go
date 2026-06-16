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

package vmop

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

func TestVMOP(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Common VMOP Suite")
}

var _ = Describe("IsFinished", func() {
	DescribeTable("detects terminal phases",
		func(phase v1alpha2.VMOPPhase, expected bool) {
			vmop := &v1alpha2.VirtualMachineOperation{
				Status: v1alpha2.VirtualMachineOperationStatus{Phase: phase},
			}

			Expect(IsFinished(vmop)).To(Equal(expected))
		},
		Entry("completed", v1alpha2.VMOPPhaseCompleted, true),
		Entry("failed", v1alpha2.VMOPPhaseFailed, true),
		Entry("superseded", v1alpha2.VMOPPhaseSuperseded, true),
		Entry("pending", v1alpha2.VMOPPhasePending, false),
		Entry("in progress", v1alpha2.VMOPPhaseInProgress, false),
		Entry("terminating", v1alpha2.VMOPPhaseTerminating, false),
	)
})
