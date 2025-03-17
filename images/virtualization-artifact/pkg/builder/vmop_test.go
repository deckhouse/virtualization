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

package builder

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

var _ = Describe("Builder VMop", func() {
	const (
		name      = "test-vmop"
		namespace = "default"
	)
	var vmopBuilder *VMOPBuilder
	BeforeEach(func() {
		vmopBuilder = NewVMOPBuilder(name, namespace)
	})

	Describe("Building VMOP", func() {
		It("should initialize with correct name and namespace", func() {
			vmop := vmopBuilder.Complete()
			Expect(vmop.ObjectMeta.Name).To(Equal(name))
			Expect(vmop.ObjectMeta.Namespace).To(Equal(namespace))
		})

		It("should set VMOP type", func() {
			vmopType := v1alpha2.VMOPTypeStart
			vmop := vmopBuilder.WithType(vmopType).Complete()
			Expect(vmop.Spec.Type).To(Equal(vmopType))
		})

		It("should set VirtualMachine name", func() {
			vmName := "test-vm"
			vmop := vmopBuilder.WithVirtualMachine(vmName).Complete()
			Expect(vmop.Spec.VirtualMachine).To(Equal(vmName))
		})

		It("should set Force flag", func() {
			vmop := vmopBuilder.WithForce(true).Complete()
			Expect(vmop.Spec.Force).To(BeTrue())
		})

		It("should set Status phase", func() {
			phase := v1alpha2.VMOPPhaseCompleted
			vmop := vmopBuilder.WithStatusPhase(phase).Complete()
			Expect(vmop.Status.Phase).To(Equal(phase))
		})

		It("should rewrite namespace and name", func() {
			vmop := vmopBuilder.WithName("rewrite").WithNamespace("rewrite").Complete()
			Expect(vmop.Name).To(Equal("rewrite"))
			Expect(vmop.Namespace).To(Equal("rewrite"))
		})

		It("should add a label and annotations", func() {
			vmop := vmopBuilder.WithLabel("key", "value").WithAnnotation("key", "value").Complete()
			Expect(vmop.Labels).To(HaveKeyWithValue("key", "value"))
			Expect(vmop.Annotations).To(HaveKeyWithValue("key", "value"))
		})
	})
})
