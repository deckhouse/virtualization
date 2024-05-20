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
	"encoding/json"
	"strings"

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
	obj, err = RenameManagedFields(rules, obj)
	if err != nil {
		return nil, err
	}

	return RenameOwnerReferences(rules, obj)
}

func RestoreResource(rules *RewriteRules, obj []byte, origGroupName string) ([]byte, error) {
	obj, err := RestoreAPIVersionAndKind(rules, obj, origGroupName)
	if err != nil {
		return nil, err
	}

	// Rewrite apiVersion in each managedFields.
	obj, err = RestoreManagedFields(rules, obj, origGroupName)
	if err != nil {
		return nil, err
	}

	return RestoreOwnerReferences(rules, obj, origGroupName)
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
	ownerRefs := gjson.GetBytes(obj, "metadata.ownerReferences").Array()
	if len(ownerRefs) == 0 {
		return obj, nil
	}

	rwrOwnerRefs := []byte(`[]`)
	rewritten := false
	for _, ownerRef := range ownerRefs {
		kind := ownerRef.Get("kind").String()
		if action == Restore {
			kind = rules.RestoreKind(kind)
		}
		// Find if kind should be rewritten.
		origGroup, origResource, _ := rules.ResourceByKind(kind)
		if origGroup == "" && origResource == "" {
			// There is no rewrite rule for kind, append ownerRef as is.
			var err error
			rwrOwnerRefs, err = sjson.SetRawBytes(rwrOwnerRefs, "-1", []byte(ownerRef.Raw))
			if err != nil {
				return nil, err
			}
			continue
		}
		if action == Rename {
			kind = rules.RenameKind(kind)
		}

		rwrOwnerRef := []byte(ownerRef.Raw)

		rwrOwnerRef, err := sjson.SetBytes(rwrOwnerRef, "kind", kind)
		if err != nil {
			return nil, err
		}

		apiVersion := ownerRef.Get("apiVersion").String()
		if action == Restore {
			apiVersion = rules.RestoreApiVersion(apiVersion, origGroup)
		}
		if action == Rename {
			apiVersion = rules.RenameApiVersion(apiVersion)
		}
		rwrOwnerRef, err = sjson.SetBytes(rwrOwnerRef, "apiVersion", apiVersion)
		if err != nil {
			return nil, err
		}

		rwrOwnerRefs, err = sjson.SetRawBytes(rwrOwnerRefs, "-1", rwrOwnerRef)
		rewritten = true
	}
	if rewritten {
		return sjson.SetRawBytes(obj, "metadata.ownerReferences", rwrOwnerRefs)
	}

	return obj, nil
}

// RenameOwnerReferences renames kind and apiVersion to send request to server.
func RenameOwnerReferences(rules *RewriteRules, obj []byte) ([]byte, error) {
	ownerRefs := gjson.GetBytes(obj, "metadata.ownerReferences").Array()
	if len(ownerRefs) == 0 {
		return obj, nil
	}

	rwrOwnerRefs := []byte(`[]`)
	var err error
	for _, ownerRef := range ownerRefs {
		apiVersion := ownerRef.Get("apiVersion").String()
		kind := ownerRef.Get("kind").String()

		rwrOwnerRef := []byte(ownerRef.Raw)

		_, resRule := rules.KindRules(apiVersion, kind)
		if resRule != nil {
			// Rename apiVersion and kind if resource has renaming rules.
			rwrOwnerRef, err = sjson.SetBytes(rwrOwnerRef, "kind", rules.RenameKind(kind))
			if err != nil {
				return nil, err
			}

			rwrOwnerRef, err = sjson.SetBytes(rwrOwnerRef, "apiVersion", rules.RenameApiVersion(apiVersion))
			if err != nil {
				return nil, err
			}
		}

		rwrOwnerRefs, err = sjson.SetRawBytes(rwrOwnerRefs, "-1", rwrOwnerRef)
		if err != nil {
			return nil, err
		}
	}
	return sjson.SetRawBytes(obj, "metadata.ownerReferences", rwrOwnerRefs)
}

