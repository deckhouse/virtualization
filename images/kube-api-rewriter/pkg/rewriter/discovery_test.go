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

func createRewriterForDiscoveryTest() *RuleBasedRewriter {
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

	webhookRules := map[string]WebhookRule{
		"/validate-prefixed-resources-group-io-v1-prefixedsomeresource": {
			Path:     "/validate-original-group-io-v1-someresource",
			Group:    "original.group.io",
			Resource: "someresources",
		},
	}

	rwRules := &RewriteRules{
		KindPrefix:         "Prefixed", // KV
		ResourceTypePrefix: "prefixed", // kv
		ShortNamePrefix:    "p",
		Categories:         []string{"prefixed"},
		Rules:              apiGroupRules,
		Webhooks:           webhookRules,
	}
	rwRules.Init()

	return &RuleBasedRewriter{
		Rules: rwRules,
	}
}

func TestRewriteRequestAPIGroupList(t *testing.T) {
	// Request APIGroupList.
	request := `GET /apis HTTP/1.1
Host: 127.0.0.1

`
	req, err := http.ReadRequest(bufio.NewReader(bytes.NewBufferString(request)))
	require.NoError(t, err, "should read hardcoded request")

	expectPath := "/apis"

	// Response body with renamed APIGroupList
	apiGroupResponse := `{
  "kind": "APIGroupList",
  "apiVersion": "v1",
  "groups": [
    {
      "name": "original.group.io",
      "versions": [
       {"groupVersion":"original.group.io/v1", "version":"v1"},
       {"groupVersion":"original.group.io/v1alpha1", "version":"v1alpha1"}
      ],
      "preferredVersion": {
        "groupVersion": "original.group.io/v1",
        "version":"v1"
      }
    },
    {
      "name": "prefixed.resources.group.io",
      "versions": [
       {"groupVersion":"prefixed.resources.group.io/v1", "version":"v1"},
       {"groupVersion":"prefixed.resources.group.io/v1alpha1", "version":"v1alpha1"}
      ],
      "preferredVersion": {
        "groupVersion": "prefixed.resources.group.io/v1",
        "version":"v1"
      }
    },
    {
      "name": "other.prefixed.resources.group.io",
      "versions": [
       {"groupVersion":"other.prefixed.resources.group.io/v2alpha3", "version":"v2alpha3"}
      ],
      "preferredVersion": {
        "groupVersion": "other.prefixed.resources.group.io/v2alpha3",
        "version":"v2alpha3"
      }
    }
  ]
}`

	// Client proxy mode.
	rwr := createRewriterForDiscoveryTest()

	var targetReq *TargetRequest

	targetReq = NewTargetRequest(rwr, req)
	require.NotNil(t, targetReq, "should get TargetRequest")
	require.Equal(t, expectPath, targetReq.Path(), "should rewrite api endpoint path")

	resultBytes, err := rwr.RewriteJSONPayload(targetReq, []byte(apiGroupResponse), Restore)
	if err != nil {
		t.Fatalf("should rewrite body with renamed resources: %v", err)
	}

	tests := []struct {
		path     string
		expected string
	}{
		// Check no prefixed groups left after rewrite.
		{`groups.#(name=="prefixed.resource.group.io").name`, ""},
		// Should have only 1 group instance, no duplicates.
		{`groups.#(name=="original.group.io")#|#`, "1"},
		{`groups.#(name=="original.group.io").name`, "original.group.io"},
		{`groups.#(name=="original.group.io").preferredVersion.groupVersion`, "original.group.io/v1"},
		// Should not add more versions than there are in response.
		{`groups.#(name=="original.group.io").versions.#`, "2"},
		{`groups.#(name=="original.group.io").versions.#(version="v1").groupVersion`, "original.group.io/v1"},
		{`groups.#(name=="original.group.io").versions.#(version="v1alpha1").groupVersion`, "original.group.io/v1alpha1"},
		// Check other.group.io is restored.
		{`groups.#(name=="other.group.io")#|#`, "1"},
		{`groups.#(name=="other.group.io").name`, "other.group.io"},
		{`groups.#(name=="other.group.io").preferredVersion.groupVersion`, "other.group.io/v2alpha3"},
		// Should not add more versions than there are in response.
		{`groups.#(name=="other.group.io").versions.#`, "1"},
		{`groups.#(name=="other.group.io").versions.#(version="v2alpha3").groupVersion`, "other.group.io/v2alpha3"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			actual := gjson.GetBytes(resultBytes, tt.path).String()
			if actual != tt.expected {
				t.Fatalf("%s value should be %s, got '%s', rewritten APIGroupList: %s", tt.path, tt.expected, actual, string(resultBytes))
			}
		})
	}
}

