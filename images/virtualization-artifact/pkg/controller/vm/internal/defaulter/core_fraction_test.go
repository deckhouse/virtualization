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

package defaulter_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/common/testutil"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vm/internal/defaulter"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha3"
)

var _ = Describe("CoreFractionDefaulter", func() {
	var (
		ctx                = testutil.ContextBackgroundWithNoOpLogger()
		coreDefaulter      *defaulter.CoreFractionDefaulter
		setupCoreDefaulter func(objs ...client.Object)
		newVM              func(className string, cores int, coreFraction string) *v1alpha2.VirtualMachine
		newVMClass         func(sizingPolicies []v1alpha3.SizingPolicy) *v1alpha3.VirtualMachineClass
	)

	BeforeEach(func() {
		setupCoreDefaulter = func(objs ...client.Object) {
			GinkgoHelper()
			fakeClient, err := testutil.NewFakeClientWithObjects(objs...)
			Expect(err).NotTo(HaveOccurred())
			coreDefaulter = defaulter.NewCoreFractionDefaulter(fakeClient)
		}

		newVM = func(className string, cores int, coreFraction string) *v1alpha2.VirtualMachine {
			return &v1alpha2.VirtualMachine{
				TypeMeta: metav1.TypeMeta{
					Kind:       v1alpha2.VirtualMachineKind,
					APIVersion: v1alpha2.SchemeGroupVersion.String(),
				},
				ObjectMeta: metav1.ObjectMeta{
					Name: "vm",
				},
				Spec: v1alpha2.VirtualMachineSpec{
					VirtualMachineClassName: className,
					CPU: v1alpha2.CPUSpec{
						Cores:        cores,
						CoreFraction: coreFraction,
					},
				},
			}
		}

		newVMClass = func(sizingPolicies []v1alpha3.SizingPolicy) *v1alpha3.VirtualMachineClass {
			return &v1alpha3.VirtualMachineClass{
				TypeMeta: metav1.TypeMeta{
					Kind:       v1alpha3.VirtualMachineClassKind,
					APIVersion: v1alpha3.SchemeGroupVersion.String(),
				},
				ObjectMeta: metav1.ObjectMeta{
					Name: "vmc",
				},
				Spec: v1alpha3.VirtualMachineClassSpec{
					SizingPolicies: sizingPolicies,
				},
			}
		}
	})

	AfterEach(func() {
		coreDefaulter = nil
	})

	Context("when coreFraction is already set", func() {
		It("should not change coreFraction and should not require VMClass", func() {
			setupCoreDefaulter()

			vm := newVM("any-class", 2, "25%")

			err := coreDefaulter.Default(ctx, vm)
			Expect(err).NotTo(HaveOccurred())
			Expect(vm.Spec.CPU.CoreFraction).To(Equal("25%"))
		})
	})

	Context("when virtualMachineClassName is empty", func() {
		It("should keep coreFraction empty", func() {
			setupCoreDefaulter()

			vm := newVM("", 2, "")

			err := coreDefaulter.Default(ctx, vm)
			Expect(err).NotTo(HaveOccurred())
			Expect(vm.Spec.CPU.CoreFraction).To(BeEmpty())
		})
	})

	Context("when VMClass cannot be found", func() {
		It("should return an error", func() {
			setupCoreDefaulter()

			vm := newVM("non-existing-class", 2, "")

			err := coreDefaulter.Default(ctx, vm)
			Expect(err).To(SatisfyAll(
				HaveOccurred(),
				MatchError(ContainSubstring("failed to get")),
				MatchError(ContainSubstring("not found")),
			))
		})
	})

	Context("when VMClass has sizing policies", func() {
		It("should set coreFraction from matching sizing policy defaultCoreFraction", func() {
			defaultCF := v1alpha3.CoreFractionValue("50%")
			vmClass := newVMClass([]v1alpha3.SizingPolicy{
				{
					Cores: &v1alpha3.SizingPolicyCores{
						Min: 1,
						Max: 4,
					},
					DefaultCoreFraction: &defaultCF,
				},
			})

			setupCoreDefaulter(vmClass)

			vm := newVM(vmClass.Name, 2, "")

			err := coreDefaulter.Default(ctx, vm)
			Expect(err).NotTo(HaveOccurred())
			Expect(vm.Spec.CPU.CoreFraction).To(Equal("50%"))
		})

		It("should set default 100% when matching policy has no defaultCoreFraction", func() {
			vmClass := newVMClass([]v1alpha3.SizingPolicy{
				{
					Cores: &v1alpha3.SizingPolicyCores{
						Min: 5,
						Max: 8,
					},
				},
			})

			setupCoreDefaulter(vmClass)

			vm := newVM(vmClass.Name, 6, "")

			err := coreDefaulter.Default(ctx, vm)
			Expect(err).NotTo(HaveOccurred())
			Expect(vm.Spec.CPU.CoreFraction).To(Equal("100%"))
		})

		It("should return error when no policy matches VM cores", func() {
			defaultCF := v1alpha3.CoreFractionValue("50%")
			vmClass := newVMClass([]v1alpha3.SizingPolicy{
				{
					Cores: &v1alpha3.SizingPolicyCores{
						Min: 5,
						Max: 8,
					},
					DefaultCoreFraction: &defaultCF,
				},
			})

			setupCoreDefaulter(vmClass)

			vm := newVM(vmClass.Name, 2, "")

			err := coreDefaulter.Default(ctx, vm)
			Expect(err).To(SatisfyAll(
				HaveOccurred(),
				MatchError(ContainSubstring("not among the sizing policies")),
			))
		})

		It("should set default 100% when coreFractions includes 100%", func() {
			vmClass := newVMClass([]v1alpha3.SizingPolicy{
				{
					Cores: &v1alpha3.SizingPolicyCores{
						Min: 1,
						Max: 4,
					},
					CoreFractions: []v1alpha3.CoreFractionValue{"50%", "100%"},
				},
			})

			setupCoreDefaulter(vmClass)

			vm := newVM(vmClass.Name, 2, "")

			err := coreDefaulter.Default(ctx, vm)
			Expect(err).NotTo(HaveOccurred())
			Expect(vm.Spec.CPU.CoreFraction).To(Equal("100%"))
		})

		It("should return error when coreFractions doesn't include 100% and no defaultCoreFraction", func() {
			vmClass := newVMClass([]v1alpha3.SizingPolicy{
				{
					Cores: &v1alpha3.SizingPolicyCores{
						Min: 1,
						Max: 4,
					},
					CoreFractions: []v1alpha3.CoreFractionValue{"25%", "50%"},
				},
			})

			setupCoreDefaulter(vmClass)

			vm := newVM(vmClass.Name, 2, "")

			err := coreDefaulter.Default(ctx, vm)
			Expect(err).To(SatisfyAll(
				HaveOccurred(),
				MatchError(ContainSubstring("default value for core fraction is not defined")),
			))
		})
	})

	Context("when VMClass has no sizing policies", func() {
		It("should set default 100%", func() {
			vmClass := newVMClass(nil)

			setupCoreDefaulter(vmClass)

			vm := newVM(vmClass.Name, 2, "")

			err := coreDefaulter.Default(ctx, vm)
			Expect(err).NotTo(HaveOccurred())
			Expect(vm.Spec.CPU.CoreFraction).Should(Equal("100%"))
		})
	})
})
