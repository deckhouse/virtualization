package rewriter

import (
	"os"

	"sigs.k8s.io/yaml"
)

func LoadRules(filename string) (*RewriteRules, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	var rules = new(RewriteRules)
	err = yaml.Unmarshal(data, rules)
	if err != nil {
		return nil, err
	}

	return rules, nil
}
