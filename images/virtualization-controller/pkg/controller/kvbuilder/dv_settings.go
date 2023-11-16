package kvbuilder

import (
	"fmt"

	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	virtv2 "github.com/deckhouse/virtualization-controller/api/v2alpha1"
	"github.com/deckhouse/virtualization-controller/pkg/dvcr"
)

func ApplyVirtualMachineDiskSpec(dv *DV, vmd *virtv2.VirtualMachineDisk, pvcSize resource.Quantity, dvcrSettings *dvcr.Settings) error {
	if vmd == nil {
		return nil
	}
	gvk := schema.GroupVersionKind{
		Group:   virtv2.SchemeGroupVersion.Group,
		Version: virtv2.SchemeGroupVersion.Version,
		Kind:    virtv2.VMDKind,
	}

	dvcrImageName := dvcr.RegistryImageName(dvcrSettings, dvcr.ImagePathForVMD(vmd))

	return applyDVSettings(dv, vmd, gvk, vmd.Spec.DataSource, pvcSize, vmd.Spec.PersistentVolumeClaim.StorageClassName, dvcrImageName)
}

func ApplyVirtualMachineImageSpec(dv *DV, vmi *virtv2.VirtualMachineImage, pvcSize resource.Quantity, dvcrSettings *dvcr.Settings) error {
	if vmi == nil {
		return nil
	}
	gvk := schema.GroupVersionKind{
		Group:   virtv2.SchemeGroupVersion.Group,
		Version: virtv2.SchemeGroupVersion.Version,
		Kind:    virtv2.VMIKind,
	}

	dvcrImageName := dvcr.RegistryImageName(dvcrSettings, dvcr.ImagePathForVMI(vmi))

	return applyDVSettings(dv, vmi, gvk, &vmi.Spec.DataSource, pvcSize, vmi.Spec.PersistentVolumeClaim.StorageClassName, dvcrImageName)
}

func applyDVSettings(
	dv *DV,
	obj metav1.Object,
	gvk schema.GroupVersionKind,
	dataSource *virtv2.DataSource,
	pvcSize resource.Quantity,
	pvcStorageClassName string,
	dvcrImageName string,
) error {
	if dataSource != nil {
		switch dataSource.Type {
		case virtv2.DataSourceTypeHTTP,
			virtv2.DataSourceTypeUpload,
			virtv2.DataSourceTypeContainerImage:
			// The image was preloaded from source into dvcr.
			// We can't use the same data source a second time, but we can set dvcr as the data source.
			dv.SetRegistryDataSource(dvcrImageName)
		default:
			return fmt.Errorf("%s/%s has unsupported dataSource type %q", gvk.Kind, obj.GetName(), dataSource.Type)
		}
	} else {
		dv.SetBlankDataSource()
	}

	dv.SetPVC(pvcStorageClassName, pvcSize)

	dv.SetOwnerRef(obj, gvk)
	dv.AddFinalizer(virtv2.FinalizerDVProtection)
	return nil
}
