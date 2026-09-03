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
	"strconv"

	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	"github.com/deckhouse/virtualization-controller/pkg/controller/supplements"
)

// ciliumNetworkPolicyGVK is the CiliumNetworkPolicy type. It is built as an
// unstructured object to avoid pulling the cilium API go module into the
// controller (the same pattern the uploader uses for Gateway API HTTPRoute).
var ciliumNetworkPolicyGVK = schema.GroupVersionKind{
	Group:   "cilium.io",
	Version: "v2",
	Kind:    "CiliumNetworkPolicy",
}

// metricsPort is the container port importer/uploader pods serve /metrics on.
const metricsPort = 8443

// ciliumNamespaceLabel is the synthesized identity label Cilium attaches to
// every pod endpoint for namespace-based matching in fromEndpoints (Cilium's
// EndpointSelector has no namespaceSelector field, unlike the standard
// NetworkPolicy peer).
const ciliumNamespaceLabel = "io.kubernetes.pod.namespace"

// PVCImporterIngressPeers returns ingress peers for pvc-importer pods:
//   - other CDI-labelled importer pods (host-assigned source/target NBD traffic);
//   - the virtualization-controller namespace (progress metrics scrape).
func PVCImporterIngressPeers(controllerNamespace string) []netv1.NetworkPolicyPeer {
	peers := []netv1.NetworkPolicyPeer{{
		PodSelector: &metav1.LabelSelector{
			MatchExpressions: []metav1.LabelSelectorRequirement{{
				Key:      annotations.AppLabel,
				Operator: metav1.LabelSelectorOpIn,
				Values:   []string{annotations.CDILabelValue},
			}},
		},
	}}
	if controllerNamespace != "" {
		peers = append(peers, netv1.NetworkPolicyPeer{
			NamespaceSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					corev1.LabelMetadataName: controllerNamespace,
				},
			},
		})
	}
	return peers
}

// CreateNetworkPolicy creates a CiliumNetworkPolicy selecting the importer pod
// (by its app label) and allowing ingress from the virtualization-controller
// namespace on the metrics port plus all egress. The controller namespace is
// needed so the controller can scrape progress metrics from the importer pod in
// network-isolated namespaces (e.g. Deckhouse Projects with networkPolicy:
// Isolated) where a default-deny policy would otherwise block the scrape.
//
// The CiliumNetworkPolicy is built as an unstructured object to avoid a go
// dependency on the cilium API module (same pattern the uploader uses for the
// Gateway API HTTPRoute). It is owned by the pod (via OwnerReferences copied
// from obj) and carries the protection finalizer so it is cleaned up together
// with the pod.
func CreateNetworkPolicy(ctx context.Context, c client.Client, obj metav1.Object, sup supplements.Generator, controllerNamespace, finalizer string) error {
	npName := sup.NetworkPolicy()
	cnp := newCiliumNetworkPolicy(npName, obj.GetOwnerReferences(), []string{finalizer}, importerCNPSpec(controllerNamespace))
	return client.IgnoreAlreadyExists(c.Create(ctx, cnp))
}

// GetNetworkPolicy fetches the CiliumNetworkPolicy for the supplements, or the
// legacy standard NetworkPolicy at legacyName if the CNP is absent. Returns a
// true nil interface (not a nil pointer wrapped in client.Object) when neither
// exists, so callers' "if obj != nil" checks behave correctly.
func GetNetworkPolicy(ctx context.Context, client client.Client, legacyName types.NamespacedName, sup supplements.Generator) (client.Object, error) {
	cnp, err := object.FetchObject(ctx, sup.NetworkPolicy(), client, newCiliumNetworkPolicyZero())
	if err != nil {
		return nil, err
	}
	if cnp != nil {
		return cnp, nil
	}
	// Fall back to the legacy standard NetworkPolicy naming.
	np, err := object.FetchObject(ctx, legacyName, client, &netv1.NetworkPolicy{})
	if err != nil {
		return nil, err
	}
	if np == nil {
		return nil, nil
	}
	return np, nil
}

