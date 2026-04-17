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

package kvbuilder

import (
	"reflect"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

func TestSetAffinity(t *testing.T) {
	name := "test-name"
	namespace := "test-namespace"

	getDefaultMatchExpressions := func() []corev1.NodeSelectorRequirement {
		return []corev1.NodeSelectorRequirement{
			{
				Key:      "node-role.kubernetes.io/worker",
				Operator: corev1.NodeSelectorOpIn,
				Values:   []string{""},
			},
		}
	}
	getDefaultAffinity := func() *corev1.Affinity {
		return &corev1.Affinity{
			NodeAffinity: &corev1.NodeAffinity{
				RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
					NodeSelectorTerms: []corev1.NodeSelectorTerm{
						{
							MatchExpressions: getDefaultMatchExpressions(),
						},
					},
				},
			},
		}
	}
	tests := []struct {
		name                  string
		expect                *corev1.Affinity
		affinity              *corev1.Affinity
		classMatchExpressions []corev1.NodeSelectorRequirement
	}{
		{
			name:                  "test affinity and classMatchExpressions is nil",
			expect:                nil,
			affinity:              nil,
			classMatchExpressions: nil,
		},
		{
			name:                  "test only affinity nil",
			expect:                getDefaultAffinity(),
			affinity:              nil,
			classMatchExpressions: getDefaultMatchExpressions(),
		},
		{
			name:                  "test only classMatchExpressions nil",
			expect:                getDefaultAffinity(),
			affinity:              getDefaultAffinity(),
			classMatchExpressions: nil,
		},
		{
			name: "test affinity and classMatchExpressions exist",
			expect: &corev1.Affinity{
				NodeAffinity: &corev1.NodeAffinity{
					RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
						NodeSelectorTerms: []corev1.NodeSelectorTerm{
							{
								MatchExpressions: append(getDefaultMatchExpressions(), corev1.NodeSelectorRequirement{
									Key:      "node-role.kubernetes.io/master",
									Operator: corev1.NodeSelectorOpIn,
									Values:   []string{""},
								}),
							},
						},
					},
				},
			},
			affinity: getDefaultAffinity(),
			classMatchExpressions: []corev1.NodeSelectorRequirement{
				{
					Key:      "node-role.kubernetes.io/master",
					Operator: corev1.NodeSelectorOpIn,
					Values:   []string{""},
				},
			},
		},
		{
			name:                  "test affinity is nil, but nodeAffinity nil",
			expect:                getDefaultAffinity(),
			affinity:              &corev1.Affinity{},
			classMatchExpressions: getDefaultMatchExpressions(),
		},
	}

	for _, test := range tests {
		builder := NewEmptyKVVM(types.NamespacedName{Name: name, Namespace: namespace}, KVVMOptions{})
		builder.SetAffinity(test.affinity, test.classMatchExpressions)
		if !reflect.DeepEqual(builder.Resource.Spec.Template.Spec.Affinity, test.expect) {
			t.Errorf("test %s failed.expected affinity %v, got %v", test.name, test.expect, builder.Resource.Spec.Template.Spec.Affinity)
		}
	}
}

