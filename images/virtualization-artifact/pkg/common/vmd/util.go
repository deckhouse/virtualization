package vmd

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	virtv2alpha1 "github.com/deckhouse/virtualization-controller/api/v1alpha2"
)

// MakeOwnerReference makes owner reference from a ClusterVirtualMachineImage.
func MakeOwnerReference(vmd *virtv2alpha1.VirtualMachineDisk) metav1.OwnerReference {
	return *metav1.NewControllerRef(vmd, schema.GroupVersionKind{
		Group:   virtv2alpha1.APIGroup,
		Version: virtv2alpha1.APIVersion,
		Kind:    virtv2alpha1.VMDKind,
	})
}

func GetDataSourceType(vmd *virtv2alpha1.VirtualMachineDisk) string {
	if vmd == nil || vmd.Spec.DataSource == nil {
		return ""
	}
	return string(vmd.Spec.DataSource.Type)
}

func IsDVCRSource(vmd *virtv2alpha1.VirtualMachineDisk) bool {
	if vmd == nil || vmd.Spec.DataSource == nil {
		return false
	}
	switch vmd.Spec.DataSource.Type {
	case virtv2alpha1.DataSourceTypeClusterVirtualMachineImage,
		virtv2alpha1.DataSourceTypeVirtualMachineImage:
		return true
	}
	return false
}

func IsTwoPhaseImport(vmd *virtv2alpha1.VirtualMachineDisk) bool {
	if vmd == nil || vmd.Spec.DataSource == nil {
		return false
	}
	switch vmd.Spec.DataSource.Type {
	case virtv2alpha1.DataSourceTypeHTTP,
		virtv2alpha1.DataSourceTypeUpload,
		virtv2alpha1.DataSourceTypeContainerImage:
		return true
	}
	return false
}

// IsBlankPVC returns true if VMD has no DataSource: only PVC should be created.
func IsBlankPVC(vmd *virtv2alpha1.VirtualMachineDisk) bool {
	if vmd == nil {
		return false
	}
	return vmd.Spec.DataSource == nil
}
