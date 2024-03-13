package rewriter

import "strings"

type RewriteRules struct {
	KindPrefix   string
	URLPrefix    string
	RenamedGroup string
	Rules        map[string]APIGroupRule
	Webhooks     map[string]WebhookRule
}

type APIGroupRule struct {
	GroupRule     GroupRule
	ResourceRules map[string]ResourceRule
}

type GroupRule struct {
	Group            string
	Versions         []string
	PreferredVersion string
}

type ResourceRule struct {
	Kind             string
	ListKind         string
	Plural           string
	Singular         string
	Versions         []string
	PreferredVersion string
}

type WebhookRule struct {
	Path     string
	Group    string
	Resource string
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

func (rr *RewriteRules) RenameResource(resource string) string {
	return rr.URLPrefix + resource
}

func (rr *RewriteRules) RenameKind(kind string) string {
	return rr.KindPrefix + kind
}

func (rr *RewriteRules) RestoreResource(resource string) string {
	return strings.TrimPrefix(resource, rr.URLPrefix)
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
