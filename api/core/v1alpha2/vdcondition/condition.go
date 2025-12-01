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
type Type string

func (s Type) String() string {
	return string(s)
}

const (
	// DatasourceReadyType indicates whether the data source (for example, a `VirtualImage`) is ready, allowing the import process for the `VirtualDisk` to start.
	DatasourceReadyType Type = "DatasourceReady"
	// ReadyType indicates whether the import process succeeded and the `VirtualDisk` is ready for use.
	ReadyType Type = "Ready"
	// ResizingType indicates whether a disk resizing operation is in progress.
	ResizingType Type = "Resizing"
	// SnapshottingType indicates whether the disk snapshotting operation is in progress.
	SnapshottingType Type = "Snapshotting"
	// StorageClassReadyType indicates whether the storage class is ready.
	StorageClassReadyType Type = "StorageClassReady"
	// InUseType indicates whether the VirtualDisk is attached to a running VirtualMachine or is being used in a process of an image creation.
	InUseType Type = "InUse"
	// MigratingType indicates that the virtual disk is in the process of migrating data from one volume to another (during the migration of a local disk or migration to another storage class).
	MigratingType Type = "Migrating"
)

type (
	// DatasourceReadyReason represents the various reasons for the DatasourceReady condition type.
	DatasourceReadyReason string
	// ReadyReason represents the various reasons for the Ready condition type.
	ReadyReason string
	// ResizedReason represents the various reasons for the Resized condition type.
	ResizedReason string
	// SnapshottingReason represents the various reasons for the Snapshotting condition type.
	SnapshottingReason string
	// StorageClassReadyReason represents the various reasons for the Storageclass ready condition type.
	StorageClassReadyReason string
	// InUseReason represents the various reasons for the InUse condition type.
	InUseReason string
	// MigratingReason represents the various reasons for the Migration condition type.
	MigratingReason string
)

func (s DatasourceReadyReason) String() string {
	return string(s)
}

func (s ReadyReason) String() string {
	return string(s)
}

func (s ResizedReason) String() string {
	return string(s)
}

func (s SnapshottingReason) String() string {
	return string(s)
}

func (s StorageClassReadyReason) String() string {
	return string(s)
}

func (s InUseReason) String() string {
	return string(s)
}

func (s MigratingReason) String() string {
	return string(s)
}

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
	// ImageNotFound indicates that the `VirtualImage` datasource is not found, which prevents the import process from starting.
	ImageNotFound DatasourceReadyReason = "ImageNotFound"
	// ClusterImageNotFound indicates that the `ClusterVirtualImage` datasource is not found, which prevents the import process from starting.
	ClusterImageNotFound DatasourceReadyReason = "ClusterImageNotFound"

	// AddingOriginalMetadataNotStarted indicates that the adding original metadata process has not started yet.
	AddingOriginalMetadataNotStarted ReadyReason = "AddingOriginalMetadataNotStarted"
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
	// Exporting indicates that the VirtualDisk is being exported.
	Exporting ReadyReason = "Exporting"
	// QuotaExceeded indicates that the VirtualDisk is reached project quotas and can not be provisioned.
	QuotaExceeded ReadyReason = "QuotaExceeded"
	// ImagePullFailed indicates that there was an issue with importing from DVCR.
	ImagePullFailed ReadyReason = "ImagePullFailed"
	// DatasourceIsNotReady indicates that Datasource is not ready for provisioning.
	DatasourceIsNotReady ReadyReason = "DatasourceIsNotReady"
	// DatasourceIsNotFound indicates that Datasource is not found.
	DatasourceIsNotFound ReadyReason = "DatasourceIsNotFound"
	// StorageClassIsNotReady indicates that Storage class is not ready.
	StorageClassIsNotReady ReadyReason = "StorageClassIsNotReady"

	// InProgress indicates that the resize request has been detected and the operation is currently in progress.
	InProgress ResizedReason = "InProgress"
	// ResizingNotAvailable indicates that the resize operation is not available for now.
	ResizingNotAvailable SnapshottingReason = "NotAvailable"

	// Snapshotting indicates that the snapshotting operation has been successfully started and is in progress now.
	Snapshotting SnapshottingReason = "Snapshotting"
	// SnapshottingNotAvailable indicates that the snapshotting operation is not available for now.
	SnapshottingNotAvailable SnapshottingReason = "NotAvailable"

	// StorageClassReady indicates that the storage class is ready
	StorageClassReady StorageClassReadyReason = "StorageClassReady"
	// StorageClassNotReady indicates that the storage class is not ready
	StorageClassNotReady StorageClassReadyReason = "StorageClassNotReady"
)

/*
The status transitions of an 'InUse' condition depend on its current usage context:

- If an image creation object (VI/CVI) is detected and its phase is `Pending` or `Provisioning`,
the condition's reason is set to `UsedForImageCreation`.

- If a VirtualMachine is detected and its phase is anything other than `Pending` or `Stopped`,
the condition's reason is set to `AttachedToVirtualMachine`.

- If the VirtualMachine is in the `Pending` phase:
  - If any of the conditions `VirtualMachineIPAddressReady`, `ProvisioningReady`, or `VirtualMachineClassReady` are `False`,
    the condition's reason is set to `NotInUse`.
  - If all these conditions are `True`, the condition's reason is set to `AttachedToVirtualMachine`.

- If the VirtualMachine is in the `Stopped` phase:
  - If there is a state change in progress (indicating a restart) or if the Pod's phase is `Running`,
    the condition's reason is set to `AttachedToVirtualMachine`.
  - Otherwise, the condition's reason is set to `NotInUse`.

- If both a VirtualMachine and an image are detected, it gives priority to the VirtualMachine and sets
the `InUse` condition's reason to `AttachedToVirtualMachine`.
*/
const (
	// UsedForImageCreation indicates that the VirtualDisk is used for create image.
	UsedForImageCreation InUseReason = "UsedForImageCreation"
	// UsedForDataExport indicates that the VirtualDisk is used for data export.
	UsedForDataExport InUseReason = "UsedForDataExport"
	// AttachedToVirtualMachine indicates that the VirtualDisk is attached to VirtualMachine.
	AttachedToVirtualMachine InUseReason = "AttachedToVirtualMachine"
	// NotInUse indicates that VirtualDisk free for use.
	NotInUse InUseReason = "NotInUse"
)

const (
	// MigratingWaitForTargetReadyReason indicates that the target for migration is ready.
	MigratingWaitForTargetReadyReason MigratingReason = "WaitForTargetReady"
	// MigratingInProgressReason indicates that the VirtualDisk is migrating.
	MigratingInProgressReason MigratingReason = "InProgress"

	ResizingInProgressReason     MigratingReason = "ResizingInProgress"
	SnapshottingInProgressReason MigratingReason = "SnapshottingInProgress"
	StorageClassNotFoundReason   MigratingReason = "StorageClassNotFound"
)
