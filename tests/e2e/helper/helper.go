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
	"github.com/deckhouse/virtualization/tests/e2e/kubectl"
	. "github.com/onsi/ginkgo/v2"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/yaml"
	"os"
	"path/filepath"
	"strings"
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

func GetFullApiResourceName(u *unstructured.Unstructured) kubectl.Resource {
	return kubectl.Resource(strings.ToLower(u.GetKind() + "." + strings.Split(u.GetAPIVersion(), "/")[0]))
}
