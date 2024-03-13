package kubevirt

import (
	"fmt"
	"testing"

	"sigs.k8s.io/yaml"
)

func TestKubevirtRulesToYAML(t *testing.T) {
	b, err := yaml.Marshal(KubevirtRewriteRules)
	if err != nil {
		t.Fatalf("should marshal kubevirt rules without error: %v", err)
	}

	fmt.Printf("%s\n", string(b))
}
