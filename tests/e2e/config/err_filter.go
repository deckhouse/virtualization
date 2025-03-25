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

package config

const (
	VirtualMachineTooManyRequestsFilter           = "Server rejected event (will not retry!)"                                                 // Msg.
	VirtualMachineInternalErrorFilter             = "Internal virtual machine error"                                                          // Msg.
	VirtualDiskObjectRefValidationFilter          = "validation failed for data source objectref"                                             // Err.
	VirtualDiskMetadataPatchingFilter             = "error patching metadata: virtualdisks"                                                   // Err.
	VirtualMachineClassMetadataPatchingFilter     = "error patching metadata: virtualmachineclasses"                                          // Err.
	VirtualDiskCleanUpFilter                      = "clean up failed for data source registry"                                                // Err.
	ImagesVirtualDiskSnapshotObjectRefIsNilFilter = "VDSnapshot object ref"                                                                   // Err
	ImagesVirtualDiskSnapshotNotReadyFilter       = "VirtualDiskSnapshot "                                                                    // Err
	ImagesVIObjectRefIsNilFilter                  = "VI object ref source "                                                                   // Err
	AllMetadataPatchingFilter                     = "error patching metadata: the server rejected our request due to an error in our request" // Err.
)
