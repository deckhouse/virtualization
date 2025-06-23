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

// MaxVirtualImageNameLen determines the max len of vi.
// Disk and volume name in kubevirt can be a valid container name (len 63) since disk name can become a container name which will fail to schedule if invalid.
// We and kubevirt add prefixes "vi-", "volume" and suffix "-init", so max len reduced to 49.
const MaxVirtualImageNameLen = 37

// MaxClusterVirtualImageNameLen determines the max len of cvi.
// Disk and volume name in kubevirt can be a valid container name (len 63) since disk name can become a container name which will fail to schedule if invalid.
// We and kubevirt add prefixes "cvi-", "volume" and suffix "-init", so max len reduced to 48.
const MaxClusterVirtualImageNameLen = 36

// MaxVirtualMachineNameLen determines the max len of vm.
// The name of the VirtualMachine can be valid with a length of up to 63 characters, as exceeding this limit may cause a failure in creating internal resources.
const MaxVirtualMachineNameLen = 63
