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

import kvalidation "k8s.io/apimachinery/pkg/util/validation"

// The underlying KubeVirt volume/disk name (and the container it may become) must
// be a valid DNS-1123 label (<=63). That name is now derived independently and
// shortened to fit (see kvbuilder.GenerateDiskName), so the user-facing name is no
// longer constrained by it and may use the full Kubernetes DNS subdomain length.

// MaxDiskNameLen determines the max len of vd.
const MaxDiskNameLen = kvalidation.DNS1123SubdomainMaxLength

// MaxVirtualImageNameLen determines the max len of vi.
const MaxVirtualImageNameLen = kvalidation.DNS1123SubdomainMaxLength

// MaxClusterVirtualImageNameLen determines the max len of cvi.
const MaxClusterVirtualImageNameLen = kvalidation.DNS1123SubdomainMaxLength

// MaxVirtualMachineNameLen determines the max len of vm.
// Unlike disks/images, a VirtualMachine name is not decoupled yet: it still flows
// into KubeVirt pod names (e.g. the launcher pod "d8v-vm-<name>-...") and label
// values that cap at 63. Raising it requires changes in the KubeVirt fork, so it
// stays limited for now.
const MaxVirtualMachineNameLen = 63
