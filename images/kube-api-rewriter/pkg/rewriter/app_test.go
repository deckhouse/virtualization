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

func createTestRewriterForApp() *RuleBasedRewriter {
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

	rules := &RewriteRules{
		KindPrefix:         "Prefixed", // KV
		ResourceTypePrefix: "prefixed", // kv
		ShortNamePrefix:    "p",
		Categories:         []string{"prefixed"},
		Rules:              apiGroupRules,
		Labels: MetadataReplace{
			Prefixes: []MetadataReplaceRule{
				{Original: "labelgroup.io", Renamed: "replacedlabelgroup.io"},
				{Original: "component.labelgroup.io", Renamed: "component.replacedlabelgroup.io"},
			},
			Names: []MetadataReplaceRule{
				{Original: "labelgroup.io", Renamed: "replacedlabelgroup.io"},
				{
					Original: "labelgroup.io", OriginalValue: "some-value",
					Renamed: "replacedlabelgroup.io", RenamedValue: "some-value-renamed",
				},
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

func TestRenameDeploymentLabels(t *testing.T) {
	deploymentReq := `POST /apis/apps/v1/deployments/testdeployment HTTP/1.1
Host: 127.0.0.1

`
	deploymentBody := `{
"apiVersion": "apiextensions.k8s.io/v1",
"kind": "Deployment",
"metadata": {
	"name":"testdeployment",
	"labels":{
		"labelgroup.io": "labelValue",
		"labelgroup.io/labelName": "labelValue",
		"component.labelgroup.io/labelName": "labelValue"
	},
	"annotations": {
		"annogroup.io": "annoValue",
		"annogroup.io/annoName": "annoValue",
		"component.annogroup.io/annoName": "annoValue"
	}
},
"spec": {
	"replicas": 1,
	"selector": {
		"matchLabels": {
			"labelgroup.io": "labelValue",
			"labelgroup.io/labelName": "labelValue",
			"component.labelgroup.io/labelName": "labelValue"
		}
	},
	"template": {
		"metadata": {
			"name":"testdeployment",
			"labels":{
				"labelgroup.io": "labelValue",
				"labelgroup.io/labelName": "labelValue",
				"component.labelgroup.io/labelName": "labelValue"
			},
			"annotations": {
				"annogroup.io": "annoValue",
				"annogroup.io/annoName": "annoValue",
				"component.annogroup.io/annoName": "annoValue"
			}
		},
		"spec": {
			"nodeSelector": {
				"labelgroup.io": "labelValue",
				"labelgroup.io/labelName": "labelValue",
				"component.labelgroup.io/labelName": "labelValue"
			},
			"affinity": {
				"podAntiAffinity": {
					"preferredDuringSchedulingIgnoredDuringExecution": [
						{
							"podAffinityTerm": {
								"labelSelector": {
									"matchExpressions":[{
										"key": "labelgroup.io",
										"operator":"In",
										"values": ["some-value"]
									}]
								},
								"topologyKey": "kubernetes.io/hostname"
							},
							"weight": 1
						}
					]
				},
				"nodeAffinity": {
					"preferredDuringSchedulingIgnoredDuringExecution": [
						{
							"preference": {
								"matchExpressions":[{
									"key": "labelgroup.io",
									"operator":"In",
									"values": ["some-value"]
								}]
							},
							"weight": 1
						}
					]
				}
			},
			"containers": []
		}
	}
}
}`
	req, err := http.ReadRequest(bufio.NewReader(bytes.NewBufferString(deploymentReq + deploymentBody)))
	require.NoError(t, err, "should parse hardcoded http request")
	require.NotNil(t, req.URL, "should parse url in hardcoded http request")

	rwr := createTestRewriterForApp()
	targetReq := NewTargetRequest(rwr, req)
	require.NotNil(t, targetReq, "should get TargetRequest")
	require.True(t, targetReq.ShouldRewriteRequest(), "should rewrite request")
	require.True(t, targetReq.ShouldRewriteResponse(), "should rewrite response")
	// require.Equal(t, origGroup, targetReq.OrigGroup(), "should set proper orig group")

	resultBytes, err := rwr.RewriteJSONPayload(targetReq, []byte(deploymentBody), Rename)
	if err != nil {
		t.Fatalf("should rename Deployment without error: %v", err)
	}
	if resultBytes == nil {
		t.Fatalf("should rename Deployment: %v", err)
	}

	tests := []struct {
		path     string
		expected string
	}{
		{`metadata.labels.replacedlabelgroup\.io`, "labelValue"},
		{`metadata.labels.labelgroup\.io`, ""},
		{`metadata.labels.replacedlabelgroup\.io/labelName`, "labelValue"},
		{`metadata.labels.labelgroup\.io/labelName`, ""},
		{`metadata.labels.component\.replacedlabelgroup\.io/labelName`, "labelValue"},
		{`metadata.labels.component\.labelgroup\.io/labelName`, ""},
		{`metadata.annotations.replacedannogroup\.io`, "annoValue"},
		{`metadata.annotations.annogroup\.io`, ""},
		{`metadata.annotations.replacedannogroup\.io/annoName`, "annoValue"},
		{`metadata.annotations.annogroup\.io/annoName`, ""},
		{`metadata.annotations.component\.replacedannogroup\.io/annoName`, "annoValue"},
		{`metadata.annotations.component\.annogroup\.io/annoName`, ""},
		{`spec.template.spec.affinity.podAntiAffinity.preferredDuringSchedulingIgnoredDuringExecution.0.podAffinityTerm.labelSelector.matchExpressions.0.key`, "replacedlabelgroup.io"},
		{`spec.template.spec.affinity.podAntiAffinity.preferredDuringSchedulingIgnoredDuringExecution.0.podAffinityTerm.labelSelector.matchExpressions.0.values`, `["some-value-renamed"]`},
		{`spec.template.spec.affinity.nodeAffinity.preferredDuringSchedulingIgnoredDuringExecution.0.preference.matchExpressions.0.key`, "replacedlabelgroup.io"},
		{`spec.template.spec.affinity.nodeAffinity.preferredDuringSchedulingIgnoredDuringExecution.0.preference.matchExpressions.0.values`, `["some-value-renamed"]`},
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
