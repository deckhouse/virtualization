package cvmi

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	virtv2alpha1 "github.com/deckhouse/virtualization-controller/api/v1alpha2"
)

// MakeOwnerReference makes owner reference from a ClusterVirtualMachineImage.
func MakeOwnerReference(cvmi *virtv2alpha1.ClusterVirtualMachineImage) metav1.OwnerReference {
	blockOwnerDeletion := true
	isController := true
	return metav1.OwnerReference{
		APIVersion:         virtv2alpha1.ClusterVirtualMachineImageGVK.GroupVersion().String(),
		Kind:               virtv2alpha1.ClusterVirtualMachineImageGVK.Kind,
		Name:               cvmi.Name,
		UID:                cvmi.GetUID(),
		BlockOwnerDeletion: &blockOwnerDeletion,
		Controller:         &isController,
	}
}

func IsDVCRSource(cvmi *virtv2alpha1.ClusterVirtualMachineImage) bool {
	if cvmi == nil {
		return false
	}
	switch cvmi.Spec.DataSource.Type {
	case virtv2alpha1.DataSourceTypeClusterVirtualMachineImage,
		virtv2alpha1.DataSourceTypeVirtualMachineImage:
		return true
	}
	return false
}
