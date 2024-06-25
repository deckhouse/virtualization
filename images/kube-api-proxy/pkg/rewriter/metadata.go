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

func RewriteMetadata(rwRules *rules.RewriteRules, obj []byte, action rules.Action) ([]byte, error) {
	//var err error
	obj, err := RewriteLabelsMap(rwRules, obj, "metadata.labels", action)
	if err != nil {
		return nil, err
	}
	obj, err = RewriteAnnotationsMap(rwRules, obj, "metadata.annotations", action)
	if err != nil {
		return nil, err
	}
	return RewriteFinalizers(rwRules, obj, "metadata.finalizers", action)
}

// RenameMetadataPatch transforms known metadata fields in patches.
// Example:
// - merge patch on metadata:
// {"metadata": { "labels": {"kubevirt.io/schedulable": "false", "cpumanager": "false"}, "annotations": {"kubevirt.io/heartbeat": "2024-06-07T23:27:53Z"}}}
// - JSON patch on metadata:
// [{"op":"test", "path":"/metadata/labels", "value":{"label":"value"}},
//
//	{"op":"replace", "path":"/metadata/labels", "value":{"label":"newValue"}}]
func RenameMetadataPatch(rwRules *rules.RewriteRules, patch []byte) ([]byte, error) {
	return transform.Patch(patch,
		func(mergePatch []byte) ([]byte, error) {
			return RewriteMetadata(rwRules, mergePatch, rules.Rename)
		},
		func(jsonPatch []byte) ([]byte, error) {
			path := gjson.GetBytes(jsonPatch, "path").String()
			switch path {
			case "/metadata/labels":
				return RewriteLabelsMap(rwRules, jsonPatch, "value", rules.Rename)
			case "/metadata/annotations":
				return RewriteAnnotationsMap(rwRules, jsonPatch, "value", rules.Rename)
			case "/metadata/finalizers":
				return RewriteFinalizers(rwRules, jsonPatch, "value", rules.Rename)
			}
			return jsonPatch, nil
		})
}

func RewriteLabelsMap(rules *rules.RewriteRules, obj []byte, path string, action rules.Action) ([]byte, error) {
	return transform.MapStringString(obj, path, func(k, v string) (string, string) {
		return rules.LabelsRewriter().Rewrite(k, action), v
	})
}

func RewriteAnnotationsMap(rules *rules.RewriteRules, obj []byte, path string, action rules.Action) ([]byte, error) {
	return transform.MapStringString(obj, path, func(k, v string) (string, string) {
		return rules.AnnotationsRewriter().Rewrite(k, action), v
	})
}

func RewriteFinalizers(rules *rules.RewriteRules, obj []byte, path string, action rules.Action) ([]byte, error) {
	return transform.ArrayOfStrings(obj, path, func(finalizer string) string {
		return rules.FinalizersRewriter().Rewrite(finalizer, action)
	})
}
