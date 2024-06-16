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

package vmi

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	virtv2alpha1 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

// MakeOwnerReference makes owner reference from a ClusterVirtualImage.
func MakeOwnerReference(vmi *virtv2alpha1.VirtualImage) metav1.OwnerReference {
	blockOwnerDeletion := true
	isController := true
	return metav1.OwnerReference{
		APIVersion:         virtv2alpha1.VirtualImageGVK.GroupVersion().String(),
		Kind:               virtv2alpha1.VirtualImageGVK.Kind,
		Name:               vmi.Name,
		UID:                vmi.GetUID(),
		BlockOwnerDeletion: &blockOwnerDeletion,
		Controller:         &isController,
	}
}

func GetDataSourceType(vmi *virtv2alpha1.VirtualImage) string {
	if vmi == nil {
		return ""
	}
	return string(vmi.Spec.DataSource.Type)
}

func IsDVCRSource(vmi *virtv2alpha1.VirtualImage) bool {
	return vmi != nil && vmi.Spec.DataSource.Type == virtv2alpha1.DataSourceTypeObjectRef
}

// IsTwoPhaseImport returns true when two phase import is required:
// 1. Import from dataSource to DVCR image using dvcr-importer or dvcr-uploader.
// 2. Import DVCR image to PVC using DataVolume.
func IsTwoPhaseImport(vmi *virtv2alpha1.VirtualImage) bool {
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
