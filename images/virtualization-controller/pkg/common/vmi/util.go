package vmi

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	virtv2alpha1 "github.com/deckhouse/virtualization-controller/api/v2alpha1"
)

// MakeOwnerReference makes owner reference from a ClusterVirtualMachineImage.
func MakeOwnerReference(vmi *virtv2alpha1.VirtualMachineImage) metav1.OwnerReference {
	blockOwnerDeletion := true
	isController := true
	return metav1.OwnerReference{
		APIVersion:         virtv2alpha1.VirtualMachineImageGVK.GroupVersion().String(),
		Kind:               virtv2alpha1.VirtualMachineImageGVK.Kind,
		Name:               vmi.Name,
		UID:                vmi.GetUID(),
		BlockOwnerDeletion: &blockOwnerDeletion,
		Controller:         &isController,
	}
}

func HasCABundle(vmi *virtv2alpha1.VirtualMachineImage) bool {
	if vmi != nil &&
		vmi.Spec.DataSource.Type == virtv2alpha1.DataSourceTypeHTTP &&
		vmi.Spec.DataSource.HTTP != nil {
		return len(vmi.Spec.DataSource.HTTP.CABundle) > 0
	}
	return false
}

func GetCABundle(vmi *virtv2alpha1.VirtualMachineImage) string {
	if HasCABundle(vmi) {
		return string(vmi.Spec.DataSource.HTTP.CABundle)
	}
	return ""
}

func GetDataSourceType(vmi *virtv2alpha1.VirtualMachineImage) string {
	if vmi == nil {
		return ""
	}
	return string(vmi.Spec.DataSource.Type)
}

// IsTwoPhaseImport returns true when two phase import is required:
// 1. Import from dataSource to DVCR image using dvcr-importer or dvcr-uploader.
// 2. Import DVCR image to PVC using DataVolume.
func IsTwoPhaseImport(vmi *virtv2alpha1.VirtualMachineImage) bool {
	if vmi == nil {
		return false
	}

	switch vmi.Spec.DataSource.Type {
	case virtv2alpha1.DataSourceTypeHTTP,
		virtv2alpha1.DataSourceTypeUpload,
		virtv2alpha1.DataSourceTypeContainerImage:
		return vmi.Spec.Storage == virtv2alpha1.StorageKubernetes
	}

	return false
}
