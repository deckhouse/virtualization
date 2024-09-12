package vmop

import virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"

type dataMetric struct {
	Name      string
	Namespace string
	UID       string
	Phase     virtv2.VMOPPhase
}

// DO NOT mutate VirtualMachineOperation!
func newDataMetric(vmop *virtv2.VirtualMachineOperation) *dataMetric {
	if vmop == nil {
		return nil
	}

	return &dataMetric{
		Name:      vmop.Name,
		Namespace: vmop.Namespace,
		UID:       string(vmop.UID),
		Phase:     vmop.Status.Phase,
	}
}
