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
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func createTestRewriter() *RuleBasedRewriter {
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

	rules := &RewriteRules{
		KindPrefix:         "Prefixed",
		ResourceTypePrefix: "prefixed",
		ShortNamePrefix:    "p",
		Categories:         []string{"prefixed"},
		Rules:              apiGroupRules,
		Webhooks:           webhookRules,
		Labels: MetadataReplace{
			Prefixes: []MetadataReplaceRule{
				{Original: "labelgroup.io", Renamed: "replacedlabelgroup.io"},
				{Original: "component.labelgroup.io", Renamed: "component.replacedlabelgroup.io"},
			},
			Names: []MetadataReplaceRule{
				{Original: "labelgroup.io", Renamed: "replacedlabelgroup.io"},
				{
					Original: "labelgroup.io", OriginalValue: "labelValueToRename",
					Renamed: "replacedlabelgroup.io", RenamedValue: "renamedLabelValue",
				},
			},
		},
		Annotations: MetadataReplace{
			Names: []MetadataReplaceRule{
				{Original: "annogroup.io", Renamed: "replacedanno.io"},
			},
		},
	}
	rules.Init()
	return &RuleBasedRewriter{
		Rules: rules,
	}
}

func TestRewriteAPIEndpoint(t *testing.T) {
	tests := []struct {
		name        string
		path        string
		expectPath  string
		expectQuery string
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
			"labelSelector one label name",
			"/api/v1/namespaces/nsname/pods?labelSelector=labelgroup.io&limit=0",
			"/api/v1/namespaces/nsname/pods",
			"labelSelector=replacedlabelgroup.io&limit=0",
		},
		{
			"labelSelector one prefixed label",
			"/api/v1/pods?labelSelector=labelgroup.io%2Fsome-attr&limit=500",
			"/api/v1/pods",
			"labelSelector=replacedlabelgroup.io%2Fsome-attr&limit=500",
		},
		{
			"labelSelector label name and value",
			"/api/v1/namespaces/d8-virtualization/pods?labelSelector=labelgroup.io%3Dlabelvalue&limit=500",
			"/api/v1/namespaces/d8-virtualization/pods",
			"labelSelector=replacedlabelgroup.io%3Dlabelvalue&limit=500",
		},
		{
			"labelSelector prefixed label and value",
			"/api/v1/namespaces/d8-virtualization/pods?labelSelector=component.labelgroup.io%2Fsome-attr%3Dlabelvalue&limit=500",
			"/api/v1/namespaces/d8-virtualization/pods",
			"labelSelector=component.replacedlabelgroup.io%2Fsome-attr%3Dlabelvalue&limit=500",
		},
		{
			"labelSelector label name not in values",
			"/api/v1/namespaces/d8-virtualization/pods?labelSelector=labelgroup.io+notin+%28value-one%2Cvalue-two%29&limit=500",
			"/api/v1/namespaces/d8-virtualization/pods",
			"labelSelector=replacedlabelgroup.io+notin+%28value-one%2Cvalue-two%29&limit=500",
		},
		{
			"labelSelector label name for deployments",
			"/apis/apps/v1/deployments?labelSelector=labelgroup.io+notin+%28value-one%2ClabelValue%29&limit=500",
			"/apis/apps/v1/deployments",
			"labelSelector=replacedlabelgroup.io+notin+%28labelValue%2Cvalue-one%29&limit=500",
		},
		{
			"labelSelector label name and renamed value",
			"/api/v1/namespaces/d8-virtualization/pods?labelSelector=labelgroup.io%3DlabelValueToRename&limit=500",
			"/api/v1/namespaces/d8-virtualization/pods",
			"labelSelector=replacedlabelgroup.io%3DrenamedLabelValue&limit=500",
		},
		{
			"labelSelector label name and renamed values",
			"/api/v1/namespaces/d8-virtualization/pods?labelSelector=labelgroup.io+notin+%28value-one%2ClabelValueToRename%29&limit=500",
			"/api/v1/namespaces/d8-virtualization/pods",
			"labelSelector=replacedlabelgroup.io+notin+%28renamedLabelValue%2Cvalue-one%29&limit=500",
		},
		{
			"labelSelector label name and renamed values for deployments",
			"/apis/apps/v1/deployments?labelSelector=labelgroup.io+notin+%28value-one%2ClabelValueToRename%29&limit=500",
			"/apis/apps/v1/deployments",
			"labelSelector=replacedlabelgroup.io+notin+%28renamedLabelValue%2Cvalue-one%29&limit=500",
		},
		{
			"labelSelector label name and renamed values for validating admission policy binding",
			"/apis/admissionregistration.k8s.io/v1/validatingadmissionpolicybindings?labelSelector=labelgroup.io+notin+%28value-one%2ClabelValueToRename%29&limit=500",
			"/apis/admissionregistration.k8s.io/v1/validatingadmissionpolicybindings",
			"labelSelector=replacedlabelgroup.io+notin+%28renamedLabelValue%2Cvalue-one%29&limit=500",
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
			require.NotNil(t, newEp, "should rewrite path '%s', got nil endpoint. Original ep: %#v", tt.path, ep)

			require.Equal(t, tt.expectPath, newEp.Path(), "expect rewrite for path '%s' to be '%s', got '%s', newEp: %#v", tt.path, tt.expectPath, newEp.Path(), newEp)
			require.Equal(t, tt.expectQuery, newEp.RawQuery, "expect rewrite query for path %q to be '%s', got '%s', newEp: %#v", tt.path, tt.expectQuery, newEp.RawQuery, newEp)
		})
	}

}

func TestRestoreControllerRevisionList(t *testing.T) {
	getControllerRevisions := `GET /apis/apps/v1/controllerrevisions HTTP/1.1
Host: 127.0.0.1

`
	responseBody := `{
"kind":"ControllerRevisionList",
"apiVersion":"apps/v1",
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

	req, err := http.ReadRequest(bufio.NewReader(bytes.NewBufferString(getControllerRevisions)))
	require.NoError(t, err, "should parse hardcoded http request")
	require.NotNil(t, req.URL, "should parse url in hardcoded http request")

	rwr := createTestRewriter()
	targetReq := NewTargetRequest(rwr, req)
	require.NotNil(t, targetReq, "should get TargetRequest")
	require.True(t, targetReq.ShouldRewriteRequest(), "should rewrite request")
	require.True(t, targetReq.ShouldRewriteResponse(), "should rewrite response")
	// require.Equal(t, origGroup, targetReq.OrigGroup(), "should set proper orig group")

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
		{`kind`, "ControllerRevisionList"},
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

func TestRenameControllerRevision(t *testing.T) {
	postControllerRevision := `POST /apis/apps/v1/controllerrevisions/namespaces/ns/ctrl-rev-name HTTP/1.1
Host: 127.0.0.1

`
	requestBody := `{
"kind":"ControllerRevision",
"apiVersion":"apps/v1",
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
		t.Fatalf("should rename RevisionController without error: %v", err)
	}
	if resultBytes == nil {
		t.Fatalf("should rename RevisionController: %v", err)
	}

	tests := []struct {
		path     string
		expected string
	}{
		{`kind`, "ControllerRevision"},
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
