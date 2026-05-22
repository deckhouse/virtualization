/*
Copyright 2026 Flant JSC

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

package kubevirtrules

import (
	"github.com/deckhouse/kube-api-rewriter/pkg/rewriter"
)

const (
	internalPrefix = "internal.virtualization.deckhouse.io"
	nodePrefix     = "node.virtualization.deckhouse.io"
	rootPrefix     = "virtualization.deckhouse.io"
)

var KubevirtRewriteRules = &rewriter.RewriteRules{
	KindPrefix:         "InternalVirtualization", // VirtualMachine -> InternalVirtualizationVirtualMachine
	ResourceTypePrefix: "internalvirtualization", // virtualmachines -> internalvirtualizationvirtualmachines
	ShortNamePrefix:    "intvirt",                // kubectl get intvirtvm
	Categories:         []string{"intvirt"},      // kubectl get intvirt to see all internal virtualization resources.
	Rules:              KubevirtAPIGroupsRules,
	Webhooks:           KubevirtWebhooks,
	Labels: rewriter.MetadataReplace{
		Names: []rewriter.MetadataReplaceRule{
			{Original: "kubevirt.io", Renamed: "kubevirt." + internalPrefix},
			{Original: "operator.kubevirt.io", Renamed: "operator.kubevirt." + internalPrefix},
			{Original: "prometheus.kubevirt.io", Renamed: "prometheus.kubevirt." + internalPrefix},
			// Special cases.
			{Original: "node-labeller.kubevirt.io/skip-node", Renamed: "node-labeller." + rootPrefix + "/skip-node"},
			{Original: "node-labeller.kubevirt.io/obsolete-host-model", Renamed: "node-labeller." + internalPrefix + "/obsolete-host-model"},
			{
				Original: "app.kubernetes.io/managed-by", OriginalValue: "virt-operator",
				Renamed: "app.kubernetes.io/managed-by", RenamedValue: "virt-operator-internal-virtualization",
			},
			{
				Original: "app.kubernetes.io/managed-by", OriginalValue: "kubevirt-operator",
				Renamed: "app.kubernetes.io/managed-by", RenamedValue: "kubevirt-operator-internal-virtualization",
			},
		},
		Prefixes: []rewriter.MetadataReplaceRule{
			// KubeVirt related labels.
			{Original: "kubevirt.io", Renamed: "kubevirt." + internalPrefix},
			{Original: "prometheus.kubevirt.io", Renamed: "prometheus.kubevirt." + internalPrefix},
			{Original: "operator.kubevirt.io", Renamed: "operator.kubevirt." + internalPrefix},
			{Original: "vm.kubevirt.io", Renamed: "vm.kubevirt." + internalPrefix},
			// Node features related labels.
			// Note: these labels are not "internal".
			{Original: "cpu-feature.node.kubevirt.io", Renamed: "cpu-feature." + nodePrefix},
			{Original: "cpu-model-migration.node.kubevirt.io", Renamed: "cpu-model-migration." + nodePrefix},
			{Original: "cpu-model.node.kubevirt.io", Renamed: "cpu-model." + nodePrefix},
			{Original: "cpu-timer.node.kubevirt.io", Renamed: "cpu-timer." + nodePrefix},
			{Original: "cpu-vendor.node.kubevirt.io", Renamed: "cpu-vendor." + nodePrefix},
			{Original: "scheduling.node.kubevirt.io", Renamed: "scheduling." + nodePrefix},
			{Original: "host-model-cpu.node.kubevirt.io", Renamed: "host-model-cpu." + nodePrefix},
			{Original: "host-model-required-features.node.kubevirt.io", Renamed: "host-model-required-features." + nodePrefix},
			{Original: "hyperv.node.kubevirt.io", Renamed: "hyperv." + nodePrefix},
			{Original: "machine-type.node.kubevirt.io", Renamed: "machine-type." + nodePrefix},
		},
	},
	Annotations: rewriter.MetadataReplace{
		Prefixes: []rewriter.MetadataReplaceRule{
			// KubeVirt related annotations.
			{Original: "kubevirt.io", Renamed: "kubevirt." + internalPrefix},
			{Original: "certificates.kubevirt.io", Renamed: "certificates.kubevirt." + internalPrefix},
		},
	},
	Finalizers: rewriter.MetadataReplace{
		Prefixes: []rewriter.MetadataReplaceRule{
			{Original: "kubevirt.io", Renamed: "kubevirt." + internalPrefix},
		},
	},
}

// TODO create generator in golang to produce below rules from KubeVirt sources so proxy can work with future versions.

var KubevirtAPIGroupsRules = map[string]rewriter.APIGroupRule{
	"cdi.kubevirt.io": {
		GroupRule: rewriter.GroupRule{
			Group:            "cdi.kubevirt.io",
			Versions:         []string{"v1beta1"},
			PreferredVersion: "v1beta1",
			Renamed:          "cdi." + internalPrefix,
		},
		ResourceRules: map[string]rewriter.ResourceRule{
			// storageprofiles.cdi.kubevirt.io
			"storageprofiles": {
				Kind:             "StorageProfile",
				ListKind:         "StorageProfileList",
				Plural:           "storageprofiles",
				Singular:         "storageprofile",
				Versions:         []string{"v1beta1"},
				PreferredVersion: "v1beta1",
				Categories:       []string{},
				ShortNames:       []string{},
			},
		},
	},
	"kubevirt.io": {
		GroupRule: rewriter.GroupRule{
			Group:            "kubevirt.io",
			Versions:         []string{"v1", "v1alpha3"},
			PreferredVersion: "v1",
			Renamed:          internalPrefix,
		},
		ResourceRules: map[string]rewriter.ResourceRule{
			// kubevirts.kubevirt.io
			"kubevirts": {
				Kind:             "KubeVirt",
				ListKind:         "KubeVirtList",
				Plural:           "kubevirts",
				Singular:         "kubevirt",
				Versions:         []string{"v1", "v1alpha3"},
				PreferredVersion: "v1",
				Categories:       []string{"all"},
				ShortNames:       []string{"kv", "kvs"},
			},
			// virtualmachines.kubevirt.io
			"virtualmachines": {
				Kind:             "VirtualMachine",
				ListKind:         "VirtualMachineList",
				Plural:           "virtualmachines",
				Singular:         "virtualmachine",
				Versions:         []string{"v1", "v1alpha3"},
				PreferredVersion: "v1",
				Categories:       []string{"all"},
				ShortNames:       []string{"vm", "vms"},
			},
			// virtualmachineinstances.kubevirt.io
			"virtualmachineinstances": {
				Kind:             "VirtualMachineInstance",
				ListKind:         "VirtualMachineInstanceList",
				Plural:           "virtualmachineinstances",
				Singular:         "virtualmachineinstance",
				Versions:         []string{"v1", "v1alpha3"},
				PreferredVersion: "v1",
				Categories:       []string{"all"},
				ShortNames:       []string{"vmi", "vmsi"},
			},
			// virtualmachineinstancemigrations.kubevirt.io
			"virtualmachineinstancemigrations": {
				Kind:             "VirtualMachineInstanceMigration",
				ListKind:         "VirtualMachineInstanceMigrationList",
				Plural:           "virtualmachineinstancemigrations",
				Singular:         "virtualmachineinstancemigration",
				Versions:         []string{"v1", "v1alpha3"},
				PreferredVersion: "v1",
				Categories:       []string{"all"},
				ShortNames:       []string{"vmim", "vmims"},
			},
			// virtualmachineinstancepresets.kubevirt.io
			"virtualmachineinstancepresets": {
				Kind:             "VirtualMachineInstancePreset",
				ListKind:         "VirtualMachineInstancePresetList",
				Plural:           "virtualmachineinstancepresets",
				Singular:         "virtualmachineinstancepreset",
				Versions:         []string{"v1", "v1alpha3"},
				PreferredVersion: "v1",
				Categories:       []string{"all"},
				ShortNames:       []string{"vmipreset", "vmipresets"},
			},
			// virtualmachineinstancereplicasets.kubevirt.io
			"virtualmachineinstancereplicasets": {
				Kind:             "VirtualMachineInstanceReplicaSet",
				ListKind:         "VirtualMachineInstanceReplicaSetList",
				Plural:           "virtualmachineinstancereplicasets",
				Singular:         "virtualmachineinstancereplicaset",
				Versions:         []string{"v1", "v1alpha3"},
				PreferredVersion: "v1",
				Categories:       []string{"all"},
				ShortNames:       []string{"vmirs", "vmirss"},
			},
		},
	},
	"clone.kubevirt.io": {
		GroupRule: rewriter.GroupRule{
			Group:            "clone.kubevirt.io",
			Versions:         []string{"v1alpha1"},
			PreferredVersion: "v1alpha1",
			Renamed:          "clone.internal.virtualization.deckhouse.io",
		},
		ResourceRules: map[string]rewriter.ResourceRule{
			// virtualmachineclones.clone.kubevirt.io
			"virtualmachineclones": {
				Kind:             "VirtualMachineClone",
				ListKind:         "VirtualMachineCloneList",
				Plural:           "virtualmachineclones",
				Singular:         "virtualmachineclone",
				Versions:         []string{"v1alpha1"},
				PreferredVersion: "v1alpha1",
				Categories:       []string{"all"},
				ShortNames:       []string{"vmclone", "vmclones"},
			},
		},
	},
	"export.kubevirt.io": {
		GroupRule: rewriter.GroupRule{
			Group:            "export.kubevirt.io",
			Versions:         []string{"v1alpha1"},
			PreferredVersion: "v1alpha1",
			Renamed:          "export.internal.virtualization.deckhouse.io",
		},
		ResourceRules: map[string]rewriter.ResourceRule{
			// virtualmachineexports.export.kubevirt.io
			"virtualmachineexports": {
				Kind:             "VirtualMachineExport",
				ListKind:         "VirtualMachineExportList",
				Plural:           "virtualmachineexports",
				Singular:         "virtualmachineexport",
				Versions:         []string{"v1alpha1"},
				PreferredVersion: "v1alpha1",
				Categories:       []string{"all"},
				ShortNames:       []string{"vmexport", "vmexports"},
			},
		},
	},
	"instancetype.kubevirt.io": {
		GroupRule: rewriter.GroupRule{
			Group:            "instancetype.kubevirt.io",
			Versions:         []string{"v1alpha1", "v1alpha2"},
			PreferredVersion: "v1alpha2",
			Renamed:          "instancetype.internal.virtualization.deckhouse.io",
		},
		ResourceRules: map[string]rewriter.ResourceRule{
			// virtualmachineinstancetypes.instancetype.kubevirt.io
			"virtualmachineinstancetypes": {
				Kind:             "VirtualMachineInstancetype",
				ListKind:         "VirtualMachineInstancetypeList",
				Plural:           "virtualmachineinstancetypes",
				Singular:         "virtualmachineinstancetype",
				Versions:         []string{"v1alpha1", "v1alpha2"},
				PreferredVersion: "v1alpha2",
				Categories:       []string{"all"},
				ShortNames:       []string{"vminstancetype", "vminstancetypes", "vmf", "vmfs"},
			},
			// virtualmachinepreferences.instancetype.kubevirt.io
			"virtualmachinepreferences": {
				Kind:             "VirtualMachinePreference",
				ListKind:         "VirtualMachinePreferenceList",
				Plural:           "virtualmachinepreferences",
				Singular:         "virtualmachinepreference",
				Versions:         []string{"v1alpha1", "v1alpha2"},
				PreferredVersion: "v1alpha2",
				Categories:       []string{"all"},
				ShortNames:       []string{"vmpref", "vmprefs", "vmp", "vmps"},
			},
			// virtualmachineclusterinstancetypes.instancetype.kubevirt.io
			"virtualmachineclusterinstancetypes": {
				Kind:             "VirtualMachineClusterInstancetype",
				ListKind:         "VirtualMachineClusterInstancetypeList",
				Plural:           "virtualmachineclusterinstancetypes",
				Singular:         "virtualmachineclusterinstancetype",
				Versions:         []string{"v1alpha1", "v1alpha2"},
				PreferredVersion: "v1alpha2",
				Categories:       []string{},
				ShortNames:       []string{"vmclusterinstancetype", "vmclusterinstancetypes", "vmcf", "vmcfs"},
			},
			// virtualmachineclusterpreferences.instancetype.kubevirt.io
			"virtualmachineclusterpreferences": {
				Kind:             "VirtualMachineClusterPreference",
				ListKind:         "VirtualMachineClusterPreferenceList",
				Plural:           "virtualmachineclusterpreferences",
				Singular:         "virtualmachineclusterpreference",
				Versions:         []string{"v1alpha1", "v1alpha2"},
				PreferredVersion: "v1alpha2",
				Categories:       []string{},
				ShortNames:       []string{"vmcp", "vmcps"},
			},
		},
	},
	"migrations.kubevirt.io": {
		GroupRule: rewriter.GroupRule{
			Group:            "migrations.kubevirt.io",
			Versions:         []string{"v1alpha1"},
			PreferredVersion: "v1alpha1",
			Renamed:          "migrations.internal.virtualization.deckhouse.io",
		},
		ResourceRules: map[string]rewriter.ResourceRule{
			// migrationpolicies.migrations.kubevirt.io
			"migrationpolicies": {
				Kind:             "MigrationPolicy",
				ListKind:         "MigrationPolicyList",
				Plural:           "migrationpolicies",
				Singular:         "migrationpolicy",
				Versions:         []string{"v1alpha1"},
				PreferredVersion: "v1alpha1",
				Categories:       []string{"all"},
				ShortNames:       []string{},
			},
		},
	},
	"pool.kubevirt.io": {
		GroupRule: rewriter.GroupRule{
			Group:            "pool.kubevirt.io",
			Versions:         []string{"v1alpha1"},
			PreferredVersion: "v1alpha1",
			Renamed:          "pool.internal.virtualization.deckhouse.io",
		},
		ResourceRules: map[string]rewriter.ResourceRule{
			// virtualmachinepools.pool.kubevirt.io
			"virtualmachinepools": {
				Kind:             "VirtualMachinePool",
				ListKind:         "VirtualMachinePoolList",
				Plural:           "virtualmachinepools",
				Singular:         "virtualmachinepool",
				Versions:         []string{"v1alpha1"},
				PreferredVersion: "v1alpha1",
				Categories:       []string{"all"},
				ShortNames:       []string{"vmpool", "vmpools"},
			},
		},
	},
	"snapshot.kubevirt.io": {
		GroupRule: rewriter.GroupRule{
			Group:            "snapshot.kubevirt.io",
			Versions:         []string{"v1alpha1"},
			PreferredVersion: "v1alpha1",
			Renamed:          "snapshot.internal.virtualization.deckhouse.io",
		},
		ResourceRules: map[string]rewriter.ResourceRule{
			// virtualmachinerestores.snapshot.kubevirt.io
			"virtualmachinerestores": {
				Kind:             "VirtualMachineRestore",
				ListKind:         "VirtualMachineRestoreList",
				Plural:           "virtualmachinerestores",
				Singular:         "virtualmachinerestore",
				Versions:         []string{"v1alpha1"},
				PreferredVersion: "v1alpha1",
				Categories:       []string{"all"},
				ShortNames:       []string{"vmrestore", "vmrestores"},
			},
			// virtualmachinesnapshotcontents.snapshot.kubevirt.io
			"virtualmachinesnapshotcontents": {
				Kind:             "VirtualMachineSnapshotContent",
				ListKind:         "VirtualMachineSnapshotContentList",
				Plural:           "virtualmachinesnapshotcontents",
				Singular:         "virtualmachinesnapshotcontent",
				Versions:         []string{"v1alpha1"},
				PreferredVersion: "v1alpha1",
				Categories:       []string{"all"},
				ShortNames:       []string{"vmsnapshotcontent", "vmsnapshotcontents"},
			},
			// virtualmachinesnapshots.snapshot.kubevirt.io
			"virtualmachinesnapshots": {
				Kind:             "VirtualMachineSnapshot",
				ListKind:         "VirtualMachineSnapshotList",
				Plural:           "virtualmachinesnapshots",
				Singular:         "virtualmachinesnapshot",
				Versions:         []string{"v1alpha1"},
				PreferredVersion: "v1alpha1",
				Categories:       []string{"all"},
				ShortNames:       []string{"vmsnapshot", "vmsnapshots"},
			},
		},
	},
}

var KubevirtWebhooks = map[string]rewriter.WebhookRule{
	// Kubevirt webhooks.
	// Run this in original Kubevirt installation:
	// kubectl get validatingwebhookconfiguration,mutatingwebhookconfiguration -l  kubevirt.io -o json | jq '.items[] | .webhooks[] | {"path": .clientConfig.service.path, "group": (.rules[]|.apiGroups|join(",")), "resource": (.rules[]|.resources|join(",")) } | "\""+.path +"\": {\nPath: \"" + .path + "\",\nGroup: \"" + .group + "\",\nResource: \"" + .resource + "\",\n}," '
	// TODO create generator in golang to extract these rules from resource definitions in the virt-operator package.
	"/virtualmachineinstances-validate-create": {
		Path:     "/virtualmachineinstances-validate-create",
		Group:    "kubevirt.io",
		Resource: "virtualmachineinstances",
	},
	"/virtualmachineinstances-validate-update": {
		Path:     "/virtualmachineinstances-validate-update",
		Group:    "kubevirt.io",
		Resource: "virtualmachineinstances",
	},
	"/virtualmachines-validate": {
		Path:     "/virtualmachines-validate",
		Group:    "kubevirt.io",
		Resource: "virtualmachines",
	},
	"/virtualmachinereplicaset-validate": {
		Path:     "/virtualmachinereplicaset-validate",
		Group:    "kubevirt.io",
		Resource: "virtualmachineinstancereplicasets",
	},
	"/virtualmachinepool-validate": {
		Path:     "/virtualmachinepool-validate",
		Group:    "pool.kubevirt.io",
		Resource: "virtualmachinepools",
	},
	"/vmipreset-validate": {
		Path:     "/vmipreset-validate",
		Group:    "kubevirt.io",
		Resource: "virtualmachineinstancepresets",
	},
	"/migration-validate-create": {
		Path:     "/migration-validate-create",
		Group:    "kubevirt.io",
		Resource: "virtualmachineinstancemigrations",
	},
	"/migration-validate-update": {
		Path:     "/migration-validate-update",
		Group:    "kubevirt.io",
		Resource: "virtualmachineinstancemigrations",
	},
	"/virtualmachinesnapshots-validate": {
		Path:     "/virtualmachinesnapshots-validate",
		Group:    "snapshot.kubevirt.io",
		Resource: "virtualmachinesnapshots",
	},
	"/virtualmachinerestores-validate": {
		Path:     "/virtualmachinerestores-validate",
		Group:    "snapshot.kubevirt.io",
		Resource: "virtualmachinerestores",
	},
	"/virtualmachineexports-validate": {
		Path:     "/virtualmachineexports-validate",
		Group:    "export.kubevirt.io",
		Resource: "virtualmachineexports",
	},
	"/virtualmachineinstancetypes-validate": {
		Path:     "/virtualmachineinstancetypes-validate",
		Group:    "instancetype.kubevirt.io",
		Resource: "virtualmachineinstancetypes",
	},
	"/virtualmachineclusterinstancetypes-validate": {
		Path:     "/virtualmachineclusterinstancetypes-validate",
		Group:    "instancetype.kubevirt.io",
		Resource: "virtualmachineclusterinstancetypes",
	},
	"/virtualmachinepreferences-validate": {
		Path:     "/virtualmachinepreferences-validate",
		Group:    "instancetype.kubevirt.io",
		Resource: "virtualmachinepreferences",
	},
	"/virtualmachineclusterpreferences-validate": {
		Path:     "/virtualmachineclusterpreferences-validate",
		Group:    "instancetype.kubevirt.io",
		Resource: "virtualmachineclusterpreferences",
	},
	"/status-validate": {
		Path:     "/status-validate",
		Group:    "kubevirt.io",
		Resource: "virtualmachines/status,virtualmachineinstancereplicasets/status,virtualmachineinstancemigrations/status",
	},
	"/migration-policy-validate-create": {
		Path:     "/migration-policy-validate-create",
		Group:    "migrations.kubevirt.io",
		Resource: "migrationpolicies",
	},
	"/vm-clone-validate-create": {
		Path:     "/vm-clone-validate-create",
		Group:    "clone.kubevirt.io",
		Resource: "virtualmachineclones",
	},
	"/kubevirt-validate-delete": {
		Path:     "/kubevirt-validate-delete",
		Group:    "kubevirt.io",
		Resource: "kubevirts",
	},
	"/kubevirt-validate-update": {
		Path:     "/kubevirt-validate-update",
		Group:    "kubevirt.io",
		Resource: "kubevirts",
	},
	"/virtualmachines-mutate": {
		Path:     "/virtualmachines-mutate",
		Group:    "kubevirt.io",
		Resource: "virtualmachines",
	},
	"/virtualmachineinstances-mutate": {
		Path:     "/virtualmachineinstances-mutate",
		Group:    "kubevirt.io",
		Resource: "virtualmachineinstances",
	},
	"/migration-mutate-create": {
		Path:     "/migration-mutate-create",
		Group:    "kubevirt.io",
		Resource: "virtualmachineinstancemigrations",
	},
	"/vm-clone-mutate-create": {
		Path:     "/vm-clone-mutate-create",
		Group:    "clone.kubevirt.io",
		Resource: "virtualmachineclones",
	},
}
