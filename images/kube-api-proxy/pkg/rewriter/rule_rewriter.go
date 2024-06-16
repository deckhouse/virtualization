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
	"regexp"
	"strings"

	"github.com/tidwall/gjson"
)

type RuleBasedRewriter struct {
	Rules *RewriteRules
}

type Action string

const (
	// Restore is an action to restore resources to original.
	Restore Action = "restore"
	// Rename is an action to rename original resources.
	Rename Action = "rename"
)

// RewriteAPIEndpoint renames group and resource in /apis/* endpoints.
// It assumes that ep contains original group and resourceType.
// Restoring of path is not implemented.
func (rw *RuleBasedRewriter) RewriteAPIEndpoint(ep *APIEndpoint) *APIEndpoint {
	// Leave paths /, /api, /api/*, and unknown paths as is.
	if ep.IsRoot || ep.IsCore || ep.IsUnknown {
		return nil
	}

	// Rename CRD name resourcetype.group for resources with rules.
	if ep.IsCRD {
		// No endpoint rewrite for CRD list.
		if ep.CRDGroup == "" && ep.CRDResourceType == "" {
			if strings.Contains(ep.RawQuery, "metadata.name") {
				// Rewrite name in field selector if any.
				newQuery := rw.rewriteFieldSelector(ep.RawQuery)
				if newQuery != "" {
					res := ep.Clone()
					res.RawQuery = newQuery
					return res
				}
			}
			return nil
		}

		// Check if resource has rules
		_, resourceRule := rw.Rules.ResourceRules(ep.CRDGroup, ep.CRDResourceType)
		if resourceRule == nil {
			// No rewrite for CRD without rules.
			return nil
		}
		// Rewrite CRD name.
		res := ep.Clone()
		res.CRDGroup = rw.Rules.RenamedGroup
		res.CRDResourceType = rw.Rules.RenameResource(res.CRDResourceType)
		res.Name = res.CRDResourceType + "." + res.CRDGroup
		return res
	}

	// Rename group and resource for CR requests.
	newGroup := ""
	if ep.Group != "" {
		groupRule := rw.Rules.GroupRule(ep.Group)
		if groupRule == nil {
			// No rewrite for group without rules.
			return nil
		}
		newGroup = rw.Rules.RenamedGroup
	}

	newResource := ""
	if ep.ResourceType != "" {
		_, resRule := rw.Rules.ResourceRules(ep.Group, ep.ResourceType)
		if resRule != nil {
			newResource = rw.Rules.RenameResource(ep.ResourceType)
		}
	}

	// Return rewritten endpoint if group or resource are changed.
	if newGroup != "" || newResource != "" {
		res := ep.Clone()
		if newGroup != "" {
			res.Group = newGroup
		}
		if newResource != "" {
			res.ResourceType = newResource
		}

		return res
	}

	return nil
}

var metadataNameRe = regexp.MustCompile(`metadata.name\%3D([a-z0-9-]+)((\.[a-z0-9-]+)*)`)

// rewriteFieldSelector rewrites value for metadata.name in fieldSelector of CRDs listing.
// Example request:
// https://APISERVER/apis/apiextensions.k8s.io/v1/customresourcedefinitions?fieldSelector=metadata.name%3Dresources.original.group.io&...
func (rw *RuleBasedRewriter) rewriteFieldSelector(rawQuery string) string {
	matches := metadataNameRe.FindStringSubmatch(rawQuery)
	if matches == nil {
		return ""
	}

	resourceType := matches[1]
	group := matches[2]
	group = strings.TrimPrefix(group, ".")

	_, resRule := rw.Rules.ResourceRules(group, resourceType)
	if resRule == nil {
		return ""
	}

	group = rw.Rules.RenamedGroup
	resourceType = rw.Rules.RenameResource(resourceType)

	newSelector := `metadata.name%3D` + resourceType + "." + group

	return metadataNameRe.ReplaceAllString(rawQuery, newSelector)
}

