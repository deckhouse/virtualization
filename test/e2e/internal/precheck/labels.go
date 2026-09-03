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

package precheck

// Precheck labels for tests.
// Tests must declare required prechecks using these labels.
// Use NoPrecheck if test doesn't require any prechecks.

const (
	// PrecheckSDN - test requires SDN module to be enabled.
	PrecheckSDN = "sdn-precheck"

	// PrecheckVMC - test requires VMC module to be enabled.
	PrecheckVMC = "vmclass-precheck"

	// PrecheckSVDM - test requires the data-export machinery: storage-foundation enabled,
	// the deprecated storage-volume-data-manager disabled.
	PrecheckSVDM = "svdm-precheck"

	// PrecheckDefaultStorageClass - test requires default StorageClass to be configured.
	PrecheckDefaultStorageClass = "default-sc-precheck"

	// PrecheckRWOImmediateStorageClass - the suite StorageClass must not be RWO with Immediate binding.
	// This is a common precheck that runs for all tests automatically.
	PrecheckRWOImmediateStorageClass = "rwo-immediate-sc-precheck"

	// PrecheckSnapshot - test requires the CSI snapshot machinery: state-snapshotter and
	// storage-foundation enabled, the deprecated snapshot-controller disabled.
	PrecheckSnapshot = "snapshot-precheck"

	// PrecheckVirtualization - test requires virtualization module to be enabled.
	PrecheckVirtualization = "virtualization-precheck"

	// PrecheckMigrationLimits - test requires the migration limits to be disabled
	// on the virtualization ModuleConfig.
	PrecheckMigrationLimits = "migration-limits-precheck"

	// PrecheckUSB - test requires USB device with dummy_hcd to be configured.
	PrecheckUSB = "usb-precheck"

	// PrecheckAffinityToleration - test requires enough ready KVM-enabled master/worker nodes.
	PrecheckAffinityToleration = "affinity-toleration-precheck"

	// PrecheckTargetMigration - test requires target migration feature to be available.
	PrecheckTargetMigration = "target-migration-precheck"

	// PrecheckMigratable - test requires enough ready KVM-enabled nodes to tell a machine that
	// has a node to migrate to from a machine that has none.
	PrecheckMigratable = "migratable-precheck"

	// PrecheckPostCleanup - test requires postcleanup to be configured.
	PrecheckPostCleanup = "post-cleanup-precheck"

	// PrecheckPrecreatedCVI - test requires precreated ClusterVirtualImages to be available.
	// This is a common precheck that runs for all tests automatically.
	PrecheckPrecreatedCVI = "precreated-cvi-precheck"

	// NoPrecheck - test doesn't require any prechecks.
	// Use this label for tests that don't depend on cluster configuration.
	NoPrecheck = "no-precheck"

	// HotplugCPUWithLiveMigrationPrecheck - test requires HotplugCPUWithLiveMigration feature gate to be enabled.
	HotplugCPUWithLiveMigrationPrecheck = "hotplugcpuwithlivemigration-precheck"

	// HotplugMemoryWithLiveMigrationPrecheck - test requires HotplugMemoryWithLiveMigration feature gate to be enabled.
	HotplugMemoryWithLiveMigrationPrecheck = "hotplugmemorywithlivemigration-precheck"

	// HotplugInPlaceResizePrecheck - test requires HotplugCPUAndMemoryWithInPlaceResize feature gate to be enabled.
	HotplugInPlaceResizePrecheck = "hotpluginplaceresize-precheck"

	// PrecheckVerticalPodAutoscaler - test requires the vertical-pod-autoscaler module to be enabled.
	PrecheckVerticalPodAutoscaler = "vertical-pod-autoscaler-precheck"
)

// KnownPrecheckLabels returns all known precheck label constants.
func KnownPrecheckLabels() []string {
	return []string{
		PrecheckSDN,
		PrecheckVMC,
		PrecheckSVDM,
		PrecheckDefaultStorageClass,
		PrecheckRWOImmediateStorageClass,
		PrecheckSnapshot,
		PrecheckVirtualization,
		PrecheckMigrationLimits,
		PrecheckUSB,
		PrecheckAffinityToleration,
		PrecheckTargetMigration,
		PrecheckMigratable,
		PrecheckPostCleanup,
		PrecheckPrecreatedCVI,
		NoPrecheck,
		HotplugCPUWithLiveMigrationPrecheck,
		HotplugMemoryWithLiveMigrationPrecheck,
		HotplugInPlaceResizePrecheck,
		PrecheckVerticalPodAutoscaler,
	}
}

// IsPrecheckLabel returns true if the given label is a known precheck label.
func IsPrecheckLabel(label string) bool {
	for _, known := range KnownPrecheckLabels() {
		if label == known {
			return true
		}
	}
	return false
}