func TestRewriteRequestAPIGroup(t *testing.T) {
	// Request APIResourcesList of original, non-renamed resources.
	request := `GET /apis/original.group.io HTTP/1.1
Host: 127.0.0.1

`
	req, err := http.ReadRequest(bufio.NewReader(bytes.NewBufferString(request)))
	require.NoError(t, err, "should read hardcoded request")

	expectPath := "/apis/prefixed.resources.group.io"

	// Response body with renamed APIResourcesList
	apiGroupResponse := `{
  "kind": "APIGroup",
  "apiVersion": "v1",
  "name": "prefixed.resources.group.io",
  "versions": [
   {"groupVersion":"prefixed.resources.group.io/v1", "version":"v1"},
   {"groupVersion":"prefixed.resources.group.io/v1alpha1", "version":"v1alpha1"}
  ],
  "preferredVersion": {
    "groupVersion": "prefixed.resources.group.io/v1",
    "version":"v1"
  }
}`

	// Client proxy mode.
	rwr := createRewriterForDiscoveryTest()

	var targetReq *TargetRequest

	targetReq = NewTargetRequest(rwr, req)
	require.NotNil(t, targetReq, "should get TargetRequest")
	require.Equal(t, expectPath, targetReq.Path(), "should rewrite api endpoint path")

	resultBytes, err := rwr.RewriteJSONPayload(targetReq, []byte(apiGroupResponse), Restore)
	if err != nil {
		t.Fatalf("should rewrite body with renamed resources: %v", err)
	}

	groupRule, _ := rwr.Rules.GroupResourceRules("someresources")
	require.NotNil(t, groupRule, "should get rule for hard-coded resource type someresources")

	tests := []struct {
		path     string
		expected string
	}{
		{"name", groupRule.Group},
		{"versions.#(version==\"v1\").groupVersion", groupRule.Group + "/v1"},
		{"versions.#(version==\"v1alpha1\").groupVersion", groupRule.Group + "/v1alpha1"},
		{"preferredVersion.groupVersion", groupRule.Group + "/v1"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			actual := gjson.GetBytes(resultBytes, tt.path).String()
			if actual != tt.expected {
				t.Fatalf("%s value should be %s, got '%s', rewritten APIGroup: %s", tt.path, tt.expected, actual, string(resultBytes))
			}
		})
	}
}

func TestRewriteRequestAPIGroupUnknownGroup(t *testing.T) {
	// Request APIGroup discovery for unknown group.
	request := `GET /apis/unknown.group.io HTTP/1.1
Host: 127.0.0.1

`
	req, err := http.ReadRequest(bufio.NewReader(bytes.NewBufferString(request)))
	require.NoError(t, err, "should read hardcoded request")

	apiGroupResponse := `{
  "kind": "APIGroup",
  "apiVersion": "v1",
  "name": "unknown.group.io",
  "versions": [
   {"groupVersion":"unknown.group.io/v1beta1", "version":"v1beta1"},
   {"groupVersion":"unknown.group.io/v1alpha3", "version":"v1alpha3"}
  ],
  "preferredVersion": {
    "groupVersion": "unknown.group.io/v1beta1",
    "version":"v1beta1"
  }
}`

	// Client proxy mode.
	rwr := createRewriterForDiscoveryTest()

	var targetReq *TargetRequest

	targetReq = NewTargetRequest(rwr, req)
	require.NotNil(t, targetReq, "should get TargetRequest")
	require.Equal(t, req.URL.Path, targetReq.Path(), "should not rewrite api endpoint path")

	resultBytes, err := rwr.RewriteJSONPayload(targetReq, []byte(apiGroupResponse), Restore)
	if err != nil {
		t.Fatalf("should rewrite body with renamed resources: %v", err)
	}

	require.Equal(t, apiGroupResponse, string(resultBytes), "should not rewrite ApiGroup for unknown group")
}

