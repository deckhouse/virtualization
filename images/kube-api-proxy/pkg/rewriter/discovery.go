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
// {"name":"x.virtualization.deckhouse.io",
//
//	 "versions":[
//	   {"groupVersion":"x.virtualization.deckhouse.io/v1","version":"v1"},
//	   {"groupVersion":"x.virtualization.deckhouse.io/v1beta1","version":"v1beta1"},
//	   {"groupVersion":"x.virtualization.deckhouse.io/v1alpha3","version":"v1alpha3"},
//	   {"groupVersion":"x.virtualization.deckhouse.io/v1alpha2","version":"v1alpha2"},
//	   {"groupVersion":"x.virtualization.deckhouse.io/v1alpha1","version":"v1alpha1"}
//	  ],
//	 "preferredVersion":{"groupVersion":"x.virtualization.deckhouse.io/v1","version":"v1"}
//	}
func RewriteAPIGroupList(rules *RewriteRules, objBytes []byte) ([]byte, error) {
	groups := gjson.GetBytes(objBytes, "groups").Array()
	// TODO get rid of RawExtension, use SetRawBytes.
	rwrGroups := make([]interface{}, 0)
	for _, group := range groups {
		groupName := gjson.Get(group.Raw, "name").String()
		// Replace renamed group with groups from rules.
		if groupName == rules.RenamedGroup {
			rwrGroups = append(rwrGroups, rules.GetAPIGroupList()...)
			continue
		}
		rwrGroups = append(rwrGroups, runtime.RawExtension{Raw: []byte(group.Raw)})
	}

	return sjson.SetBytes(objBytes, "groups", rwrGroups)
}

// RewriteAPIGroup rewrites responses for
// /apis/x.virtualization.deckhouse.io
//
// This call returns all versions for x.virtualization.deckhouse.io.
// Rewriter should reduce versions for only available in original group
// To reduce further requests with specific versions.
//
// Example response:
// {  "kind":"APIGroup",
//
//	   "apiVersion":"v1",
//	   "name":"x.virtualization.deckhouse.io",
//	   "versions":[
//		  {"groupVersion":"x.virtualization.deckhouse.io/v1","version":"v1"},
//	      {"groupVersion":"x.virtualization.deckhouse.io/v1beta1","version":"v1beta1"},
//		  {"groupVersion":"x.virtualization.deckhouse.io/v1alpha3","version":"v1alpha3"},
//		  {"groupVersion":"x.virtualization.deckhouse.io/v1alpha2","version":"v1alpha2"},
//		  {"groupVersion":"x.virtualization.deckhouse.io/v1alpha1","version":"v1alpha1"}
//	   ],
//	  "preferredVersion": {
//	    "groupVersion":"x.virtualization.deckhouse.io/v1",
//		"version":"v1"}
//	  }
//
// Rewrite for kubevirt.io group should be:
// {  "kind":"APIGroup",
//
//	   "apiVersion":"v1",
//	   "name":"kubevirt.io",
//	   "versions":[
//		    {"groupVersion":"kubevirt.io/v1","version":"v1"},
//	     {"groupVersion":"kubevirt.io/v1alpha3","version":"v1alpha3"}
//	   ],
//		  "preferredVersion": {
//	     "groupVersion":"kubevirt.io/v1",
//			"version":"v1"}
//		  }
//
// And rewrite for clone.kubevirt.io group should be:
// {  "kind":"APIGroup",
//
//	   "apiVersion":"v1",
//	   "name":"clone.kubevirt.io",
//	   "versions":[
//	     {"groupVersion":"clone.kubevirt.io/v1alpha1","version":"v1alpha1"}
//	   ],
//		  "preferredVersion": {
//	     "groupVersion":"clone.kubevirt.io/v1alpha1",
//			"version":"v1alpha1"}
//		  }
func RewriteAPIGroup(rules *RewriteRules, obj []byte, origGroup string) ([]byte, error) {
	apiGroupRule, ok := rules.Rules[origGroup]
	if !ok {
		return nil, fmt.Errorf("no APIGroup rewrites for group '%s'", origGroup)
	}

	// Grab all versions from rules.
	versions := make([]interface{}, 0)
	for _, ver := range apiGroupRule.GroupRule.Versions {
		versions = append(versions, map[string]interface{}{
			"groupVersion": origGroup + "/" + ver,
			"version":      ver,
		})
	}
	preferredVersion := map[string]interface{}{
		"groupVersion": origGroup + "/" + apiGroupRule.GroupRule.PreferredVersion,
		"version":      apiGroupRule.GroupRule.PreferredVersion,
	}

	obj, err := sjson.SetBytes(obj, "name", origGroup)
	if err != nil {
		return nil, err
	}
	obj, err = sjson.SetBytes(obj, "versions", versions)
	if err != nil {
		return nil, err
	}
	obj, err = sjson.SetBytes(obj, "preferredVersion", preferredVersion)
	if err != nil {
		return nil, err
	}
	return obj, nil
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
