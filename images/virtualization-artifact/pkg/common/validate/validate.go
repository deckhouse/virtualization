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

package validate

// VirtualDisk/VirtualImage/ClusterVirtualImage names are not length-limited by DVP:
// the derived KubeVirt volume/disk name is shortened independently (see
// kvbuilder.GenerateDiskName), and the overall name is already bounded by Kubernetes
// (DNS subdomain, <=253). Only VirtualMachine keeps a DVP limit.

// MaxVirtualMachineNameLen determines the max len of vm.
// Unlike disks/images, a VirtualMachine name is not decoupled: it flows into KubeVirt
// pod names (e.g. the launcher pod "d8v-vm-<name>-...") and label values that cap at
// 63 (Kubernetes does not enforce this, so DVP must). Raising it requires changes in
// the KubeVirt fork.
const MaxVirtualMachineNameLen = 63
