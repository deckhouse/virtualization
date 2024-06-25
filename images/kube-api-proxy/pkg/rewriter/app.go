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
	"github.com/tidwall/gjson"

	"kube-api-proxy/pkg/rewriter/rules"
	"kube-api-proxy/pkg/rewriter/transform"
)

const (
	DeploymentKind      = "Deployment"
	DeploymentListKind  = "DeploymentList"
	DaemonSetKind       = "DaemonSet"
	DaemonSetListKind   = "DaemonSetList"
	StatefulSetKind     = "StatefulSet"
	StatefulSetListKind = "StatefulSetList"
)

func RewriteDeployment(rwRules *rules.RewriteRules, deploymentObj []byte, action rules.Action) ([]byte, error) {
	return RewriteSpecTemplateLabelsAnno(rwRules, deploymentObj, "spec", action)
}

func RewriteDaemonSet(rwRules *rules.RewriteRules, daemonSetObj []byte, action rules.Action) ([]byte, error) {
	return RewriteSpecTemplateLabelsAnno(rwRules, daemonSetObj, "spec", action)
}

func RewriteStatefulSet(rwRules *rules.RewriteRules, stsObj []byte, action rules.Action) ([]byte, error) {
	return RewriteSpecTemplateLabelsAnno(rwRules, stsObj, "spec", action)
}

func RenameSpecTemplatePatch(rwRules *rules.RewriteRules, obj []byte) ([]byte, error) {
	obj, err := RenameMetadataPatch(rwRules, obj)
	if err != nil {
		return nil, err
	}

	return transform.Patch(obj, func(mergePatch []byte) ([]byte, error) {
		return RewriteSpecTemplateLabelsAnno(rwRules, mergePatch, "spec", rules.Rename)
	}, func(jsonPatch []byte) ([]byte, error) {
		path := gjson.GetBytes(jsonPatch, "path").String()
		if path == "/spec" {
			return RewriteSpecTemplateLabelsAnno(rwRules, jsonPatch, "value", rules.Rename)
		}
		return jsonPatch, nil
	})
}

// RewriteSpecTemplateLabelsAnno transforms labels and annotations in spec fields:
// - selector as LabelSelector
// - template.metadata.labels as labels map
// - template.metadata.annotations as annotations map
// - template.affinity as Affinity
// - template.nodeSelector as labels map.
func RewriteSpecTemplateLabelsAnno(rwRules *rules.RewriteRules, obj []byte, path string, action rules.Action) ([]byte, error) {
	return transform.Object(obj, path, func(obj []byte) ([]byte, error) {
		obj, err := RewriteLabelsMap(rwRules, obj, "template.metadata.labels", action)
		if err != nil {
			return nil, err
		}
		obj, err = RewriteLabelsMap(rwRules, obj, "selector.matchLabels", action)
		if err != nil {
			return nil, err
		}
		obj, err = RewriteLabelsMap(rwRules, obj, "template.spec.nodeSelector", action)
		if err != nil {
			return nil, err
		}
		obj, err = RewriteAffinity(rwRules, obj, "template.spec.affinity", action)
		if err != nil {
			return nil, err
		}
		return RewriteAnnotationsMap(rwRules, obj, "template.metadata.annotations", action)
	})
}
