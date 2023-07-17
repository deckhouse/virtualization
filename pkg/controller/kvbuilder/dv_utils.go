package kvbuilder

import (
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/runtime/schema"

	virtv2 "github.com/deckhouse/virtualization-controller/api/v2alpha1"
)

func ApplyVirtualMachineDiskSpec(dv *DV, vmd *virtv2.VirtualMachineDisk) {
	if vmd.Spec.DataSource.HTTP != nil {
		dv.SetHTTPDataSource(vmd.Spec.DataSource.HTTP.URL)
	}

	// FIXME: resource.Quantity should be defined directly in the spec struct (see PVC impl. for details)
	pvcSize, err := resource.ParseQuantity(vmd.Spec.PersistentVolumeClaim.Size)
	if err != nil {
		panic(err.Error())
	}
	dv.SetPVC(vmd.Spec.PersistentVolumeClaim.StorageClassName, pvcSize)

	dv.AddOwnerRef(vmd, schema.GroupVersionKind{
		Group:   virtv2.SchemeGroupVersion.Group,
		Version: virtv2.SchemeGroupVersion.Version,
		Kind:    "VirtualMachineDisk",
	})
	dv.AddFinalizer(virtv2.FinalizerDVProtection)
}
