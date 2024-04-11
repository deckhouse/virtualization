package v1alpha2

const (
	FinalizerPodProtection     = "virtualization.deckhouse.io/pod-protection"
	FinalizerServiceProtection = "virtualization.deckhouse.io/svc-protection"
	FinalizerIngressProtection = "virtualization.deckhouse.io/ingress-protection"
	FinalizerSecretProtection  = "virtualization.deckhouse.io/secret-protection"
	FinalizerDVProtection      = "virtualization.deckhouse.io/dv-protection"
	FinalizerPVCProtection     = "virtualization.deckhouse.io/pvc-protection"
	FinalizerPVProtection      = "virtualization.deckhouse.io/pv-protection"

	FinalizerCVMIProtection  = "virtualization.deckhouse.io/cvi-protection"
	FinalizerVMIProtection   = "virtualization.deckhouse.io/vi-protection"
	FinalizerVMDProtection   = "virtualization.deckhouse.io/vd-protection"
	FinalizerKVVMProtection  = "virtualization.deckhouse.io/kvvm-protection"
	FinalizerVMOPProtection  = "virtualization.deckhouse.io/vmop-protection"
	FinalizerVMCPUProtection = "virtualization.deckhouse.io/vmcpu-protection"

	FinalizerCVMICleanup           = "virtualization.deckhouse.io/cvi-cleanup"
	FinalizerVMICleanup            = "virtualization.deckhouse.io/vi-cleanup"
	FinalizerVMDCleanup            = "virtualization.deckhouse.io/vd-cleanup"
	FinalizerVMCleanup             = "virtualization.deckhouse.io/vm-cleanup"
	FinalizerIPAddressClaimCleanup = "virtualization.deckhouse.io/vmip-cleanup"
	FinalizerIPAddressLeaseCleanup = "virtualization.deckhouse.io/vmipl-cleanup"
	FinalizerVMBDACleanup          = "virtualization.deckhouse.io/vmbda-cleanup"
	FinalizerVMOPCleanup           = "virtualization.deckhouse.io/vmop-cleanup"
)
