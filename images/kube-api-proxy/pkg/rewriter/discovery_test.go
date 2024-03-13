package rewriter

import (
	"bufio"
	"bytes"
	"net/http"
	"strconv"
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
