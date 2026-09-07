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

package nodeaffinity_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/deckhouse/virtualization-controller/pkg/common/nodeaffinity"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

var _ = Describe("MatchesVMPlacement", func() {
	makeNode := func(labels map[string]string, taints ...corev1.Taint) *corev1.Node {
		return &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: "node-1", Labels: labels},
			Spec:       corev1.NodeSpec{Taints: taints},
		}
	}

	It("returns true for a node matching all rules", func() {
		node := makeNode(map[string]string{"zone": "a"})
		vm := &v1alpha2.VirtualMachine{Spec: v1alpha2.VirtualMachineSpec{
			NodeSelector: map[string]string{"zone": "a"},
		}}
		vmClass := &v1alpha2.VirtualMachineClass{}
		match, err := nodeaffinity.MatchesVMPlacement(node, vm, vmClass)
		Expect(err).NotTo(HaveOccurred())
		Expect(match).To(BeTrue())
	})

	It("returns false when node selector does not match", func() {
		node := makeNode(map[string]string{"zone": "a"})
		vm := &v1alpha2.VirtualMachine{Spec: v1alpha2.VirtualMachineSpec{
			NodeSelector: map[string]string{"zone": "b"},
		}}
		match, err := nodeaffinity.MatchesVMPlacement(node, vm, &v1alpha2.VirtualMachineClass{})
		Expect(err).NotTo(HaveOccurred())
		Expect(match).To(BeFalse())
	})

	It("returns false when an untolerated NoSchedule taint is present", func() {
		node := makeNode(nil, corev1.Taint{Key: "gpu", Value: "true", Effect: corev1.TaintEffectNoSchedule})
		vm := &v1alpha2.VirtualMachine{}
		match, err := nodeaffinity.MatchesVMPlacement(node, vm, &v1alpha2.VirtualMachineClass{})
		Expect(err).NotTo(HaveOccurred())
		Expect(match).To(BeFalse())
	})

	It("tolerates a taint when toleration is present", func() {
		node := makeNode(nil, corev1.Taint{Key: "gpu", Value: "true", Effect: corev1.TaintEffectNoSchedule})
		vm := &v1alpha2.VirtualMachine{Spec: v1alpha2.VirtualMachineSpec{
			Tolerations: []corev1.Toleration{{Key: "gpu", Operator: corev1.TolerationOpEqual, Value: "true", Effect: corev1.TaintEffectNoSchedule}},
		}}
		match, err := nodeaffinity.MatchesVMPlacement(node, vm, &v1alpha2.VirtualMachineClass{})
		Expect(err).NotTo(HaveOccurred())
		Expect(match).To(BeTrue())
	})

	It("matches VM nodeAffinity", func() {
		node := makeNode(map[string]string{"zone": "a"})
		vm := &v1alpha2.VirtualMachine{Spec: v1alpha2.VirtualMachineSpec{
			Affinity: &v1alpha2.VMAffinity{
				NodeAffinity: &corev1.NodeAffinity{
					RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
						NodeSelectorTerms: []corev1.NodeSelectorTerm{{
							MatchExpressions: []corev1.NodeSelectorRequirement{{
								Key: "zone", Operator: corev1.NodeSelectorOpIn, Values: []string{"a"},
							}},
						}},
					},
				},
			},
		}}
		match, err := nodeaffinity.MatchesVMPlacement(node, vm, &v1alpha2.VirtualMachineClass{})
		Expect(err).NotTo(HaveOccurred())
		Expect(match).To(BeTrue())
	})

	It("rejects a node failing VM nodeAffinity", func() {
		node := makeNode(map[string]string{"zone": "b"})
		vm := &v1alpha2.VirtualMachine{Spec: v1alpha2.VirtualMachineSpec{
			Affinity: &v1alpha2.VMAffinity{
				NodeAffinity: &corev1.NodeAffinity{
					RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
						NodeSelectorTerms: []corev1.NodeSelectorTerm{{
							MatchExpressions: []corev1.NodeSelectorRequirement{{
								Key: "zone", Operator: corev1.NodeSelectorOpIn, Values: []string{"a"},
							}},
						}},
					},
				},
			},
		}}
		match, err := nodeaffinity.MatchesVMPlacement(node, vm, &v1alpha2.VirtualMachineClass{})
		Expect(err).NotTo(HaveOccurred())
		Expect(match).To(BeFalse())
	})

	It("returns error when VM nodeAffinity has a malformed operator", func() {
		node := makeNode(map[string]string{"zone": "a"})
		vm := &v1alpha2.VirtualMachine{Spec: v1alpha2.VirtualMachineSpec{
			Affinity: &v1alpha2.VMAffinity{
				NodeAffinity: &corev1.NodeAffinity{
					RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
						NodeSelectorTerms: []corev1.NodeSelectorTerm{{
							MatchExpressions: []corev1.NodeSelectorRequirement{{
								Key: "zone", Operator: "Bogus", Values: []string{"a"},
							}},
						}},
					},
				},
			},
		}}
		_, err := nodeaffinity.MatchesVMPlacement(node, vm, &v1alpha2.VirtualMachineClass{})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("VM affinity"))
	})

	It("matches VM class node selector (MatchLabels)", func() {
		node := makeNode(map[string]string{"role": "worker"})
		vmClass := &v1alpha2.VirtualMachineClass{Spec: v1alpha2.VirtualMachineClassSpec{
			NodeSelector: v1alpha2.NodeSelector{
				MatchLabels: map[string]string{"role": "worker"},
			},
		}}
		match, err := nodeaffinity.MatchesVMPlacement(node, &v1alpha2.VirtualMachine{}, vmClass)
		Expect(err).NotTo(HaveOccurred())
		Expect(match).To(BeTrue())
	})

	It("rejects node not matching VM class MatchLabels", func() {
		node := makeNode(map[string]string{"role": "controlplane"})
		vmClass := &v1alpha2.VirtualMachineClass{Spec: v1alpha2.VirtualMachineClassSpec{
			NodeSelector: v1alpha2.NodeSelector{
				MatchLabels: map[string]string{"role": "worker"},
			},
		}}
		match, err := nodeaffinity.MatchesVMPlacement(node, &v1alpha2.VirtualMachine{}, vmClass)
		Expect(err).NotTo(HaveOccurred())
		Expect(match).To(BeFalse())
	})

	It("returns error when VM class nodeSelector has a malformed operator", func() {
		node := makeNode(map[string]string{"zone": "a"})
		vmClass := &v1alpha2.VirtualMachineClass{Spec: v1alpha2.VirtualMachineClassSpec{
			NodeSelector: v1alpha2.NodeSelector{
				MatchExpressions: []corev1.NodeSelectorRequirement{{
					Key: "zone", Operator: "Bogus", Values: []string{"a"},
				}},
			},
		}}
		_, err := nodeaffinity.MatchesVMPlacement(node, &v1alpha2.VirtualMachine{}, vmClass)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("VM class node selector"))
	})
})

