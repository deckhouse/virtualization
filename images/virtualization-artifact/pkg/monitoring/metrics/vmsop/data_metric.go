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

package vmsop

import "github.com/deckhouse/virtualization/api/core/v1alpha2"

type dataMetric struct {
	Name      string
	Namespace string
	UID       string
	Phase     v1alpha2.VMSOPPhase
}

// DO NOT mutate VirtualMachineSnapshotOperation!
func newDataMetric(vmsop *v1alpha2.VirtualMachineSnapshotOperation) *dataMetric {
	if vmsop == nil {
		return nil
	}

	return &dataMetric{
		Name:      vmsop.Name,
		Namespace: vmsop.Namespace,
		UID:       string(vmsop.UID),
		Phase:     vmsop.Status.Phase,
	}
}
