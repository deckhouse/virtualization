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

package service

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/utils/ptr"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

func TestPowerstateService(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Powerstate Service Suite")
}

var _ = Describe("Operation applicability", func() {
	DescribeTable("Stop operation in Stopping phase",
		func(force *bool, expected bool) {
			op := NewStopOperation(nil, vmop(v1alpha2.VMOPTypeStop, force))
			Expect(op.IsApplicableForVMPhase(v1alpha2.MachineStopping)).To(Equal(expected))
		},
		Entry("without force", nil, false),
		Entry("with force=false", ptr.To(false), false),
		Entry("with force=true", ptr.To(true), true),
	)

	DescribeTable("Restart operation in Stopping phase",
		func(force *bool, expected bool) {
			op := NewRestartOperation(nil, vmop(v1alpha2.VMOPTypeRestart, force))
			Expect(op.IsApplicableForVMPhase(v1alpha2.MachineStopping)).To(Equal(expected))
		},
		Entry("without force", nil, false),
		Entry("with force=false", ptr.To(false), false),
		Entry("with force=true", ptr.To(true), true),
	)
})

func vmop(vmopType v1alpha2.VMOPType, force *bool) *v1alpha2.VirtualMachineOperation {
	return &v1alpha2.VirtualMachineOperation{
		Spec: v1alpha2.VirtualMachineOperationSpec{
			Type:  vmopType,
			Force: force,
		},
	}
}
