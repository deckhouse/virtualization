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

package migration

import (
	"context"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/common/testutil"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

func TestCoreFractionsFormat(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "CoreFractionsFormat Migration Suite")
}

var _ = Describe("CoreFractionsFormat Migration", func() {
	var (
		ctx    context.Context
		client client.Client
	)

	BeforeEach(func() {
		ctx = context.Background()
		var err error
		client, err = testutil.NewFakeClientWithObjects()
		Expect(err).NotTo(HaveOccurred())
	})

	Context("normalizeCoreFraction", func() {
		It("should add % sign to plain integer values", func() {
			result, changed := normalizeCoreFraction("5")
			Expect(changed).To(BeTrue())
			Expect(result).To(Equal("5%"))

			result, changed = normalizeCoreFraction("10")
			Expect(changed).To(BeTrue())
			Expect(result).To(Equal("10%"))

			result, changed = normalizeCoreFraction("100")
			Expect(changed).To(BeTrue())
			Expect(result).To(Equal("100%"))
		})

		It("should not change values that already have % sign", func() {
			result, changed := normalizeCoreFraction("5%")
			Expect(changed).To(BeFalse())
			Expect(result).To(Equal("5%"))

			result, changed = normalizeCoreFraction("25%")
			Expect(changed).To(BeFalse())
			Expect(result).To(Equal("25%"))
		})

		It("should handle values with spaces", func() {
			result, changed := normalizeCoreFraction(" 10 ")
			Expect(changed).To(BeTrue())
			Expect(result).To(Equal("10%"))

			result, changed = normalizeCoreFraction(" 50% ")
			Expect(changed).To(BeFalse())
			Expect(result).To(Equal("50%"))
		})

		It("should not change invalid values", func() {
			result, changed := normalizeCoreFraction("invalid")
			Expect(changed).To(BeFalse())
			Expect(result).To(Equal("invalid"))

			result, changed = normalizeCoreFraction("0")
			Expect(changed).To(BeFalse())
			Expect(result).To(Equal("0"))

			result, changed = normalizeCoreFraction("101")
			Expect(changed).To(BeFalse())
			Expect(result).To(Equal("101"))
		})
	})

	Context("Migrate", func() {
		It("should migrate VMClass with plain integer coreFractions", func() {
			vmc := &v1alpha2.VirtualMachineClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-class",
				},
				Spec: v1alpha2.VirtualMachineClassSpec{
					CPU: v1alpha2.CPU{Type: v1alpha2.CPUTypeHost},
					SizingPolicies: []v1alpha2.SizingPolicy{
						{
							Cores:         &v1alpha2.SizingPolicyCores{Min: 1, Max: 4},
							CoreFractions: []v1alpha2.CoreFractionValue{"5", "10", "25", "50", "100"},
						},
					},
				},
			}

			err := client.Create(ctx, vmc)
			Expect(err).NotTo(HaveOccurred())

			migration, err := newCoreFractionsFormat(client, testutil.NewNoOpLogger())
			Expect(err).NotTo(HaveOccurred())

			err = migration.Migrate(ctx)
			Expect(err).NotTo(HaveOccurred())

			updated := &v1alpha2.VirtualMachineClass{}
			err = client.Get(ctx, types.NamespacedName{Name: vmc.Name}, updated)
			Expect(err).NotTo(HaveOccurred())

			Expect(updated.Spec.SizingPolicies[0].CoreFractions).To(Equal([]v1alpha2.CoreFractionValue{
				"5%", "10%", "25%", "50%", "100%",
			}))
		})

		It("should not change VMClass with already formatted coreFractions", func() {
			vmc := &v1alpha2.VirtualMachineClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-class-formatted",
				},
				Spec: v1alpha2.VirtualMachineClassSpec{
					CPU: v1alpha2.CPU{Type: v1alpha2.CPUTypeHost},
					SizingPolicies: []v1alpha2.SizingPolicy{
						{
							Cores:         &v1alpha2.SizingPolicyCores{Min: 1, Max: 4},
							CoreFractions: []v1alpha2.CoreFractionValue{"5%", "10%", "25%", "50%", "100%"},
						},
					},
				},
			}

			err := client.Create(ctx, vmc)
			Expect(err).NotTo(HaveOccurred())

			migration, err := newCoreFractionsFormat(client, testutil.NewNoOpLogger())
			Expect(err).NotTo(HaveOccurred())

			err = migration.Migrate(ctx)
			Expect(err).NotTo(HaveOccurred())

			updated := &v1alpha2.VirtualMachineClass{}
			err = client.Get(ctx, types.NamespacedName{Name: vmc.Name}, updated)
			Expect(err).NotTo(HaveOccurred())

			Expect(updated.Spec.SizingPolicies[0].CoreFractions).To(Equal([]v1alpha2.CoreFractionValue{
				"5%", "10%", "25%", "50%", "100%",
			}))
		})

		It("should handle VMClass with multiple sizing policies", func() {
			vmc := &v1alpha2.VirtualMachineClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-class-multi",
				},
				Spec: v1alpha2.VirtualMachineClassSpec{
					CPU: v1alpha2.CPU{Type: v1alpha2.CPUTypeHost},
					SizingPolicies: []v1alpha2.SizingPolicy{
						{
							Cores:         &v1alpha2.SizingPolicyCores{Min: 1, Max: 4},
							CoreFractions: []v1alpha2.CoreFractionValue{"5", "10", "25"},
						},
						{
							Cores:         &v1alpha2.SizingPolicyCores{Min: 5, Max: 8},
							CoreFractions: []v1alpha2.CoreFractionValue{"50", "100"},
						},
					},
				},
			}

			err := client.Create(ctx, vmc)
			Expect(err).NotTo(HaveOccurred())

			migration, err := newCoreFractionsFormat(client, testutil.NewNoOpLogger())
			Expect(err).NotTo(HaveOccurred())

			err = migration.Migrate(ctx)
			Expect(err).NotTo(HaveOccurred())

			updated := &v1alpha2.VirtualMachineClass{}
			err = client.Get(ctx, types.NamespacedName{Name: vmc.Name}, updated)
			Expect(err).NotTo(HaveOccurred())

			Expect(updated.Spec.SizingPolicies[0].CoreFractions).To(Equal([]v1alpha2.CoreFractionValue{
				"5%", "10%", "25%",
			}))
			Expect(updated.Spec.SizingPolicies[1].CoreFractions).To(Equal([]v1alpha2.CoreFractionValue{
				"50%", "100%",
			}))
		})

		It("should handle VMClass without coreFractions", func() {
			vmc := &v1alpha2.VirtualMachineClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-class-no-fractions",
				},
				Spec: v1alpha2.VirtualMachineClassSpec{
					CPU: v1alpha2.CPU{Type: v1alpha2.CPUTypeHost},
					SizingPolicies: []v1alpha2.SizingPolicy{
						{
							Cores: &v1alpha2.SizingPolicyCores{Min: 1, Max: 4},
						},
					},
				},
			}

			err := client.Create(ctx, vmc)
			Expect(err).NotTo(HaveOccurred())

			migration, err := newCoreFractionsFormat(client, testutil.NewNoOpLogger())
			Expect(err).NotTo(HaveOccurred())

			err = migration.Migrate(ctx)
			Expect(err).NotTo(HaveOccurred())

			updated := &v1alpha2.VirtualMachineClass{}
			err = client.Get(ctx, types.NamespacedName{Name: vmc.Name}, updated)
			Expect(err).NotTo(HaveOccurred())

			Expect(updated.Spec.SizingPolicies[0].CoreFractions).To(BeNil())
		})
	})
})
