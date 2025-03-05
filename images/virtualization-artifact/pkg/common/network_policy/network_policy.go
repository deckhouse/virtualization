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

package networkpolicy

import (
	"context"

	netv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/object"
)

func CreateNetworkPolicy(ctx context.Context, c client.Client, obj metav1.Object, finalizer string) error {
	networkPolicy := netv1.NetworkPolicy{
		TypeMeta: metav1.TypeMeta{
			Kind:       "NetworkPolicy",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:            obj.GetName(),
			Namespace:       obj.GetNamespace(),
			OwnerReferences: obj.GetOwnerReferences(),
			Finalizers:      []string{finalizer},
		},
		Spec: netv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{
				MatchExpressions: []metav1.LabelSelectorRequirement{
					{
						Key:      annotations.AppLabel,
						Operator: metav1.LabelSelectorOpIn,
						Values:   []string{annotations.CDILabelValue, annotations.DVCRLabelValue},
					},
				},
			},
			Egress:      []netv1.NetworkPolicyEgressRule{{}},
			PolicyTypes: []netv1.PolicyType{netv1.PolicyTypeEgress},
		},
	}

	err := c.Create(ctx, &networkPolicy)
	return client.IgnoreAlreadyExists(err)
}

func GetNetworkPolicy(ctx context.Context, client client.Client, name types.NamespacedName) (*netv1.NetworkPolicy, error) {
	return object.FetchObject(ctx, name, client, &netv1.NetworkPolicy{})
}

func GetNetworkPolicyFromObject(ctx context.Context, client client.Client, obj client.Object) (*netv1.NetworkPolicy, error) {
	return object.FetchObject(ctx, types.NamespacedName{Name: obj.GetName(), Namespace: obj.GetNamespace()}, client, &netv1.NetworkPolicy{})
}
