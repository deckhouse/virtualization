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
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestRewriteMetadata(t *testing.T) {
	tests := []struct {
		name              string
		obj               client.Object
		newObj            client.Object
		action            Action
		expectLabels      map[string]string
		expectAnnotations map[string]string
	}{
		{
			"rename labels on Pod",
			&corev1.Pod{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Pod",
					APIVersion: "v1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "bar",
					Labels: map[string]string{
						"labelgroup.io":                    "labelvalue",
						"component.labelgroup.io/labelkey": "labelvalue",
					},
					Annotations: map[string]string{
						"annogroup.io": "annovalue",
					},
				},
			},
			&corev1.Pod{},
			Rename,
			map[string]string{
				"replacedlabelgroup.io":                    "labelvalue",
				"component.replacedlabelgroup.io/labelkey": "labelvalue",
			},
			map[string]string{
				"replacedanno.io": "annovalue",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.NotNil(t, tt.obj, "should not be nil")

			rwr := createTestRewriter()
			bytes, err := json.Marshal(tt.obj)
			require.NoError(t, err, "should marshal object %q %s/%s", tt.obj.GetObjectKind().GroupVersionKind().Kind, tt.obj.GetName(), tt.obj.GetNamespace())

			rwBytes, err := TransformObject(bytes, "metadata", func(metadataObj []byte) ([]byte, error) {
				return RewriteMetadata(rwr.Rules, metadataObj, tt.action)
			})
			require.NoError(t, err, "should rewrite object")

			err = json.Unmarshal(rwBytes, &tt.newObj)

			require.NoError(t, err, "should unmarshal object")

			require.Equal(t, tt.expectLabels, tt.newObj.GetLabels(), "expect rewrite labels '%v' to be '%s', got '%s'", tt.obj.GetLabels(), tt.expectLabels, tt.newObj.GetLabels())
			require.Equal(t, tt.expectAnnotations, tt.newObj.GetAnnotations(), "expect rewrite annotations '%v' to be '%s', got '%s'", tt.obj.GetAnnotations(), tt.expectAnnotations, tt.newObj.GetAnnotations())
		})
	}
}

func TestRestoreKnownCustomResourceList(t *testing.T) {
	listKnownCR := `GET /apis/original.group.io/v1/someresources HTTP/1.1
Host: 127.0.0.1

`
	responseBody := `{
"kind":"PrefixedSomeResourceList",
"apiVersion":"prefixed.resources.group.io/v1",
"metadata":{"resourceVersion":"412742959"},
"items":[
	{
      	"metadata": {
			"name": "resource-name",
			"namespace": "ns-name",
			"labels": {
				"component.replacedlabelgroup.io/labelName": "labelValue"
			},
			"annotations":{
				"replacedanno.io": "annoValue"
			},
        	"ownerReferences": [
			{
            	"apiVersion": "prefixed.resources.group.io/v1",
            	"kind": "PrefixedSomeResource",
            	"name": "owner-name",
            	"uid": "30b43f23-0c36-442f-897f-fececdf54620",
            	"controller": true,
            	"blockOwnerDeletion": true
          	},
			{
            	"apiVersion": "other.product.group.io/v1alpha1",
            	"kind": "SomeResource",
            	"name": "another-owner-name",
            	"controller": true,
            	"blockOwnerDeletion": true
          	}
        	]
		},
		"data": {"somekey":"somevalue"}
	}
]}`

	req, err := http.ReadRequest(bufio.NewReader(bytes.NewBufferString(listKnownCR)))
	require.NoError(t, err, "should parse hardcoded http request")
	require.NotNil(t, req.URL, "should parse url in hardcoded http request")

	rwr := createTestRewriter()
	targetReq := NewTargetRequest(rwr, req)
	require.NotNil(t, targetReq, "should get TargetRequest")
	require.True(t, targetReq.ShouldRewriteRequest(), "should rewrite request")
	require.True(t, targetReq.ShouldRewriteResponse(), "should rewrite response")
	require.Equal(t, "original.group.io", targetReq.OrigGroup(), "should set proper orig group")

	resultBytes, err := rwr.RewriteJSONPayload(targetReq, []byte(responseBody), Restore)
	if err != nil {
		t.Fatalf("should restore RevisionControllerList without error: %v", err)
	}
	if resultBytes == nil {
		t.Fatalf("should restore RevisionControllerList: %v", err)
	}

	tests := []struct {
		path     string
		expected string
	}{
		{`kind`, "SomeResourceList"},
		{`items.0.metadata.labels.component\.replacedlabelgroup\.io/labelName`, ""},
		{`items.0.metadata.labels.component\.labelgroup\.io/labelName`, "labelValue"},
		{`items.0.metadata.annotations.replacedanno\.io`, ""},
		{`items.0.metadata.annotations.annogroup\.io`, "annoValue"},
		{`items.0.metadata.ownerReferences.0.apiVersion`, "original.group.io/v1"},
		{`items.0.metadata.ownerReferences.0.kind`, "SomeResource"},
		// "other.progduct.group.io" is not known for rules, this ownerRef should not be rewritten.
		{`items.0.metadata.ownerReferences.1.apiVersion`, "other.product.group.io/v1alpha1"},
		{`items.0.metadata.ownerReferences.1.kind`, "SomeResource"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			actual := gjson.GetBytes(resultBytes, tt.path).String()
			if actual != tt.expected {
				t.Log(string(resultBytes))
				t.Fatalf("%s value should be %s, got %s", tt.path, tt.expected, actual)
			}
		})
	}
}

