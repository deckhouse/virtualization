package cvmi

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	virtv2alpha1 "github.com/deckhouse/virtualization-controller/api/v2alpha1"
)

// MakeOwnerReference makes owner reference from a ClusterVirtualMachineImage.
func MakeOwnerReference(vmi *virtv2alpha1.VirtualMachineImage) metav1.OwnerReference {
	blockOwnerDeletion := true
	isController := true
	return metav1.OwnerReference{
		APIVersion:         virtv2alpha1.APIVersion,
		Kind:               virtv2alpha1.VMIKind,
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