func TestRewriteRequestAPIResourceList(t *testing.T) {
	// Request APIResourcesList of original, non-renamed resources.
	// Note: use non preferred version.
	request := `GET /apis/original.group.io/v1alpha1 HTTP/1.1
Host: 127.0.0.1

`
	req, err := http.ReadRequest(bufio.NewReader(bytes.NewBufferString(request)))
	require.NoError(t, err, "should read hardcoded request")

	expectPath := "/apis/prefixed.resources.group.io/v1alpha1"

	// Response body with renamed APIResourcesList
	resourceListPayload := `{
  "kind": "APIResourceList",
  "apiVersion": "v1",
  "groupVersion": "prefixed.resources.group.io/v1alpha1",
  "resources": [
    {"name":"prefixedsomeresources",
     "singularName":"prefixedsomeresource",
     "namespaced":true,
     "kind":"PrefixedSomeResource",
     "verbs":["delete","deletecollection","get","list","patch","create","update","watch"],
     "shortNames":["psr","psrs"],
     "categories":["prefixed"],
     "storageVersionHash":"1qIJ90Mhvd8="},

    {"name":"prefixedsomeresources/status",
     "singularName":"",
     "namespaced":true,
     "kind":"PrefixedSomeResource",
     "verbs":["get","patch","update"]},

    {"name":"norulesresources",
     "singularName":"norulesresource",
     "namespaced":true,
     "kind":"NoRulesResource",
     "verbs":["delete","deletecollection","get","list","patch","create","update","watch"],
     "shortNames":["nrr"],
     "categories":["prefixed"],
     "storageVersionHash":"Nwlto9QquX0="},

    {"name":"norulesresources/status",
     "singularName":"",
     "namespaced":true,
     "kind":"NoRulesResource",
     "verbs":["get","patch","update"]}
]}`

	// Client proxy mode.
	rwr := createRewriterForDiscoveryTest()

	var targetReq *TargetRequest

	targetReq = NewTargetRequest(rwr, req)
	require.NotNil(t, targetReq, "should get TargetRequest")
	require.Equal(t, expectPath, targetReq.Path(), "should rewrite api endpoint path")

	resultBytes, err := rwr.RewriteJSONPayload(targetReq, []byte(resourceListPayload), Restore)
	if err != nil {
		t.Fatalf("should rewrite body with renamed resources: %v", err)
	}

	tests := []struct {
		path     string
		expected string
	}{
		{"groupVersion", "original.group.io/v1alpha1"},
		{"resources.#(name==\"someresources\").name", "someresources"},
		{"resources.#(name==\"someresources\").kind", "SomeResource"},
		{"resources.#(name==\"someresources\").singularName", "someresource"},
		{"resources.#(name==\"someresources\").categories.0", "all"},
		{"resources.#(name==\"someresources\").shortNames.0", "sr"},
		{"resources.#(name==\"someresources\").shortNames.1", "srs"},
		{"resources.#(name==\"someresources/status\").name", "someresources/status"},
		{"resources.#(name==\"someresources/status\").kind", "SomeResource"},
		{"resources.#(name==\"someresources/status\").singularName", ""},
		// norulesresources should not be restored.
		{"resources.#(name==\"norulesresources\").name", "norulesresources"},
		{"resources.#(name==\"norulesresources\").kind", "NoRulesResource"},
		{"resources.#(name==\"norulesresources\").singularName", "norulesresource"},
		{"resources.#(name==\"norulesresources\").categories.0", "prefixed"},
		{"resources.#(name==\"norulesresources\").shortNames.0", "nrr"},
		{"resources.#(name==\"norulesresources/status\").name", "norulesresources/status"},
		{"resources.#(name==\"norulesresources/status\").kind", "NoRulesResource"},
		{"resources.#(name==\"norulesresources/status\").singularName", ""},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			actual := gjson.GetBytes(resultBytes, tt.path).String()
			if actual != tt.expected {
				t.Fatalf("%s value should be %s, got '%s', rewritten APIGroupDiscovery: %s", tt.path, tt.expected, actual, string(resultBytes))
			}
		})
	}
}

