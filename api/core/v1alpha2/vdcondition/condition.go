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

package vdcondition

// Type represents the various condition types for the `VirtualDisk`.
type Type = string

const (
	// DatasourceReadyType indicates whether the data source (for example, a `VirtualImage`) is ready, allowing the import process for the `VirtualDisk` to start.
	DatasourceReadyType Type = "DatasourceReady"
	// ReadyType indicates whether the import process succeeded and the `VirtualDisk` is ready for use.
	ReadyType Type = "Ready"
	// ResizedType indicates whether the disk resizing operation is completed.
	ResizedType Type = "Resized"
	// SnapshottingType indicates whether the disk snapshotting operation is in progress.
	SnapshottingType Type = "Snapshotting"
	// StorageclassReady
	StorageclassReadyType Type = "DataclassReady"
)

type (
	// DatasourceReadyReason represents the various reasons for the DatasourceReady condition type.
	DatasourceReadyReason = string
	// ReadyReason represents the various reasons for the Ready condition type.
	ReadyReason = string
	// ResizedReason represents the various reasons for the Resized condition type.
	ResizedReason = string
	// SnapshottingReason represents the various reasons for the Snapshotting condition type.
	SnapshottingReason = string
	// placeholder
	StorageclassReadyReason = string
)

const (
	// DatasourceReady indicates that the datasource is ready for use, allowing the import process to start.
	DatasourceReady DatasourceReadyReason = "DatasourceReady"
	// ContainerRegistrySecretNotFound indicates that the container registry secret was not found, which prevents the import process from starting.
	ContainerRegistrySecretNotFound DatasourceReadyReason = "ContainerRegistrySecretNotFound"
	// ImageNotReady indicates that the `VirtualImage` datasource is not ready yet, which prevents the import process from starting.
	ImageNotReady DatasourceReadyReason = "ImageNotReady"
	// ClusterImageNotReady indicates that the `VirtualDisk` datasource is not ready, which prevents the import process from starting.
	ClusterImageNotReady DatasourceReadyReason = "ClusterImageNotReady"
	// VirtualDiskSnapshotNotReady indicates that the `VirtualDiskSnapshot` datasource is not ready, which prevents the import process from starting.
	VirtualDiskSnapshotNotReady DatasourceReadyReason = "VirtualDiskSnapshot"

	// WaitForUserUpload indicates that the `VirtualDisk` is waiting for the user to upload a datasource for the import process to continue.
	WaitForUserUpload ReadyReason = "WaitForUserUpload"
	// Provisioning indicates that the provisioning process is currently in progress.
	Provisioning ReadyReason = "Provisioning"
	// ProvisioningNotStarted indicates that the provisioning process has not started yet.
	ProvisioningNotStarted ReadyReason = "ProvisioningNotStarted"
	// WaitingForFirstConsumer indicates that the provisioning has been suspended: a created and scheduled virtual machine is awaited.
	WaitingForFirstConsumer ReadyReason = "WaitingForFirstConsumer"
	// ProvisioningFailed indicates that the provisioning process has failed.
	ProvisioningFailed ReadyReason = "ProvisioningFailed"
	// Ready indicates that the import process is complete and the `VirtualDisk` is ready for use.
	Ready ReadyReason = "Ready"
	// Lost indicates that the underlying PersistentVolumeClaim has been lost and the `VirtualDisk` can no longer be used.
	Lost ReadyReason = "PVCLost"

	// ResizingNotRequested indicates that the resize operation has not been requested yet.
	ResizingNotRequested ResizedReason = "NotRequested"
	// InProgress indicates that the resize request has been detected and the operation is currently in progress.
	InProgress ResizedReason = "InProgress"
	// Resized indicates that the resize operation has been successfully completed.
	Resized ResizedReason = "Resized"
	// ResizingNotAvailable indicates that the resize operation is not available for now.
	ResizingNotAvailable SnapshottingReason = "NotAvailable"

	// SnapshottingNotRequested indicates that the snapshotting operation has been successfully started and is in progress now.
	SnapshottingNotRequested SnapshottingReason = "NotRequested"
	// Snapshotting indicates that the snapshotting operation has been successfully started and is in progress now.
	Snapshotting SnapshottingReason = "Snapshotting"
	// SnapshottingNotAvailable indicates that the snapshotting operation is not available for now.
	SnapshottingNotAvailable SnapshottingReason = "NotAvailable"

	// placeholder
	StorageclassReady StorageclassReadyReason = "DataclassReady"
	// placeholder
	StorageclassNotReady StorageclassReadyReason = "DataclassNotReady"
)
