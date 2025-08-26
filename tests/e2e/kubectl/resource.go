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

package kubectl

const (
	ResourceNode         Resource = "node"
	ResourceNamespace    Resource = "namespace"
	ResourcePod          Resource = "pod"
	ResourceService      Resource = "service"
	ResourceStorageClass Resource = "storageclasses.storage.k8s.io"
	ResourceProject      Resource = "projects.deckhouse.io"
	ResourceKubevirtVM   Resource = "internalvirtualizationvirtualmachines.internal.virtualization.deckhouse.io"
	ResourceKubevirtVMI  Resource = "internalvirtualizationvirtualmachineinstances.internal.virtualization.deckhouse.io"
	ResourceKubevirtVMIM Resource = "internalvirtualizationvirtualmachineinstancemigrations.internal.virtualization.deckhouse.io"
	ResourceModuleConfig Resource = "moduleconfigs.deckhouse.io"
	ResourceVD           Resource = "virtualdisks.virtualization.deckhouse.io"
	ResourceVDSnapshot   Resource = "virtualdisksnapshots.virtualization.deckhouse.io"
	ResourceVM           Resource = "virtualmachine.virtualization.deckhouse.io"
	ResourceVMClass      Resource = "virtualmachineclasses.virtualization.deckhouse.io"
	ResourceVMIP         Resource = "virtualmachineipaddresses.virtualization.deckhouse.io"
	ResourceVMIPLease    Resource = "virtualmachineipaddressleases.virtualization.deckhouse.io"
	ResourceCVI          Resource = "clustervirtualimages.virtualization.deckhouse.io"
	ResourceVI           Resource = "virtualimages.virtualization.deckhouse.io"
	ResourceVMBDA        Resource = "virtualmachineblockdeviceattachments.virtualization.deckhouse.io"
	ResourceVMOP         Resource = "virtualmachineoperations.virtualization.deckhouse.io"
)
