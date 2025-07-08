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

// MaxDiskNameLen determines the max len of vd.
// Disk and volume name in kubevirt can be a valid container name (len 63) since disk name can become a container name which will fail to schedule if invalid.
// We add prefix "vd-" for the vd name, so max len reduced to 60.
const MaxDiskNameLen = 60

// MaxVirtualImageNameLen determines the max len of vi on dvcr.
// Disk and volume name in kubevirt can be a valid container name (len 63) since disk name can become a container name which will fail to schedule if invalid.
// We add prefixes "vi-", so max len reduced to 60.
const MaxVirtualImageNameLen = 60

// MaxClusterVirtualImageNameLen determines the max len of cvi.
// Disk and volume name in kubevirt can be a valid container name (len 63) since disk name can become a container name which will fail to schedule if invalid.
// We add prefixes "cvi-", so max len reduced to 59.
const MaxClusterVirtualImageNameLen = 59

// MaxVirtualMachineNameLen determines the max len of vm.
// The limitation is reportedly associated with the PodDisruptionBudget resource, which has a label containing the virtual machine's name, and the label's value cannot exceed 63 characters.
const MaxVirtualMachineNameLen = 63
