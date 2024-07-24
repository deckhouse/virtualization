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
	"k8s.io/apimachinery/pkg/runtime"
)

// RewriteAPIGroupList restores groups and kinds in /apis/ response.
//
// Rewrite each APIGroup in "groups".
// Response example:
// {"groups":[
// {"name":"prefixed.resources.group.io",
//
//	 "versions":[
//	   {"groupVersion":"prefixed.resources.group.io/v1","version":"v1"},
//	   {"groupVersion":"prefixed.resources.group.io/v1beta1","version":"v1beta1"},
//	   {"groupVersion":"prefixed.resources.group.io/v1alpha3","version":"v1alpha3"}
//	  ],
//	 "preferredVersion":{"groupVersion":"prefixed.resources.group.io/v1","version":"v1"}
//	}]}
func RewriteAPIGroupList(rules *RewriteRules, objBytes []byte) ([]byte, error) {
	groups := gjson.GetBytes(objBytes, "groups").Array()
	// TODO get rid of RawExtension, use SetRawBytes.
	rwrGroups := make([]interface{}, 0)
	for _, group := range groups {
		groupName := gjson.Get(group.Raw, "name").String()
		// Replace renamed group with groups from rules.
		if rules.IsRenamedGroup(groupName) {
			rwrGroups = append(rwrGroups, rules.GetAPIGroupList()...)
			continue
		}
		// Remove duplicates if cluster have CRDs with original group names.
		if rules.HasGroup(groupName) {
			continue
		}
		rwrGroups = append(rwrGroups, runtime.RawExtension{Raw: []byte(group.Raw)})
	}

	return sjson.SetBytes(objBytes, "groups", rwrGroups)
}

// RewriteAPIGroup restores apiGroup, kinds and versions in responses from renamed APIGroup query:
// /apis/renamed.resource.group.io
//
// This call returns all versions for renamed.resource.group.io.
// Rewriter should reduce versions for only available in original group
// To reduce further requests with specific versions.
//
// Example response with renamed group:
// {  "kind":"APIGroup",
//
//	   "apiVersion":"v1",
//	   "name":"renamed.resource.group.io",
//	   "versions":[
//		  {"groupVersion":"renamed.resource.group.io/v1","version":"v1"},
//		  {"groupVersion":"renamed.resource.group.io/v1alpha1","version":"v1alpha1"}
//	   ],
//	  "preferredVersion": {
//	    "groupVersion":"renamed.resource.group.io/v1",
//		"version":"v1"}
//	  }
//
// Restored response should be:
// {  "kind":"APIGroup",
//
//	   "apiVersion":"v1",
//	   "name":"original.group.io",
//	   "versions":[
//		    {"groupVersion":"original.group.io/v1","version":"v1"},
//	     {"groupVersion":"original.group.io/v1alpha1","version":"v1alpha1"}
//	   ],
//		  "preferredVersion": {
//	     "groupVersion":"original.group.io/v1",
//			"version":"v1"}
//		  }
func RewriteAPIGroup(rules *RewriteRules, obj []byte) ([]byte, error) {
	groupName := gjson.GetBytes(obj, "name").String()
	// Return as-is for group without rules.
	if !rules.IsRenamedGroup(groupName) {
		return obj, nil
	}
	obj, err := sjson.SetBytes(obj, "name", rules.RestoreApiVersion(groupName))
	if err != nil {
		return nil, err
	}

	obj, err = RewriteArray(obj, "versions", func(versionObj []byte) ([]byte, error) {
		return TransformString(versionObj, "groupVersion", func(groupVersion string) string {
			return rules.RestoreApiVersion(groupVersion)
		})
	})
	if err != nil {
		return nil, err
	}

	return TransformString(obj, "preferredVersion.groupVersion", func(preferredGroupVersion string) string {
		return rules.RestoreApiVersion(preferredGroupVersion)
	})
}