// GetNetworkPolicyFromObject is GetNetworkPolicy with the legacy lookup key
// derived from an existing object (the importer pod).
func GetNetworkPolicyFromObject(ctx context.Context, client client.Client, legacyObjectKey client.Object, sup supplements.Generator) (client.Object, error) {
	cnp, err := object.FetchObject(ctx, sup.NetworkPolicy(), client, newCiliumNetworkPolicyZero())
	if err != nil {
		return nil, err
	}
	if cnp != nil {
		return cnp, nil
	}
	// Fall back to the legacy standard NetworkPolicy naming.
	np, err := object.FetchObject(ctx, types.NamespacedName{Name: legacyObjectKey.GetName(), Namespace: legacyObjectKey.GetNamespace()}, client, &netv1.NetworkPolicy{})
	if err != nil {
		return nil, err
	}
	if np == nil {
		return nil, nil
	}
	return np, nil
}

// newCiliumNetworkPolicy builds an unstructured CiliumNetworkPolicy with the
// given name/namespace, owner references, finalizers and spec. The spec must be
// an object (map), as required by the CiliumNetworkPolicy CRD schema.
func newCiliumNetworkPolicy(name types.NamespacedName, ownerRefs []metav1.OwnerReference, finalizers []string, spec map[string]interface{}) *unstructured.Unstructured {
	cnp := &unstructured.Unstructured{}
	cnp.SetGroupVersionKind(ciliumNetworkPolicyGVK)
	cnp.SetName(name.Name)
	cnp.SetNamespace(name.Namespace)
	cnp.SetOwnerReferences(ownerRefs)
	cnp.SetFinalizers(finalizers)
	cnp.Object["spec"] = spec
	return cnp
}

// newCiliumNetworkPolicyZero returns a zero unstructured CiliumNetworkPolicy to
// fetch an existing one.
func newCiliumNetworkPolicyZero() *unstructured.Unstructured {
	cnp := &unstructured.Unstructured{}
	cnp.SetGroupVersionKind(ciliumNetworkPolicyGVK)
	return cnp
}

// importerCNPSpec builds the spec of the importer CiliumNetworkPolicy as an
// object (the CRD requires spec to be an object, not an array):
//   - endpointSelector: the importer app labels;
//   - ingress: from the controller namespace on the metrics port;
//   - egress: allow-all.
//
// The spec is built as unstructured maps/slices so the object can be applied
// without a cilium API go dependency.
func importerCNPSpec(controllerNamespace string) map[string]interface{} {
	spec := map[string]interface{}{
		"endpointSelector": map[string]interface{}{
			"matchExpressions": []interface{}{
				map[string]interface{}{
					"key":      annotations.AppLabel,
					"operator": "In",
					"values":   []interface{}{annotations.CDILabelValue, annotations.DVCRLabelValue},
				},
			},
		},
		// An egress rule with toEntities: [all] allows all egress (cluster pods, hosts,
		// world). The importer/uploader pods must reach DVCR and external data sources;
		// the project isolated NetworkPolicy imposes a default egress deny, so this
		// rule is required to override it (Cilium union: an allow from any policy wins).
		// Unlike the standard NetworkPolicy egress: [{}] (wildcard), a Cilium egress
		// rule with no destination matcher matches nothing — toEntities: [all] is the
		// Cilium equivalent of allow-all egress.
		"egress": []interface{}{map[string]interface{}{"toEntities": []interface{}{"all"}}},
		"enableDefaultDeny": map[string]interface{}{
			"ingress": true,
			"egress":  false,
		},
	}
	if controllerNamespace != "" {
		spec["ingress"] = []interface{}{
			map[string]interface{}{
				"fromEndpoints": []interface{}{
					map[string]interface{}{
						"matchLabels": map[string]interface{}{
							ciliumNamespaceLabel: controllerNamespace,
						},
					},
				},
				"toPorts": []interface{}{
					map[string]interface{}{
						"ports": []interface{}{
							map[string]interface{}{
								"port":     strconv.Itoa(metricsPort),
								"protocol": "TCP",
							},
						},
					},
				},
			},
		}
	}
	return spec
}
