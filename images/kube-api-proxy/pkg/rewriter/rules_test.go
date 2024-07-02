package rewriter

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func newTestExcludeRules() *RewriteRules {
	rules := RewriteRules{
		Rules: map[string]APIGroupRule{
			"originalgroup.io": {
				ResourceRules: map[string]ResourceRule{
					"someresources": {
						Kind:     "SomeResource",
						ListKind: "SomeResourceList",
					},
				},
			},
			"anothergroup.io": {
				ResourceRules: map[string]ResourceRule{
					"anotheresources": {
						Kind:     "AnotherResource",
						ListKind: "AnotherResourceList",
					},
				},
			},
		},
		Excludes: []ExcludeRule{
			{
				Kinds: []string{"RoleBinding"},
				MatchLabels: map[string]string{
					"labelName": "labelValue",
				},
			},
			{
				Kinds:      []string{"Role"},
				MatchNames: []string{"role1", "role2"},
			},
		},
	}
	rules.Init()
	return &rules
}

func TestExcludeRuleKindsOnly(t *testing.T) {
	rules := newTestExcludeRules()

	tests := []struct {
		name           string
		obj            string
		expectExcluded bool
	}{
		{
			"original kind SomeResource in excludes",
			`{"kind":"SomeResource"}`,
			true,
		},
		{
			"kind UnknownResource not in excludes",
			`{"kind":"UnknownResource"}`,
			false,
		},
		{
			"RoleBinding with label in excludes",
			`{"kind":"RoleBinding","metadata":{"labels":{"labelName":"labelValue"}}}`,
			true,
		},
		{
			"RoleBinding with label not in excludes",
			`{"kind":"RoleBinding","metadata":{"labels":{"labelName":"nonExcludedValue"}}}`,
			false,
		},
		{
			"Role with name in excludes",
			`{"kind":"Role","metadata":{"name":"role1"}}`,
			true,
		},
		{
			"Role with name not in excludes",
			`{"kind":"Role","metadata":{"name":"role-not-excluded"}}`,
			false,
		},
		{
			"RoleBinding with name as role in excludes",
			`{"kind":"RoleBinding","metadata":{"name":"role1"}}`,
			false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := rules.ShouldExclude([]byte(tt.obj))

			if tt.expectExcluded {
				require.True(t, actual, "'%s' should be excluded. Not excluded obj: %s", tt.name, tt.obj)
			} else {
				require.False(t, actual, "'%s' should not be excluded. Excluded obj: %s", tt.name, tt.obj)

			}
		})
	}
}
