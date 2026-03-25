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

package nodeaffinity

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	corev1helpers "k8s.io/component-helpers/scheduling/corev1"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

func IntersectTerms(perPVTerms [][]corev1.NodeSelectorTerm) []corev1.NodeSelectorTerm {
	if len(perPVTerms) == 0 {
		return nil
	}
	result := perPVTerms[0]
	for i := 1; i < len(perPVTerms); i++ {
		result = CrossProductTerms(result, perPVTerms[i])
	}
	return result
}

func MatchesVMPlacement(node *corev1.Node, vm *v1alpha2.VirtualMachine, vmClass *v1alpha2.VirtualMachineClass) bool {
	return matchesNodeSelector(node, vm.Spec.NodeSelector) &&
		matchesVMAffinity(node, vm.Spec.Affinity) &&
		matchesVMClassNodeSelector(node, vmClass) &&
		toleratesNodeTaints(node, vm.Spec.Tolerations)
}

func matchesNodeSelector(node *corev1.Node, nodeSelector map[string]string) bool {
	if len(nodeSelector) == 0 {
		return true
	}
	return labels.SelectorFromSet(nodeSelector).Matches(labels.Set(node.Labels))
}

func matchesVMAffinity(node *corev1.Node, affinity *v1alpha2.VMAffinity) bool {
	if affinity == nil || affinity.NodeAffinity == nil ||
		affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution == nil {
		return true
	}
	match, err := corev1helpers.MatchNodeSelectorTerms(node, affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution)
	if err != nil {
		return true
	}
	return match
}

func matchesVMClassNodeSelector(node *corev1.Node, vmClass *v1alpha2.VirtualMachineClass) bool {
	nodeSelector := vmClass.Spec.NodeSelector
	if len(nodeSelector.MatchLabels) > 0 {
		if !labels.SelectorFromSet(nodeSelector.MatchLabels).Matches(labels.Set(node.Labels)) {
			return false
		}
	}
	if len(nodeSelector.MatchExpressions) > 0 {
		match, err := corev1helpers.MatchNodeSelectorTerms(node, &corev1.NodeSelector{
			NodeSelectorTerms: []corev1.NodeSelectorTerm{{
				MatchExpressions: nodeSelector.MatchExpressions,
			}},
		})
		if err != nil {
			return true
		}
		return match
	}
	return true
}

func toleratesNodeTaints(node *corev1.Node, tolerations []corev1.Toleration) bool {
	_, untolerated := corev1helpers.FindMatchingUntoleratedTaint(
		node.Spec.Taints, tolerations,
		func(t *corev1.Taint) bool {
			return t.Effect == corev1.TaintEffectNoSchedule || t.Effect == corev1.TaintEffectNoExecute
		},
	)
	return !untolerated
}

func CrossProductTerms(a, b []corev1.NodeSelectorTerm) []corev1.NodeSelectorTerm {
	var result []corev1.NodeSelectorTerm
	for _, termA := range a {
		for _, termB := range b {
			merged := corev1.NodeSelectorTerm{
				MatchExpressions: append(
					append([]corev1.NodeSelectorRequirement{}, termA.MatchExpressions...),
					termB.MatchExpressions...,
				),
			}
			if len(termA.MatchFields) > 0 || len(termB.MatchFields) > 0 {
				merged.MatchFields = append(
					append([]corev1.NodeSelectorRequirement{}, termA.MatchFields...),
					termB.MatchFields...,
				)
			}
			result = append(result, merged)
		}
	}
	return result
}
