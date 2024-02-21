package v1alpha2

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

const (
	APIGroup   = "virtualization.deckhouse.io"
	APIVersion = "v1alpha2"
)

// SchemeGroupVersion is group version used to register these objects
var SchemeGroupVersion = schema.GroupVersion{Group: APIGroup, Version: APIVersion}

// ClusterVirtualMachineImageGVK is group version kind for ClusterVirtualMachineImage
var ClusterVirtualMachineImageGVK = schema.GroupVersionKind{Group: SchemeGroupVersion.Group, Version: SchemeGroupVersion.Version, Kind: CVMIKind}

// VirtualMachineImageGVK is group version kind for VirtualMachineImage
var VirtualMachineImageGVK = schema.GroupVersionKind{Group: SchemeGroupVersion.Group, Version: SchemeGroupVersion.Version, Kind: VMIKind}

// VirtualMachineDiskGVK is group version kind for VirtualMachineDisk
var VirtualMachineDiskGVK = schema.GroupVersionKind{Group: SchemeGroupVersion.Group, Version: SchemeGroupVersion.Version, Kind: VMDKind}

// Kind takes an unqualified kind and returns back a Group qualified GroupKind
func Kind(kind string) schema.GroupKind {
	return SchemeGroupVersion.WithKind(kind).GroupKind()
}

// Resource takes an unqualified resource and returns a Group qualified GroupResource
func Resource(resource string) schema.GroupResource {
	return SchemeGroupVersion.WithResource(resource).GroupResource()
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
		&ClusterVirtualMachineImage{},
		&ClusterVirtualMachineImageList{},
		&VirtualMachineImage{},
		&VirtualMachineImageList{},
		&VirtualMachineDisk{},
		&VirtualMachineDiskList{},
		&VirtualMachine{},
		&VirtualMachineList{},
		&VirtualMachineBlockDeviceAttachment{},
		&VirtualMachineBlockDeviceAttachmentList{},
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
