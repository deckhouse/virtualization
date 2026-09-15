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

package validate_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"k8s.io/apiextensions-apiserver/pkg/apis/apiextensions"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	structuralschema "k8s.io/apiextensions-apiserver/pkg/apiserver/schema"
	schemacel "k8s.io/apiextensions-apiserver/pkg/apiserver/schema/cel"
	"k8s.io/apimachinery/pkg/util/yaml"
	celconfig "k8s.io/apiserver/pkg/apis/cel"
)

// The snapshot CRDs carry their source-vs-mode rules in CEL, where nothing else can check them: the Go
// markers are only comments, and crds/virtualdisksnapshots.yaml is not even generated from them — it is
// hand-maintained, because that kind is not in update-codegen.sh's ALLOWED_RESOURCE_GEN_CRD. So the rules
// are evaluated here the way the apiserver evaluates them, against the YAML that actually ships.
//
// What the two rules have to get right is one case each, kept separate so a rejected object is told which
// one it broke: a capture needs exactly one source, and an import must carry none at all — `d8 snapshot`
// creates an import marker with nothing in spec but the mode.

// repoRoot walks up from the test's working directory to the checkout root, identified by its crds
// directory.
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("working directory: %v", err)
	}
	for {
		if info, err := os.Stat(filepath.Join(dir, "crds")); err == nil && info.IsDir() {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Skip("no crds directory above the working directory: not running from a repository checkout")
		}
		dir = parent
	}
}

// specValidator builds the CEL validator the apiserver would build for a CRD's spec schema.
func specValidator(t *testing.T, crdName string) (*schemacel.Validator, *structuralschema.Structural) {
	t.Helper()

	f, err := os.Open(filepath.Join(repoRoot(t), "crds", crdName))
	if err != nil {
		t.Fatalf("open the CRD: %v", err)
	}
	defer f.Close()

	var crd apiextensionsv1.CustomResourceDefinition
	if err := yaml.NewYAMLOrJSONDecoder(f, 4096).Decode(&crd); err != nil {
		t.Fatalf("decode %s: %v", crdName, err)
	}
	if len(crd.Spec.Versions) == 0 || crd.Spec.Versions[0].Schema == nil {
		t.Fatalf("%s has no versions[0].schema", crdName)
	}

	versioned := crd.Spec.Versions[0].Schema.OpenAPIV3Schema.Properties["spec"]
	var internal apiextensions.JSONSchemaProps
	if err := apiextensionsv1.Convert_v1_JSONSchemaProps_To_apiextensions_JSONSchemaProps(&versioned, &internal, nil); err != nil {
		t.Fatalf("convert the spec schema of %s: %v", crdName, err)
	}
	if len(internal.XValidations) == 0 {
		t.Fatalf("the spec schema of %s carries no x-kubernetes-validations", crdName)
	}

	structural, err := structuralschema.NewStructural(&internal)
	if err != nil {
		t.Fatalf("structural schema for %s: %v", crdName, err)
	}

	validator := schemacel.NewValidator(structural, true, celconfig.PerCallLimit)
	if validator == nil {
		t.Fatalf("no CEL validator was built for %s", crdName)
	}
	return validator, structural
}

type celCase struct {
	name string
	spec map[string]interface{}
	// want is a fragment of the message the rejection must carry, or "" when the spec must be accepted.
	// It is the message that is asserted, not just the rejection: a single merged rule would still reject
	// every invalid case here, while telling the user the wrong thing about which one they hit.
	want string
}

func runCELCases(t *testing.T, crdName string, cases []celCase) {
	t.Helper()
	validator, structural := specValidator(t, crdName)

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			errs, _ := validator.Validate(t.Context(), nil, structural, tt.spec, nil, celconfig.RuntimeCELCostBudget)

			var got []string
			for _, err := range errs {
				got = append(got, err.Error())
			}
			joined := strings.Join(got, " | ")

			if tt.want == "" {
				if joined != "" {
					t.Fatalf("rejected, want accepted: %s", joined)
				}
				return
			}
			if joined == "" {
				t.Fatalf("accepted, want a rejection mentioning %q", tt.want)
			}
			if !strings.Contains(joined, tt.want) {
				t.Fatalf("rejection is %q, want it to mention %q", joined, tt.want)
			}
		})
	}
}

