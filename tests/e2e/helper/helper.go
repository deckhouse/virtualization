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

package helper

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	"github.com/deckhouse/virtualization/tests/e2e/kubectl"
)

func GetFilesDir(yamlPath string) []string {
	files, err := filepath.Glob(yamlPath + "*.yaml")
	if err != nil {
		fmt.Println(GinkgoWriter)
	}

	return files
}

func ParseYaml(filepath string) ([]*unstructured.Unstructured, error) {
	data, err := os.ReadFile(filepath)
	if err != nil {
		return nil, err
	}
	out := make([]*unstructured.Unstructured, 0)
	manifests := strings.Split(string(data), "---")
	for _, m := range manifests {
		m := strings.TrimSpace(m)
		if len(m) == 0 {
			continue
		}
		var obj map[string]interface{}
		if err := yaml.Unmarshal([]byte(m), &obj); err != nil {
			return out, err
		}
		u := &unstructured.Unstructured{Object: obj}
		out = append(out, u)
	}
	return out, nil
}

func GetFullAPIResourceName(u *unstructured.Unstructured) kubectl.Resource {
	return kubectl.Resource(strings.ToLower(u.GetKind() + "." + strings.Split(u.GetAPIVersion(), "/")[0]))
}

func WriteYamlObject(filePath string, obj client.Object) error {
	data, err := yaml.Marshal(obj)
	if err != nil {
		return err
	}

	writeErr := os.WriteFile(filePath, data, 0o644)
	if writeErr != nil {
		return writeErr
	}

	return nil
}

func UnmarshalResource(filePath string, obj client.Object) error {
	file, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}

	err = yaml.Unmarshal(file, obj)
	if err != nil {
		return err
	}

	return nil
}
