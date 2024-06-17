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
