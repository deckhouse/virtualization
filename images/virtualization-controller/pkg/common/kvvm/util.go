package kvvm

import (
	"fmt"

	"k8s.io/apimachinery/pkg/types"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// PatchRunStrategy returns JSON merge patch to set 'runStrategy' field to the desired value
// and reset deprecated 'running' field to null.
func PatchRunStrategy(runStrategy virtv1.VirtualMachineRunStrategy) client.Patch {
	runStrategyPatch := fmt.Sprintf(`{"spec":{"runStrategy": "%s", "running": null}}`, runStrategy)
	return client.RawPatch(types.MergePatchType, []byte(runStrategyPatch))
}

// GetRunStrategy returns runStrategy field value.
func GetRunStrategy(kvvm *virtv1.VirtualMachine) virtv1.VirtualMachineRunStrategy {
	if kvvm == nil || kvvm.Spec.RunStrategy == nil {
		return virtv1.RunStrategyUnknown
	}
	return *kvvm.Spec.RunStrategy
}
