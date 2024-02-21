package datavolume

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	cdiv1beta1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"
)

// MakeOwnerReference makes controller owner reference for a DataVolume object.
// NOTE: GetObjectKind resets after creation, hence this method with hardcoded
// GVK as a workaround.
func MakeOwnerReference(dv *cdiv1beta1.DataVolume) metav1.OwnerReference {
	gvk := schema.GroupVersionKind{
		Group:   cdiv1beta1.SchemeGroupVersion.Group,
		Version: cdiv1beta1.SchemeGroupVersion.Version,
		Kind:    "DataVolume",
	}
	return *metav1.NewControllerRef(dv, gvk)
}
