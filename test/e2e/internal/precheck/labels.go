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
	PrecheckVMC = "vmc-precheck"

	// PrecheckSVDM - test requires SVDM module to be enabled.
	PrecheckSVDM = "svdm-precheck"

	// PrecheckStorageClass - test requires default StorageClass to be configured.
	PrecheckStorageClass = "storageclass-precheck"

	// PrecheckSnapshot - test requires snapshot-controller module to be enabled.
	PrecheckSnapshot = "snapshot-precheck"

	// PrecheckVirtualization - test requires virtualization module to be enabled.
	PrecheckVirtualization = "virtualization-precheck"

	// PrecheckUSB - test requires USB device with dummy_hcd to be configured.
	PrecheckUSB = "usb-precheck"

	// NoPrecheck - test doesn't require any prechecks.
	// Use this label for tests that don't depend on cluster configuration.
	NoPrecheck = "no-precheck"
)

// KnownPrecheckLabels returns all known precheck label constants.
func KnownPrecheckLabels() []string {
	return []string{
		PrecheckSDN,
		PrecheckVMC,
		PrecheckSVDM,
		PrecheckStorageClass,
		PrecheckSnapshot,
		PrecheckVirtualization,
		PrecheckUSB,
		NoPrecheck,
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