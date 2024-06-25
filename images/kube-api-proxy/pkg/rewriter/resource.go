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
	"github.com/tidwall/sjson"
)

func RewriteCustomResourceOrList(rules *RewriteRules, obj []byte, action Action) ([]byte, error) {
	kind := gjson.GetBytes(obj, "kind").String()
	if action == Restore {
		kind = rules.RestoreKind(kind)
	}
	origGroupName, origResName, isList := rules.ResourceByKind(kind)
	if origGroupName == "" && origResName == "" {
		// Return as-is if kind is not in rules.
		return obj, nil
	}
	if isList {
		if action == Restore {
			return RestoreResourcesList(rules, obj, origGroupName)
		}

		return RenameResourcesList(rules, obj)
	}

	// Responses of GET, LIST, DELETE requests.
	// AdmissionReview requests from API Server.
	if action == Restore {
		return RestoreResource(rules, obj, origGroupName)
	}
	// CREATE, UPDATE, PATCH requests.
	// TODO need to implement for
	return RenameResource(rules, obj)
}

func RenameResourcesList(rules *RewriteRules, obj []byte) ([]byte, error) {
	obj, err := RenameAPIVersionAndKind(rules, obj)
	if err != nil {
		return nil, err
	}

	// Rewrite apiVersion and kind in each item.
	items := gjson.GetBytes(obj, "items").Array()
	rwrItems := []byte(`[]`)
	for _, item := range items {
		rwrItem, err := RenameResource(rules, []byte(item.Raw))
		if err != nil {
			return nil, err
		}
		rwrItems, err = sjson.SetRawBytes(rwrItems, "-1", rwrItem)
	}

	obj, err = sjson.SetRawBytes(obj, "items", rwrItems)
	if err != nil {
		return nil, err
	}

	return obj, nil
}

func RestoreResourcesList(rules *RewriteRules, obj []byte, origGroupName string) ([]byte, error) {
	obj, err := RestoreAPIVersionAndKind(rules, obj, origGroupName)
	if err != nil {
		return nil, err
	}

	// Rewrite apiVersion and kind in each item.
	items := gjson.GetBytes(obj, "items").Array()
	rwrItems := []byte(`[]`)
	for _, item := range items {
		rwrItem, err := RestoreResource(rules, []byte(item.Raw), origGroupName)
		if err != nil {
			return nil, err
		}
		rwrItems, err = sjson.SetRawBytes(rwrItems, "-1", rwrItem)
	}

	obj, err = sjson.SetRawBytes(obj, "items", rwrItems)
	if err != nil {
		return nil, err
	}

	return obj, nil
}

func RenameResource(rules *RewriteRules, obj []byte) ([]byte, error) {
	obj, err := RenameAPIVersionAndKind(rules, obj)
	if err != nil {
		return nil, err
	}

	// Rewrite apiVersion in each managedFields.
	return RenameManagedFields(rules, obj)
}

func RestoreResource(rules *RewriteRules, obj []byte, origGroupName string) ([]byte, error) {
	obj, err := RestoreAPIVersionAndKind(rules, obj, origGroupName)
	if err != nil {
		return nil, err
	}

	// Rewrite apiVersion in each managedFields.
	return RestoreManagedFields(rules, obj, origGroupName)
}

func RenameAPIVersionAndKind(rules *RewriteRules, obj []byte) ([]byte, error) {
	apiVersion := gjson.GetBytes(obj, "apiVersion").String()
	obj, err := sjson.SetBytes(obj, "apiVersion", rules.RenameApiVersion(apiVersion))
	if err != nil {
		return nil, err
	}

	kind := gjson.GetBytes(obj, "kind").String()
	return sjson.SetBytes(obj, "kind", rules.RenameKind(kind))
}

func RestoreAPIVersionAndKind(rules *RewriteRules, obj []byte, origGroupName string) ([]byte, error) {
	apiVersion := gjson.GetBytes(obj, "apiVersion").String()
	apiVersion = rules.RestoreApiVersion(apiVersion, origGroupName)
	obj, err := sjson.SetBytes(obj, "apiVersion", apiVersion)
	if err != nil {
		return nil, err
	}

	kind := gjson.GetBytes(obj, "kind").String()
	return sjson.SetBytes(obj, "kind", rules.RestoreKind(kind))
}