// RewriteAPIResourceList rewrites server responses on
// /apis/GROUP/VERSION requests.
// This method excludes resources not belonging to original group from request.
//
// Example:
//
// Path rewrite: https://10.222.0.1:443/apis/kubevirt.io/v1 -> https://10.222.0.1:443/apis/x.virtualization.deckhouse.io/v1
// original Group:  kubevirt.io
// rewrite name,singularName,kind for each resource.
// /status -> name and kind
// /scale -> rewrite resource name in the name field
//
// Response from /apis/x.virtualization.deckhouse.io/v1:
//
//	{
//	    "kind":"APIResourceList",
//	    "apiVersion":"v1",
//
// --> "groupVersion":"x.virtualization.deckhouse.io/v1"  --> rewrite to origGroup+version: kubevirt.io/v1
//
//	    "resources":[
//		   {
//
// -->   "name":"virtualmachineinstancereplicasets",
// -->   "singularName":"virtualmachineinstancereplicaset",
//
//	"namespaced":true,
//
// -->   "kind":"VirtualMachineInstanceReplicaSet",
//
//	  "verbs":["delete","deletecollection","get","list","patch","create","update","watch"],
//	  "shortNames":["xvmirs","xvmirss"],
//	  "categories":["kubevirt"],
//	  "storageVersionHash":"QUMxLW9gfYs="
//	},{
//
// -->   "name":"virtualmachineinstancereplicasets/status",
//
//	"singularName":"",
//	"namespaced":true,
//
// -->   "kind":"VirtualMachineInstanceReplicaSet",
//
//		     "verbs":["get","patch","update"]
//	    },{
//
// -->   "name":"virtualmachineinstancereplicasets/scale",
//
//	      "singularName":"",
//		     "namespaced":true,
//		     "group":"autoscaling",
//		     "version":"v1",
//		     "kind":"Scale",
//		     "verbs":["get","patch","update"]
//		   }]
//	}
func RewriteAPIResourceList(rules *RewriteRules, obj []byte, origGroup string) ([]byte, error) {
	// Ignore apiGroups not in rules.
	apiGroupRule, ok := rules.Rules[origGroup]
	if !ok {
		return obj, nil
	}
	// MVP: rewrite only group for now. (No prefixes in the cluster yet).
	obj, err := sjson.SetBytes(obj, "groupVersion", origGroup+"/"+apiGroupRule.GroupRule.PreferredVersion)
	if err != nil {
		return nil, err
	}

	resources := []byte(`[]`)

	for _, resource := range gjson.GetBytes(obj, "resources").Array() {
		name := resource.Get("name").String()
		nameParts := strings.Split(name, "/")
		resourceName := rules.RestoreResource(nameParts[0])

		_, resourceRule := rules.ResourceRules(origGroup, resourceName)
		if resourceRule == nil {
			continue
		}

		// Rewrite name and kind.
		resBytes, err := sjson.SetBytes([]byte(resource.Raw), "name", rules.RestoreResource(name))
		if err != nil {
			return nil, err
		}

		kind := gjson.GetBytes(resBytes, "kind").String()
		if kind != "" {
			resBytes, err = sjson.SetBytes(resBytes, "kind", rules.RestoreKind(kind))
			if err != nil {
				return nil, err
			}
		}

		singular := gjson.GetBytes(resBytes, "singularName").String()
		if singular != "" {
			resBytes, err = sjson.SetBytes(resBytes, "singularName", rules.RestoreResource(singular))
			if err != nil {
				return nil, err
			}
		}

		shortNames := gjson.GetBytes(resBytes, "shortNames").Array()
		if len(shortNames) > 0 {
			strShortNames := make([]string, 0, len(shortNames))
			for _, shortName := range shortNames {
				strShortNames = append(strShortNames, shortName.String())
			}
			newShortNames := rules.RestoreShortNames(strShortNames)
			resBytes, err = sjson.SetBytes(resBytes, "shortNames", newShortNames)
			if err != nil {
				return nil, err
			}
		}

		categories := gjson.GetBytes(resBytes, "categories")
		if categories.Exists() {
			restoredCategories := rules.RestoreCategories(resourceRule)
			resBytes, err = sjson.SetBytes(resBytes, "categories", restoredCategories)
			if err != nil {
				return nil, err
			}
		}

		resources, err = sjson.SetRawBytes(resources, "-1", resBytes)
		if err != nil {
			return nil, err
		}
	}

	return sjson.SetRawBytes(obj, "resources", resources)
}

