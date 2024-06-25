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
	PodKind                       = "Pod"
	PodListKind                   = "PodList"
	ServiceKind                   = "Service"
	ServiceListKind               = "ServiceList"
	JobKind                       = "Job"
	JobListKind                   = "JobList"
	PersistentVolumeClaimKind     = "PersistentVolumeClaim"
	PersistentVolumeClaimListKind = "PersistentVolumeClaimList"
)

func RewritePod(rwRules *rules.RewriteRules, podObj []byte, action rules.Action) ([]byte, error) {
	podObj, err := RewriteLabelsMap(rwRules, podObj, "spec.nodeSelector", action)
	if err != nil {
		return nil, err
	}
	return RewriteAffinity(rwRules, podObj, "spec.affinity", action)
}

func RewriteService(rwRules *rules.RewriteRules, serviceObj []byte, action rules.Action) ([]byte, error) {
	return RewriteLabelsMap(rwRules, serviceObj, "spec.selector", action)
}

// RewriteJob transforms known fields in the Job manifest.
func RewriteJob(rwRules *rules.RewriteRules, jobObj []byte, action rules.Action) ([]byte, error) {
	return RewriteSpecTemplateLabelsAnno(rwRules, jobObj, "spec", action)
}

// RewritePVC transforms known fields in the PersistentVolumeClaim manifest.
func RewritePVC(rwRules *rules.RewriteRules, pvcObj []byte, action rules.Action) ([]byte, error) {
	pvcObj, err := transform.Object(pvcObj, "spec.dataSource", func(specDataSource []byte) ([]byte, error) {
		return RewriteAPIGroupAndKind(rwRules, specDataSource, action)
	})
	if err != nil {
		return nil, err
	}
	return transform.Object(pvcObj, "spec.dataSourceRef", func(specDataSourceRef []byte) ([]byte, error) {
		return RewriteAPIGroupAndKind(rwRules, specDataSourceRef, action)
	})
}

func RenameServicePatch(rwRules *rules.RewriteRules, obj []byte) ([]byte, error) {
	obj, err := RenameMetadataPatch(rwRules, obj)
	if err != nil {
		return nil, err
	}

	// Also rename patch on spec field.
	return transform.Patch(obj, nil, func(jsonPatch []byte) ([]byte, error) {
		path := gjson.GetBytes(jsonPatch, "path").String()
		switch path {
		case "/spec":
			return RewriteLabelsMap(rwRules, jsonPatch, "value.selector", rules.Rename)
		}
		return jsonPatch, nil
	})
}

func RewriteAPIGroupAndKind(rwRules *rules.RewriteRules, obj []byte, action rules.Action) ([]byte, error) {
	var err error
	kind := gjson.GetBytes(obj, "kind").String()

	obj, err = transform.String(obj, "kind", func(field string) string {
		if action == rules.Rename {
			return rwRules.RenameKind(field)
		}
		return rwRules.RestoreKind(field)
	})
	if err != nil {
		return nil, err
	}

	return transform.String(obj, "apiGroup", func(apiGroup string) string {
		if action == rules.Rename {
			return rwRules.RenamedGroup
		}
		// Renamed to original is a one-to-many relation, so we
		// need an original kind to get proper group for Restore action.
		groupRule, _ := rwRules.GroupResourceRulesByKind(rwRules.RestoreKind(kind))
		if groupRule == nil {
			return apiGroup
		}
		return groupRule.Group
	})
}
