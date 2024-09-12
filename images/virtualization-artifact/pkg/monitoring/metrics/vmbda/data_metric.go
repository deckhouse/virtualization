package vmbda

import virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"

type dataMetric struct {
	Name      string
	Namespace string
	UID       string
	Phase     virtv2.BlockDeviceAttachmentPhase
}

// DO NOT mutate VirtualMachineBlockDeviceAttachment!
func newDataMetric(vmbda *virtv2.VirtualMachineBlockDeviceAttachment) *dataMetric {
	if vmbda == nil {
		return nil
	}

	return &dataMetric{
		Name:      vmbda.Name,
		Namespace: vmbda.Namespace,
		UID:       string(vmbda.UID),
		Phase:     vmbda.Status.Phase,
	}
}
