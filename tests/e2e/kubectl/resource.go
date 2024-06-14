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
	ResourceNode        Resource = "node"
	ResourceNamespace   Resource = "namespace"
	ResourcePod         Resource = "pod"
	ResourceService     Resource = "service"
	ResourceKubevirtVM  Resource = "virtualmachines.x.virtualization.deckhouse.io"
	ResourceKubevirtVMI Resource = "virtualmachineinstances.x.virtualization.deckhouse.io"
	ResourceVM          Resource = "virtualmachine.virtualization.deckhouse.io"
	ResourceVMIPClaim   Resource = "virtualmachineipaddressclaims.virtualization.deckhouse.io"
	ResourceVMIPLeas    Resource = "virtualmachineipaddressleases.virtualization.deckhouse.io"
)