// RewriteAPIGroupDiscoveryList restores renamed groups and resources in the aggregated
// discovery response (APIGroupDiscoveryList kind).
//
// Example of APIGroupDiscoveryList structure:
//
//		  {
//		    "kind": "APIGroupDiscoveryList",
//		    "apiVersion": "apidiscovery.k8s.io/v2beta1",
//		    "metadata": {},
//		    "items": [
//		      An array of APIGroupDiscovery objects ...
//		      {
//	         "metadata": {
//				  "name": "internal.virtualization.deckhouse.io", <-- should be renamed group
//				  "creationTimestamp": null
//				},
//				"versions": [
//		          APIVersionDiscovery, .. , APIVersionDiscovery
//		        ]
//		      }, ...
//		    ]
//
// NOTE: Can't use RewriteArray here, because one APIGroupDiscovery with renamed
// resource produces many APIGroupDiscovery objects with restored resource.
func RewriteAPIGroupDiscoveryList(rules *RewriteRules, obj []byte) ([]byte, error) {
	items := gjson.GetBytes(obj, "items").Array()
	if len(items) == 0 {
		return obj, nil
	}

	rwrItems := []byte(`[]`)
	for _, item := range items {
		itemBytes := []byte(item.Raw)
		var err error

		groupName := gjson.GetBytes(itemBytes, "metadata.name").String()

		if !rules.IsRenamedGroup(groupName) {
			// Remove duplicates if cluster have CRDs with original group names.
			if rules.HasGroup(groupName) {
				continue
			}
			// No transform for non-renamed groups.
			rwrItems, err = sjson.SetRawBytes(rwrItems, "-1", itemBytes)
			if err != nil {
				return nil, err
			}
			continue
		}

		newItems, err := RestoreAggregatedGroupDiscovery(rules, itemBytes)
		if err != nil {
			return nil, err
		}
		if newItems == nil {
			// No transform for nil result.
			rwrItems, err = sjson.SetRawBytes(rwrItems, "-1", itemBytes)
		} else {
			// Replace renamed group with restored groups.
			for _, newItem := range newItems {
				rwrItems, err = sjson.SetRawBytes(rwrItems, "-1", newItem)
				if err != nil {
					return nil, err
				}
			}
		}
	}

	return sjson.SetRawBytes(obj, "items", rwrItems)
}

// RestoreAggregatedGroupDiscovery returns an array of APIGroupDiscovery objects with restored resources.
//
// obj is an APIGroupDiscovery object with renamed resources:
//
//	 {
//		"metadata": {
//		  "name": "internal.virtualization.deckhouse.io", <-- renamed group
//		  "creationTimestamp": null
//		},
//	    "versions": [
//	      {  // APIVersionDiscovery
//		    "version": "v1",
//		    "resources": [ APIResourceDiscovery{}, ..., APIResourceDiscovery{}] ,
//		    "freshness": "Current"
//	      }, ... , more APIVersionDiscovery objects.
//	    ]
//	 }
//
// Renamed resources in one version may belong to different original groups,
// so this method indexes and restores all resources in APIResourceDiscovery
// and then produces APIGroupDiscovery for each restored group.
func RestoreAggregatedGroupDiscovery(rules *RewriteRules, obj []byte) ([][]byte, error) {
	// restoredResources holds restored resources indexed by group and version to construct final APIGroupDiscovery items later.
	// A  APIGroupDiscovery "metadata" object field and a version item "version" field are not stored and will be reconstructed.
	restoredResources := make(map[string]map[string][][]byte)

	// versionFreshness stores freshness values for versions
	versionFreshness := make(map[string]string)

	versions := gjson.GetBytes(obj, "versions").Array()
	if len(versions) == 0 {
		return nil, nil
	}

	for _, version := range versions {
		versionBytes := []byte(version.Raw)

		versionName := gjson.GetBytes(versionBytes, "version").String()
		if versionName == "" {
			continue
		}

		// Save freshness.
		freshness := gjson.GetBytes(versionBytes, "freshness").String()
		versionFreshness[versionName] = freshness

		// Loop over resources.
		resources := gjson.GetBytes(versionBytes, "resources").Array()
		if len(resources) == 0 {
			continue
		}

		for _, resource := range resources {
			restoredGroup, restoredResource, err := RestoreAggregatedDiscoveryResource(rules, []byte(resource.Raw))
			if err != nil {
				return nil, nil
			}

			if _, ok := restoredResources[restoredGroup]; !ok {
				restoredResources[restoredGroup] = make(map[string][][]byte)
			}
			if _, ok := restoredResources[restoredGroup][versionName]; !ok {
				restoredResources[restoredGroup][versionName] = make([][]byte, 0)
			}
			restoredResources[restoredGroup][versionName] = append(restoredResources[restoredGroup][versionName], restoredResource)
		}
	}

	// Produce restored APIGroupDiscovery items from indexed APIResourceDiscovery.
	restoredGroupList := make([][]byte, 0, len(restoredResources))
	var err error
	for groupName, groupVersions := range restoredResources {
		// Restore metadata for APIGroupDiscovery.
		restoredGroupObj := []byte(fmt.Sprintf(`{"metadata":{"name":"%s", "creationTimestamp":null}}`, groupName))

		// Construct an array of APIVersionDiscovery objects.
		restoredVersions := []byte(`[]`)
		for versionName, versionResources := range groupVersions {
			// Init restored APIVersionDiscovery object.
			restoredVersionObj := []byte(fmt.Sprintf(`{"version":"%s"}`, versionName))

			// Construct an array of APIResourceDiscovery objects.
			{
				restoredVersionResources := []byte(`[]`)
				for _, resource := range versionResources {
					restoredVersionResources, err = sjson.SetRawBytes(restoredVersionResources, "-1", resource)
					if err != nil {
						return nil, err
					}
				}
				// Set resources field.
				restoredVersionObj, err = sjson.SetRawBytes(restoredVersionObj, "resources", restoredVersionResources)
				if err != nil {
					return nil, err
				}
			}

			// Append restored APIVersionDiscovery object.
			restoredVersions, err = sjson.SetRawBytes(restoredVersions, "-1", restoredVersionObj)
			if err != nil {
				return nil, err
			}
		}

		restoredGroupObj, err := sjson.SetRawBytes(restoredGroupObj, "versions", restoredVersions)
		if err != nil {
			return nil, err
		}

		restoredGroupList = append(restoredGroupList, restoredGroupObj)
	}

	return restoredGroupList, nil
}

