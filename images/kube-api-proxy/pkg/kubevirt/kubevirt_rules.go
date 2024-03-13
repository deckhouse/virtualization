package kubevirt

import (
	. "kube-api-proxy/pkg/rewriter"
)

var KubevirtRewriteRules = &RewriteRules{
	KindPrefix:         "", // KV
	ResourceTypePrefix: "", // kv
	ShortNamePrefix:    "x",
	Categories:         []string{"kubevirt"},
	RenamedGroup:       "x.virtualization.deckhouse.io",
	Rules:              KubevirtAPIGroupsRules,
	Webhooks:           KubevirtWebhooks,
}

var KubevirtAPIGroupsRules = map[string]APIGroupRule{
	"cdi.kubevirt.io": {
		GroupRule: GroupRule{
			Group:            "cdi.kubevirt.io",
			Versions:         []string{"v1beta1"},
			PreferredVersion: "v1beta1",
		},
		ResourceRules: map[string]ResourceRule{
			// cdiconfigs.cdi.kubevirt.io
			"cdiconfigs": {
				Kind:             "CDIConfig",
				ListKind:         "CDIConfigList",
				Plural:           "cdiconfigs",
				Singular:         "cdiconfig",
				Versions:         []string{"v1beta1"},
				PreferredVersion: "v1beta1",
				Categories:       []string{},
				ShortNames:       []string{},
			},
			// cdis.cdi.kubevirt.io
			"cdis": {
				Kind:             "CDI",
				ListKind:         "CDIList",
				Plural:           "cdis",
				Singular:         "cdi",
				Versions:         []string{"v1beta1"},
				PreferredVersion: "v1beta1",
				Categories:       []string{},
				ShortNames:       []string{"cdi", "cdis"},
			},
			// dataimportcrons.cdi.kubevirt.io
			"dataimportcrons": {
				Kind:             "DataImportCron",
				ListKind:         "DataImportCronList",
				Plural:           "dataimportcrons",
				Singular:         "dataimportcron",
				Versions:         []string{"v1beta1"},
				PreferredVersion: "v1beta1",
				Categories:       []string{"all"},
				ShortNames:       []string{"dic", "dics"},
			},
			// datasources.cdi.kubevirt.io
			"datasources": {
				Kind:             "DataSource",
				ListKind:         "DataSourceList",
				Plural:           "datasources",
				Singular:         "datasource",
				Versions:         []string{"v1beta1"},
				PreferredVersion: "v1beta1",
				Categories:       []string{"all"},
				ShortNames:       []string{"das"},
			},
			// datavolumes.cdi.kubevirt.io
			"datavolumes": {
				Kind:             "DataVolume",
				ListKind:         "DataVolumeList",
				Plural:           "datavolumes",
				Singular:         "datavolume",
				Versions:         []string{"v1beta1"},
				PreferredVersion: "v1beta1",
				Categories:       []string{"all"},
				ShortNames:       []string{"dv", "dvs"},
			},
			// objecttransfers.cdi.kubevirt.io
			"objecttransfers": {
				Kind:             "ObjectTransfer",
				ListKind:         "ObjectTransferList",
				Plural:           "objecttransfers",
				Singular:         "objecttransfer",
				Versions:         []string{"v1beta1"},
				PreferredVersion: "v1beta1",
				Categories:       []string{},
				ShortNames:       []string{"ot", "ots"},
			},
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
			// volumeclonesources.cdi.kubevirt.io
			"volumeclonesources": {
				Kind:             "VolumeCloneSource",
				ListKind:         "VolumeCloneSourceList",
				Plural:           "volumeclonesources",
				Singular:         "volumeclonesource",
				Versions:         []string{"v1beta1"},
				PreferredVersion: "v1beta1",
				Categories:       []string{},
				ShortNames:       []string{},
			},
			// volumeimportsources.cdi.kubevirt.io
			"volumeimportsources": {
				Kind:             "VolumeImportSource",
				ListKind:         "VolumeImportSourceList",
				Plural:           "volumeimportsources",
				Singular:         "volumeimportsource",
				Versions:         []string{"v1beta1"},
				PreferredVersion: "v1beta1",
				Categories:       []string{},
				ShortNames:       []string{},
			},
			// volumeuploadsources.cdi.kubevirt.io
			"volumeuploadsources": {
				Kind:             "VolumeUploadSource",
				ListKind:         "VolumeUploadSourceList",
				Plural:           "volumeuploadsources",
				Singular:         "volumeuploadsource",
				Versions:         []string{"v1beta1"},
				PreferredVersion: "v1beta1",
				Categories:       []string{},
				ShortNames:       []string{},
			},
		},
	},
	"kubevirt.io": {
		GroupRule: GroupRule{
			Group:            "kubevirt.io",
			Versions:         []string{"v1", "v1alpha3"},
			PreferredVersion: "v1",
		},
		ResourceRules: map[string]ResourceRule{
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
		GroupRule: GroupRule{
			Group:            "clone.kubevirt.io",
			Versions:         []string{"v1alpha1"},
			PreferredVersion: "v1alpha1",
		},
		ResourceRules: map[string]ResourceRule{
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
		GroupRule: GroupRule{
			Group:            "export.kubevirt.io",
			Versions:         []string{"v1alpha1"},
			PreferredVersion: "v1alpha1",
		},
		ResourceRules: map[string]ResourceRule{
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
		GroupRule: GroupRule{
			Group:            "instancetype.kubevirt.io",
			Versions:         []string{"v1alpha1", "v1alpha2"},
			PreferredVersion: "v1alpha2",
		},
		ResourceRules: map[string]ResourceRule{
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
		GroupRule: GroupRule{
			Group:            "migrations.kubevirt.io",
			Versions:         []string{"v1alpha1"},
			PreferredVersion: "v1alpha1",
		},
		ResourceRules: map[string]ResourceRule{
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
		GroupRule: GroupRule{
			Group:            "pool.kubevirt.io",
			Versions:         []string{"v1alpha1"},
			PreferredVersion: "v1alpha1",
		},
		ResourceRules: map[string]ResourceRule{
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
		GroupRule: GroupRule{
			Group:            "snapshot.kubevirt.io",
			Versions:         []string{"v1alpha1"},
			PreferredVersion: "v1alpha1",
		},
		ResourceRules: map[string]ResourceRule{
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

var KubevirtWebhooks = map[string]WebhookRule{
	"/validate-x-virtualization-deckhouse-io-v1-virtualmachine": {
		Path:     "/validate-kubevirt-io-v1-virtualmachine",
		Group:    "kubevirt.io",
		Resource: "virtualmachines",
	},
}