// RewriteJSONPayload does rewrite based on kind.
func (rw *RuleBasedRewriter) RewriteJSONPayload(targetReq *TargetRequest, obj []byte, action Action) ([]byte, error) {
	// Detect Kind
	kind := gjson.GetBytes(obj, "kind").String()

	var rwrBytes []byte
	var err error

	//// Handle core resources: rewrite only for specific kind.
	//if targetReq.IsCore() {
	//	pass := true
	//	switch kind {
	//	case "APIGroupList":
	//	case "APIGroup":
	//	case "APIResourceList":
	//	default:
	//		pass = shouldPassCoreResource(kind)
	//	}
	//	if pass {
	//		return obj, nil
	//	}
	//}

	switch kind {
	case "APIGroupList":
		rwrBytes, err = RewriteAPIGroupList(rw.Rules, obj)

	case "APIGroup":
		rwrBytes, err = RewriteAPIGroup(rw.Rules, obj, targetReq.OrigGroup())

	case "APIResourceList":
		rwrBytes, err = RewriteAPIResourceList(rw.Rules, obj, targetReq.OrigGroup())

	case "APIGroupDiscoveryList":
		rwrBytes, err = RewriteAPIGroupDiscoveryList(rw.Rules, obj)

	case "AdmissionReview":
		rwrBytes, err = RewriteAdmissionReview(rw.Rules, obj, targetReq.OrigGroup())

	case CRDKind, CRDListKind:
		rwrBytes, err = RewriteCRDOrList(rw.Rules, obj, action)

	case MutatingWebhookConfigurationKind,
		MutatingWebhookConfigurationListKind:
		rwrBytes, err = RewriteMutatingOrList(rw.Rules, obj, action)

	case ValidatingWebhookConfigurationKind,
		ValidatingWebhookConfigurationListKind:
		rwrBytes, err = RewriteValidatingOrList(rw.Rules, obj, action)

	case ClusterRoleKind, ClusterRoleListKind:
		rwrBytes, err = RewriteClusterRoleOrList(rw.Rules, obj, action)

	case RoleKind, RoleListKind:
		rwrBytes, err = RewriteRoleOrList(rw.Rules, obj, action)

	default:
		if targetReq.IsCore() {
			rwrBytes, err = RewriteOwnerReferences(rw.Rules, obj, action)
		} else {
			rwrBytes, err = RewriteCustomResourceOrList(rw.Rules, obj, action)
		}
	}

	// Return obj bytes as-is in case of the error.
	if err != nil {
		return obj, err
	}

	if shouldRewriteOwnerReferences(kind) {
		rwrBytes, err = RewriteOwnerReferences(rw.Rules, rwrBytes, action)
	}

	// Return obj bytes as-is in case of the error.
	if err != nil {
		return obj, err
	}

	return rwrBytes, nil
}

// RewritePatch rewrites patches for some known objects.
// Only rename action is required for patches.
func (rw *RuleBasedRewriter) RewritePatch(targetReq *TargetRequest, obj []byte) ([]byte, error) {
	if targetReq.IsCRD() {
		// Check if CRD is known.
		_, resRule := rw.Rules.ResourceRules(targetReq.OrigGroup(), targetReq.OrigResourceType())
		if resRule == nil {
			return obj, nil
		}

		return RenameCRDPatch(rw.Rules, resRule, obj)
	}

	return obj, nil
}

func shouldRewriteOwnerReferences(resourceType string) bool {
	switch resourceType {
	case CRDKind, CRDListKind,
		RoleKind, RoleListKind,
		RoleBindingKind, RoleBindingListKind,
		PodDisruptionBudgetKind, PodDisruptionBudgetListKind,
		ControllerRevisionKind, ControllerRevisionListKind,
		ClusterRoleKind, ClusterRoleListKind,
		ClusterRoleBindingKind, ClusterRoleBindingListKind,
		APIServiceKind, APIServiceListKind,
		DeploymentKind, DeploymentListKind,
		ValidatingWebhookConfigurationKind,
		ValidatingWebhookConfigurationListKind,
		MutatingWebhookConfigurationKind,
		MutatingWebhookConfigurationListKind:
		return true
	}

	return false
}
