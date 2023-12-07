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
