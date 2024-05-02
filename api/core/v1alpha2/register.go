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
		&VirtualMachineCPUModel{},
		&VirtualMachineCPUModelList{},
		&VirtualMachineIPAddressClaim{},
		&VirtualMachineIPAddressClaimList{},
		&VirtualMachineIPAddressLease{},
		&VirtualMachineIPAddressLeaseList{},
		&VirtualMachineOperation{},
		&VirtualMachineOperationList{},
	)
	metav1.AddToGroupVersion(scheme, SchemeGroupVersion)
	return nil
}
