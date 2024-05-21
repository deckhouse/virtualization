package rewriter

import (
	"bufio"
	"bytes"
	"fmt"
	"net/http"
	"strconv"
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
		RenamedGroup:       "prefixed.resources.group.io",
		Rules:              apiGroupRules,
		Webhooks:           webhookRules,
	}

	return &RuleBasedRewriter{
		Rules: rwRules,
	}
}

func TestRewriteRequestAPIResourceList(t *testing.T) {
	// Request APIResourcesList of original, non-renamed resources.
	request := `GET /apis/original.group.io/v1 HTTP/1.1
Host: 127.0.0.1

`
	req, err := http.ReadRequest(bufio.NewReader(bytes.NewBufferString(request)))
	require.NoError(t, err, "should read hardcoded request")

	expectPath := "/apis/prefixed.resources.group.io/v1"

	// Response body with renamed APIResourcesList
	resourceListPayload := `{
  "kind": "APIResourceList",
  "apiVersion": "v1",
  "groupVersion": "prefixed.resources.group.io/v1",
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

    {"name":"prefixedotherresources",
     "singularName":"prefixedotherresource",
     "namespaced":true,
     "kind":"PrefixedOtherResource",
     "verbs":["delete","deletecollection","get","list","patch","create","update","watch"],
     "shortNames":["por"],
     "categories":["prefixed"],
     "storageVersionHash":"Nwlto9QquX0="},

    {"name":"prefixedotherresources/status",
     "singularName":"",
     "namespaced":true,
     "kind":"PrefixOtherResource",
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

	actual := string(resultBytes)

	require.Contains(t, actual, `"someresources"`, "should contains original someresources, got: %s", actual)
	require.Contains(t, actual, `"someresources/status"`, "should contains original someresources/status, got: %s", actual)
	require.NotContains(t, actual, `"otherresources"`, "should not contains not requested otherresources, got: %s", actual)
	require.NotContains(t, actual, `"otherresources/status"`, "should not contains not requested otherresources/status, got: %s", actual)
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
    },
    { "version": "v2alpha3",
      "resources": [
        { "resource": "prefixedotherresources",
          "responseKind": {"group": "prefixed.resources.group.io", "version": "v1alpha1", "kind": "PrefixedOtherResource"},
          "scope": "Namespaced",
          "singularResource": "prefixedotherresource",
          "verbs": ["create", "patch"],
          "subresources": [
            { "subresource": "status",
              "responseKind": {"group": "prefixed.resources.group.io", "version": "v1alpha1", "kind": "PrefixedOtherResource"},
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
			{"versions.0.resources.0.resource", resRule.Plural},
			{"versions.0.resources.0.responseKind.group", groupRule.Group},
			{"versions.0.resources.0.responseKind.kind", resRule.Kind},
			{"versions.0.resources.0.singularResource", resRule.Singular},
			{"versions.0.resources.0.categories.0", resRule.Categories[0]},
			{"versions.0.resources.0.shortNames.0", resRule.ShortNames[0]},
			{"versions.0.resources.0.subresources.0.responseKind.group", groupRule.Group},
			{"versions.0.resources.0.subresources.0.responseKind.kind", resRule.Kind},
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

func TestRewriteAdmissionReviewRequestForResource(t *testing.T) {
	admissionReview := `{
  "kind":"AdmissionReview",
  "apiVersion":"admission.k8s.io/v1",
  "request":{
    "uid":"389cfe15-34a1-4829-ad4d-de2576385711",
    "kind":{"group":"prefixed.resources.group.io","version":"v1","kind":"PrefixedSomeResource"},
    "resource":{"group":"prefixed.resources.group.io","version":"v1","resource":"prefixedsomeresources"},
    "requestKind":{"group":"prefixed.resources.group.io","version":"v1","kind":"PrefixedSomeResource"},
    "requestResource":{"group":"prefixed.resources.group.io","version":"v1","resource":"prefixedsomeresources"},
    "name":"some-resource-name",
    "namespace":"nsname",
    "operation":"UPDATE",
    "userInfo":{"username":"kubernetes-admin","groups":["system:masters","system:authenticated"]},
    "object":{
      "apiVersion":"prefixed.resources.group.io/v1",
      "kind":"PrefixedSomeResource",
      "metadata":{
        "annotations":{
          "anno":"value",
        },
        "creationTimestamp":"2024-02-05T12:42:32Z",
        "finalizers":["group.io/protection","other.group.io/protection"],
        "name":"some-resource-name",
        "namespace":"nsname",
        "ownerReferences":[
          {"apiVersion":"controller.group.io/v2",
           "blockOwnerDeletion":true,
           "controller":true,
           "kind":"SomeKind","name":"some-controller-name","uid":"904cfea9-c9d6-4d3a-82f7-5790b1a1b3e0"}
        ],
	    "resourceVersion":"265111919","uid":"4c74c3ff-2199-4f20-a71c-3b0e5fb505ca"
      },
      "spec":{"field1":"value1", "field2":"value2"},
      "status":{
        "conditions":[
          {"lastProbeTime":null,"lastTransitionTime":"2024-03-06T14:38:39Z","status":"True","type":"Ready"},
          {"lastProbeTime":"2024-02-29T14:11:05Z","lastTransitionTime":null,"status":"True","type":"Healthy"}],
        "printableStatus":"Ready"
	  }
    },

    "oldObject":{
      "apiVersion":"prefixed.resources.group.io/v1",
      "kind":"PrefixedSomeResource",
      "metadata":{
        "annotations":{
          "anno":"value",
        },
        "creationTimestamp":"2024-02-05T12:42:32Z",
        "finalizers":["group.io/protection","other.group.io/protection"],
        "name":"some-resource-name",
        "namespace":"nsname",
        "ownerReferences":[
          {"apiVersion":"controller.group.io/v2",
           "blockOwnerDeletion":true,
           "controller":true,
           "kind":"SomeKind","name":"some-controller-name","uid":"904cfea9-c9d6-4d3a-82f7-5790b1a1b3e0"}
        ],
	    "resourceVersion":"265111919","uid":"4c74c3ff-2199-4f20-a71c-3b0e5fb505ca"
      },
      "spec":{"field1":"value1", "field2":"value2"},
      "status":{
        "conditions":[
          {"lastProbeTime":null,"lastTransitionTime":"2024-03-06T14:38:39Z","status":"True","type":"Ready"},
          {"lastProbeTime":"2024-02-29T14:11:05Z","lastTransitionTime":null,"status":"True","type":"Healthy"}],
        "printableStatus":"Ready"
	  }
    }
  }
}
`
	admissionReviewRequest := `POST /validate-prefixed-resources-group-io-v1-prefixedsomeresource HTTP/1.1
Host: 127.0.0.1
Content-Type: application/json
Content-Length: ` + strconv.Itoa(len(admissionReview)) + `

` + admissionReview

	req, err := http.ReadRequest(bufio.NewReader(bytes.NewBufferString(admissionReviewRequest)))
	require.NoError(t, err, "should read hardcoded AdmissionReview request")

	rwr := createRewriterForDiscoveryTest()

	// Check getting TargetRequest from the webhook request.
	var targetReq *TargetRequest
	targetReq = NewTargetRequest(rwr, req)
	require.NotNil(t, targetReq, "should get TargetRequest")
	require.True(t, targetReq.ShouldRewriteRequest(), "should rewrite request in TargetRequest")

	// Check payload rewriting.
	resultBytes, err := rwr.RewriteJSONPayload(targetReq, []byte(admissionReview), Restore)
	require.NoError(t, err, "should rewrite request")
	if err != nil {
		t.Fatalf("should rewrite request: %v", err)
	}

	require.Greater(t, len(resultBytes), 0, "result bytes from RewriteJSONPayload should not be empty")

	groupRule, resRule := rwr.Rules.ResourceRules("original.group.io", "someresources")
	require.NotNil(t, resRule, "should get resourceRule for hardcoded group and resourceType")

	tests := []struct {
		path     string
		expected string
	}{
		{"request.kind.group", groupRule.Group},
		{"request.kind.kind", resRule.Kind},
		{"request.requestKind.group", groupRule.Group},
		{"request.requestKind.kind", resRule.Kind},
		{"request.resource.group", groupRule.Group},
		{"request.resource.resource", resRule.Plural},
		{"request.requestResource.group", groupRule.Group},
		{"request.requestResource.resource", resRule.Plural},
		{"request.object.apiVersion", groupRule.Group + "/v1"},
		{"request.object.kind", resRule.Kind},
		{"request.oldObject.apiVersion", groupRule.Group + "/v1"},
		{"request.oldObject.kind", resRule.Kind},
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
