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
	"strings"
)

type RewriteRules struct {
	KindPrefix         string                  `json:"kindPrefix"`
	ResourceTypePrefix string                  `json:"resourceTypePrefix"`
	ShortNamePrefix    string                  `json:"shortNamePrefix"`
	Categories         []string                `json:"categories"`
	RenamedGroup       string                  `json:"renamedGroup"`
	Rules              map[string]APIGroupRule `json:"rules"`
	Webhooks           map[string]WebhookRule  `json:"webhooks"`
	Labels             []ReplaceRule           `json:"labels"`
	Annotations        []ReplaceRule           `json:"annotations"`
	labelsOldToNew     map[string]string
	labelsNewToOld     map[string]string
	annoOldToNew       map[string]string
	annoNewToOld       map[string]string
}

func (rr *RewriteRules) Complete() {
	rr.labelsOldToNew = make(map[string]string, len(rr.Labels))
	rr.labelsNewToOld = make(map[string]string, len(rr.Labels))
	for _, l := range rr.Labels {
		rr.labelsOldToNew[l.Old] = l.New
		rr.labelsNewToOld[l.New] = l.Old
	}
	rr.annoOldToNew = make(map[string]string, len(rr.Annotations))
	rr.annoNewToOld = make(map[string]string, len(rr.Annotations))

	for _, a := range rr.Annotations {
		rr.annoOldToNew[a.Old] = a.New
		rr.annoNewToOld[a.New] = a.Old
	}
}

type APIGroupRule struct {
	GroupRule     GroupRule               `json:"groupRule"`
	ResourceRules map[string]ResourceRule `json:"resourceRules"`
}

type GroupRule struct {
	Group            string   `json:"group"`
	Versions         []string `json:"versions"`
	PreferredVersion string   `json:"preferredVersion"`
}

type ResourceRule struct {
	Kind             string   `json:"kind"`
	ListKind         string   `json:"listKind"`
	Plural           string   `json:"plural"`
	Singular         string   `json:"singular"`
	ShortNames       []string `json:"shortNames"`
	Categories       []string `json:"categories"`
	Versions         []string `json:"versions"`
	PreferredVersion string   `json:"preferredVersion"`
}

type WebhookRule struct {
	Path     string `json:"path"`
	Group    string `json:"group"`
	Resource string `json:"resource"`
}

type ReplaceRule struct {
	Old string `json:"old"`
	New string `json:"new"`
}

// GetAPIGroupList returns an array of groups in format applicable to use in APIGroupList:
//
//	{
//	  name
//	  versions: [ { groupVersion, version } ... ]
//	  preferredVersion: { groupVersion, version }
//	}
func (rr *RewriteRules) GetAPIGroupList() []interface{} {
	groups := make([]interface{}, 0)

	for _, rGroup := range rr.Rules {
		group := map[string]interface{}{
			"name": rGroup.GroupRule.Group,
			"preferredVersion": map[string]interface{}{
				"groupVersion": rGroup.GroupRule.Group + "/" + rGroup.GroupRule.PreferredVersion,
				"version":      rGroup.GroupRule.PreferredVersion,
			},
		}
		versions := make([]interface{}, 0)
		for _, ver := range rGroup.GroupRule.Versions {
			versions = append(versions, map[string]interface{}{
				"groupVersion": rGroup.GroupRule.Group + "/" + ver,
				"version":      ver,
			})
		}
		group["versions"] = versions
		groups = append(groups, group)
	}

	return groups
}

func (rr *RewriteRules) ResourceByKind(kind string) (string, string, bool) {
	for groupName, group := range rr.Rules {
		for resName, res := range group.ResourceRules {
			if res.Kind == kind {
				return groupName, resName, false
			}
			if res.ListKind == kind {
				return groupName, resName, true
			}
		}
	}
	return "", "", false
}

func (rr *RewriteRules) WebhookRule(path string) *WebhookRule {
	if webhookRule, ok := rr.Webhooks[path]; ok {
		return &webhookRule
	}
	return nil
}

func (rr *RewriteRules) HasGroup(group string) bool {
	_, ok := rr.Rules[group]
	return ok
}

func (rr *RewriteRules) GroupRule(group string) *GroupRule {
	if groupRule, ok := rr.Rules[group]; ok {
		return &groupRule.GroupRule
	}
	return nil
}