func TestApplyPVNodeAffinity(t *testing.T) {
	nn := types.NamespacedName{Name: "test", Namespace: "test-ns"}

	pvTerm := func(key string, nodes ...string) corev1.NodeSelectorTerm {
		return corev1.NodeSelectorTerm{
			MatchExpressions: []corev1.NodeSelectorRequirement{{
				Key:      key,
				Operator: corev1.NodeSelectorOpIn,
				Values:   nodes,
			}},
		}
	}

	t.Run("No PV terms should not modify affinity", func(t *testing.T) {
		b := NewEmptyKVVM(nn, KVVMOptions{})
		b.ApplyPVNodeAffinity(nil)
		if b.Resource.Spec.Template.Spec.Affinity != nil {
			t.Error("affinity should remain nil when no PV terms provided")
		}
	})

	t.Run("No PV terms should preserve existing affinity", func(t *testing.T) {
		b := NewEmptyKVVM(nn, KVVMOptions{})
		existing := &corev1.Affinity{
			NodeAffinity: &corev1.NodeAffinity{
				RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
					NodeSelectorTerms: []corev1.NodeSelectorTerm{pvTerm("k", "v")},
				},
			},
		}
		b.Resource.Spec.Template.Spec.Affinity = existing
		b.ApplyPVNodeAffinity(nil)
		if !reflect.DeepEqual(b.Resource.Spec.Template.Spec.Affinity, existing) {
			t.Error("affinity should not change when no PV terms provided")
		}
	})

	t.Run("PV terms applied to empty affinity", func(t *testing.T) {
		b := NewEmptyKVVM(nn, KVVMOptions{})
		terms := []corev1.NodeSelectorTerm{pvTerm("topology/node", "node-1")}
		b.ApplyPVNodeAffinity(terms)

		a := b.Resource.Spec.Template.Spec.Affinity
		if a == nil || a.NodeAffinity == nil || a.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution == nil {
			t.Fatal("affinity should be set")
		}
		got := a.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms
		if !reflect.DeepEqual(got, terms) {
			t.Errorf("expected %v, got %v", terms, got)
		}
	})

	t.Run("PV terms merged with existing class affinity via cross-product", func(t *testing.T) {
		b := NewEmptyKVVM(nn, KVVMOptions{})
		classExpr := []corev1.NodeSelectorRequirement{{
			Key:      "node-role.kubernetes.io/control-plane",
			Operator: corev1.NodeSelectorOpDoesNotExist,
		}}
		b.SetAffinity(nil, classExpr)

		pvTerms := []corev1.NodeSelectorTerm{pvTerm("topology/node", "node-2")}
		b.ApplyPVNodeAffinity(pvTerms)

		a := b.Resource.Spec.Template.Spec.Affinity
		got := a.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms
		if len(got) != 1 {
			t.Fatalf("expected 1 term (cross-product of 1x1), got %d", len(got))
		}
		if len(got[0].MatchExpressions) != 2 {
			t.Errorf("expected 2 match expressions (class + PV), got %d", len(got[0].MatchExpressions))
		}
	})

	t.Run("PV terms cross-product with multiple existing terms", func(t *testing.T) {
		b := NewEmptyKVVM(nn, KVVMOptions{})
		b.Resource.Spec.Template.Spec.Affinity = &corev1.Affinity{
			NodeAffinity: &corev1.NodeAffinity{
				RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
					NodeSelectorTerms: []corev1.NodeSelectorTerm{
						pvTerm("zone", "us-east-1a"),
						pvTerm("zone", "us-east-1b"),
					},
				},
			},
		}

		pvTerms := []corev1.NodeSelectorTerm{
			pvTerm("topology/node", "node-1"),
			pvTerm("topology/node", "node-2"),
		}
		b.ApplyPVNodeAffinity(pvTerms)

		got := b.Resource.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms
		// 2 existing x 2 PV = 4 terms
		if len(got) != 4 {
			t.Fatalf("expected 4 terms (cross-product 2x2), got %d", len(got))
		}
		for i, term := range got {
			if len(term.MatchExpressions) != 2 {
				t.Errorf("term %d: expected 2 match expressions, got %d", i, len(term.MatchExpressions))
			}
		}
	})
}

func TestSetOsType(t *testing.T) {
	name := "test-name"
	namespace := "test-namespace"

	t.Run("Change from Windows to Generic should remove TPM", func(t *testing.T) {
		builder := NewEmptyKVVM(types.NamespacedName{Name: name, Namespace: namespace}, KVVMOptions{})

		err := builder.SetOSType(v1alpha2.Windows)
		if err != nil {
			t.Fatalf("SetOSType(Windows) failed: %v", err)
		}

		if builder.Resource.Spec.Template.Spec.Domain.Devices.TPM == nil {
			t.Error("TPM should be present after setting Windows OS")
		}

		err = builder.SetOSType(v1alpha2.GenericOs)
		if err != nil {
			t.Fatalf("SetOSType(GenericOs) failed: %v", err)
		}

		if builder.Resource.Spec.Template.Spec.Domain.Devices.TPM != nil {
			t.Error("TPM should be removed after changing from Windows to Generic OS")
		}
	})
}
