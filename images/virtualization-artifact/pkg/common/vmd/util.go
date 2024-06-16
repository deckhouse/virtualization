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

package vmd

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	virtvcore "github.com/deckhouse/virtualization/api/core"
	virtv2alpha1 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

// MakeOwnerReference makes owner reference from a ClusterVirtualImage.
func MakeOwnerReference(vmd *virtv2alpha1.VirtualDisk) metav1.OwnerReference {
	return *metav1.NewControllerRef(vmd, schema.GroupVersionKind{
		Group:   virtvcore.GroupName,
		Version: virtv2alpha1.Version,
		Kind:    virtv2alpha1.VirtualDiskKind,
	})
}

func GetDataSourceType(vmd *virtv2alpha1.VirtualDisk) string {
	if vmd == nil || vmd.Spec.DataSource == nil {
		return ""
	}
	return string(vmd.Spec.DataSource.Type)
}

func IsDVCRSource(vmd *virtv2alpha1.VirtualDisk) bool {
	if vmd == nil || vmd.Spec.DataSource == nil {
		return false
	}

	return vmd.Spec.DataSource.Type == virtv2alpha1.DataSourceTypeObjectRef
}

func IsTwoPhaseImport(vmd *virtv2alpha1.VirtualDisk) bool {
	if vmd == nil || vmd.Spec.DataSource == nil {
		return false
	}
	switch vmd.Spec.DataSource.Type {
	case virtv2alpha1.DataSourceTypeHTTP,
		virtv2alpha1.DataSourceTypeUpload,
		virtv2alpha1.DataSourceTypeContainerImage:
		return true
	}
	return false
}

// IsBlankPVC returns true if VMD has no DataSource: only PVC should be created.
func IsBlankPVC(vmd *virtv2alpha1.VirtualDisk) bool {
	if vmd == nil {
		return false
	}
	return vmd.Spec.DataSource == nil
}