// TODO this rewrite will be enabled later. Uncomment TestRestoreUnknownCustomResourceListWithKnownKind after enabling.
func TestNoRewriteForUnknownCustomResourceListWithKnownKind(t *testing.T) {
	// Request list of  resources with known kind but with unknown apiGroup.
	// Check that RestoreResourceList will not rewrite apiVersion.
	listUnknownCR := `GET /apis/other.product.group.io/v1alpha1/someresources HTTP/1.1
Host: 127.0.0.1

`
	req, err := http.ReadRequest(bufio.NewReader(bytes.NewBufferString(listUnknownCR)))
	require.NoError(t, err, "should parse hardcoded http request")
	require.NotNil(t, req.URL, "should parse url in hardcoded http request")

	rwr := createTestRewriter()
	targetReq := NewTargetRequest(rwr, req)
	require.NotNil(t, targetReq, "should get TargetRequest")

	require.False(t, targetReq.ShouldRewriteRequest(), "should not rewrite request")
	require.False(t, targetReq.ShouldRewriteResponse(), "should not rewrite response")
}

// TODO Uncomment after enabling rewrite detection by apiVersion/kind for all resources.
/*
func TestRestoreUnknownCustomResourceListWithKnownKind(t *testing.T) {
	// Request list of  resources with known kind but with unknown apiGroup.
	// Check that RestoreResourceList will not rewrite apiVersion.
	listUnknownCR := `GET /apis/other.product.group.io/v1alpha1/someresources HTTP/1.1
Host: 127.0.0.1

`
	responseBody := `{
"kind":"SomeResourceList",
"apiVersion":"other.product.group.io/v1alpha1",
"metadata":{"resourceVersion":"412742959"},
"items":[
	{
      	"metadata": {
			"name": "resource-name",
			"namespace": "ns-name",
			"labels": {
				"component.replacedlabelgroup.io/labelName": "labelValue"
			},
			"annotations":{
				"replacedanno.io": "annoValue"
			},
        	"ownerReferences": [
			{
            	"apiVersion": "prefixed.resources.group.io/v1",
            	"kind": "PrefixedSomeResource",
            	"name": "owner-name",
            	"uid": "30b43f23-0c36-442f-897f-fececdf54620",
            	"controller": true,
            	"blockOwnerDeletion": true
          	},
			{
            	"apiVersion": "other.product.group.io/v1alpha1",
            	"kind": "SomeResource",
            	"name": "another-owner-name",
            	"controller": true,
            	"blockOwnerDeletion": true
          	}
        	]
		},
		"data": {"somekey":"somevalue"}
	}
]}`

	req, err := http.ReadRequest(bufio.NewReader(bytes.NewBufferString(listUnknownCR)))
	require.NoError(t, err, "should parse hardcoded http request")
	require.NotNil(t, req.URL, "should parse url in hardcoded http request")

	rwr := createTestRewriter()
	targetReq := NewTargetRequest(rwr, req)
	require.NotNil(t, targetReq, "should get TargetRequest")

	require.False(t, targetReq.ShouldRewriteRequest(), "should not rewrite request")
	require.False(t, targetReq.ShouldRewriteResponse(), "should not rewrite response")

	require.True(t, targetReq.ShouldRewriteRequest(), "should rewrite request")
	require.True(t, targetReq.ShouldRewriteResponse(), "should rewrite response")
	require.Equal(t, "original.group.io", targetReq.OrigGroup(), "should set proper orig group")

	resultBytes, err := rwr.RewriteJSONPayload(targetReq, []byte(responseBody), Restore)
	if err != nil {
		t.Fatalf("should restore RevisionControllerList without error: %v", err)
	}
	if resultBytes == nil {
		t.Fatalf("should restore RevisionControllerList: %v", err)
	}

	tests := []struct {
		path     string
		expected string
	}{
		{`kind`, "SomeResourceList"},
		{`apiVersion`, "other.product.group.io/v1alpha1"},
		{`items.0.metadata.labels.component\.replacedlabelgroup\.io/labelName`, ""},
		{`items.0.metadata.labels.component\.labelgroup\.io/labelName`, "labelValue"},
		{`items.0.metadata.annotations.replacedanno\.io`, ""},
		{`items.0.metadata.annotations.annogroup\.io`, "annoValue"},
		{`items.0.metadata.ownerReferences.0.apiVersion`, "original.group.io/v1"},
		{`items.0.metadata.ownerReferences.0.kind`, "SomeResource"},
		// "other.progduct.group.io" is not known for rules, this ownerRef should not be rewritten.
		{`items.0.metadata.ownerReferences.1.apiVersion`, "other.product.group.io/v1alpha1"},
		{`items.0.metadata.ownerReferences.1.kind`, "SomeResource"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			actual := gjson.GetBytes(resultBytes, tt.path).String()
			if actual != tt.expected {
				t.Log(string(resultBytes))
				t.Fatalf("%s value should be %s, got %s", tt.path, tt.expected, actual)
			}
		})
	}
}
*/

