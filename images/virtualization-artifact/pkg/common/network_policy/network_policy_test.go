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

package networkpolicy

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/controller/supplements"
)

func TestPVCImporterIngressPeers(t *testing.T) {
	t.Run("allows CDI pods and the controller namespace", func(t *testing.T) {
		peers := PVCImporterIngressPeers("d8-virtualization")
		if len(peers) != 2 {
			t.Fatalf("expected 2 peers, got %d", len(peers))
		}
		cdiExpr := peers[0].PodSelector.MatchExpressions[0]
		if cdiExpr.Key != annotations.AppLabel ||
			cdiExpr.Operator != metav1.LabelSelectorOpIn ||
			len(cdiExpr.Values) != 1 || cdiExpr.Values[0] != annotations.CDILabelValue {
			t.Fatalf("unexpected CDI peer selector: %#v", cdiExpr)
		}
		wantNS := map[string]string{corev1.LabelMetadataName: "d8-virtualization"}
		if got := peers[1].NamespaceSelector.MatchLabels; got == nil || got[corev1.LabelMetadataName] != wantNS[corev1.LabelMetadataName] {
			t.Fatalf("unexpected controller namespace peer: %#v", got)
		}
	})

	t.Run("allows only CDI pods when controller namespace is empty", func(t *testing.T) {
		if len(PVCImporterIngressPeers("")) != 1 {
			t.Fatal("expected a single CDI peer when controller namespace is empty")
		}
	})
}

func TestCreateNetworkPolicy(t *testing.T) {
	sup := supplements.NewGenerator("vi", "test-vi", "my-project", "uid-1")

	// Build the CNP directly to assert its structure (CreateNetworkPolicy only wraps
	// newCiliumNetworkPolicy in a client.Create call).
	npName := sup.NetworkPolicy()
	cnp := newCiliumNetworkPolicy(npName, nil, []string{"test-finalizer"}, importerCNPSpec("d8-virtualization"))
	if cnp.GetKind() != "CiliumNetworkPolicy" || cnp.GetAPIVersion() != "cilium.io/v2" {
		t.Fatalf("unexpected GVK: kind=%q apiVersion=%q", cnp.GetKind(), cnp.GetAPIVersion())
	}
	if cnp.GetName() != npName.Name || cnp.GetNamespace() != npName.Namespace {
		t.Fatalf("unexpected name/namespace: %s/%s", cnp.GetNamespace(), cnp.GetName())
	}
	if fin := cnp.GetFinalizers(); len(fin) != 1 || fin[0] != "test-finalizer" {
		t.Fatalf("unexpected finalizers: %#v", fin)
	}

	spec, ok := cnp.Object["spec"].(map[string]interface{})
	if !ok {
		t.Fatalf("spec is not an object: %#v", cnp.Object["spec"])
	}

	sel, ok := spec["endpointSelector"].(map[string]interface{})
	if !ok {
		t.Fatalf("missing endpointSelector: %#v", spec)
	}
	exprs, _ := sel["matchExpressions"].([]interface{})
	if len(exprs) != 1 {
		t.Fatalf("expected 1 endpointSelector expression, got %d", len(exprs))
	}
	e, _ := exprs[0].(map[string]interface{})
	if e["key"] != annotations.AppLabel || e["operator"] != "In" {
		t.Fatalf("unexpected endpointSelector expression: %#v", e)
	}

	if _, ok := spec["egress"]; !ok {
		t.Fatal("missing egress")
	}
	dd, ok := spec["enableDefaultDeny"].(map[string]interface{})
	if !ok {
		t.Fatalf("missing enableDefaultDeny: %#v", spec)
	}
	if dd["ingress"] != true || dd["egress"] != false {
		t.Fatalf("unexpected enableDefaultDeny: %#v", dd)
	}

	in, ok := spec["ingress"].([]interface{})
	if !ok {
		t.Fatal("missing ingress rule")
	}
	if len(in) != 1 {
		t.Fatalf("expected 1 ingress rule, got %d", len(in))
	}
	rule, _ := in[0].(map[string]interface{})
	fromEndpoints, _ := rule["fromEndpoints"].([]interface{})
	if len(fromEndpoints) != 1 {
		t.Fatalf("expected 1 fromEndpoint, got %d", len(fromEndpoints))
	}
	ep, _ := fromEndpoints[0].(map[string]interface{})
	labels, _ := ep["matchLabels"].(map[string]interface{})
	if labels[ciliumNamespaceLabel] != "d8-virtualization" {
		t.Fatalf("unexpected controller namespace in ingress: %#v", labels)
	}
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
	if p["port"] != "8443" || p["protocol"] != "TCP" {
		t.Fatalf("unexpected metrics port: %#v", p)
	}
}

func TestImporterCNPSpecEmptyControllerNamespace(t *testing.T) {
	spec := importerCNPSpec("")
	if _, ok := spec["ingress"]; ok {
		t.Fatal("expected no ingress rule when controller namespace is empty")
	}
}

// Compile-time assertion that the unstructured helper produces a client.Object.
var _ client.Object = &unstructured.Unstructured{}

// TestGetNetworkPolicyReturnsNilInterfaceWhenAbsent is a regression test for the
// nil-interface trap: a nil *netv1.NetworkPolicy wrapped in a client.Object
// interface is NOT nil, so callers' "if obj != nil" checks would pass and then
// panic on a method call. GetNetworkPolicy must return a true nil interface
// (not a nil pointer in an interface) when neither the CiliumNetworkPolicy nor
// the legacy NetworkPolicy exists.
func TestGetNetworkPolicyReturnsNilInterfaceWhenAbsent(t *testing.T) {
	sup := supplements.NewGenerator("vi", "absent-vi", "my-project", "uid-absent")
	c := fake.NewClientBuilder().Build()

	obj, err := GetNetworkPolicy(t.Context(), c, sup.LegacyImporterPod(), sup)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// This must be a true nil interface, not a nil pointer wrapped in one.
	if obj != nil {
		t.Fatalf("expected nil interface when no policy exists, got %T: %#v", obj, obj)
	}
}

func TestGetNetworkPolicyFromObjectReturnsNilInterfaceWhenAbsent(t *testing.T) {
	sup := supplements.NewGenerator("vi", "absent-vi", "my-project", "uid-absent")
	c := fake.NewClientBuilder().Build()
	podKey := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "absent-pod", Namespace: "my-project"}}

	obj, err := GetNetworkPolicyFromObject(t.Context(), c, podKey, sup)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if obj != nil {
		t.Fatalf("expected nil interface when no policy exists, got %T: %#v", obj, obj)
	}
}
