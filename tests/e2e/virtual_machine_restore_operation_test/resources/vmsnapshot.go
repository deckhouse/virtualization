/*
Copyright 2025 Flant JSC

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

package resources

import (
	vmsnapshotbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vmsnapshot"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

func NewVMSnapshot(
	name, namespace, vmName string,
	requiredConsistency bool,
	keepIPAddress v1alpha2.KeepIPAddress,
) *v1alpha2.VirtualMachineSnapshot {
	return vmsnapshotbuilder.New(
		vmsnapshotbuilder.WithName(name),
		vmsnapshotbuilder.WithNamespace(namespace),
		vmsnapshotbuilder.WithVM(vmName),
		vmsnapshotbuilder.WithKeepIPAddress(keepIPAddress),
		vmsnapshotbuilder.WithRequiredConsistency(requiredConsistency),
	)
}
