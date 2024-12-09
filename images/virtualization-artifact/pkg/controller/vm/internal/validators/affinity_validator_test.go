package validators_test

import (
	"github.com/deckhouse/virtualization-controller/pkg/controller/vm/internal/validators"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("AffinityValidator", func() {
	validator := validators.NewAffinityValidator()

	Context("VM with no affinities", func() {
		//testAffinity := &v1alpha2.VMAffinity{}
		vm := &v1alpha2.VirtualMachine{}

		It("Should be no error", func() {
			warnings, err := validator.Validate(vm)
			Expect(len(warnings)).Should(BeNumerically("==", 0))
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
			Expect(len(warnings)).Should(BeNumerically("==", 0))
			Expect(err).ShouldNot(BeNil())
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
			Expect(len(warnings)).Should(BeNumerically("==", 0))
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
			Expect(len(warnings)).Should(BeNumerically("==", 0))
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
			Expect(len(warnings)).Should(BeNumerically("==", 0))
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
			Expect(len(warnings)).Should(BeNumerically("==", 0))
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
			Expect(len(warnings)).Should(BeNumerically("==", 0))
			Expect(err).ShouldNot(BeNil())
		})
	})

	Context("Correct input for ValidateLabelSelectorRequirement", func() {
		errors := validator.ValidateLabelSelectorRequirement(metav1.LabelSelectorRequirement{
			Operator: metav1.LabelSelectorOpExists,
		})

		It("Should be correct validation", func() {
			Expect(len(errors)).Should(BeNumerically("==", 0))
		})
	})

	Context("Incorrect input for ValidateLabelSelectorRequirement", func() {
		errors := validator.ValidateLabelSelectorRequirement(metav1.LabelSelectorRequirement{
			Operator: metav1.LabelSelectorOpIn,
		})

		It("Should be correct validation", func() {
			Expect(len(errors)).Should(BeNumerically(">", 0))
		})
	})
})
