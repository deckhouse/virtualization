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

package vmclass

import (
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type dataMetric struct {
	Name           string
	UID            string
	CPUType        string
	Phase          v1alpha2.VirtualMachineClassPhase
	AvailableNodes []string
}

// DO NOT mutate VirtualMachineClass!
func newDataMetric(vmclass *v1alpha2.VirtualMachineClass) *dataMetric {
	if vmclass == nil {
		return nil
	}

	return &dataMetric{
		Name:           vmclass.Name,
		UID:            string(vmclass.UID),
		CPUType:        string(vmclass.Spec.CPU.Type),
		Phase:          vmclass.Status.Phase,
		AvailableNodes: vmclass.Status.AvailableNodes,
	}
}
