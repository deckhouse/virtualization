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

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"

	"kube-api-proxy/pkg/rewriter/rules"
	"kube-api-proxy/pkg/rewriter/transform"
)

const (
	ClusterRoleKind     = "ClusterRole"
	ClusterRoleListKind = "ClusterRoleList"
	RoleKind            = "Role"
	RoleListKind        = "RoleList"
)

// RewriteClusterRole rewrites rules array in a ClusterRole.
func RewriteClusterRole(rwRules *rules.RewriteRules, clusterRoleObj []byte, action rules.Action) ([]byte, error) {
	return transform.Array(clusterRoleObj, "rules", func(ruleObj []byte) ([]byte, error) {
		return RewriteRoleRule(rwRules, ruleObj, action)
	})
}

// RewriteRole rewrites rules array in a namespaced Role.
func RewriteRole(rwRules *rules.RewriteRules, roleObj []byte, action rules.Action) ([]byte, error) {
	return transform.Array(roleObj, "rules", func(ruleObj []byte) ([]byte, error) {
		return RewriteRoleRule(rwRules, ruleObj, action)
	})
}

// RewriteRoleRule rewrites apiGroups and resources in a single rule.
func RewriteRoleRule(rwRules *rules.RewriteRules, ruleObj []byte, action rules.Action) ([]byte, error) {
	if action == rules.Rename {
		return renameRoleRule(rwRules, ruleObj)
	}
	return restoreRoleRule(rwRules, ruleObj)
}

// renameRoleRule renames apiGroups and resources in a single rule.
// Rule examples:
//   - apiGroups:
//   - original.group.io
//     resources:
//   - '*'
//     verbs:
//   - '*'
//   - apiGroups:
//   - original.group.io
//     resources:
//   - someresources
//   - someresources/finalizers
//   - someresources/status
//   - someresources/scale
//     verbs:
//   - watch
//   - list
//   - create
func renameRoleRule(rwRules *rules.RewriteRules, obj []byte) ([]byte, error) {
	var err error

	apiGroups := gjson.GetBytes(obj, "apiGroups").Array()
	newGroups := []byte(`[]`)
	shouldRename := false
	for _, apiGroup := range apiGroups {
		group := apiGroup.String()
		if group == "*" {
			shouldRename = true
		} else if rwRules.HasGroup(group) {
			group = rwRules.RenamedGroup
			shouldRename = true
		}

		newGroups, err = sjson.SetBytes(newGroups, "-1", group)
		if err != nil {
			return nil, err
		}
	}

	if !shouldRename {
		return obj, nil
	}

	resources := gjson.GetBytes(obj, "resources").Array()
	newResources := []byte(`[]`)
	for _, resource := range resources {
		resourceType := resource.String()
		if strings.Contains(resourceType, "/") {
			resourceType, _, _ = strings.Cut(resource.String(), "/")
		}
		if resourceType != "*" {
			_, resRule := rwRules.GroupResourceRules(resourceType)
			if resRule != nil {
				// TODO(future) make it work with suffix and subresource.
				resourceType = rwRules.RenameResource(resource.String())
			}
		}

		newResources, err = sjson.SetBytes(newResources, "-1", resourceType)
		if err != nil {
			return nil, err
		}
	}

	obj, err = sjson.SetRawBytes(obj, "apiGroups", newGroups)
	if err != nil {
		return nil, err
	}
	return sjson.SetRawBytes(obj, "resources", newResources)
}

// restoreRoleRule restores apiGroups and resources in a single rule.
func restoreRoleRule(rwRules *rules.RewriteRules, obj []byte) ([]byte, error) {
	var err error

	apiGroups := gjson.GetBytes(obj, "apiGroups").Array()
	newGroups := []byte(`[]`)
	shouldRestore := false
	shouldAddGroup := false
	for _, apiGroup := range apiGroups {
		group := apiGroup.String()
		if group == "*" {
			shouldRestore = true
		}
		if group == rwRules.RenamedGroup {
			shouldRestore = true
			shouldAddGroup = true
			// Group will be restored later, do not add now.
			continue
		}
		newGroups, err = sjson.SetBytes(newGroups, "-1", group)
		if err != nil {
			return nil, err
		}
	}

	if !shouldRestore {
		return obj, nil
	}

	// Loop over resources and detect group from rules.

	resources := gjson.GetBytes(obj, "resources").Array()
	newResources := []byte(`[]`)
	shouldRestore = false
	groupToAdd := ""
	for _, resource := range resources {
		newResource := resource.String()
		resourceType := resource.String()
		//subresource := ""
		if strings.Contains(resourceType, "/") {
			resourceType, _, _ = strings.Cut(resourceType, "/")
		}
		if resourceType != "*" {
			// Restore resourceType to get rules.
			originalResourceType := rwRules.RestoreResource(resourceType)
			groupRule, resRule := rwRules.GroupResourceRules(originalResourceType)
			if groupRule != nil && resRule != nil {
				shouldRestore = true
				groupToAdd = groupRule.Group
				// NOTE: Restore resource with subresource.
				// TODO(future) make it work with suffixes.
				newResource = rwRules.RestoreResource(resource.String())
			}
		}

		newResources, err = sjson.SetBytes(newResources, "-1", newResource)
		if err != nil {
			return nil, err
		}
	}

	// Add restored group to apiGroups.
	if shouldAddGroup && groupToAdd != "" {
		newGroups, err = sjson.SetBytes(newGroups, "-1", groupToAdd)
		if err != nil {
			return nil, err
		}
	}

	obj, err = sjson.SetRawBytes(obj, "apiGroups", newGroups)
	if err != nil {
		return nil, err
	}
	return sjson.SetRawBytes(obj, "resources", newResources)
}
