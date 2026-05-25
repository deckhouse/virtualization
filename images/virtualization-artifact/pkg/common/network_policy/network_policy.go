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

	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	"github.com/deckhouse/virtualization-controller/pkg/controller/supplements"
)

const (
	moduleNamespaceLabelName = "module"
	moduleVirtualization     = "virtualization"

	provisioningMetricsPort = 8443
	uploaderPort            = 8444
)

func CreateNetworkPolicy(ctx context.Context, c client.Client, obj metav1.Object, sup supplements.DataVolumeSupplement, finalizer string) error {
	npName := sup.NetworkPolicy()
	networkPolicy := netv1.NetworkPolicy{
		TypeMeta: metav1.TypeMeta{
			Kind:       "NetworkPolicy",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:            npName.Name,
			Namespace:       npName.Namespace,
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
			Ingress: []netv1.NetworkPolicyIngressRule{
				{
					From: []netv1.NetworkPolicyPeer{
						{
							NamespaceSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									moduleNamespaceLabelName: moduleVirtualization,
								},
							},
						},
					},
					Ports: []netv1.NetworkPolicyPort{
						tcpPort(provisioningMetricsPort),
					},
				},
				{
					From: []netv1.NetworkPolicyPeer{
						{
							NamespaceSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									annotations.HeritageLabel: annotations.HeritageValue,
								},
							},
						},
					},
					Ports: []netv1.NetworkPolicyPort{
						tcpPort(uploaderPort),
					},
				},
			},
			Egress:      []netv1.NetworkPolicyEgressRule{{}},
			PolicyTypes: []netv1.PolicyType{netv1.PolicyTypeIngress, netv1.PolicyTypeEgress},
		},
	}

	err := c.Create(ctx, &networkPolicy)
	return client.IgnoreAlreadyExists(err)
}

func tcpPort(port int) netv1.NetworkPolicyPort {
	protocol := corev1.ProtocolTCP
	targetPort := intstr.FromInt(port)

	return netv1.NetworkPolicyPort{
		Protocol: &protocol,
		Port:     &targetPort,
	}
}

func GetNetworkPolicy(ctx context.Context, client client.Client, legacyName types.NamespacedName, sup supplements.Generator) (*netv1.NetworkPolicy, error) {
	np, err := object.FetchObject(ctx, sup.NetworkPolicy(), client, &netv1.NetworkPolicy{})
	if err != nil {
		return nil, err
	}
	if np != nil {
		return np, nil
	}

	// Return object with legacy naming otherwise
	return object.FetchObject(ctx, legacyName, client, &netv1.NetworkPolicy{})
}

func GetNetworkPolicyFromObject(ctx context.Context, client client.Client, legacyObjectKey client.Object, sup supplements.Generator) (*netv1.NetworkPolicy, error) {
	np, err := object.FetchObject(ctx, sup.NetworkPolicy(), client, &netv1.NetworkPolicy{})
	if err != nil {
		return nil, err
	}
	if np != nil {
		return np, nil
	}

	// Return object with legacy naming otherwise
	return object.FetchObject(ctx, types.NamespacedName{Name: legacyObjectKey.GetName(), Namespace: legacyObjectKey.GetNamespace()}, client, &netv1.NetworkPolicy{})
}
