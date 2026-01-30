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

package v1alpha2

const (
	FinalizerCVIProtection                        = "virtualization.deckhouse.io/cvi-protection"
	FinalizerVIProtection                         = "virtualization.deckhouse.io/vi-protection"
	FinalizerVDProtection                         = "virtualization.deckhouse.io/vd-protection"
	FinalizerKVVMProtection                       = "virtualization.deckhouse.io/kvvm-protection"
	FinalizerIPAddressProtection                  = "virtualization.deckhouse.io/vmip-protection"
	FinalizerPodProtection                        = "virtualization.deckhouse.io/pod-protection"
	FinalizerVDSnapshotProtection                 = "virtualization.deckhouse.io/vdsnapshot-protection"
	FinalizerVMSnapshotProtection                 = "virtualization.deckhouse.io/vmsnapshot-protection"
	FinalizerVMOPProtectionByEvacuationController = "virtualization.deckhouse.io/vmop-protection-by-evacuation-controller"
	FinalizerVMOPProtectionByVMController         = "virtualization.deckhouse.io/vmop-protection-by-vm-controller"

	FinalizerCVICleanup             = "virtualization.deckhouse.io/cvi-cleanup"
	FinalizerVDCleanup              = "virtualization.deckhouse.io/vd-cleanup"
	FinalizerVICleanup              = "virtualization.deckhouse.io/vi-cleanup"
	FinalizerVMCleanup              = "virtualization.deckhouse.io/vm-cleanup"
	FinalizerIPAddressCleanup       = "virtualization.deckhouse.io/vmip-cleanup"
	FinalizerIPAddressLeaseCleanup  = "virtualization.deckhouse.io/vmipl-cleanup"
	FinalizerVDSnapshotCleanup      = "virtualization.deckhouse.io/vdsnapshot-cleanup"
	FinalizerVMOPCleanup            = "virtualization.deckhouse.io/vmop-cleanup"
	FinalizerVMSOPCleanup           = "virtualization.deckhouse.io/vmsop-cleanup"
	FinalizerVMClassCleanup         = "virtualization.deckhouse.io/vmclass-cleanup"
	FinalizerVMBDACleanup           = "virtualization.deckhouse.io/vmbda-cleanup"
	FinalizerMACAddressCleanup      = "virtualization.deckhouse.io/vmmac-cleanup"
	FinalizerMACAddressLeaseCleanup = "virtualization.deckhouse.io/vmmacl-cleanup"
	FinalizerNodeUSBDeviceCleanup  = "virtualization.deckhouse.io/nodeusbdevice-cleanup"
	FinalizerUSBDeviceCleanup      = "virtualization.deckhouse.io/usbdevice-cleanup"
)
