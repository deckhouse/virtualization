package kvbuilder

import (
	"fmt"

	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	virtv2 "github.com/deckhouse/virtualization-controller/api/v2alpha1"
)

func ApplyVirtualMachineDiskSpec(dv *DV, vmd *virtv2.VirtualMachineDisk) {
	if vmd == nil {
		return
	}
	gvk := schema.GroupVersionKind{
		Group:   virtv2.SchemeGroupVersion.Group,
		Version: virtv2.SchemeGroupVersion.Version,
		Kind:    virtv2.VMDKind,
	}

	// FIXME: return error and change vmd_reconciler.
	err := applyDVSettings(dv, vmd, gvk, vmd.Spec.DataSource, vmd.Spec.PersistentVolumeClaim)
	if err != nil {
		panic(err.Error())
	}
}

func ApplyVirtualMachineImageSpec(dv *DV, vmi *virtv2.VirtualMachineImage, size string) error {
	if vmi == nil {
		return nil
	}
	gvk := schema.GroupVersionKind{
		Group:   virtv2.SchemeGroupVersion.Group,
		Version: virtv2.SchemeGroupVersion.Version,
		Kind:    virtv2.VMIKind,
	}

	// Get size from Status
	pvc := vmi.Spec.PersistentVolumeClaim
	if size != "" {
		pvc.Size = size
	}
	return applyDVSettings(dv, vmi, gvk, &vmi.Spec.DataSource, pvc)
}

func applyDVSettings(dv *DV, obj metav1.Object, gvk schema.GroupVersionKind, dataSource *virtv2.DataSource, pvc virtv2.VirtualMachinePersistentVolumeClaim) error {
	if dataSource != nil {
		switch dataSource.Type {
		case virtv2.DataSourceTypeHTTP:
			dv.SetHTTPDataSource(dataSource.HTTP.URL)
		default:
			return fmt.Errorf("%s/%s has unsupported dataSource type %q", gvk.Kind, obj.GetName(), dataSource.Type)
		}
	} else {
		dv.SetBlankDataSource()
	}

	// FIXME: resource.Quantity should be defined directly in the spec struct (see PVC impl. for details)
	pvcSize, err := resource.ParseQuantity(pvc.Size)
	if err != nil {
		return err
	}
	dv.SetPVC(pvc.StorageClassName, pvcSize)

	dv.SetOwnerRef(obj, gvk)
	dv.AddFinalizer(virtv2.FinalizerDVProtection)
	return nil
}
