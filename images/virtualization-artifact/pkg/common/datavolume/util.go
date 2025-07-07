/*
Copyright 2024 Flant JSC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

     http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

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