// RestoreOwnerReferences restores kind and apiVersion to consume by the client.
// There are no checks if resource should be restored. This should be determined by the caller.
//
// Example response from the server:
// apiVersion: x.virtualization.deckhouse.io/v1
// kind: VirtualMachineInstance
// metadata:
//
//	name: ...
//	namespace: ..
//	ownerReferences:
//	- apiVersion: x.virtualization.deckhouse.io/v1  <--- restore apiVersion
//	  blockOwnerDeletion: true
//	  controller: true
//	  kind: VirtualMachine   <--- restore kind
//	  name: cloud-alpine
//	  uid: 4c74c3ff-2199-4f20-a71c-3b0e5fb505ca
func RestoreOwnerReferences(rules *RewriteRules, obj []byte, groupName string) ([]byte, error) {
	ownerRefs := gjson.GetBytes(obj, "metadata.ownerReferences").Array()
	if len(ownerRefs) == 0 {
		return obj, nil
	}
	rOwnerRefs := []byte(`[]`)
	restored := false
	for _, ownerRef := range ownerRefs {
		apiVersion := ownerRef.Get("apiVersion").String()
		rOwnerRef := []byte(ownerRef.Raw)
		var err error
		if strings.HasPrefix(apiVersion, rules.RenamedGroup) {
			rOwnerRef, err = sjson.SetBytes([]byte(ownerRef.Raw), "apiVersion", rules.RestoreApiVersion(apiVersion, groupName))
			if err != nil {
				return nil, err
			}
			kind := gjson.GetBytes(rOwnerRef, "kind").String()
			rOwnerRef, err = sjson.SetBytes(rOwnerRef, "kind", rules.RestoreKind(kind))
			if err != nil {
				return nil, err
			}
			restored = true
		}
		rOwnerRefs, err = sjson.SetRawBytes(rOwnerRefs, "-1", rOwnerRef)
	}
	if restored {
		return sjson.SetRawBytes(obj, "metadata.ownerReferences", rOwnerRefs)
	}
	return obj, nil
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

func RewriteMetadata(rules *RewriteRules, obj []byte, action Action) ([]byte, error) {
	items := gjson.GetBytes(obj, "items").Array()
	if len(items) > 0 {
		newObj, err := RewriteArray(obj, "items", func(item []byte) ([]byte, error) {
			return RewriteMetadataLabels(rules, item, action)
		})
		if err != nil {
			return nil, err
		}
		return RewriteArray(newObj, "items", func(item []byte) ([]byte, error) {
			return RewriteMetadataAnnotations(rules, item, action)
		})
	}

	newObj, err := RewriteMetadataLabels(rules, obj, action)
	if err != nil {
		return nil, err
	}
	return RewriteMetadataAnnotations(rules, newObj, action)
}

func RewriteMetadataLabels(rules *RewriteRules, obj []byte, action Action) ([]byte, error) {
	labels := gjson.GetBytes(obj, "metadata.labels").Map()
	if len(labels) == 0 {
		return obj, nil
	}

	newLabels := make(map[string]string, len(labels))
	for k, v := range labels {
		newLabels[k] = v.String()
	}
	switch action {
	case Rename:
		newLabels = rules.RenameLabels(newLabels)
	case Restore:
		newLabels = rules.RestoreLabels(newLabels)
	}
	rwrLabels, err := json.Marshal(newLabels)
	if err != nil {
		return nil, err
	}

	return sjson.SetRawBytes(obj, "metadata.labels", rwrLabels)
}

func RewriteMetadataAnnotations(rules *RewriteRules, obj []byte, action Action) ([]byte, error) {
	annos := gjson.GetBytes(obj, "metadata.annotations").Map()
	if len(annos) == 0 {
		return obj, nil
	}
	newAnnos := make(map[string]string, len(annos))
	for k, v := range annos {
		newAnnos[k] = v.String()
	}
	switch action {
	case Rename:
		newAnnos = rules.RenameAnnotations(newAnnos)
	case Restore:
		newAnnos = rules.RestoreAnnotations(newAnnos)
	}

	rwrAnnons, err := json.Marshal(newAnnos)
	if err != nil {
		return nil, err
	}

	return sjson.SetRawBytes(obj, "metadata.annotations", rwrAnnons)
}