func TestRewriteRequestAPIGroupDiscoveryList(t *testing.T) {
	// Request aggregated discovery as APIGroupDiscoveryList kind.
	request := `GET /apis HTTP/1.1
Host: 127.0.0.1
Accept: application/json;g=apidiscovery.k8s.io;v=v2beta1;as=APIGroupDiscoveryList

`
	req, err := http.ReadRequest(bufio.NewReader(bytes.NewBufferString(request)))
	require.NoError(t, err, "should read hardcoded request")

	// This group contains resources from 2 original groups:
	// - someresources.original.group.io with v1 and v1alpha1 version
	// - otherresources.other.group.io of v2alpha3 version
	// Restored list should contain 2 APIGroupDiscovery.
	renamedAPIGroupDiscovery := `{
	"metadata":{
      "name": "prefixed.resources.group.io",
      "creationTimestamp": null
    },
    "versions":[
    { "version": "v1",
      "freshness": "Current",
      "resources": [
        { "resource": "prefixedsomeresources",
          "responseKind": {"group": "prefixed.resources.group.io", "version": "v1", "kind": "PrefixedSomeResource"},
          "scope": "Namespaced",
          "singularResource": "prefixedsomeresource",
          "shortNames": ["psr"],
          "categories": ["prefixed"],
          "verbs": ["create", "patch"],
          "subresources": [
            { "subresource": "status",
              "responseKind": {"group": "prefixed.resources.group.io", "version": "v1", "kind": "PrefixedSomeResource"},
              "verbs": ["get", "patch"]
            }
          ]
        }
      ]
    },
    { "version": "v1alpha1",
      "resources": [
        { "resource": "prefixedsomeresources",
          "responseKind": {"group": "prefixed.resources.group.io", "version": "v1alpha1", "kind": "PrefixedSomeResource"},
          "scope": "Namespaced",
          "singularResource": "prefixedsomeresource",
          "verbs": ["create", "patch"],
          "subresources": [
            { "subresource": "status",
              "responseKind": {"group": "prefixed.resources.group.io", "version": "v1alpha1", "kind": "PrefixedSomeResource"},
              "verbs": ["get", "patch"]
            }
          ]
        }
      ]
    }
    ]
}`
	renamedOtherAPIGroupDiscovery := `{
	"metadata":{
      "name": "other.prefixed.resources.group.io",
      "creationTimestamp": null
    },
    "versions":[
    { "version": "v2alpha3",
      "resources": [
        { "resource": "prefixedotherresources",
          "responseKind": {"group": "other.prefixed.resources.group.io", "version": "v1alpha1", "kind": "PrefixedOtherResource"},
          "scope": "Namespaced",
          "singularResource": "prefixedotherresource",
          "verbs": ["create", "patch"],
          "subresources": [
            { "subresource": "status",
              "responseKind": {"group": "other.prefixed.resources.group.io", "version": "v1alpha1", "kind": "PrefixedOtherResource"},
              "verbs": ["get", "patch"]
            }
          ]
        }
      ]
    }
    ]
}`
	// This groups should not be rewritten.
	appsAPIGroupDiscovery := `{
  "metadata": {
    "name": "apps",
    "creationTimestamp": null
  },
  "versions": [
    {"version": "v1",
     "freshness": "Current",
     "resources": [
      {"resource": "deployments",
       "responseKind": {"group": "", "version": "", "kind": "Deployment"},
       "scope": "Namespaced",
       "singularResource": "deployment",
       "verbs": ["create", "patch"]
      }
     ]}
  ]
}`
	// This groups should not be rewritten.
	nonRewritableAPIGroupDiscovery := `{
  "metadata": {
    "name": "custom.resources.io",
    "creationTimestamp": null
  },
  "versions": [
    {"version": "v1",
     "freshness": "Current",
     "resources": [
      {"resource": "somecustomresources",
       "responseKind": {"group": "custom.resources.io", "version": "v1", "kind": "SomeCustomResource"},
       "scope": "Namespaced",
       "singularResource": "somecustomresource",
       "verbs": ["create", "patch"]
      }
     ]}
  ]
}`

	// Response body with renamed APIGroupDiscoveryList
	apiGroupDiscoveryListPayload := fmt.Sprintf(`{
  "kind": "APIGroupDiscoveryList",
  "apiVersion": "apidiscovery.k8s.io/v2beta1",
  "metadata": {},
  "items": [ %s ]
}`, strings.Join([]string{
		appsAPIGroupDiscovery,
		renamedAPIGroupDiscovery,
		renamedOtherAPIGroupDiscovery,
		nonRewritableAPIGroupDiscovery,
	}, ","))

	// Initialize rewriter using hard-coded client http request.
	rwr := createRewriterForDiscoveryTest()
	targetReq := NewTargetRequest(rwr, req)
	require.NotNil(t, targetReq, "should get TargetRequest")

	resultBytes, err := rwr.RewriteJSONPayload(targetReq, []byte(apiGroupDiscoveryListPayload), Restore)
	if err != nil {
		t.Fatalf("should rewrite body with renamed resources: %v", err)
	}

	// Get rules for rewritable resource.
	groupRule, resRule := rwr.Rules.GroupResourceRules("someresources")
	require.NotNil(t, groupRule, "should get groupRule for hardcoded resourceType")
	require.NotNil(t, resRule, "should get resourceRule for hardcoded resourceType")

	// Expect renamed groups present in the restored object.
	{
		expected := []string{
			"apps",
			"original.group.io",
			"other.group.io",
			"custom.resources.io",
		}

		groups := gjson.GetBytes(resultBytes, `items.#.metadata.name`).Array()

		actual := []string{}
		for _, group := range groups {
			actual = append(actual, group.String())
		}

		require.Equal(t, len(expected), len(groups), "restored object should have %d groups, got %d: %#v", len(expected), len(groups), actual)
		for _, expect := range expected {
			require.Contains(t, actual, expect, "restored object should have group %s, got %v", expect, actual)
		}
	}

	// Test renamed fields for someresources in original.group.io.
	{
		group := gjson.GetBytes(resultBytes, `items.#(metadata.name=="original.group.io")`)
		groupRule, resRule := rwr.Rules.GroupResourceRules("someresources")

		require.NotNil(t, resRule, "should get rule for hard-coded resource type someresources")

		tests := []struct {
			path     string
			expected string
		}{
			{"versions.#(version==\"v1\").resources.0.resource", resRule.Plural},
			{"versions.#(version==\"v1\").resources.0.responseKind.group", groupRule.Group},
			{"versions.#(version==\"v1\").resources.0.responseKind.kind", resRule.Kind},
			{"versions.#(version==\"v1\").resources.0.singularResource", resRule.Singular},
			{"versions.#(version==\"v1\").resources.0.categories.0", resRule.Categories[0]},
			{"versions.#(version==\"v1\").resources.0.shortNames.0", resRule.ShortNames[0]},
			{"versions.#(version==\"v1\").resources.0.subresources.0.responseKind.group", groupRule.Group},
			{"versions.#(version==\"v1\").resources.0.subresources.0.responseKind.kind", resRule.Kind},
		}

		groupBytes := []byte(group.Raw)
		for _, tt := range tests {
			t.Run(tt.path, func(t *testing.T) {
				actual := gjson.GetBytes(groupBytes, tt.path).String()
				if actual != tt.expected {
					t.Fatalf("%s value should be %s, got '%s', rewritten APIGroupDiscovery: %s", tt.path, tt.expected, actual, string(groupBytes))
				}
			})
		}
	}
}
