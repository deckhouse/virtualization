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
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func createTestRewriterForCore() *RuleBasedRewriter {
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
				{Original: "labelgroup.io", Renamed: "replacedlabelgroup.io"},
				{Original: "component.labelgroup.io", Renamed: "component.replacedlabelgroup.io"},
			},
			Names: []MetadataReplaceRule{
				{Original: "labelgroup.io", Renamed: "replacedlabelgroup.io"},
			},
		},
		Annotations: MetadataReplace{
			Prefixes: []MetadataReplaceRule{
				{Original: "annogroup.io", Renamed: "replacedannogroup.io"},
				{Original: "component.annogroup.io", Renamed: "component.replacedannogroup.io"},
			},
			Names: []MetadataReplaceRule{
				{Original: "annogroup.io", Renamed: "replacedannogroup.io"},
			},
		},
	}
	rules.Init()
	return &RuleBasedRewriter{
		Rules: rules,
	}
}

func TestRewriteServicePatch(t *testing.T) {
	serviceReq := `PATCH /api/v1/namespaces/default/services/testservice HTTP/1.1
Host: 127.0.0.1

`
	servicePatch := `[{
	"op":"replace",
	"path":"/spec",
	"value": {
	   "selector":{ "labelgroup.io":"true" }
	}
}]`

	req, err := http.ReadRequest(bufio.NewReader(bytes.NewBufferString(serviceReq + servicePatch)))
	require.NoError(t, err, "should parse hardcoded http request")
	require.NotNil(t, req.URL, "should parse url in hardcoded http request")

	rwr := createTestRewriterForCore()
	targetReq := NewTargetRequest(rwr, req)
	require.NotNil(t, targetReq, "should get TargetRequest")
	require.True(t, targetReq.ShouldRewriteRequest(), "should rewrite request")
	require.True(t, targetReq.ShouldRewriteResponse(), "should rewrite response")
	// require.Equal(t, origGroup, targetReq.OrigGroup(), "should set proper orig group")

	resultBytes, err := rwr.RewritePatch(targetReq, []byte(servicePatch))
	if err != nil {
		t.Fatalf("should rename Service patch without error: %v", err)
	}
	if resultBytes == nil {
		t.Fatalf("should rename Service patch: %v", err)
	}

	tests := []struct {
		path     string
		expected string
	}{
		{`0.value.selector.labelgroup\.io`, ""},
		{`0.value.selector.replacedlabelgroup\.io`, "true"},
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

func TestRewriteMetadataPatch(t *testing.T) {
	serviceReq := `PATCH /apis/admissionregistration.k8s.io/v1/validatingwebhookconfigurations/test-validator HTTP/1.1
Host: 127.0.0.1

`
	servicePatch := `[{
	"op":"replace",
	"path":"/metadata/labels",
	"value": {"labelgroup.io":"true" }
}]`

	req, err := http.ReadRequest(bufio.NewReader(bytes.NewBufferString(serviceReq + servicePatch)))
	require.NoError(t, err, "should parse hardcoded http request")
	require.NotNil(t, req.URL, "should parse url in hardcoded http request")

	rwr := createTestRewriterForCore()
	targetReq := NewTargetRequest(rwr, req)
	require.NotNil(t, targetReq, "should get TargetRequest")
	require.True(t, targetReq.ShouldRewriteRequest(), "should rewrite request")
	require.True(t, targetReq.ShouldRewriteResponse(), "should rewrite response")
	// require.Equal(t, origGroup, targetReq.OrigGroup(), "should set proper orig group")

	resultBytes, err := rwr.RewritePatch(targetReq, []byte(servicePatch))
	if err != nil {
		t.Fatalf("should rename Service patch without error: %v", err)
	}
	if resultBytes == nil {
		t.Fatalf("should rename Service patch: %v", err)
	}

	tests := []struct {
		path     string
		expected string
	}{
		{`0.value.labelgroup\.io`, ""},
		{`0.value.replacedlabelgroup\.io`, "true"},
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
