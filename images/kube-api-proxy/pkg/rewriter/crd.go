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
	"fmt"
	"strings"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"

	"kube-api-proxy/pkg/rewriter/rules"
)

const (
	CRDKind     = "CustomResourceDefinition"
	CRDListKind = "CustomResourceDefinitionList"
)

func RewriteCRD(rwRules *rules.RewriteRules, crdObj []byte, action rules.Action) ([]byte, error) {
	if action == rules.Rename {
		return RenameCRD(rwRules, crdObj)
	}
	return RestoreCRD(rwRules, crdObj)
}

// RestoreCRD restores fields in CRD to original.
//
// Example:
// .metadata.name     prefixedvirtualmachines.x.virtualization.deckhouse.io -> virtualmachines.kubevirt.io
// .spec.group        x.virtualization.deckhouse.io -> kubevirt.io
// .spec.names
//
//	categories      kubevirt -> all
//	kind            PrefixedVirtualMachines -> VirtualMachine
//	listKind        PrefixedVirtualMachineList -> VirtualMachineList
//	plural          prefixedvirtualmachines -> virtualmachines
//	singular        prefixedvirtualmachine -> virtualmachine
//	shortNames      [xvm xvms] -> [vm vms]
func RestoreCRD(rwRules *rules.RewriteRules, obj []byte) ([]byte, error) {
	crdName := gjson.GetBytes(obj, "metadata.name").String()
	resource, group, found := strings.Cut(crdName, ".")
	if !found {
		return nil, fmt.Errorf("malformed CRD name: should be resourcetype.group, got %s", crdName)
	}
	// Do not restore CRDs in unknown groups.
	if group != rwRules.RenamedGroup {
		return nil, nil
	}

	origResource := rwRules.RestoreResource(resource)

	groupRule, resourceRule := rwRules.GroupResourceRules(origResource)
	if resourceRule == nil {
		return nil, nil
	}

	newName := resourceRule.Plural + "." + groupRule.Group
	obj, err := sjson.SetBytes(obj, "metadata.name", newName)
	if err != nil {
		return nil, err
	}

	obj, err = sjson.SetBytes(obj, "spec.group", groupRule.Group)
	if err != nil {
		return nil, err
	}

	names := []byte(gjson.GetBytes(obj, "spec.names").Raw)

	names, err = sjson.SetBytes(names, "categories", rwRules.RestoreCategories(resourceRule))
	if err != nil {
		return nil, err
	}
	names, err = sjson.SetBytes(names, "kind", rwRules.RestoreKind(resourceRule.Kind))
	if err != nil {
		return nil, err
	}
	names, err = sjson.SetBytes(names, "listKind", rwRules.RestoreKind(resourceRule.ListKind))
	if err != nil {
		return nil, err
	}
	names, err = sjson.SetBytes(names, "plural", rwRules.RestoreResource(resourceRule.Plural))
	if err != nil {
		return nil, err
	}
	names, err = sjson.SetBytes(names, "singular", rwRules.RestoreResource(resourceRule.Singular))
	if err != nil {
		return nil, err
	}
	names, err = sjson.SetBytes(names, "shortNames", rwRules.RestoreShortNames(resourceRule.ShortNames))
	if err != nil {
		return nil, err
	}

	obj, err = sjson.SetRawBytes(obj, "spec.names", names)
	if err != nil {
		return nil, err
	}

	return obj, nil
}

