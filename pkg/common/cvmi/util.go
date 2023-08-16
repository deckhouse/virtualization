package cvmi

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	virtv2alpha1 "github.com/deckhouse/virtualization-controller/api/v2alpha1"
)

// MakeOwnerReference makes owner reference from a ClusterVirtualMachineImage.
func MakeOwnerReference(cvmi *virtv2alpha1.ClusterVirtualMachineImage) metav1.OwnerReference {
	blockOwnerDeletion := true
	isController := true
	return metav1.OwnerReference{
		APIVersion:         virtv2alpha1.APIVersion,
		Kind:               virtv2alpha1.CVMIKind,
		Name:               cvmi.Name,
		UID:                cvmi.GetUID(),
		BlockOwnerDeletion: &blockOwnerDeletion,
		Controller:         &isController,
	}
}

func HasCABundle(cvmi *virtv2alpha1.ClusterVirtualMachineImage) bool {
	if cvmi != nil &&
		cvmi.Spec.DataSource.Type == virtv2alpha1.DataSourceTypeHTTP &&
		cvmi.Spec.DataSource.HTTP != nil {
		return len(cvmi.Spec.DataSource.HTTP.CABundle) > 0
	}
	return false
}

func GetCABundle(cvmi *virtv2alpha1.ClusterVirtualMachineImage) string {
	if HasCABundle(cvmi) {
		return string(cvmi.Spec.DataSource.HTTP.CABundle)
	}
	return ""
}
