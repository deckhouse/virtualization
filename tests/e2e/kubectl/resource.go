package kubectl

const (
	ResourceNode       Resource = "node"
	ResourceNamespace  Resource = "namespace"
	ResourcePod        Resource = "pod"
	ResourceKubevirtVM Resource = "virtualmachines.x.virtualization.deckhouse.io"
	ResourceVM         Resource = "virtualmachine.virtualization.deckhouse.io"
	ResourceVMIPClaim  Resource = "virtualmachineipaddressclaims.virtualization.deckhouse.io"
	ResourceVMIPLeas   Resource = "virtualmachineipaddressleases.virtualization.deckhouse.io"
)
