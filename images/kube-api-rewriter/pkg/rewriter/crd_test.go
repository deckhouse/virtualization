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
	"bufio"
	"bytes"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func createRewriterForCRDTest() *RuleBasedRewriter {
	apiGroupRules := map[string]APIGroupRule{
		"original.group.io": {
			GroupRule: GroupRule{
				Group:            "original.group.io",
				Versions:         []string{"v1", "v1alpha1"},
				PreferredVersion: "v1",
				Renamed:          "prefixed.resources.group.io",
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
				Group:            "other.group.io",
				Versions:         []string{"v2alpha3"},
				PreferredVersion: "v2alpha3",
				Renamed:          "other.prefixed.resources.group.io",
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

	rwRules := &RewriteRules{
		KindPrefix:         "Prefixed", // KV
		ResourceTypePrefix: "prefixed", // kv
		ShortNamePrefix:    "p",
		Categories:         []string{"prefixed"},
		Rules:              apiGroupRules,
	}

	rwRules.Init()
	return &RuleBasedRewriter{
		Rules: rwRules,
	}
}

// TestCRDRename - rename of a single CRD.
func TestCRDRename(t *testing.T) {
	reqBody := `{
"apiVersion": "apiextensions.k8s.io/v1",
"kind": "CustomResourceDefinition",
"metadata": {
	"name":"someresources.original.group.io"
}
"spec": {
	"group": "original.group.io",
	"names": {
		"kind": "SomeResource",
		"listKind": "SomeResourceList",
		"plural": "someresources",
		"singular": "someresource",
		"shortNames": ["sr"],
		"categories": ["all"]
	},
	"scope":"Namespaced",
	"versions": {}
}
}`
	rwr := createRewriterForCRDTest()
	testCRDRules := rwr.Rules

	restored, err := RewriteCRDOrList(testCRDRules, []byte(reqBody), Rename)
	if err != nil {
		t.Fatalf("should rename CRD without error: %v", err)
	}
	if restored == nil {
		t.Fatalf("should rename CRD: %v", err)
	}

	groupRule, resRule := testCRDRules.KindRules("original.group.io", "SomeResource")

	tests := []struct {
		path     string
		expected string
	}{
		{"metadata.name", testCRDRules.RenameResource(resRule.Plural) + "." + groupRule.Renamed},
		{"spec.group", groupRule.Renamed},
		{"spec.names.kind", testCRDRules.RenameKind(resRule.Kind)},
		{"spec.names.listKind", testCRDRules.RenameKind(resRule.ListKind)},
		{"spec.names.plural", testCRDRules.RenameResource(resRule.Plural)},
		{"spec.names.singular", testCRDRules.RenameResource(resRule.Singular)},
		{"spec.names.shortNames", `["psr","psrs"]`},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			actual := gjson.GetBytes(restored, tt.path).String()
			if actual != tt.expected {
				t.Fatalf("%s value should be %s, got %s", tt.path, tt.expected, actual)
			}
		})
	}
}

// TestCRDPatch tests renaming /spec in a CRD patch.
func TestCRDPatch(t *testing.T) {
	patches := `[{ "op": "add", "path": "/metadata/ownerReferences", "value": null },
{ "op": "replace", "path": "/spec", "value": {
"group":"original.group.io",
"names":{"plural":"someresources","singular":"someresource","shortNames":["sr","srs"],"kind":"SomeResource","categories":["all"]},
"scope":"Namespaced","versions":[{"name":"v1alpha1","schema":{}}]
} }
]`
	patches = strings.ReplaceAll(patches, "\n", "")

	expect := `[{ "op": "add", "path": "/metadata/ownerReferences", "value": null },
{ "op": "replace", "path": "/spec", "value": {
"group":"prefixed.resources.group.io",
"names":{"plural":"prefixedsomeresources","singular":"prefixedsomeresource","shortNames":["psr","psrs"],"kind":"PrefixedSomeResource","categories":["prefixed"]},
"scope":"Namespaced","versions":[{"name":"v1alpha1","schema":{}}]
} }
]`
	expect = strings.ReplaceAll(expect, "\n", "")

	rwr := createRewriterForCRDTest()
	_, resRule := rwr.Rules.ResourceRules("original.group.io", "someresources")
	require.NotNil(t, resRule, "should get resource rule for hardcoded group and resourceType")

	resBytes, err := RenameCRDPatch(rwr.Rules, resRule, []byte(patches))
	require.NoError(t, err, "should rename CRD patch")

	actual := string(resBytes)
	require.Equal(t, expect, actual)
}

// TestCRDRestore test restoring of a single CRD.
func TestCRDRestore(t *testing.T) {
	crdHTTPRequest := `GET /apis/apiextensions.k8s.io/v1/customresourcedefinitions/someresources.original.group.io HTTP/1.1
Host: 127.0.0.1

`
	origGroup := "original.group.io"
	crdPayload := `{
"apiVersion": "apiextensions.k8s.io/v1",
"kind": "CustomResourceDefinition",
"metadata": {
	"name":"prefixedsomeresources.prefixed.resources.group.io"
}
"spec": {
	"group": "prefixed.resources.group.io",
	"names": {
		"kind": "PrefixedSomeResource",
		"listKind": "PrefixedSomeResourceList",
		"plural": "prefixedsomeresources",
		"singular": "prefixedsomeresource",
		"shortNames": ["psr"],
		"categories": ["prefixed"]
	},
	"scope":"Namespaced",
	"versions": {}
}
}`

	req, err := http.ReadRequest(bufio.NewReader(bytes.NewBufferString(crdHTTPRequest)))
	require.NoError(t, err, "should parse hardcoded http request")
	require.NotNil(t, req.URL, "should parse url in hardcoded http request")

	rwr := createRewriterForCRDTest()
	targetReq := NewTargetRequest(rwr, req)
	require.NotNil(t, targetReq, "should get TargetRequest")
	require.Equal(t, origGroup, targetReq.OrigGroup(), "should set proper orig group")

	resultBytes, err := rwr.RewriteJSONPayload(targetReq, []byte(crdPayload), Restore) // RewriteCRDOrList(crdPayload, []byte(reqBody), Restore, origGroup)
	if err != nil {
		t.Fatalf("should restore CRD without error: %v", err)
	}
	if resultBytes == nil {
		t.Fatalf("should restore CRD: %v", err)
	}

	resRule := rwr.Rules.Rules[origGroup].ResourceRules["someresources"]

	tests := []struct {
		path     string
		expected string
	}{
		{"metadata.name", resRule.Plural + "." + origGroup},
		{"spec.group", origGroup},
		{"spec.names.kind", resRule.Kind},
		{"spec.names.listKind", resRule.ListKind},
		{"spec.names.plural", resRule.Plural},
		{"spec.names.singular", resRule.Singular},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			actual := gjson.GetBytes(resultBytes, tt.path).String()
			if actual != tt.expected {
				t.Fatalf("%s value should be %s, got %s", tt.path, tt.expected, actual)
			}
		})
	}
}

