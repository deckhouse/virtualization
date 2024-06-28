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
)

const (
	ClusterRoleKind            = "ClusterRole"
	ClusterRoleListKind        = "ClusterRoleList"
	RoleKind                   = "Role"
	RoleListKind               = "RoleList"
	RoleBindingKind            = "RoleBinding"
	RoleBindingListKind        = "RoleBindingList"
	ControllerRevisionKind     = "ControllerRevision"
	ControllerRevisionListKind = "ControllerRevisionList"
	ClusterRoleBindingKind     = "ClusterRoleBinding"
	ClusterRoleBindingListKind = "ClusterRoleBindingList"
	APIServiceKind             = "APIService"
	APIServiceListKind         = "APIServiceList"
)

func RewriteClusterRoleOrList(rules *RewriteRules, obj []byte, action Action) ([]byte, error) {
	if action == Rename {
		return RewriteResourceOrList(obj, ClusterRoleListKind, func(singleObj []byte) ([]byte, error) {
			return RewriteArray(singleObj, "rules", func(item []byte) ([]byte, error) {
				return renameRoleRule(rules, item)
			})
		})
	}
	return RewriteResourceOrList(obj, ClusterRoleListKind, func(singleObj []byte) ([]byte, error) {
		return RewriteArray(singleObj, "rules", func(item []byte) ([]byte, error) {
			return restoreRoleRule(rules, item)
		})
	})
}

func RewriteRoleOrList(rules *RewriteRules, obj []byte, action Action) ([]byte, error) {
	if action == Rename {
		return RewriteResourceOrList(obj, RoleListKind, func(singleObj []byte) ([]byte, error) {
			return RewriteArray(singleObj, "rules", func(item []byte) ([]byte, error) {
				return renameRoleRule(rules, item)
			})
		})
	}
	return RewriteResourceOrList(obj, RoleListKind, func(singleObj []byte) ([]byte, error) {
		return RewriteArray(singleObj, "rules", func(item []byte) ([]byte, error) {
			return restoreRoleRule(rules, item)
		})
	})
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
func renameRoleRule(rules *RewriteRules, obj []byte) ([]byte, error) {
	var err error

	apiGroups := gjson.GetBytes(obj, "apiGroups")
	newGroups := []byte(`[]`)
	shouldRenameResources := false
	shouldAddRenamedGroup := true
	for _, apiGroup := range apiGroups.Array() {
		group := apiGroup.String()
		if group == "*" {
			shouldRenameResources = true
		} else if rules.HasGroup(group) {
			shouldAddRenamedGroup = true
			shouldRenameResources = true
		}
		// Put group as-is in a new array.
		newGroups, err = sjson.SetBytes(newGroups, "-1", group)
		if err != nil {
			return nil, err
		}
	}

	// Add renamed group to apiGroups to enable proper restoring.
	// Removing original group from rule will make restoring ambiguous.
	if shouldAddRenamedGroup {
		newGroups, err = sjson.SetBytes(newGroups, "-1", rules.RenamedGroup)
		if err != nil {
			return nil, err
		}
	}

	if !shouldRenameResources {
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
			_, resRule := rules.GroupResourceRules(resourceType)
			if resRule != nil {
				// TODO(future) make it work with suffix and subresource.
				resourceType = rules.RenameResource(resource.String())
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
func restoreRoleRule(rules *RewriteRules, obj []byte) ([]byte, error) {
	var err error

	apiGroups := gjson.GetBytes(obj, "apiGroups").Array()
	newGroups := []byte(`[]`)
	shouldRestore := false
	for _, apiGroup := range apiGroups {
		group := apiGroup.String()
		if group == "*" {
			shouldRestore = true
		}
		if group == rules.RenamedGroup {
			shouldRestore = true
			// Just ignore renamed group. Original groups are already present in array.
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

	// Restore resources from rules.
	resources := gjson.GetBytes(obj, "resources").Array()
	newResources := []byte(`[]`)
	shouldRestore = false
	for _, resource := range resources {
		newResource := resource.String()
		resourceType := resource.String()
		//subresource := ""
		if strings.Contains(resourceType, "/") {
			resourceType, _, _ = strings.Cut(resourceType, "/")
		}
		if resourceType != "*" {
			// Restore resourceType to get rules.
			originalResourceType := rules.RestoreResource(resourceType)
			groupRule, resRule := rules.GroupResourceRules(originalResourceType)
			if groupRule != nil && resRule != nil {
				// NOTE: Restore resource with subresource.
				// TODO(future) make it work with suffixes.
				newResource = rules.RestoreResource(resource.String())
			}
		}

		newResources, err = sjson.SetBytes(newResources, "-1", newResource)
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
