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

func TestRewriteEvent(t *testing.T) {
	eventReq := `POST /api/v1/namespaces/vm/events HTTP/1.1
Host: 127.0.0.1

`
	eventPayload := `{
  "kind": "Event",
  "apiVersion": "v1",
  "metadata": {
    "name": "some-event-name",
    "namespace": "vm",
  },
  "involvedObject": {
    "kind": "SomeResource",
    "namespace": "vm",
    "name": "some-vm-name",
    "uid": "ad9f7357-f6b0-4679-8571-042c75ec53fb",
    "apiVersion": "original.group.io/v1"
  },
  "reason": "EventReason",
  "message": "Event message for some-vm-name",
  "source": {
    "component": "some-component",
    "host": "some-node"
  },
  "count": 1000,
  "type": "Warning",
  "eventTime": null,
  "reportingComponent": "some-component",
  "reportingInstance": "some-node"
}`

	req, err := http.ReadRequest(bufio.NewReader(bytes.NewBufferString(eventReq + eventPayload)))
	require.NoError(t, err, "should parse hardcoded http request")
	require.NotNil(t, req.URL, "should parse url in hardcoded http request")

	rwr := createTestRewriterForCore()
	targetReq := NewTargetRequest(rwr, req)
	require.NotNil(t, targetReq, "should get TargetRequest")
	require.True(t, targetReq.ShouldRewriteRequest(), "should rewrite request")
	require.True(t, targetReq.ShouldRewriteResponse(), "should rewrite response")
	// require.Equal(t, origGroup, targetReq.OrigGroup(), "should set proper orig group")

	resultBytes, err := rwr.RewriteJSONPayload(targetReq, []byte(eventPayload), Rename)
	if err != nil {
		t.Fatalf("should rename Error without error: %v", err)
	}
	if resultBytes == nil {
		t.Fatalf("should rename Error: %v", err)
	}

	tests := []struct {
		path     string
		expected string
	}{
		{`involvedObject.kind`, "PrefixedSomeResource"},
		{`involvedObject.apiVersion`, "prefixed.resources.group.io/v1"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			actual := gjson.GetBytes(resultBytes, tt.path).String()
			if actual != tt.expected {
				t.Fatalf("%s value should be %s, got %s", tt.path, tt.expected, actual)
			}
		})
	}

	// Restore.
	resultBytes, err = rwr.RewriteJSONPayload(targetReq, []byte(eventPayload), Restore)
	if err != nil {
		t.Fatalf("should restore PVC without error: %v", err)
	}
	if resultBytes == nil {
		t.Fatalf("should restore PVC: %v", err)
	}

	tests = []struct {
		path     string
		expected string
	}{
		{`involvedObject.kind`, "SomeResource"},
		{`involvedObject.apiVersion`, "original.group.io/v1"},
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