var _ = Describe("IntersectTerms", func() {
	It("returns nil for empty input", func() {
		Expect(nodeaffinity.IntersectTerms(nil)).To(BeNil())
		Expect(nodeaffinity.IntersectTerms([][]corev1.NodeSelectorTerm{})).To(BeNil())
	})

	It("returns the only term set unchanged", func() {
		terms := []corev1.NodeSelectorTerm{{
			MatchExpressions: []corev1.NodeSelectorRequirement{{Key: "a", Operator: corev1.NodeSelectorOpIn, Values: []string{"1"}}},
		}}
		result := nodeaffinity.IntersectTerms([][]corev1.NodeSelectorTerm{terms})
		Expect(result).To(Equal(terms))
	})

	It("computes the cross product of two term sets", func() {
		a := []corev1.NodeSelectorTerm{{
			MatchExpressions: []corev1.NodeSelectorRequirement{{Key: "a", Operator: corev1.NodeSelectorOpIn, Values: []string{"1"}}},
		}}
		b := []corev1.NodeSelectorTerm{{
			MatchExpressions: []corev1.NodeSelectorRequirement{{Key: "b", Operator: corev1.NodeSelectorOpIn, Values: []string{"2"}}},
		}}
		result := nodeaffinity.IntersectTerms([][]corev1.NodeSelectorTerm{a, b})
		Expect(result).To(HaveLen(1))
		Expect(result[0].MatchExpressions).To(HaveLen(2))
	})
})

