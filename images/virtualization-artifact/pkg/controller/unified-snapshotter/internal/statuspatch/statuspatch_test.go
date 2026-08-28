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

package statuspatch

import (
	"encoding/json"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestForIncludesKindAndOnlyGivenStatusFields(t *testing.T) {
	type ownedStatus struct {
		Phase string `json:"phase,omitempty"`
	}

	gvk := schema.GroupVersionKind{Group: "virtualization.deckhouse.io", Version: "v1alpha2", Kind: "VirtualMachineSnapshot"}
	patch, err := For(gvk, ownedStatus{Phase: "Ready"})
	if err != nil {
		t.Fatal(err)
	}

	data, err := patch.Data(&corev1.ConfigMap{}) // Data() ignores its argument for RawPatch
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("patch bytes: %s", data)

	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	if m["kind"] != "VirtualMachineSnapshot" {
		t.Fatalf("kind missing or wrong: %v", m["kind"])
	}
	if m["apiVersion"] != "virtualization.deckhouse.io/v1alpha2" {
		t.Fatalf("apiVersion missing or wrong: %v", m["apiVersion"])
	}
	status, ok := m["status"].(map[string]any)
	if !ok || status["phase"] != "Ready" || len(status) != 1 {
		t.Fatalf("status should carry only the given field: %v", m["status"])
	}
}
