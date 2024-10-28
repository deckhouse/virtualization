package rewriter

const (
	ValidatingAdmissionPolicyKind            = "ValidatingAdmissionPolicy"
	ValidatingAdmissionPolicyListKind        = "ValidatingAdmissionPolicyList"
	ValidatingAdmissionPolicyBindingKind     = "ValidatingAdmissionPolicyBinding"
	ValidatingAdmissionPolicyBindingListKind = "ValidatingAdmissionPolicyBindingList"
)

func RewriteValidatingAdmissionPolicyOrList(rules *RewriteRules, obj []byte, action Action) ([]byte, error) {
	if action == Rename {
		return RewriteResourceOrList(obj, ValidatingAdmissionPolicyListKind, func(singleObj []byte) ([]byte, error) {
			return RewriteArray(singleObj, "spec.matchConstraints.resourceRules", func(item []byte) ([]byte, error) {
				return renameResourceRules(rules, item)
			})
		})
	}
	return RewriteResourceOrList(obj, ValidatingAdmissionPolicyListKind, func(singleObj []byte) ([]byte, error) {
		return RewriteArray(singleObj, "spec.matchConstraints.resourceRules", func(item []byte) ([]byte, error) {
			return restoreResourceRules(rules, item)
		})
	})
}

func RewriteValidatingAdmissionPolicyBindingOrList(rules *RewriteRules, obj []byte, action Action) ([]byte, error) {
	if action == Rename {
		return RewriteResourceOrList(obj, ValidatingAdmissionPolicyBindingListKind, func(singleObj []byte) ([]byte, error) {
			return RewriteArray(singleObj, "spec.matchResources.resourceRules", func(item []byte) ([]byte, error) {
				return renameResourceRules(rules, item)
			})
		})
	}
	return RewriteResourceOrList(obj, ValidatingAdmissionPolicyBindingListKind, func(singleObj []byte) ([]byte, error) {
		return RewriteArray(singleObj, "spec.matchResources.resourceRules", func(item []byte) ([]byte, error) {
			return restoreResourceRules(rules, item)
		})
	})
}

// renameValidatingAdmissionPolicyBinding renames apiGroups and resources in a single resourceRule.
// Rule examples:
//	resourceRules:
//	- apiGroups:
//	    - ""
//	  apiVersions:
//      - '*'
//    operations:
//      - '*'
//    resources:
//      - nodes
//    scope: '*'

func renameResourceRules(rules *RewriteRules, obj []byte) ([]byte, error) {
	var err error

	renameResources := false
	obj, err = TransformArrayOfStrings(obj, "apiGroups", func(apiGroup string) string {
		if rules.HasGroup(apiGroup) {
			renameResources = true
			return rules.RenameApiVersion(apiGroup)
		}
		if apiGroup == "*" {
			renameResources = true
		}
		return apiGroup
	})
	if err != nil {
		return nil, err
	}

	// Do not rename resources for unknown group.
	if !renameResources {
		return obj, nil
	}

	return TransformArrayOfStrings(obj, "resources", func(resourceType string) string {
		if resourceType == "*" || resourceType == "" {
			return resourceType
		}

		// Rename if there is rule for resourceType.
		_, resRule := rules.GroupResourceRules(resourceType)
		if resRule != nil {
			return rules.RenameResource(resourceType)
		}
		return resourceType
	})
}

func restoreResourceRules(rules *RewriteRules, obj []byte) ([]byte, error) {
	var err error

	restoreResources := false
	obj, err = TransformArrayOfStrings(obj, "apiGroups", func(apiGroup string) string {
		if rules.IsRenamedGroup(apiGroup) {
			restoreResources = true
			return rules.RestoreApiVersion(apiGroup)
		}
		if apiGroup == "*" {
			restoreResources = true
		}
		return apiGroup
	})
	if err != nil {
		return nil, err
	}

	// Do not rename resources for unknown group.
	if !restoreResources {
		return obj, nil
	}

	return TransformArrayOfStrings(obj, "resources", func(resourceType string) string {
		if resourceType == "*" || resourceType == "" {
			return resourceType
		}
		// Get rules for resource by restored resourceType.
		originalResourceType := rules.RestoreResource(resourceType)
		_, resRule := rules.GroupResourceRules(originalResourceType)
		if resRule != nil {
			// NOTE: subresource not trimmed.
			return originalResourceType
		}

		// No rules for resourceType, return as-is
		return resourceType
	})
}