// KindRules returns rule for group and resource by apiGroup and kind.
// apiGroup may be a group or a group with version.
func (rr *RewriteRules) KindRules(apiGroup, kind string) (*GroupRule, *ResourceRule) {
	group, _, _ := strings.Cut(apiGroup, "/")
	groupRule, ok := rr.Rules[group]
	if !ok {
		return nil, nil
	}

	for _, resRule := range groupRule.ResourceRules {
		if resRule.Kind == kind {
			return &groupRule.GroupRule, &resRule
		}
		if resRule.ListKind == kind {
			return &groupRule.GroupRule, &resRule
		}
	}
	return nil, nil
}

func (rr *RewriteRules) ResourceRules(group, resource string) (*GroupRule, *ResourceRule) {
	groupRule, ok := rr.Rules[group]
	if !ok {
		return nil, nil
	}
	resourceRule, ok := rr.Rules[group].ResourceRules[resource]
	if !ok {
		return nil, nil
	}
	return &groupRule.GroupRule, &resourceRule
}

func (rr *RewriteRules) GroupResourceRules(resourceType string) (*GroupRule, *ResourceRule) {
	for _, group := range rr.Rules {
		for _, res := range group.ResourceRules {
			if res.Plural == resourceType {
				return &group.GroupRule, &res
			}
		}
	}
	return nil, nil
}

func (rr *RewriteRules) RenameResource(resource string) string {
	return rr.ResourceTypePrefix + resource
}

func (rr *RewriteRules) RenameKind(kind string) string {
	return rr.KindPrefix + kind
}

func (rr *RewriteRules) RestoreResource(resource string) string {
	return strings.TrimPrefix(resource, rr.ResourceTypePrefix)
}

func (rr *RewriteRules) RestoreKind(kind string) string {
	return strings.TrimPrefix(kind, rr.KindPrefix)
}

func (rr *RewriteRules) RestoreApiVersion(apiVersion string, group string) string {
	// Replace group, keep version.
	slashVersion := strings.TrimPrefix(apiVersion, rr.RenamedGroup)
	return group + slashVersion
}

func (rr *RewriteRules) RenameApiVersion(apiVersion string) string {
	// Replace group, keep version.
	apiVerParts := strings.Split(apiVersion, "/")
	if len(apiVerParts) != 2 {
		return apiVersion
	}
	return rr.RenamedGroup + "/" + apiVerParts[1]
}

func (rr *RewriteRules) RenameCategories(categories []string) []string {
	if len(categories) == 0 {
		return []string{}
	}
	return rr.Categories
}

func (rr *RewriteRules) RestoreCategories(resourceRule *ResourceRule) []string {
	if resourceRule == nil {
		return []string{}
	}
	return resourceRule.Categories
}

func (rr *RewriteRules) RenameShortNames(shortNames []string) []string {
	newNames := make([]string, 0, len(shortNames))
	for _, shortName := range shortNames {
		newNames = append(newNames, rr.ShortNamePrefix+shortName)
	}
	return newNames
}

func (rr *RewriteRules) RestoreShortNames(shortNames []string) []string {
	newNames := make([]string, 0, len(shortNames))
	for _, shortName := range shortNames {
		newNames = append(newNames, strings.TrimPrefix(shortName, rr.ShortNamePrefix))
	}
	return newNames
}

func (rr *RewriteRules) RenameLabel(label string) (string, bool) {
	v, ok := rr.labelsOldToNew[label]
	return v, ok
}

func (rr *RewriteRules) RestoreLabel(label string) (string, bool) {
	v, ok := rr.labelsOldToNew[label]
	return v, ok
}

func (rr *RewriteRules) RenameLabels(labels map[string]string) map[string]string {
	return rr.rewriteMaps(labels, rr.RenameLabel)
}

func (rr *RewriteRules) RestoreLabels(labels map[string]string) map[string]string {
	return rr.rewriteMaps(labels, rr.RestoreLabel)
}

func (rr *RewriteRules) RenameAnnotation(anno string) (string, bool) {
	v, ok := rr.annoOldToNew[anno]
	return v, ok
}

func (rr *RewriteRules) RestoreAnnotation(anno string) (string, bool) {
	v, ok := rr.annoNewToOld[anno]
	return v, ok
}

func (rr *RewriteRules) RenameAnnotations(anno map[string]string) map[string]string {
	return rr.rewriteMaps(anno, rr.RenameAnnotation)

}

func (rr *RewriteRules) RestoreAnnotations(anno map[string]string) map[string]string {
	return rr.rewriteMaps(anno, rr.RestoreAnnotation)
}

func (rr *RewriteRules) rewriteMaps(m map[string]string, fn func(s string) (string, bool)) map[string]string {
	result := make(map[string]string, len(m))
	for k, v := range m {
		newKey, found := fn(k)
		if !found {
			result[k] = v
			continue
		}
		result[newKey] = v
	}
	return result
}