// RestoreAggregatedDiscoveryResource restores fields in a renamed APIResourceDiscovery object.
//
// Example of the APIResourceDiscovery object:
//
//	{
//	  "resource": "internalvirtualizationkubevirts",
//	  "responseKind": {
//	    "group": "internal.virtualization.deckhouse.io",
//	    "version": "v1",
//	    "kind": "InternalVirtualizationKubeVirt"
//	  },
//	  "scope": "Namespaced",
//	  "singularResource": "internalvirtualizationkubevirt",
//	  "verbs": [ "delete", "deletecollection", "get", ... ], // Optional
//	  "categories": [ "intvirt" ], // Optional
//	  "subresources": [ // Optional
//	    {
//	      "subresource": "status",
//	      "responseKind": {
//	        "group": "internal.virtualization.deckhouse.io",
//	        "version": "v1",
//	        "kind": "InternalVirtualizationKubeVirt"
//	      },
//	      "verbs": [ "get", "patch", "update" ]
//	    }
//	  ]
//	}
func RestoreAggregatedDiscoveryResource(rules *RewriteRules, obj []byte) (string, []byte, error) {
	var err error

	// Get resource plural.
	resource := gjson.GetBytes(obj, "resource").String()
	origResource := rules.RestoreResource(resource)

	groupRule, resRule := rules.GroupResourceRules(origResource)

	// Ignore resource without rules.
	if resRule == nil {
		return "", nil, err
	}

	origGroup := groupRule.Group

	obj, err = sjson.SetBytes(obj, "resource", origResource)
	if err != nil {
		return "", nil, err
	}

	// Reconstruct group and kind in responseKind field.
	responseKind := gjson.GetBytes(obj, "responseKind")
	if responseKind.IsObject() {
		obj, err = sjson.SetBytes(obj, "responseKind.group", origGroup)
		if err != nil {
			return "", nil, err
		}
		obj, err = sjson.SetBytes(obj, "responseKind.kind", resRule.Kind)
		if err != nil {
			return "", nil, err
		}
	}

	singular := gjson.GetBytes(obj, "singularResource").String()
	if singular != "" {
		obj, err = sjson.SetBytes(obj, "singularResource", rules.RestoreResource(singular))
		if err != nil {
			return "", nil, err
		}
	}

	shortNames := gjson.GetBytes(obj, "shortNames").Array()
	if len(shortNames) > 0 {
		strShortNames := make([]string, 0, len(shortNames))
		for _, shortName := range shortNames {
			strShortNames = append(strShortNames, shortName.String())
		}
		newShortNames := rules.RestoreShortNames(strShortNames)
		obj, err = sjson.SetBytes(obj, "shortNames", newShortNames)
		if err != nil {
			return "", nil, err
		}
	}

	categories := gjson.GetBytes(obj, "categories")
	if categories.Exists() {
		restoredCategories := rules.RestoreCategories(resRule)
		obj, err = sjson.SetBytes(obj, "categories", restoredCategories)
		if err != nil {
			return "", nil, err
		}
	}

	obj, err = RewriteArray(obj, "subresources", func(item []byte) ([]byte, error) {
		// Reconstruct group and kind in responseKind field.
		responseKind := gjson.GetBytes(item, "responseKind")
		if responseKind.IsObject() {
			item, err = sjson.SetBytes(item, "responseKind.group", origGroup)
			if err != nil {
				return nil, err
			}
			item, err = sjson.SetBytes(item, "responseKind.kind", resRule.Kind)
			if err != nil {
				return nil, err
			}
		}
		return item, nil
	})

	return origGroup, obj, nil
}
