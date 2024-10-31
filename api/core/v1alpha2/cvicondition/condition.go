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

package cvicondition

// Type represents the various condition types for the `ClusterVirtualImage`.
type Type string

func (s Type) String() string {
	return string(s)
}

const (
	// DatasourceReadyType indicates whether the datasource (for example, a `VirtualImage`) is ready, allowing the import process for the `ClusterVirtualImage` to start.
	DatasourceReadyType Type = "DatasourceReady"
	// ReadyType indicates whether the import process succeeded and the `ClusterVirtualImage` is ready for use.
	ReadyType Type = "Ready"
)

type Reason string

func (r Reason) String() string {
	return string(r)
}

const (
	// DatasourceReady indicates that the datasource is ready for use, allowing the import process to start.
	DatasourceReady Reason = "DatasourceReady"
	// ContainerRegistrySecretNotFound indicates that the container registry secret was not found, which prevents the import process from starting.
	ContainerRegistrySecretNotFound Reason = "ContainerRegistrySecretNotFound"
	// ImageNotReady indicates that the `VirtualImage` datasource is not ready yet, which prevents the import process from starting.
	ImageNotReady Reason = "ImageNotReady"
	// ClusterImageNotReady indicates that the `ClusterVirtualImage` datasource is not ready, which prevents the import process from starting.
	ClusterImageNotReady Reason = "ClusterImageNotReady"
	// VirtualDiskNotReady indicates that the `VirtualDisk` datasource is not ready, which prevents the import process from starting.
	VirtualDiskNotReady Reason = "VirtualDiskNotReady"

	// WaitForUserUpload indicates that the `ClusterVirtualImage` is waiting for the user to upload a datasource for the import process to continue.
	WaitForUserUpload Reason = "WaitForUserUpload"
	// Provisioning indicates that the provisioning process is currently in progress.
	Provisioning Reason = "Provisioning"
	// ProvisioningNotStarted indicates that the provisioning process has not started yet.
	ProvisioningNotStarted Reason = "ProvisioningNotStarted"
	// ProvisioningFailed indicates that the provisioning process has failed.
	ProvisioningFailed Reason = "ProvisioningFailed"
	// Ready indicates that the import process is complete and the `ClusterVirtualImage` is ready for use.
	Ready Reason = "Ready"
)
