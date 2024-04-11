package cvmi

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	virtv2alpha1 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

// MakeOwnerReference makes owner reference from a ClusterVirtualImage.
func MakeOwnerReference(cvmi *virtv2alpha1.ClusterVirtualImage) metav1.OwnerReference {
	blockOwnerDeletion := true
	isController := true
	return metav1.OwnerReference{
		APIVersion:         virtv2alpha1.ClusterVirtualImageGVK.GroupVersion().String(),
		Kind:               virtv2alpha1.ClusterVirtualImageGVK.Kind,
		Name:               cvmi.Name,
		UID:                cvmi.GetUID(),
		BlockOwnerDeletion: &blockOwnerDeletion,
		Controller:         &isController,
	}
}

func IsDVCRSource(cvmi *virtv2alpha1.ClusterVirtualImage) bool {
	return cvmi != nil && cvmi.Spec.DataSource.Type == virtv2alpha1.DataSourceTypeObjectRef
}
