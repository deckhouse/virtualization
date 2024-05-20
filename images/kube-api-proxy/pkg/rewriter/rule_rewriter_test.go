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

package rewriter

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"
)

func createTestRewriter() *RuleBasedRewriter {
	apiGroupRules := map[string]APIGroupRule{
		"original.group.io": {
			GroupRule: GroupRule{
				Group:            "original.group.io",
				Versions:         []string{"v1", "v1alpha1"},
				PreferredVersion: "v1",
			},
			ResourceRules: map[string]ResourceRule{
				"someresources": {
					Kind:             "SomeResource",
					ListKind:         "SomeResourceList",
					Plural:           "someresources",
					Singular:         "someresource",
					Versions:         []string{"v1", "v1alpha1"},
					PreferredVersion: "v1",
					Categories:       []string{"all"},
					ShortNames:       []string{"sr", "srs"},
				},
				"anotherresources": {
					Kind:             "AnotherResource",
					ListKind:         "AnotherResourceList",
					Plural:           "anotherresources",
					Singular:         "anotherresource",
					Versions:         []string{"v1", "v1alpha1"},
					PreferredVersion: "v1",
					ShortNames:       []string{"ar"},
				},
			},
		},
		"other.group.io": {
			GroupRule: GroupRule{
				Group:            "original.group.io",
				Versions:         []string{"v2alpha3"},
				PreferredVersion: "v2alpha3",
			},
			ResourceRules: map[string]ResourceRule{
				"otherresources": {
					Kind:             "OtherResource",
					ListKind:         "OtherResourceList",
					Plural:           "otherresources",
					Singular:         "otherresource",
					Versions:         []string{"v1", "v1alpha1"},
					PreferredVersion: "v1",
					ShortNames:       []string{"or"},
				},
			},
		},
	}

	rules := &RewriteRules{
		KindPrefix:         "Prefixed", // KV
		ResourceTypePrefix: "prefixed", // kv
		ShortNamePrefix:    "p",
		Categories:         []string{"prefixed"},
		RenamedGroup:       "prefixed.resources.group.io",
		Rules:              apiGroupRules,
		Labels: MetadataReplace{
			Prefixes: []MetadataReplaceRule{
				{Old: "original.prefix", New: "rewrite.prefix"},
			},
			Names: []MetadataReplaceRule{
				{Old: "original.label.io", New: "rewrite.label.io"},
			},
		},
		Annotations: MetadataReplace{
			Names: []MetadataReplaceRule{
				{Old: "original.annotation.io", New: "rewrite.annotation.io"},
			},
		},
	}
	rules.Complete()
	return &RuleBasedRewriter{
		Rules: rules,
	}
}

func TestRewriteAPIEndpoint(t *testing.T) {
	tests := []struct {
		name        string
		path        string
		expectPath  string
		exepctQuery string
	}{
		{
			"rewritable group",
			"/apis/original.group.io",
			"/apis/prefixed.resources.group.io",
			"",
		},
		{
			"rewritable group and version",
			"/apis/original.group.io/v1",
			"/apis/prefixed.resources.group.io/v1",
			"",
		},
		{
			"rewritable resource list",
			"/apis/original.group.io/v1/someresources",
			"/apis/prefixed.resources.group.io/v1/prefixedsomeresources",
			"",
		},
		{
			"rewritable resource by name",
			"/apis/original.group.io/v1/someresources/srname",
			"/apis/prefixed.resources.group.io/v1/prefixedsomeresources/srname",
			"",
		},
		{
			"rewritable resource status",
			"/apis/original.group.io/v1/someresources/srname/status",
			"/apis/prefixed.resources.group.io/v1/prefixedsomeresources/srname/status",
			"",
		},
		{
			"rewritable CRD",
			"/apis/apiextensions.k8s.io/v1/customresourcedefinitions/someresources.original.group.io",
			"/apis/apiextensions.k8s.io/v1/customresourcedefinitions/prefixedsomeresources.prefixed.resources.group.io",
			"",
		},
		{
			"rewritable labelSelector",
			"/api/v1/namespaces/d8-virtualization/pods?labelSelector=original.label.io%3Dlabelvalue&limit=500",
			"/api/v1/namespaces/d8-virtualization/pods",
			"labelSelector=rewrite.label.io%3Dlabelvalue&limit=500",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u, err := url.Parse(tt.path)
			require.NoError(t, err, "should parse path '%s'", tt.path)

			ep := ParseAPIEndpoint(u)
			rwr := createTestRewriter()

			newEp := rwr.RewriteAPIEndpoint(ep)

			if tt.expectPath == "" {
				require.Nil(t, newEp, "should not rewrite path '%s', got %+v", tt.path, newEp)
			}
			require.NotNil(t, newEp, "should rewrite path '%s', got nil originEndpoint")

			require.Equal(t, tt.expectPath, newEp.Path(), "expect rewrite for path '%s' to be '%s', got '%s'", tt.path, tt.expectPath, ep.Path())
			require.Equal(t, tt.exepctQuery, newEp.RawQuery, "expect rewrite query for path %q to be '%s', got '%s'", tt.path, tt.exepctQuery, ep.RawQuery)
		})
	}

}