func TestRenameKnownCustomResource(t *testing.T) {
	postControllerRevision := `POST /apis/original.group.io/v1/someresources/namespaces/ns-name/resource-name HTTP/1.1
Host: 127.0.0.1

`
	requestBody := `{
"kind":"SomeResource",
"apiVersion":"original.group.io/v1",
"metadata": {
	"name": "resource-name",
	"namespace": "ns-name",
	"labels": {
		"component.labelgroup.io/labelName": "labelValue"
	},
	"annotations":{
		"annogroup.io": "annoValue"
	},
	"ownerReferences": [
	{
		"apiVersion": "original.group.io/v1",
		"kind": "SomeResource",
		"name": "owner-name",
		"uid": "30b43f23-0c36-442f-897f-fececdf54620",
		"controller": true,
		"blockOwnerDeletion": true
	},
	{
		"apiVersion": "other.product.group.io/v1alpha1",
		"kind": "SomeResource",
		"name": "another-owner-name",
		"controller": true,
		"blockOwnerDeletion": true
	}
	]
},
"data": {"somekey":"somevalue"}
}`

	req, err := http.ReadRequest(bufio.NewReader(bytes.NewBufferString(postControllerRevision + requestBody)))
	require.NoError(t, err, "should parse hardcoded http request")
	require.NotNil(t, req.URL, "should parse url in hardcoded http request")

	rwr := createTestRewriter()
	targetReq := NewTargetRequest(rwr, req)
	require.NotNil(t, targetReq, "should get TargetRequest")
	require.True(t, targetReq.ShouldRewriteRequest(), "should rewrite request")
	require.True(t, targetReq.ShouldRewriteResponse(), "should rewrite response")

	resultBytes, err := rwr.RewriteJSONPayload(targetReq, []byte(requestBody), Rename)
	if err != nil {
		t.Fatalf("should rename SomeResource without error: %v", err)
	}
	if resultBytes == nil {
		t.Fatalf("should rename SomeResource: %v", err)
	}

	tests := []struct {
		path     string
		expected string
	}{
		{`kind`, "PrefixedSomeResource"},
		{`metadata.labels.component\.replacedlabelgroup\.io/labelName`, "labelValue"},
		{`metadata.labels.component\.labelgroup\.io/labelName`, ""},
		{`metadata.annotations.replacedanno\.io`, "annoValue"},
		{`metadata.annotations.annogroup\.io`, ""},
		{`metadata.ownerReferences.0.apiVersion`, "prefixed.resources.group.io/v1"},
		{`metadata.ownerReferences.0.kind`, "PrefixedSomeResource"},
		// "other.progduct.group.io" is not known for rules, this ownerRef should not be rewritten.
		{`metadata.ownerReferences.1.apiVersion`, "other.product.group.io/v1alpha1"},
		{`metadata.ownerReferences.1.kind`, "SomeResource"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			actual := gjson.GetBytes(resultBytes, tt.path).String()
			if actual != tt.expected {
				t.Log(string(resultBytes))
				t.Fatalf("%s value should be %s, got %s", tt.path, tt.expected, actual)
			}
		})
	}
}
