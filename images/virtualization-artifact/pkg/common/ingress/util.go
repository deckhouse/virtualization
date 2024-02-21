package ingress

import (
	netv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// MakeOwnerReference makes owner reference from Ingress
func MakeOwnerReference(ing *netv1.Ingress) metav1.OwnerReference {
	blockOwnerDeletion := true
	isController := true
	return metav1.OwnerReference{
		APIVersion:         netv1.SchemeGroupVersion.String(),
		Kind:               "Ingress",
		Name:               ing.Name,
		UID:                ing.GetUID(),
		BlockOwnerDeletion: &blockOwnerDeletion,
		Controller:         &isController,
	}
}
