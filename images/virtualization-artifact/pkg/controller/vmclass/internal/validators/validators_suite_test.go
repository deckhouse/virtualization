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

package validators_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/deckhouse/virtualization-controller/pkg/controller/vmclass/internal/validators"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

func TestValidators(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Validators Suite")
}

var _ = Describe("Spec policies validator", func() {
	var vmclass v1alpha2.VirtualMachineClass

	Context("empty vmclass", func() {
		BeforeEach(func() {
			vmclass = v1alpha2.VirtualMachineClass{}
		})

		It("Should return no problem when empty value", func() {
			Expect(validators.HasCPUSizePoliciesCrosses(&vmclass.Spec)).Should(BeFalse())
		})
	})

	Context("vmclass with no cpu size policies crosses", func() {
		BeforeEach(func() {
			vmclass = v1alpha2.VirtualMachineClass{}
			vmclass.Spec.SizingPolicies = append(vmclass.Spec.SizingPolicies, v1alpha2.SizingPolicy{
				Cores: &v1alpha2.SizingPolicyCores{
					Min:  1,
					Max:  4,
					Step: 1,
				},
			})
			vmclass.Spec.SizingPolicies = append(vmclass.Spec.SizingPolicies, v1alpha2.SizingPolicy{
				Cores: &v1alpha2.SizingPolicyCores{
					Min:  5,
					Max:  9,
					Step: 1,
				},
			})
			vmclass.Spec.SizingPolicies = append(vmclass.Spec.SizingPolicies, v1alpha2.SizingPolicy{
				Cores: &v1alpha2.SizingPolicyCores{
					Min:  10,
					Max:  15,
					Step: 1,
				},
			})
		})

		It("Should return no problem with correct values", func() {
			Expect(validators.HasCPUSizePoliciesCrosses(&vmclass.Spec)).Should(BeFalse())
		})
	})

	Context("vmclass with cpu size policies crosses", func() {
		BeforeEach(func() {
			vmclass = v1alpha2.VirtualMachineClass{}
			vmclass.Spec.SizingPolicies = append(vmclass.Spec.SizingPolicies, v1alpha2.SizingPolicy{
				Cores: &v1alpha2.SizingPolicyCores{
					Min:  1,
					Max:  4,
					Step: 1,
				},
			})
			vmclass.Spec.SizingPolicies = append(vmclass.Spec.SizingPolicies, v1alpha2.SizingPolicy{
				Cores: &v1alpha2.SizingPolicyCores{
					Min:  4,
					Max:  9,
					Step: 1,
				},
			})
			vmclass.Spec.SizingPolicies = append(vmclass.Spec.SizingPolicies, v1alpha2.SizingPolicy{
				Cores: &v1alpha2.SizingPolicyCores{
					Min:  10,
					Max:  15,
					Step: 1,
				},
			})
		})

		It("Should return problem with incorrect values", func() {
			Expect(validators.HasCPUSizePoliciesCrosses(&vmclass.Spec)).Should(BeTrue())
		})
	})
})
