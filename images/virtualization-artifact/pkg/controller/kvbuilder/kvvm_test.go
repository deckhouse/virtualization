package kvbuilder

import (
	"reflect"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
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
								MatchExpressions: getDefaultMatchExpressions(),
							},
							{
								MatchExpressions: []corev1.NodeSelectorRequirement{
									{
										Key:      "node-role.kubernetes.io/master",
										Operator: corev1.NodeSelectorOpIn,
										Values:   []string{""},
									},
								},
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
	}

	for _, test := range tests {
		builder := NewEmptyKVVM(types.NamespacedName{Name: name, Namespace: namespace}, KVVMOptions{})
		builder.SetAffinity(test.affinity, test.classMatchExpressions)
		if !reflect.DeepEqual(builder.Resource.Spec.Template.Spec.Affinity, test.expect) {
			t.Errorf("test %s failed.expected affinity %v, got %v", test.name, test.expect, builder.Resource.Spec.Template.Spec.Affinity)
		}
	}
}
