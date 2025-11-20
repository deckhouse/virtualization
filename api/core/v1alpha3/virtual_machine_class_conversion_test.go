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

package v1alpha3

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

var _ = Describe("VirtualMachineClass Conversion", func() {
	Context("ConvertTo v1alpha2", func() {
		DescribeTable("should convert valid CoreFractionValue strings",
			func(coreFractions []CoreFractionValue) {
				v3Class := &VirtualMachineClass{
					ObjectMeta: metav1.ObjectMeta{Name: "test-class"},
					Spec: VirtualMachineClassSpec{
						SizingPolicies: []SizingPolicy{
							{
								CoreFractions: coreFractions,
								Cores: &SizingPolicyCores{
									Min:  1,
									Max:  8,
									Step: 1,
								},
							},
						},
					},
				}

				v2Class := &v1alpha2.VirtualMachineClass{}
				err := v3Class.ConvertTo(v2Class)

				Expect(err).NotTo(HaveOccurred())
				Expect(v2Class.Name).To(Equal(v3Class.Name))
				Expect(v2Class.Spec.SizingPolicies).To(HaveLen(1))
				Expect(v2Class.Spec.SizingPolicies[0].CoreFractions).To(HaveLen(len(coreFractions)))
			},
			Entry("single value", []CoreFractionValue{"5%"}),
			Entry("multiple values", []CoreFractionValue{"5%", "10%", "25%", "50%", "100%"}),
			Entry("minimum value 1%", []CoreFractionValue{"1%"}),
			Entry("maximum value 100%", []CoreFractionValue{"100%"}),
			Entry("mixed valid values", []CoreFractionValue{"1%", "50%", "100%"}),
			Entry("value without percent sign", []CoreFractionValue{"50"}),
		)

		DescribeTable("should fail on invalid CoreFractionValue strings",
			func(coreFractions []CoreFractionValue, expectedErrorSubstring string) {
				v3Class := &VirtualMachineClass{
					ObjectMeta: metav1.ObjectMeta{Name: "test-class"},
					Spec: VirtualMachineClassSpec{
						SizingPolicies: []SizingPolicy{
							{
								CoreFractions: coreFractions,
								Cores: &SizingPolicyCores{
									Min:  1,
									Max:  4,
									Step: 1,
								},
							},
						},
					},
				}

				v2Class := &v1alpha2.VirtualMachineClass{}
				err := v3Class.ConvertTo(v2Class)

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring(expectedErrorSubstring))
			},
			Entry("value below minimum (0%)", []CoreFractionValue{"0%"}, "must be between 1 and 100, got 0"),
			Entry("value above maximum (101%)", []CoreFractionValue{"101%"}, "must be between 1 and 100, got 101"),
			Entry("negative value", []CoreFractionValue{"-5%"}, "must be between 1 and 100, got -5"),
			Entry("non-numeric value", []CoreFractionValue{"abc%"}, "failed to parse core fraction"),
			Entry("empty string", []CoreFractionValue{""}, "failed to parse core fraction"),
			Entry("percent sign in wrong position", []CoreFractionValue{"%50"}, "failed to parse core fraction"),
			Entry("one invalid in multiple", []CoreFractionValue{"5%", "150%", "100%"}, "must be between 1 and 100, got 150"),
		)

		It("should preserve ObjectMeta", func() {
			v3Class := &VirtualMachineClass{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-class",
					Namespace: "test-ns",
					Labels: map[string]string{
						"test-label": "test-value",
					},
				},
				Spec: VirtualMachineClassSpec{},
			}

			v2Class := &v1alpha2.VirtualMachineClass{}
			err := v3Class.ConvertTo(v2Class)

			Expect(err).NotTo(HaveOccurred())
			Expect(v2Class.Name).To(Equal("test-class"))
			Expect(v2Class.Namespace).To(Equal("test-ns"))
			Expect(v2Class.Labels).To(HaveKeyWithValue("test-label", "test-value"))
		})
	})

	Context("ConvertFrom v1alpha2", func() {
		DescribeTable("should convert v1alpha2 CoreFractionValue integers to percentage strings",
			func(v2CoreFractions []v1alpha2.CoreFractionValue, expectedV3Values []CoreFractionValue) {
				v2Class := &v1alpha2.VirtualMachineClass{
					ObjectMeta: metav1.ObjectMeta{Name: "test-class"},
					Spec: v1alpha2.VirtualMachineClassSpec{
						SizingPolicies: []v1alpha2.SizingPolicy{
							{
								CoreFractions: v2CoreFractions,
								Cores: &v1alpha2.SizingPolicyCores{
									Min:  1,
									Max:  8,
									Step: 1,
								},
							},
						},
					},
				}

				v3Class := &VirtualMachineClass{}
				err := v3Class.ConvertFrom(v2Class)

				Expect(err).NotTo(HaveOccurred())
				Expect(v3Class.Spec.SizingPolicies).To(HaveLen(1))
				Expect(v3Class.Spec.SizingPolicies[0].CoreFractions).To(Equal(expectedV3Values))
			},
			Entry("single value", []v1alpha2.CoreFractionValue{5}, []CoreFractionValue{"5%"}),
			Entry("multiple values", []v1alpha2.CoreFractionValue{5, 10, 25, 50, 100}, []CoreFractionValue{"5%", "10%", "25%", "50%", "100%"}),
			Entry("minimum value", []v1alpha2.CoreFractionValue{1}, []CoreFractionValue{"1%"}),
			Entry("maximum value", []v1alpha2.CoreFractionValue{100}, []CoreFractionValue{"100%"}),
		)
	})

	Context("Round-trip conversion", func() {
		DescribeTable("should preserve values through v3 -> v2 -> v3 conversion",
			func(v3CoreFractions []CoreFractionValue) {
				originalV3 := &VirtualMachineClass{
					ObjectMeta: metav1.ObjectMeta{Name: "test-class"},
					Spec: VirtualMachineClassSpec{
						SizingPolicies: []SizingPolicy{
							{
								CoreFractions: v3CoreFractions,
								Cores: &SizingPolicyCores{
									Min:  1,
									Max:  8,
									Step: 1,
								},
							},
						},
					},
				}

				v2Class := &v1alpha2.VirtualMachineClass{}
				err := originalV3.ConvertTo(v2Class)
				Expect(err).NotTo(HaveOccurred())

				roundTripV3 := &VirtualMachineClass{}
				err = roundTripV3.ConvertFrom(v2Class)
				Expect(err).NotTo(HaveOccurred())

				Expect(roundTripV3.Spec.SizingPolicies).To(HaveLen(1))
				Expect(roundTripV3.Spec.SizingPolicies[0].CoreFractions).To(Equal(v3CoreFractions))
			},
			Entry("single value", []CoreFractionValue{"5%"}),
			Entry("multiple values", []CoreFractionValue{"5%", "10%", "25%", "50%", "100%"}),
			Entry("boundary values", []CoreFractionValue{"1%", "100%"}),
		)
	})

	Context("Full spec conversion", func() {
		var (
			minMem, maxMem, stepMem      resource.Quantity
			minPerCoreMem, maxPerCoreMem resource.Quantity
		)

		BeforeEach(func() {
			minMem = resource.MustParse("1Gi")
			maxMem = resource.MustParse("8Gi")
			stepMem = resource.MustParse("1Gi")
			minPerCoreMem = resource.MustParse("512Mi")
			maxPerCoreMem = resource.MustParse("2Gi")
		})

		It("should preserve all fields in ConvertTo", func() {
			v3Class := &VirtualMachineClass{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "full-test-class",
					Namespace: "test-namespace",
					Labels: map[string]string{
						"test-label": "test-value",
					},
					Annotations: map[string]string{
						"test-annotation": "test-value",
					},
				},
				Spec: VirtualMachineClassSpec{
					NodeSelector: NodeSelector{
						MatchLabels: map[string]string{
							"node-role": "worker",
							"zone":      "us-east-1a",
						},
						MatchExpressions: []corev1.NodeSelectorRequirement{
							{
								Key:      "cpu-type",
								Operator: corev1.NodeSelectorOpIn,
								Values:   []string{"intel", "amd"},
							},
						},
					},
					Tolerations: []corev1.Toleration{
						{
							Key:      "dedicated",
							Operator: corev1.TolerationOpEqual,
							Value:    "virtualization",
							Effect:   corev1.TaintEffectNoSchedule,
						},
					},
					CPU: CPU{
						Type:      CPUTypeModel,
						Model:     "IvyBridge",
						Features:  nil,
						Discovery: nil,
					},
					SizingPolicies: []SizingPolicy{
						{
							Memory: &SizingPolicyMemory{
								MemoryMinMax: MemoryMinMax{
									Min: minMem,
									Max: maxMem,
								},
								Step: stepMem,
								PerCore: SizingPolicyMemoryPerCore{
									MemoryMinMax: MemoryMinMax{
										Min: minPerCoreMem,
										Max: maxPerCoreMem,
									},
								},
							},
							CoreFractions:  []CoreFractionValue{"5%", "10%", "50%", "100%"},
							DedicatedCores: []bool{false, true},
							Cores: &SizingPolicyCores{
								Min:  1,
								Max:  16,
								Step: 2,
							},
						},
					},
				},
			}

			v2Class := &v1alpha2.VirtualMachineClass{}
			err := v3Class.ConvertTo(v2Class)
			Expect(err).NotTo(HaveOccurred())

			Expect(v2Class.Name).To(Equal("full-test-class"))
			Expect(v2Class.Namespace).To(Equal("test-namespace"))
			Expect(v2Class.Labels).To(HaveKeyWithValue("test-label", "test-value"))
			Expect(v2Class.Annotations).To(HaveKeyWithValue("test-annotation", "test-value"))

			Expect(v2Class.Spec.NodeSelector.MatchLabels).To(Equal(map[string]string{
				"node-role": "worker",
				"zone":      "us-east-1a",
			}))
			Expect(v2Class.Spec.NodeSelector.MatchExpressions).To(HaveLen(1))
			Expect(v2Class.Spec.NodeSelector.MatchExpressions[0].Key).To(Equal("cpu-type"))

			Expect(v2Class.Spec.Tolerations).To(HaveLen(1))
			Expect(v2Class.Spec.Tolerations[0].Key).To(Equal("dedicated"))
			Expect(v2Class.Spec.Tolerations[0].Value).To(Equal("virtualization"))

			Expect(string(v2Class.Spec.CPU.Type)).To(Equal("Model"))
			Expect(v2Class.Spec.CPU.Model).To(Equal("IvyBridge"))

			Expect(v2Class.Spec.SizingPolicies).To(HaveLen(1))
			policy := v2Class.Spec.SizingPolicies[0]

			Expect(policy.Memory).NotTo(BeNil())
			Expect(policy.Memory.Min.Equal(minMem)).To(BeTrue())
			Expect(policy.Memory.Max.Equal(maxMem)).To(BeTrue())
			Expect(policy.Memory.Step.Equal(stepMem)).To(BeTrue())
			Expect(policy.Memory.PerCore.Min.Equal(minPerCoreMem)).To(BeTrue())
			Expect(policy.Memory.PerCore.Max.Equal(maxPerCoreMem)).To(BeTrue())

			Expect(policy.CoreFractions).To(Equal([]v1alpha2.CoreFractionValue{5, 10, 50, 100}))
			Expect(policy.DedicatedCores).To(Equal([]bool{false, true}))

			Expect(policy.Cores).NotTo(BeNil())
			Expect(policy.Cores.Min).To(Equal(1))
			Expect(policy.Cores.Max).To(Equal(16))
			Expect(policy.Cores.Step).To(Equal(2))
		})

		It("should preserve all fields in ConvertFrom", func() {
			v2Class := &v1alpha2.VirtualMachineClass{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "full-test-class",
					Namespace: "test-namespace",
					Labels: map[string]string{
						"test-label": "test-value",
					},
				},
				Spec: v1alpha2.VirtualMachineClassSpec{
					NodeSelector: v1alpha2.NodeSelector{
						MatchLabels: map[string]string{
							"node-role": "worker",
						},
					},
					Tolerations: []corev1.Toleration{
						{
							Key:      "dedicated",
							Operator: corev1.TolerationOpEqual,
							Value:    "virtualization",
							Effect:   corev1.TaintEffectNoSchedule,
						},
					},
					CPU: v1alpha2.CPU{
						Type:     v1alpha2.CPUTypeFeatures,
						Features: []string{"mmx", "sse2", "vmx"},
					},
					SizingPolicies: []v1alpha2.SizingPolicy{
						{
							Memory: &v1alpha2.SizingPolicyMemory{
								MemoryMinMax: v1alpha2.MemoryMinMax{
									Min: minMem,
									Max: maxMem,
								},
								Step: stepMem,
								PerCore: v1alpha2.SizingPolicyMemoryPerCore{
									MemoryMinMax: v1alpha2.MemoryMinMax{
										Min: minPerCoreMem,
										Max: maxPerCoreMem,
									},
								},
							},
							CoreFractions:  []v1alpha2.CoreFractionValue{10, 50, 100},
							DedicatedCores: []bool{true},
							Cores: &v1alpha2.SizingPolicyCores{
								Min:  2,
								Max:  8,
								Step: 2,
							},
						},
					},
				},
			}

			v3Class := &VirtualMachineClass{}
			err := v3Class.ConvertFrom(v2Class)
			Expect(err).NotTo(HaveOccurred())

			Expect(v3Class.Name).To(Equal("full-test-class"))
			Expect(v3Class.Namespace).To(Equal("test-namespace"))
			Expect(v3Class.Labels).To(HaveKeyWithValue("test-label", "test-value"))

			Expect(v3Class.Spec.NodeSelector.MatchLabels).To(HaveKeyWithValue("node-role", "worker"))

			Expect(v3Class.Spec.Tolerations).To(HaveLen(1))
			Expect(v3Class.Spec.Tolerations[0].Key).To(Equal("dedicated"))

			Expect(string(v3Class.Spec.CPU.Type)).To(Equal("Features"))
			Expect(v3Class.Spec.CPU.Features).To(Equal([]string{"mmx", "sse2", "vmx"}))

			Expect(v3Class.Spec.SizingPolicies).To(HaveLen(1))
			policy := v3Class.Spec.SizingPolicies[0]

			Expect(policy.Memory).NotTo(BeNil())
			Expect(policy.Memory.Min.Equal(minMem)).To(BeTrue())
			Expect(policy.Memory.Max.Equal(maxMem)).To(BeTrue())
			Expect(policy.Memory.Step.Equal(stepMem)).To(BeTrue())

			Expect(policy.CoreFractions).To(Equal([]CoreFractionValue{"10%", "50%", "100%"}))
			Expect(policy.DedicatedCores).To(Equal([]bool{true}))

			Expect(policy.Cores).NotTo(BeNil())
			Expect(policy.Cores.Min).To(Equal(2))
			Expect(policy.Cores.Max).To(Equal(8))
			Expect(policy.Cores.Step).To(Equal(2))
		})
	})
})
