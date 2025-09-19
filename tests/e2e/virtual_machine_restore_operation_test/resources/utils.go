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
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/utils/ptr"

	vdbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vd"
	vmopbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vmop"
)

func GenerateRestoreVMOP(name, namespace, vmSnapshotName, vmName string, restoreMode v1alpha2.VMOPRestoreMode) *v1alpha2.VirtualMachineOperation {
	restoreSpec := &v1alpha2.VirtualMachineOperationRestoreSpec{
		VirtualMachineSnapshotName: vmSnapshotName,
		Mode:                       restoreMode,
	}

	return vmopbuilder.New(
		vmopbuilder.WithName(name),
		vmopbuilder.WithNamespace(namespace),
		vmopbuilder.WithType(v1alpha2.VMOPTypeRestore),
		vmopbuilder.WithRestoreSpec(restoreSpec),
		vmopbuilder.WithVirtualMachine(vmName),
	)
}

func GenerateVDBlank(name, namespace, size string) *v1alpha2.VirtualDisk {
	return vdbuilder.New(
		vdbuilder.WithName(name),
		vdbuilder.WithNamespace(namespace),
		vdbuilder.WithSize(ptr.To(resource.MustParse(size))),
	)
}

func GenerateVDFromHttp(name, namespace, size, url string) *v1alpha2.VirtualDisk {
	return vdbuilder.New(
		vdbuilder.WithName(name),
		vdbuilder.WithNamespace(namespace),
		vdbuilder.WithSize(ptr.To(resource.MustParse(size))),
		vdbuilder.WithDataSourceHTTPWithOnlyURL(url),
	)
}