func TestCRDPathRewrite(t *testing.T) {
	tests := []struct {
		name             string
		urlPath          string
		expected         string
		origGroup        string
		origResourceType string
	}{
		{
			"crd with rule",
			"/apis/apiextensions.k8s.io/v1/customresourcedefinitions/someresources.original.group.io",
			"/apis/apiextensions.k8s.io/v1/customresourcedefinitions/prefixedsomeresources.prefixed.resources.group.io",
			"original.group.io",
			"someresources",
		},
		{
			"crd watch by name",
			"/apis/apiextensions.k8s.io/v1/customresourcedefinitions?fieldSelector=metadata.name%3Dsomeresources.original.group.io&resourceVersion=0&watch=true",
			"/apis/apiextensions.k8s.io/v1/customresourcedefinitions?fieldSelector=metadata.name%3Dprefixedsomeresources.prefixed.resources.group.io&resourceVersion=0&watch=true",
			"",
			"",
		},
		{
			"unknown crd watch by name",
			"/apis/apiextensions.k8s.io/v1/customresourcedefinitions?fieldSelector=metadata.name%3Dresource.unknown.group.io&resourceVersion=0&watch=true",
			"/apis/apiextensions.k8s.io/v1/customresourcedefinitions?fieldSelector=metadata.name%3Dresource.unknown.group.io&resourceVersion=0&watch=true",
			"",
			"",
		},
		{
			"crd without rule",
			"/apis/apiextensions.k8s.io/v1/customresourcedefinitions/unknown.group.io",
			"",
			"",
			"",
		},
		{
			"crd list",
			"/apis/apiextensions.k8s.io/v1/customresourcedefinitions",
			"",
			"",
			"",
		},
		{
			"non crd apiextension",
			"/apis/apiextensions.k8s.io/v1/unknown",
			"",
			"",
			"",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			httpReqHead := fmt.Sprintf(`GET %s HTTP/1.1`, tt.urlPath)
			httpReq := httpReqHead + "\n" + "Host: 127.0.0.1\n\n"
			req, err := http.ReadRequest(bufio.NewReader(bytes.NewBufferString(httpReq)))
			require.NoError(t, err, "should parse hardcoded http request")
			require.NotNil(t, req.URL, "should parse url in hardcoded http request")

			rwr := createRewriterForCRDTest()
			targetReq := NewTargetRequest(rwr, req)
			require.NotNil(t, targetReq, "should get TargetRequest")

			if tt.expected == "" {
				require.Equal(t, tt.urlPath, targetReq.Path(), "should not rewrite api endpoint path")
				return
			}

			if tt.origGroup != "" {
				require.Equal(t, tt.origGroup, targetReq.OrigGroup())
			}

			actual := targetReq.Path()
			if targetReq.RawQuery() != "" {
				actual += "?" + targetReq.RawQuery()
			}

			require.Equal(t, tt.expected, actual, "should rewrite api endpoint path")
		})
	}
}
