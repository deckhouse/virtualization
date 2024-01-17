package vm

import virtv2 "github.com/deckhouse/virtualization-controller/api/v1alpha2"

func ApprovalMode(vm *virtv2.VirtualMachine) virtv2.ApprovalMode {
	if vm.Spec.Disruptions == nil {
		return virtv2.Manual
	}
	return vm.Spec.Disruptions.ApprovalMode
}