func RewriteOwnerReferences(rules *RewriteRules, obj []byte, action Action) ([]byte, error) {
	return RewriteArray(obj, "metadata.ownerReferences", func(ownerRefObj []byte) ([]byte, error) {
		kind := gjson.GetBytes(ownerRefObj, "kind").String()
		apiVersion := gjson.GetBytes(ownerRefObj, "apiVersion").String()

		rwrApiVersion := ""
		rwrKind := ""
		if action == Rename {
			groupRule, resourceRule := rules.KindRules(apiVersion, kind)
			if groupRule != nil && resourceRule != nil {
				rwrApiVersion = rules.RenameApiVersion(apiVersion)
				rwrKind = rules.RenameKind(kind)
			}
		}
		if action == Restore {
			if rules.IsRenamedGroup(apiVersion) {
				restoredKind := rules.RestoreKind(kind)
				origGroup, origResource, _ := rules.ResourceByKind(restoredKind)
				if origGroup != "" && origResource != "" {
					rwrApiVersion = rules.RestoreApiVersion(apiVersion, origGroup)
					rwrKind = restoredKind
				}
			}
		}

		if rwrApiVersion == "" || rwrKind == "" {
			// No rewrite for OwnerReference without rules.
			return ownerRefObj, nil
		}

		ownerRefObj, err := sjson.SetBytes(ownerRefObj, "kind", rwrKind)
		if err != nil {
			return nil, err
		}

		return sjson.SetBytes(ownerRefObj, "apiVersion", rwrApiVersion)
	})
}

// RestoreManagedFields restores apiVersion in managedFields items.
//
// Example response from the server:
//
//	"metadata": {
//	  "managedFields":[
//	    { "apiVersion":"x.virtualization.deckhouse.io/v1", "fieldsType":"FieldsV1", "fieldsV1":{ ... }}, "manager": "Go-http-client", ...},
//	    { "apiVersion":"x.virtualization.deckhouse.io/v1", "fieldsType":"FieldsV1", "fieldsV1":{ ... }}, "manager": "kubectl-edit", ...}
//	  ],
func RestoreManagedFields(rules *RewriteRules, obj []byte, origGroupName string) ([]byte, error) {
	mgFields := gjson.GetBytes(obj, "metadata.managedFields")
	if !mgFields.Exists() || len(mgFields.Array()) == 0 {
		return obj, nil
	}

	newFields := []byte(`[]`)
	for _, mgField := range mgFields.Array() {
		apiVersion := mgField.Get("apiVersion").String()
		restoredAPIVersion := rules.RestoreApiVersion(apiVersion, origGroupName)
		newField, err := sjson.SetBytes([]byte(mgField.Raw), "apiVersion", restoredAPIVersion)
		if err != nil {
			return nil, err
		}
		newFields, err = sjson.SetRawBytes(newFields, "-1", newField)
		if err != nil {
			return nil, err
		}
	}
	return sjson.SetRawBytes(obj, "metadata.managedFields", newFields)
}

// RenameManagedFields renames apiVersion in managedFields items.
//
// Example request from the client:
//
//	"metadata": {
//	  "managedFields":[
//	    { "apiVersion":"kubevirt.io/v1", "fieldsType":"FieldsV1", "fieldsV1":{ ... }}, "manager": "Go-http-client", ...},
//	    { "apiVersion":"kubevirt.io/v1", "fieldsType":"FieldsV1", "fieldsV1":{ ... }}, "manager": "kubectl-edit", ...}
//	  ],
func RenameManagedFields(rules *RewriteRules, obj []byte) ([]byte, error) {
	mgFields := gjson.GetBytes(obj, "metadata.managedFields")
	if !mgFields.Exists() || len(mgFields.Array()) == 0 {
		return obj, nil
	}

	newFields := []byte(`[]`)
	for _, mgField := range mgFields.Array() {
		apiVersion := mgField.Get("apiVersion").String()
		renamedAPIVersion := rules.RenameApiVersion(apiVersion)
		newField, err := sjson.SetBytes([]byte(mgField.Raw), "apiVersion", renamedAPIVersion)
		if err != nil {
			return nil, err
		}
		newFields, err = sjson.SetRawBytes(newFields, "-1", newField)
		if err != nil {
			return nil, err
		}
	}
	return sjson.SetRawBytes(obj, "metadata.managedFields", newFields)
}

func RenameResourcePatch(rules *RewriteRules, patch []byte) ([]byte, error) {
	return RenameMetadataPatch(rules, patch)
}
