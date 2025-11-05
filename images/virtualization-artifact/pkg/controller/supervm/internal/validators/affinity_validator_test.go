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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"

	"github.com/deckhouse/virtualization-controller/pkg/controller/supervm/internal/validators"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

var _ = Describe("AffinityValidator", func() {
	validator := validators.NewAffinityValidator()

	Context("VM with no affinities", func() {
		vm := &v1alpha2.VirtualMachine{}

		It("Should be no error", func() {
			warnings, err := validator.Validate(vm)
			Expect(warnings).Should(BeEmpty())
			Expect(err).Should(BeNil())
		})
	})

	Context("VM with node affinity with nodeselectorterms empty requirement", func() {
		vm := &v1alpha2.VirtualMachine{
			Spec: v1alpha2.VirtualMachineSpec{
				Affinity: &v1alpha2.VMAffinity{
					NodeAffinity: &corev1.NodeAffinity{
						RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
							NodeSelectorTerms: []corev1.NodeSelectorTerm{},
						},
					},
				},
			},
		}

		It("Should return error", func() {
			warnings, err := validator.Validate(vm)
			Expect(warnings).Should(BeEmpty())
			Expect(err).Should(HaveOccurred())
		})
	})

	Context("VM with node affinity with correct nodeselectorterms requirement", func() {
		vm := &v1alpha2.VirtualMachine{
			Spec: v1alpha2.VirtualMachineSpec{
				Affinity: &v1alpha2.VMAffinity{
					NodeAffinity: &corev1.NodeAffinity{
						RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
							NodeSelectorTerms: []corev1.NodeSelectorTerm{
								{
									MatchExpressions: []corev1.NodeSelectorRequirement{
										{
											Key:      "key",
											Operator: corev1.NodeSelectorOpExists,
										},
									},
								},
							},
						},
					},
				},
			},
		}

		It("Should pass validation", func() {
			warnings, err := validator.Validate(vm)
			Expect(warnings).Should(BeEmpty())
			Expect(err).Should(BeNil())
		})
	})

	Context("VM with node affinity with incorrect nodeselectorterms requirement", func() {
		vm := &v1alpha2.VirtualMachine{
			Spec: v1alpha2.VirtualMachineSpec{
				Affinity: &v1alpha2.VMAffinity{
					NodeAffinity: &corev1.NodeAffinity{
						RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
							NodeSelectorTerms: []corev1.NodeSelectorTerm{
								{
									MatchExpressions: []corev1.NodeSelectorRequirement{
										{
											Key:      "key",
											Operator: corev1.NodeSelectorOpIn,
										},
									},
								},
							},
						},
					},
				},
			},
		}

		It("Should not pass validation", func() {
			warnings, err := validator.Validate(vm)
			Expect(warnings).Should(BeEmpty())
			Expect(err).ShouldNot(BeNil())
		})
	})

	Context("VM with node affinity with correct nodeselectorterms requirement", func() {
		vm := &v1alpha2.VirtualMachine{
			Spec: v1alpha2.VirtualMachineSpec{
				Affinity: &v1alpha2.VMAffinity{
					NodeAffinity: &corev1.NodeAffinity{
						PreferredDuringSchedulingIgnoredDuringExecution: []corev1.PreferredSchedulingTerm{
							{
								Weight: 50,
								Preference: corev1.NodeSelectorTerm{
									MatchExpressions: []corev1.NodeSelectorRequirement{
										{
											Key:      "key",
											Operator: corev1.NodeSelectorOpExists,
										},
									},
								},
							},
						},
					},
				},
			},
		}

		It("Should pass validation", func() {
			warnings, err := validator.Validate(vm)
			Expect(warnings).Should(BeEmpty())
			Expect(err).Should(BeNil())
		})
	})

	Context("VM with node affinity with incorrect nodeselectorterms requirement", func() {
		vm := &v1alpha2.VirtualMachine{
			Spec: v1alpha2.VirtualMachineSpec{
				Affinity: &v1alpha2.VMAffinity{
					NodeAffinity: &corev1.NodeAffinity{
						PreferredDuringSchedulingIgnoredDuringExecution: []corev1.PreferredSchedulingTerm{
							{
								Weight: 50,
								Preference: corev1.NodeSelectorTerm{
									MatchExpressions: []corev1.NodeSelectorRequirement{
										{
											Key:      "key",
											Operator: corev1.NodeSelectorOpIn,
										},
									},
								},
							},
						},
					},
				},
			},
		}

		It("Should not pass validation", func() {
			warnings, err := validator.Validate(vm)
			Expect(warnings).Should(BeEmpty())
			Expect(err).ShouldNot(BeNil())
		})
	})

	Context("VM with node affinity with incorrect nodeselectorterms requirement", func() {
		vm := &v1alpha2.VirtualMachine{
			Spec: v1alpha2.VirtualMachineSpec{
				Affinity: &v1alpha2.VMAffinity{
					NodeAffinity: &corev1.NodeAffinity{
						PreferredDuringSchedulingIgnoredDuringExecution: []corev1.PreferredSchedulingTerm{
							{
								Weight: -1,
								Preference: corev1.NodeSelectorTerm{
									MatchExpressions: []corev1.NodeSelectorRequirement{
										{
											Key:      "key",
											Operator: corev1.NodeSelectorOpExists,
										},
									},
								},
							},
						},
					},
				},
			},
		}

		It("Should not pass validation", func() {
			warnings, err := validator.Validate(vm)
			Expect(warnings).Should(BeEmpty())
			Expect(err).ShouldNot(BeNil())
		})
	})
})
