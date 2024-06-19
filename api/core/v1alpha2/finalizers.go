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
	FinalizerClusterVirtualImageProtection = "virtualization.deckhouse.io/cvi-protection"
	FinalizerVirtualImageProtection        = "virtualization.deckhouse.io/vi-protection"
	FinalizerVirtualDiskProtection         = "virtualization.deckhouse.io/vd-protection"
	FinalizerPodProtection                 = "virtualization.deckhouse.io/pod-protection"
	FinalizerServiceProtection             = "virtualization.deckhouse.io/svc-protection"
	FinalizerIngressProtection             = "virtualization.deckhouse.io/ingress-protection"
	FinalizerSecretProtection              = "virtualization.deckhouse.io/secret-protection"
	FinalizerDVProtection                  = "virtualization.deckhouse.io/dv-protection"
	FinalizerPVCProtection                 = "virtualization.deckhouse.io/pvc-protection"
	FinalizerPVProtection                  = "virtualization.deckhouse.io/pv-protection"

	FinalizerCVIProtection            = "virtualization.deckhouse.io/cvi-protection"
	FinalizerVIProtection             = "virtualization.deckhouse.io/vi-protection"
	FinalizerVDProtection             = "virtualization.deckhouse.io/vd-protection"
	FinalizerKVVMProtection           = "virtualization.deckhouse.io/kvvm-protection"
	FinalizerVMOPProtection           = "virtualization.deckhouse.io/vmop-protection"
	FinalizerVMCPUProtection          = "virtualization.deckhouse.io/vmcpu-protection"
	FinalizerIPAddressClaimProtection = "virtualization.deckhouse.io/vmip-protection"

	FinalizerCVICleanup            = "virtualization.deckhouse.io/cvi-cleanup"
	FinalizerVDCleanup             = "virtualization.deckhouse.io/vd-cleanup"
	FinalizerVICleanup             = "virtualization.deckhouse.io/vi-cleanup"
	FinalizerVMCleanup             = "virtualization.deckhouse.io/vm-cleanup"
	FinalizerIPAddressClaimCleanup = "virtualization.deckhouse.io/vmip-cleanup"
	FinalizerIPAddressLeaseCleanup = "virtualization.deckhouse.io/vmipl-cleanup"
	FinalizerVMBDACleanup          = "virtualization.deckhouse.io/vmbda-cleanup"
	FinalizerVMOPCleanup           = "virtualization.deckhouse.io/vmop-cleanup"
)