var _ = Describe("PlacementTerms", func() {
	req := func(key string) corev1.NodeSelectorRequirement {
		return corev1.NodeSelectorRequirement{Key: key, Operator: corev1.NodeSelectorOpExists}
	}

	It("returns nothing when neither the machine nor its class restricts the node", func() {
		Expect(nodeaffinity.PlacementTerms(nil, nil)).To(BeEmpty())
		Expect(nodeaffinity.PlacementTerms(&corev1.Affinity{}, nil)).To(BeEmpty())
	})

	It("returns the terms of the machine when the class adds nothing", func() {
		affinity := &corev1.Affinity{NodeAffinity: &corev1.NodeAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
				NodeSelectorTerms: []corev1.NodeSelectorTerm{{MatchExpressions: []corev1.NodeSelectorRequirement{req("a")}}},
			},
		}}
		Expect(nodeaffinity.PlacementTerms(affinity, nil)).To(HaveLen(1))
	})

	It("turns the expressions of the class into a term of their own when the machine has none", func() {
		result := nodeaffinity.PlacementTerms(nil, []corev1.NodeSelectorRequirement{req("class")})
		Expect(result).To(HaveLen(1))
		Expect(result[0].MatchExpressions).To(ConsistOf(req("class")))
	})

	// A node has to satisfy the class whichever term of the machine it satisfies.
	It("adds the expressions of the class to every term of the machine", func() {
		affinity := &corev1.Affinity{NodeAffinity: &corev1.NodeAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
				NodeSelectorTerms: []corev1.NodeSelectorTerm{
					{MatchExpressions: []corev1.NodeSelectorRequirement{req("a")}},
					{MatchExpressions: []corev1.NodeSelectorRequirement{req("b")}},
				},
			},
		}}
		result := nodeaffinity.PlacementTerms(affinity, []corev1.NodeSelectorRequirement{req("class")})
		Expect(result).To(HaveLen(2))
		Expect(result[0].MatchExpressions).To(ConsistOf(req("a"), req("class")))
		Expect(result[1].MatchExpressions).To(ConsistOf(req("b"), req("class")))
	})

	It("leaves the terms of the machine untouched", func() {
		terms := []corev1.NodeSelectorTerm{{MatchExpressions: []corev1.NodeSelectorRequirement{req("a")}}}
		affinity := &corev1.Affinity{NodeAffinity: &corev1.NodeAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{NodeSelectorTerms: terms},
		}}
		nodeaffinity.PlacementTerms(affinity, []corev1.NodeSelectorRequirement{req("class")})
		Expect(terms[0].MatchExpressions).To(ConsistOf(req("a")))
	})
})

var _ = Describe("NarrowTerms", func() {
	terms := func(key string) []corev1.NodeSelectorTerm {
		return []corev1.NodeSelectorTerm{{
			MatchExpressions: []corev1.NodeSelectorRequirement{{Key: key, Operator: corev1.NodeSelectorOpExists}},
		}}
	}

	It("returns the other side when one of them restricts nothing", func() {
		Expect(nodeaffinity.NarrowTerms(nil, terms("b"))).To(Equal(terms("b")))
		Expect(nodeaffinity.NarrowTerms(terms("a"), nil)).To(Equal(terms("a")))
		Expect(nodeaffinity.NarrowTerms(nil, nil)).To(BeEmpty())
	})

	It("merges both sides into a term a node has to satisfy as a whole", func() {
		result := nodeaffinity.NarrowTerms(terms("a"), terms("b"))
		Expect(result).To(HaveLen(1))
		Expect(result[0].MatchExpressions).To(HaveLen(2))
	})
})
