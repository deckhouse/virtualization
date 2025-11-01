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

package object

import (
	"github.com/deckhouse/virtualization-controller/pkg/builder/vmbda"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

func NewVMBDAFromDisk(name, vmName string, vd *v1alpha2.VirtualDisk, opts ...vmbda.Option) *v1alpha2.VirtualMachineBlockDeviceAttachment {
	bda := vmbda.New(
		vmbda.WithName(name),
		vmbda.WithNamespace(vd.Namespace),
		vmbda.WithVirtualMachineName(vmName),
		vmbda.WithBlockDeviceRef(v1alpha2.VMBDAObjectRefKindVirtualDisk, vd.Name),
	)
	vmbda.ApplyOptions(bda, opts...)
	return bda
}
