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
	Labels             MetadataReplace         `json:"labels"`
	Annotations        MetadataReplace         `json:"annotations"`
	Finalizers         MetadataReplace         `json:"finalizers"`
	labelsIndexer      *MetadataIndexer
	annoIndexer        *MetadataIndexer
	finalizerIndexer   *MetadataIndexer
}

func (rr *RewriteRules) Complete() {
	rr.labelsIndexer = rr.Labels.Complete()
	rr.annoIndexer = rr.Annotations.Complete()
	rr.finalizerIndexer = rr.Finalizers.Complete()
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

type MetadataReplace struct {
	Prefixes []MetadataReplaceRule
	Names    []MetadataReplaceRule
}

func (mr *MetadataReplace) Complete() *MetadataIndexer {
	namesOldToNew := make(map[string]string, len(mr.Names))
	namesNewToOld := make(map[string]string, len(mr.Names))
	for _, l := range mr.Names {
		namesOldToNew[l.Old] = l.New
		namesNewToOld[l.New] = l.Old
	}
	prefixOldToNew := make(map[string]string, len(mr.Prefixes))
	prefixNewToOld := make(map[string]string, len(mr.Prefixes))
	for _, l := range mr.Prefixes {
		prefixOldToNew[l.Old] = l.New
		prefixNewToOld[l.New] = l.Old
	}
	return &MetadataIndexer{
		namesOldToNew:  namesOldToNew,
		namesNewToOld:  namesNewToOld,
		prefixOldToNew: prefixOldToNew,
		prefixNewToOld: prefixNewToOld,
	}
}

type MetadataIndexer struct {
	namesOldToNew  map[string]string
	namesNewToOld  map[string]string
	prefixOldToNew map[string]string
	prefixNewToOld map[string]string
}

func (mi *MetadataIndexer) GetOld(s string) (string, bool) {
	v, found := mi.namesNewToOld[s]
	return v, found
}

func (mi *MetadataIndexer) GetNew(s string) (string, bool) {
	v, found := mi.namesOldToNew[s]
	return v, found
}

func (mi *MetadataIndexer) GetOldPrefix(s string) (string, bool) {
	v, found := mi.prefixNewToOld[s]
	return v, found
}

func (mi *MetadataIndexer) GetNewPrefix(s string) (string, bool) {
	v, found := mi.prefixOldToNew[s]
	return v, found
}

type MetadataReplaceRule struct {
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

func (rr *RewriteRules) RenameLabel(label string) string {
	return rr.rename(label, rr.labelsIndexer)

}

func (rr *RewriteRules) RestoreLabel(label string) string {
	return rr.restore(label, rr.labelsIndexer)
}

func (rr *RewriteRules) RenameLabels(labels map[string]string) map[string]string {
	return rr.rewriteMaps(labels, rr.RenameLabel)
}

func (rr *RewriteRules) RestoreLabels(labels map[string]string) map[string]string {
	return rr.rewriteMaps(labels, rr.RestoreLabel)
}

func (rr *RewriteRules) RenameAnnotation(anno string) string {
	return rr.rename(anno, rr.annoIndexer)
}

func (rr *RewriteRules) RestoreAnnotation(anno string) string {
	return rr.restore(anno, rr.annoIndexer)
}

func (rr *RewriteRules) RenameAnnotations(annotations map[string]string) map[string]string {
	return rr.rewriteMaps(annotations, rr.RenameAnnotation)

}

func (rr *RewriteRules) RestoreAnnotations(annotations map[string]string) map[string]string {
	return rr.rewriteMaps(annotations, rr.RestoreAnnotation)
}

func (rr *RewriteRules) RenameFinalizer(fin string) string {
	return rr.rename(fin, rr.annoIndexer)
}

func (rr *RewriteRules) RestoreFinalizer(fin string) string {
	return rr.restore(fin, rr.annoIndexer)
}

func (rr *RewriteRules) RenameFinalizers(fins []string) []string {
	return rr.rewriteSlices(fins, rr.RenameFinalizer)

}

func (rr *RewriteRules) RestoreFinalizers(fins []string) []string {
	return rr.rewriteSlices(fins, rr.RestoreFinalizer)
}

func (rr *RewriteRules) rewriteMaps(m map[string]string, fn func(s string) string) map[string]string {
	result := make(map[string]string, len(m))
	for k, v := range m {
		result[fn(k)] = v
	}
	return result
}

func (rr *RewriteRules) rewriteSlices(s []string, fn func(s string) string) []string {
	result := make([]string, len(s))
	for i, ss := range s {
		result[i] = fn(ss)
	}
	return result
}

func (rr *RewriteRules) rename(s string, indexer *MetadataIndexer) string {
	if indexer == nil {
		return s
	}
	if v, ok := indexer.GetNew(s); ok {
		return v
	}
	prefix, _, found := strings.Cut(s, "/")
	if !found {
		return s
	}
	if v, ok := indexer.GetNewPrefix(prefix); ok {
		return v + strings.TrimPrefix(s, prefix)
	}
	return s
}

func (rr *RewriteRules) restore(s string, indexer *MetadataIndexer) string {
	if indexer == nil {
		return s
	}
	if v, ok := indexer.GetOld(s); ok {
		return v
	}
	prefix, _, found := strings.Cut(s, "/")
	if !found {
		return s
	}
	if v, ok := indexer.GetOldPrefix(prefix); ok {
		return v + strings.TrimPrefix(s, prefix)
	}
	return s
}
