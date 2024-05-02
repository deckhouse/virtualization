package rewriter

import (
	"strings"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

const (
	ClusterRoleKind     = "ClusterRole"
	ClusterRoleListKind = "ClusterRoleList"
	RoleKind            = "Role"
	RoleListKind        = "RoleList"
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

	apiGroups := gjson.GetBytes(obj, "apiGroups").Array()
	newGroups := []byte(`[]`)
	shouldRename := false
	for _, apiGroup := range apiGroups {
		group := apiGroup.String()
		if group == "*" {
			shouldRename = true
		} else if rules.HasGroup(group) {
			group = rules.RenamedGroup
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
	shouldAddGroup := false
	for _, apiGroup := range apiGroups {
		group := apiGroup.String()
		if group == "*" {
			shouldRestore = true
		}
		if group == rules.RenamedGroup {
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
			originalResourceType := rules.RestoreResource(resourceType)
			groupRule, resRule := rules.GroupResourceRules(originalResourceType)
			if groupRule != nil && resRule != nil {
				shouldRestore = true
				groupToAdd = groupRule.Group
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