func TestVirtualMachineSnapshotSpecCEL(t *testing.T) {
	sourceRef := map[string]interface{}{
		"apiVersion": "virtualization.deckhouse.io/v1alpha2",
		"kind":       "VirtualMachine",
		"name":       "vm",
	}

	runCELCases(t, "virtualmachinesnapshots.yaml", []celCase{
		{name: "a capture naming its machine", spec: map[string]interface{}{"mode": "Capture", "virtualMachineName": "vm"}},
		{name: "a capture referencing its machine", spec: map[string]interface{}{"mode": "Capture", "sourceRef": sourceRef}},
		// The whole of the marker `d8 snapshot` creates, and the reason the capture rule cannot simply
		// require a source.
		{name: "a bare import marker", spec: map[string]interface{}{"mode": "Import"}},

		{name: "a capture naming nothing", spec: map[string]interface{}{"mode": "Capture"}, want: "exactly one of"},
		{
			name: "a capture naming its machine twice over",
			spec: map[string]interface{}{"mode": "Capture", "virtualMachineName": "vm", "sourceRef": sourceRef},
			want: "exactly one of",
		},
		{
			name: "an import naming a machine",
			spec: map[string]interface{}{"mode": "Import", "virtualMachineName": "vm"},
			want: "must both be unset",
		},
		{
			name: "an import referencing a machine",
			spec: map[string]interface{}{"mode": "Import", "sourceRef": sourceRef},
			want: "must both be unset",
		},

		// mode is defaulted before validation runs, so a spec without it is not reachable through the
		// API. These pin that the rules read it defensively anyway: an unguarded self.mode would raise a
		// CEL evaluation error here rather than reject the object cleanly.
		{name: "no mode, naming a machine", spec: map[string]interface{}{"virtualMachineName": "vm"}},
		{name: "no mode, naming nothing", spec: map[string]interface{}{}, want: "exactly one of"},
	})
}

func TestVirtualDiskSnapshotSpecCEL(t *testing.T) {
	sourceRef := map[string]interface{}{
		"apiVersion": "virtualization.deckhouse.io/v1alpha2",
		"kind":       "VirtualDisk",
		"name":       "vd",
	}

	runCELCases(t, "virtualdisksnapshots.yaml", []celCase{
		{name: "a capture naming its disk", spec: map[string]interface{}{"mode": "Capture", "virtualDiskName": "vd"}},
		{name: "a capture referencing its disk", spec: map[string]interface{}{"mode": "Capture", "sourceRef": sourceRef}},
		{name: "a bare import marker", spec: map[string]interface{}{"mode": "Import"}},

		{name: "a capture naming nothing", spec: map[string]interface{}{"mode": "Capture"}, want: "exactly one of"},
		{
			name: "a capture naming its disk twice over",
			spec: map[string]interface{}{"mode": "Capture", "virtualDiskName": "vd", "sourceRef": sourceRef},
			want: "exactly one of",
		},
		{
			name: "an import naming a disk",
			spec: map[string]interface{}{"mode": "Import", "virtualDiskName": "vd"},
			want: "must both be unset",
		},
		{
			name: "an import referencing a disk",
			spec: map[string]interface{}{"mode": "Import", "sourceRef": sourceRef},
			want: "must both be unset",
		},

		{name: "no mode, naming a disk", spec: map[string]interface{}{"virtualDiskName": "vd"}},
		{name: "no mode, naming nothing", spec: map[string]interface{}{}, want: "exactly one of"},
	})
}
