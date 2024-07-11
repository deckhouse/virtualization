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

package v1alpha2

type BlockDeviceSpecRef struct {
	Kind BlockDeviceKind `json:"kind"`
	Name string          `json:"name"`
}

type BlockDeviceStatusRef struct {
	Kind                                    BlockDeviceKind `json:"kind"`
	Name                                    string          `json:"name"`
	Size                                    string          `json:"size"`
	Target                                  string          `json:"target"`
	Attached                                bool            `json:"attached"`
	Hotpluggable                            bool            `json:"hotpluggable,omitempty"`
	VirtualMachineBlockDeviceAttachmentName string          `json:"virtualMachineBlockDeviceAttachmentName,omitempty"`
}

type BlockDeviceKind string

const (
	ClusterImageDevice BlockDeviceKind = "ClusterVirtualImage"
	ImageDevice        BlockDeviceKind = "VirtualImage"
	DiskDevice         BlockDeviceKind = "VirtualDisk"
)
