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

package vicondition

// Type represents the various condition types for the `VirtualImage`.
type Type string

func (s Type) String() string {
	return string(s)
}

const (
	// DatasourceReadyType indicates whether the datasource (for example, a `VirtualImage`) is ready, allowing the import process for the `VirtualImage` to start.
	DatasourceReadyType Type = "DatasourceReady"
	// ReadyType indicates whether the import process succeeded and the `VirtualImage` is ready for use.
	ReadyType Type = "Ready"
	// StorageClassReadyType indicates whether the storageClass ready
	StorageClassReadyType Type = "StorageClassReady"
)

type (
	// DatasourceReadyReason represents the various reasons for the DatasourceReady condition type.
	DatasourceReadyReason string
	// ReadyReason represents the various reasons for the Ready condition type.
	ReadyReason string
	// StorageClassReadyReason represents the various reasons for the StorageClassReady condition type.
	StorageClassReadyReason string
)

func (s DatasourceReadyReason) String() string {
	return string(s)
}

func (s ReadyReason) String() string {
	return string(s)
}

const (
	// DatasourceReady indicates that the datasource is ready for use, allowing the import process to start.
	DatasourceReady DatasourceReadyReason = "DatasourceReady"
	// ContainerRegistrySecretNotFound indicates that the container registry secret was not found, which prevents the import process from starting.
	ContainerRegistrySecretNotFound DatasourceReadyReason = "ContainerRegistrySecretNotFound"
	// ImageNotReady indicates that the `VirtualImage` datasource is not ready yet, which prevents the import process from starting.
	ImageNotReady DatasourceReadyReason = "ImageNotReady"
	// ClusterImageNotReady indicates that the `VirtualImage` datasource is not ready, which prevents the import process from starting.
	ClusterImageNotReady DatasourceReadyReason = "ClusterImageNotReady"
	// VirtualDiskNotReady indicates that the `VirtualDisk` datasource is not ready, which prevents the import process from starting.
	VirtualDiskNotReady DatasourceReadyReason = "VirtualDiskNotReady"

	// WaitForUserUpload indicates that the `VirtualImage` is waiting for the user to upload a datasource for the import process to continue.
	WaitForUserUpload ReadyReason = "WaitForUserUpload"
	// Provisioning indicates that the provisioning process is currently in progress.
	Provisioning ReadyReason = "Provisioning"
	// ProvisioningNotStarted indicates that the provisioning process has not started yet.
	ProvisioningNotStarted ReadyReason = "ProvisioningNotStarted"
	// ProvisioningFailed indicates that the provisioning process has failed.
	ProvisioningFailed ReadyReason = "ProvisioningFailed"
	// StorageClassNotReady indicates that the provisioning process pending because `StorageClass` not ready.
	StorageClassNotReady ReadyReason = "StorageClassNotReady"
	// Ready indicates that the import process is complete and the `VirtualImage` is ready for use.
	Ready ReadyReason = "Ready"

	// Lost indicates that the underlying PersistentVolumeClaim has been lost and the `VirtualImage` can no longer be used.
	Lost ReadyReason = "PVCLost"

	// StorageClassReady indicates that the chosen StorageClass exists.
	StorageClassReady StorageClassReadyReason = "StorageClassReady"
	// StorageClassNotFound indicates that the chosen StorageClass not found.
	StorageClassNotFound StorageClassReadyReason = "StorageClassNotFound"
	// DVCRTypeUsed indicates that the DVCR provisioning chosen.
	DVCRTypeUsed StorageClassReadyReason = "DVCRTypeUsed"
)
