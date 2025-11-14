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

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/deckhouse/virtualization/api/core"
)

const Version = "v1alpha2"

// SchemeGroupVersion is group version used to register these objects
var SchemeGroupVersion = schema.GroupVersion{Group: core.GroupName, Version: Version}

// ClusterVirtualImageGVK is group version kind for ClusterVirtualImage
var ClusterVirtualImageGVK = schema.GroupVersionKind{Group: SchemeGroupVersion.Group, Version: SchemeGroupVersion.Version, Kind: ClusterVirtualImageKind}

// VirtualImageGVK is group version kind for VirtualImage
var VirtualImageGVK = schema.GroupVersionKind{Group: SchemeGroupVersion.Group, Version: SchemeGroupVersion.Version, Kind: VirtualImageKind}

// VirtualDiskGVK is group version kind for VirtualDisk
var VirtualDiskGVK = schema.GroupVersionKind{Group: SchemeGroupVersion.Group, Version: SchemeGroupVersion.Version, Kind: VirtualDiskKind}

// Kind takes an unqualified kind and returns back a Group qualified GroupKind
func Kind(kind string) schema.GroupKind {
	return SchemeGroupVersion.WithKind(kind).GroupKind()
}

// Resource takes an unqualified resource and returns a Group qualified GroupResource
func Resource(resource string) schema.GroupResource {
	return SchemeGroupVersion.WithResource(resource).GroupResource()
}

func GroupVersionResource(resource string) schema.GroupVersionResource {
	return SchemeGroupVersion.WithResource(resource)
}

var (
	// SchemeBuilder tbd
	SchemeBuilder = runtime.NewSchemeBuilder(addKnownTypes)
	// AddToScheme tbd
	AddToScheme = SchemeBuilder.AddToScheme
)

// Adds the list of known types to Scheme.
func addKnownTypes(scheme *runtime.Scheme) error {
	scheme.AddKnownTypes(SchemeGroupVersion,
		&ClusterVirtualImage{},
		&ClusterVirtualImageList{},
		&VirtualImage{},
		&VirtualImageList{},
		&VirtualDisk{},
		&VirtualDiskList{},
		&VirtualMachine{},
		&VirtualMachineList{},
		&VirtualMachineBlockDeviceAttachment{},
		&VirtualMachineBlockDeviceAttachmentList{},
		&VirtualMachineClass{},
		&VirtualMachineClassList{},
		&VirtualMachineIPAddress{},
		&VirtualMachineIPAddressList{},
		&VirtualMachineIPAddressLease{},
		&VirtualMachineIPAddressLeaseList{},
		&VirtualMachineOperation{},
		&VirtualMachineOperationList{},
		&VirtualDiskSnapshot{},
		&VirtualDiskSnapshotList{},
		&VirtualMachineSnapshot{},
		&VirtualMachineSnapshotList{},
		&VirtualMachineSnapshotOperation{},
		&VirtualMachineSnapshotOperationList{},
		&VirtualMachineRestore{},
		&VirtualMachineRestoreList{},
		&VirtualMachineMACAddress{},
		&VirtualMachineMACAddressList{},
		&VirtualMachineMACAddressLease{},
		&VirtualMachineMACAddressLeaseList{},
	)
	metav1.AddToGroupVersion(scheme, SchemeGroupVersion)
	return nil
}
