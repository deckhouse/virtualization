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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/testutil"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmclass/internal/validators"
	"github.com/deckhouse/virtualization/api/core/v1alpha3"
)

func TestValidators(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Validators Suite")
}

var _ = Describe("Spec policies validator", func() {
	var vmclass v1alpha3.VirtualMachineClass

	Context("empty vmclass", func() {
		BeforeEach(func() {
			vmclass = v1alpha3.VirtualMachineClass{}
		})

		It("Should return no problem when empty value", func() {
			Expect(validators.HasCPUSizePoliciesCrosses(&vmclass.Spec)).Should(BeFalse())
		})
	})

	Context("vmclass with no cpu size policies crosses", func() {
		BeforeEach(func() {
			vmclass = v1alpha3.VirtualMachineClass{}
			vmclass.Spec.SizingPolicies = append(vmclass.Spec.SizingPolicies, v1alpha3.SizingPolicy{
				Cores: &v1alpha3.SizingPolicyCores{
					Min:  1,
					Max:  4,
					Step: 1,
				},
			})
			vmclass.Spec.SizingPolicies = append(vmclass.Spec.SizingPolicies, v1alpha3.SizingPolicy{
				Cores: &v1alpha3.SizingPolicyCores{
					Min:  5,
					Max:  9,
					Step: 1,
				},
			})
			vmclass.Spec.SizingPolicies = append(vmclass.Spec.SizingPolicies, v1alpha3.SizingPolicy{
				Cores: &v1alpha3.SizingPolicyCores{
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
			vmclass = v1alpha3.VirtualMachineClass{}
			vmclass.Spec.SizingPolicies = append(vmclass.Spec.SizingPolicies, v1alpha3.SizingPolicy{
				Cores: &v1alpha3.SizingPolicyCores{
					Min:  1,
					Max:  4,
					Step: 1,
				},
			})
			vmclass.Spec.SizingPolicies = append(vmclass.Spec.SizingPolicies, v1alpha3.SizingPolicy{
				Cores: &v1alpha3.SizingPolicyCores{
					Min:  4,
					Max:  9,
					Step: 1,
				},
			})
			vmclass.Spec.SizingPolicies = append(vmclass.Spec.SizingPolicies, v1alpha3.SizingPolicy{
				Cores: &v1alpha3.SizingPolicyCores{
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

var _ = Describe("Single default class validator", func() {
	var (
		ctx       = testutil.ContextBackgroundWithNoOpLogger()
		validator *validators.SingleDefaultClassValidator
	)

	setup := func(objs ...client.Object) {
		GinkgoHelper()
		fakeClient, err := testutil.NewFakeClientWithObjects(objs...)
		Expect(err).Should(BeNil())
		vmClassService := service.NewVirtualMachineClassService(fakeClient)
		validator = validators.NewSingleDefaultClassValidator(fakeClient, vmClassService)
	}

	newVMClass := func(name string) *v1alpha3.VirtualMachineClass {
		return &v1alpha3.VirtualMachineClass{
			TypeMeta: metav1.TypeMeta{
				Kind:       v1alpha3.VirtualMachineClassKind,
				APIVersion: v1alpha3.SchemeGroupVersion.String(),
			},
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
			},
			Spec:   v1alpha3.VirtualMachineClassSpec{},
			Status: v1alpha3.VirtualMachineClassStatus{},
		}
	}

	newDefaultVMClass := func(name string) *v1alpha3.VirtualMachineClass {
		vmClass := newVMClass(name)
		vmClass.Annotations = map[string]string{
			annotations.AnnVirtualMachineClassDefault: "true",
		}
		return vmClass
	}

	AfterEach(func() {
		validator = nil
	})

	Context("vmclass with is-default-class annotation", func() {
		It("should not fail on marking the single class as default", func() {
			// Initialize fake client with existing non default class.
			name := "single-custom-class"
			setup(
				newVMClass(name),
			)

			// Validate adding is-default-class annotation.
			updatedClass := newDefaultVMClass(name)
			warns, err := validator.ValidateUpdate(ctx, nil, updatedClass)
			Expect(err).Should(BeNil())
			Expect(warns).Should(BeEmpty(), "should not return warnings")
		})

		It("should fail on marking the second class as default", func() {
			// Initialize fake client with existing default class.
			name := "custom-class"
			setup(
				newVMClass(name),
				newDefaultVMClass("existing-default-class"),
			)

			// Validate adding is-default-class annotation.
			updatedClass := newDefaultVMClass(name)
			warns, err := validator.ValidateUpdate(ctx, nil, updatedClass)
			Expect(warns).Should(BeEmpty(), "should not return warnings")
			Expect(err).ShouldNot(BeNil(), "should fail if default class is already present")
		})

		It("should not fail on creating the single default class", func() {
			// Initialize empty fake client.
			setup()

			// Validate creating single default class.
			defaultClass := newDefaultVMClass("single-default-class")
			warns, err := validator.ValidateCreate(ctx, defaultClass)
			Expect(err).Should(BeNil())
			Expect(warns).Should(BeEmpty(), "should not return warnings")
		})

		It("should fail on creating the second default class", func() {
			// Initialize fake client with existing default class.
			setup(
				newDefaultVMClass("existing-default-class"),
			)

			// Validate creating second default class.
			updatedClass := newDefaultVMClass("second-default-class")
			warns, err := validator.ValidateCreate(ctx, updatedClass)
			Expect(warns).Should(BeEmpty(), "should not return warnings")
			Expect(err).ShouldNot(BeNil(), "should fail if default class is already present")
		})
	})
})
