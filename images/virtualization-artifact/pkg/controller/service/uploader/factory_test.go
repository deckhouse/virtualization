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

package uploader

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/controller/supplements"
)

// newTestFactory builds a factory with the given network-policy settings so the
// uploader CiliumNetworkPolicy structure can be asserted without a real client.
func newTestFactory(sup supplements.Generator, nps *NetworkPolicySettings) Factory {
	return NewFactory(
		sup,
		PodSettings{},
		IngressSettings{},
		ListenerSetSettings{},
		nps,
		metav1.OwnerReference{},
	)
}

func TestNetworkPolicyIsCiliumNetworkPolicy(t *testing.T) {
	sup := supplements.NewGenerator("vi", "test-vi", "my-project", "uid-1")
	f := newTestFactory(sup, &NetworkPolicySettings{
		ControllerNamespace: "d8-virtualization",
		OwnNamespace:        "my-project",
	})

	np := f.NetworkPolicy()
	if np.GetKind() != "CiliumNetworkPolicy" || np.GetAPIVersion() != "cilium.io/v2" {
		t.Fatalf("unexpected GVK: kind=%q apiVersion=%q", np.GetKind(), np.GetAPIVersion())
	}
	name := sup.NetworkPolicy()
	if np.GetName() != name.Name || np.GetNamespace() != name.Namespace {
		t.Fatalf("unexpected name/namespace: %s/%s", np.GetNamespace(), np.GetName())
	}
	if l := np.GetLabels(); l[annotations.AppLabel] != annotations.DVCRLabelValue || l[annotations.HeritageLabel] != annotations.HeritageValue {
		t.Fatalf("unexpected labels: %#v", l)
	}
}

// ingressRuleBySourceNamespace returns the first toPorts-bearing ingress rule whose
// fromEndpoints select the given namespace, or nil.
func ingressRuleBySourceNamespace(spec map[string]interface{}, ns string) map[string]interface{} {
	in, ok := spec["ingress"].([]interface{})
	if !ok {
		return nil
	}
	for _, r := range in {
		rm, _ := r.(map[string]interface{})
		fe, _ := rm["fromEndpoints"].([]interface{})
		for _, ep := range fe {
			epm, _ := ep.(map[string]interface{})
			labels, _ := epm["matchLabels"].(map[string]interface{})
			if labels[ciliumNamespaceLabel] == ns {
				return rm
			}
		}
	}
	return nil
}

// entitiesRule returns the first ingress rule with fromEntities, or nil.
func entitiesRule(spec map[string]interface{}) map[string]interface{} {
	in, ok := spec["ingress"].([]interface{})
	if !ok {
		return nil
	}
	for _, r := range in {
		rm, _ := r.(map[string]interface{})
		if _, ok := rm["fromEntities"]; ok {
			return rm
		}
	}
	return nil
}

func assertPort(t *testing.T, rule map[string]interface{}, wantPort string) {
	t.Helper()
	toPorts, _ := rule["toPorts"].([]interface{})
	if len(toPorts) != 1 {
		t.Fatalf("expected 1 toPorts, got %d", len(toPorts))
	}
	tp, _ := toPorts[0].(map[string]interface{})
	ports, _ := tp["ports"].([]interface{})
	if len(ports) != 1 {
		t.Fatalf("expected 1 port, got %d", len(ports))
	}
	p, _ := ports[0].(map[string]interface{})
	if p["port"] != wantPort || p["protocol"] != "TCP" {
		t.Fatalf("unexpected port: %#v", p)
	}
}

func TestUploaderCNPSpecIngressPath(t *testing.T) {
	nps := &NetworkPolicySettings{
		ControllerNamespace: "d8-virtualization",
		IngressNamespace:    "d8-ingress-nginx",
		OwnNamespace:        "my-project",
		UseAPIGateway:       false,
	}
	spec := uploaderCNPSpec(nps)

	// Controller namespace: metrics (8443) + upload (8444).
	ctrlRule := ingressRuleBySourceNamespace(spec, "d8-virtualization")
	if ctrlRule == nil {
		t.Fatal("missing ingress rule for controller namespace")
	}
	assertPort(t, ctrlRule, "8443")

	// Ingress-nginx: upload (8444).
	ingRule := ingressRuleBySourceNamespace(spec, "d8-ingress-nginx")
	if ingRule == nil {
		t.Fatal("missing ingress rule for ingress-nginx namespace")
	}
	assertPort(t, ingRule, "8444")

	// Gateway namespace must NOT be present on the Ingress path.
	if r := ingressRuleBySourceNamespace(spec, "d8-alb"); r != nil {
		t.Fatal("unexpected ingress rule for gateway namespace on the Ingress path")
	}

	// Own namespace: upload (8444).
	ownRule := ingressRuleBySourceNamespace(spec, "my-project")
	if ownRule == nil {
		t.Fatal("missing ingress rule for own namespace")
	}
	assertPort(t, ownRule, "8444")

	// Cluster nodes (host-network in-cluster upload): upload (8444).
	clusterRule := entitiesRule(spec)
	if clusterRule == nil {
		t.Fatal("missing fromEntities ingress rule for cluster nodes")
	}
	assertPort(t, clusterRule, "8444")
	entities, _ := clusterRule["fromEntities"].([]interface{})
	if len(entities) != 1 || entities[0] != "cluster" {
		t.Fatalf("unexpected fromEntities: %#v", entities)
	}
}

func TestUploaderCNPSpecGatewayPath(t *testing.T) {
	nps := &NetworkPolicySettings{
		ControllerNamespace: "d8-virtualization",
		GatewayNamespace:    "d8-alb",
		OwnNamespace:        "my-project",
		UseAPIGateway:       true,
	}
	spec := uploaderCNPSpec(nps)

	// Gateway namespace: upload (8444).
	gwRule := ingressRuleBySourceNamespace(spec, "d8-alb")
	if gwRule == nil {
		t.Fatal("missing ingress rule for gateway namespace")
	}
	assertPort(t, gwRule, "8444")

	// Ingress-nginx must NOT be present on the API-Gateway path.
	if r := ingressRuleBySourceNamespace(spec, "d8-ingress-nginx"); r != nil {
		t.Fatal("unexpected ingress rule for ingress-nginx namespace on the API-Gateway path")
	}
}

func TestUploaderCNPSpecEmptySourcesSkip(t *testing.T) {
	// Only the controller namespace + cluster entities survive; empty ingress/gateway
	// namespaces skip their rules.
	nps := &NetworkPolicySettings{
		ControllerNamespace: "d8-virtualization",
		UseAPIGateway:       true,
	}
	spec := uploaderCNPSpec(nps)
	if r := ingressRuleBySourceNamespace(spec, "d8-alb"); r != nil {
		t.Fatal("expected no gateway rule when gateway namespace is empty")
	}
	// Controller + cluster rules still present.
	if r := ingressRuleBySourceNamespace(spec, "d8-virtualization"); r == nil {
		t.Fatal("missing controller ingress rule")
	}
	if r := entitiesRule(spec); r == nil {
		t.Fatal("missing cluster entities ingress rule")
	}
}

func TestUploaderCNPSpecNilSettings(t *testing.T) {
	spec := uploaderCNPSpec(nil)
	// No ingress rules when settings are nil, but egress + selector + defaultDeny remain.
	if _, ok := spec["ingress"]; ok {
		t.Fatal("expected no ingress rules when settings are nil")
	}
}

// Compile-time assertion that the unstructured helper produces a client.Object.
var _ interface{ GetKind() string } = &unstructured.Unstructured{}
