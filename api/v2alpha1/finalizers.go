package v2alpha1

const (
	FinalizerDVProtection  = "virtualization.deckhouse.io/dv-protection"
	FinalizerPVCProtection = "virtualization.deckhouse.io/pvc-protection"
	FinalizerPVProtection  = "virtualization.deckhouse.io/pv-protection"
	FinalizerVMDCleanup    = "virtualization.deckhouse.io/vmd-cleanup"

	FinalizerVMIProtection  = "virtualization.deckhouse.io/vmi-protection"
	FinalizerCVMIProtection = "virtualization.deckhouse.io/cvmi-protection"
	FinalizerVMDProtection  = "virtualization.deckhouse.io/vmd-protection"
	// FinalizerKVVMIProtection = "virtualization.deckhouse.io/kvvmi-protection"
	FinalizerKVVMProtection = "virtualization.deckhouse.io/kvvm-protection"
	FinalizerVMCleanup      = "virtualization.deckhouse.io/vm-cleanup"
)
