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

package vmpool

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"

	apiextv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"sigs.k8s.io/yaml"
)

// TestVMPoolCRDContract guards the hand-tuned shape of the VirtualMachinePool CRD
// that update-codegen.sh produces via a post-process step. If someone regenerates
// the CRD without that step (or the schema path it edits moves), this fails —
// keeping the contract from being silently broken:
//   - virtualMachineTemplate.spec must NOT expose blockDeviceRefs (a pool derives
//     a replica's devices from virtualDiskTemplates; the field is stripped);
//   - virtualDiskTemplates is the sole source of devices → required, minItems>=1.
func TestVMPoolCRDContract(t *testing.T) {
	crd := loadPoolCRD(t)
	for _, v := range crd.Spec.Versions {
		if v.Schema == nil || v.Schema.OpenAPIV3Schema == nil {
			t.Fatalf("version %s: missing schema", v.Name)
		}
		specProps := v.Schema.OpenAPIV3Schema.Properties["spec"]

		tmplSpec := specProps.Properties["virtualMachineTemplate"].Properties["spec"]
		if _, ok := tmplSpec.Properties["blockDeviceRefs"]; ok {
			t.Errorf("version %s: virtualMachineTemplate.spec must NOT expose blockDeviceRefs (it must be stripped by update-codegen.sh)", v.Name)
		}
		if slices.Contains(tmplSpec.Required, "blockDeviceRefs") {
			t.Errorf("version %s: blockDeviceRefs must not be in the pool template's required list", v.Name)
		}

		vdt, ok := specProps.Properties["virtualDiskTemplates"]
		if !ok {
			t.Fatalf("version %s: virtualDiskTemplates property is missing", v.Name)
		}
		if vdt.MinItems == nil || *vdt.MinItems < 1 {
			t.Errorf("version %s: virtualDiskTemplates must have minItems>=1 (it is the sole source of devices)", v.Name)
		}
		if !slices.Contains(specProps.Required, "virtualDiskTemplates") {
			t.Errorf("version %s: virtualDiskTemplates must be required", v.Name)
		}
	}
}

func loadPoolCRD(t *testing.T) *apiextv1.CustomResourceDefinition {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed to locate the test source")
	}
	dir := filepath.Dir(thisFile)
	for {
		candidate := filepath.Join(dir, "crds", "virtualmachinepools.yaml")
		if _, err := os.Stat(candidate); err == nil {
			data, err := os.ReadFile(candidate)
			if err != nil {
				t.Fatalf("read %s: %v", candidate, err)
			}
			crd := &apiextv1.CustomResourceDefinition{}
			if err := yaml.Unmarshal(data, crd); err != nil {
				t.Fatalf("unmarshal %s: %v", candidate, err)
			}
			return crd
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("could not locate crds/virtualmachinepools.yaml walking up from %s", thisFile)
		}
		dir = parent
	}
}
