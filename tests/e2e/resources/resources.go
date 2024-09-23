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

package resourses

type VirtualMachineMigration struct {
	ApiVersion string          `yaml:"apiVersion"`
	Kind       string          `yaml:"kind"`
	Metadata   Metadata        `yaml:"metadata"`
	Spec       MigrationSpec   `yaml:"spec"`
	Status     MigrationStatus `yaml:"status,omitempty"`
}

type Metadata struct {
	Name   string            `yaml:"name"`
	Labels map[string]string `yaml:"labels,omitempty"`
}

type MigrationSpec struct {
	VmiName string `yaml:"vmiName"`
}

type MigrationStatus struct {
	Phase string `yaml:"phase,omitempty"`
}

type VirtualMachineBlockDeviceAttachment struct {
	ApiVersion string                                  `yaml:"apiVersion"`
	Kind       string                                  `yaml:"kind"`
	Metadata   Metadata                                `yaml:"metadata"`
	Spec       VirtualMachineBlockDeviceAttachmentSpec `yaml:"spec"`
}

type VirtualMachineBlockDeviceAttachmentSpec struct {
	VirtualMachineName string         `yaml:"virtualMachineName"`
	BlockDeviceRef     BlockDeviceRef `yaml:"blockDeviceRef"`
}

type BlockDeviceRef struct {
	Kind string `yaml:"kind"`
	Name string `yaml:"name"`
}