// RenameCRD renames fields in CRD.
//
// Example:
// .metadata.name     virtualmachines.kubevirt.io -> prefixedvirtualmachines.x.virtualization.deckhouse.io
// .spec.group        kubevirt.io -> x.virtualization.deckhouse.io
// .spec.names
//
//	categories      all -> kubevirt
//	kind            VirtualMachine -> PrefixedVirtualMachines
//	listKind        VirtualMachineList -> PrefixedVirtualMachineList
//	plural          virtualmachines -> prefixedvirtualmachines
//	singular        virtualmachine -> prefixedvirtualmachine
//	shortNames      [vm vms] -> [xvm xvms]
func RenameCRD(rwRules *rules.RewriteRules, obj []byte) ([]byte, error) {
	crdName := gjson.GetBytes(obj, "metadata.name").String()
	resource, group, found := strings.Cut(crdName, ".")
	if !found {
		return nil, fmt.Errorf("malformed CRD name: should be resourcetype.group, got %s", crdName)
	}

	_, resourceRule := rwRules.ResourceRules(group, resource)
	if resourceRule == nil {
		return nil, nil
	}

	newName := rwRules.RenameResource(resource) + "." + rwRules.RenamedGroup
	obj, err := sjson.SetBytes(obj, "metadata.name", newName)
	if err != nil {
		return nil, err
	}

	spec := gjson.GetBytes(obj, "spec")
	newSpec, err := renameCRDSpec(rwRules, resourceRule, []byte(spec.Raw))
	if err != nil {
		return nil, err
	}
	return sjson.SetRawBytes(obj, "spec", newSpec)
}

func renameCRDSpec(rwRules *rules.RewriteRules, resourceRule *rules.ResourceRule, spec []byte) ([]byte, error) {
	var err error

	spec, err = sjson.SetBytes(spec, "group", rwRules.RenamedGroup)
	if err != nil {
		return nil, err
	}

	// Rename fields in the 'names' object.
	names := []byte(gjson.GetBytes(spec, "names").Raw)

	if gjson.GetBytes(names, "categories").Exists() {
		names, err = sjson.SetBytes(names, "categories", rwRules.RenameCategories(resourceRule.Categories))
		if err != nil {
			return nil, err
		}
	}
	if gjson.GetBytes(names, "kind").Exists() {
		names, err = sjson.SetBytes(names, "kind", rwRules.RenameKind(resourceRule.Kind))
		if err != nil {
			return nil, err
		}
	}
	if gjson.GetBytes(names, "listKind").Exists() {
		names, err = sjson.SetBytes(names, "listKind", rwRules.RenameKind(resourceRule.ListKind))
		if err != nil {
			return nil, err
		}
	}
	if gjson.GetBytes(names, "plural").Exists() {
		names, err = sjson.SetBytes(names, "plural", rwRules.RenameResource(resourceRule.Plural))
		if err != nil {
			return nil, err
		}
	}
	if gjson.GetBytes(names, "singular").Exists() {
		names, err = sjson.SetBytes(names, "singular", rwRules.RenameResource(resourceRule.Singular))
		if err != nil {
			return nil, err
		}
	}
	if gjson.GetBytes(names, "shortNames").Exists() {
		names, err = sjson.SetBytes(names, "shortNames", rwRules.RenameShortNames(resourceRule.ShortNames))
		if err != nil {
			return nil, err
		}
	}

	spec, err = sjson.SetRawBytes(spec, "names", names)
	if err != nil {
		return nil, err
	}

	return spec, nil
}

func RenameCRDPatch(rwRules *rules.RewriteRules, resourceRule *rules.ResourceRule, obj []byte) ([]byte, error) {
	var err error

	patches := gjson.ParseBytes(obj).Array()
	if len(patches) == 0 {
		return obj, nil
	}

	newPatches := []byte(`[]`)
	isRenamed := false
	for _, patch := range patches {
		newPatch := []byte(patch.Raw)

		op := gjson.GetBytes(newPatch, "op").String()
		path := gjson.GetBytes(newPatch, "path").String()

		if (op == "replace" || op == "add") && path == "/spec" {
			isRenamed = true
			value := []byte(gjson.GetBytes(newPatch, "value").Raw)
			newValue, err := renameCRDSpec(rwRules, resourceRule, value)
			if err != nil {
				return nil, err
			}
			newPatch, err = sjson.SetRawBytes(newPatch, "value", newValue)
			if err != nil {
				return nil, err
			}
		}

		newPatches, err = sjson.SetRawBytes(newPatches, "-1", newPatch)
		if err != nil {
			return nil, err
		}
	}

	if !isRenamed {
		return obj, nil
	}

	return newPatches, nil
}
