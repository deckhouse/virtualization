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
	"encoding/base64"
	"fmt"
	"net/http"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

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

	rwr := createTestRewriter()

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

func TestRewriteAdmissionReviewResponse(t *testing.T) {
	admissionReviewResponseTpl := `{
  "kind":"AdmissionReview",
  "apiVersion":"admission.k8s.io/v1",
  "response":{
    "uid":"389cfe15-34a1-4829-ad4d-de2576385711",
    "allowed": true,
    "patchType": "JSONPatch",
    "patch": "%s"
  }
}
`
	admissionReviewRequest := `POST /validate-prefixed-resources-group-io-v1-prefixedsomeresource HTTP/1.1
Host: 127.0.0.1
Content-Type: application/json

`

	req, err := http.ReadRequest(bufio.NewReader(bytes.NewBufferString(admissionReviewRequest)))
	require.NoError(t, err, "should read hardcoded AdmissionReview request")

	rwr := createTestRewriter()

	// Check getting TargetRequest from the webhook request.
	var targetReq *TargetRequest
	targetReq = NewTargetRequest(rwr, req)
	require.NotNil(t, targetReq, "should get TargetRequest")
	require.True(t, targetReq.ShouldRewriteRequest(), "should rewrite request in TargetRequest")

	// Check patches rewriting.

	tests := []struct {
		name     string
		patch    string
		expected string
	}{
		{
			"rename label in replace op",
			`[{"op":"replace","path":"/metadata/labels","value":{"labelgroup.io":"labelValue"}}]`,
			`[{"op":"replace","path":"/metadata/labels","value":{"replacedlabelgroup.io":"labelValue"}}]`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b64Patch := base64.StdEncoding.EncodeToString([]byte(tt.patch))
			payload := fmt.Sprintf(admissionReviewResponseTpl, b64Patch)

			resultBytes, err := rwr.RewriteJSONPayload(targetReq, []byte(payload), Rename)
			require.NoError(t, err, "should rewrite AdmissionRequest response")
			if err != nil {
				t.Fatalf("should rewrite AdmissionRequest response: %v", err)
			}

			require.Greater(t, len(resultBytes), 0, "result bytes from RewriteJSONPayload should not be empty")

			b64Actual := gjson.GetBytes(resultBytes, "response.patch").String()
			actual, err := base64.StdEncoding.DecodeString(b64Actual)
			require.NoError(t, err, "should decode result patch: '%s'", b64Actual)

			require.NotEqual(t, tt.expected, actual, "%s value should be %s, got %s", tt.name, tt.expected, actual)
		})
	}
}
