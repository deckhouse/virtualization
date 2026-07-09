/*
Copyright 2026 Flant JSC

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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	cdiv1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"
)

const StorageProfileKind = "StorageProfile"

// StorageProfile provides storage capability recommendations for a StorageClass.
// It replaces storageprofiles.cdi.kubevirt.io: the module maintains these
// objects itself and CDI is not involved.
//
// +genclient
// +genclient:nonNamespaced
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type StorageProfile struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   StorageProfileSpec   `json:"spec"`
	Status StorageProfileStatus `json:"status,omitempty"`
}

// StorageProfileSpec defines specification for StorageProfile.
// Nested property types are reused from the CDI API package: the shapes are
// identical and consumers already operate on them.
type StorageProfileSpec struct {
	// CloneStrategy defines the preferred method for cloning a volume.
	CloneStrategy *cdiv1.CDICloneStrategy `json:"cloneStrategy,omitempty"`
	// ClaimPropertySets is a provided set of properties applicable to PVC.
	// +kubebuilder:validation:MaxItems=8
	ClaimPropertySets []cdiv1.ClaimPropertySet `json:"claimPropertySets,omitempty"`
	// DataImportCronSourceFormat defines the format of the DataImportCron-created disk image sources.
	DataImportCronSourceFormat *cdiv1.DataImportCronSourceFormat `json:"dataImportCronSourceFormat,omitempty"`
	// SnapshotClass is optional specific VolumeSnapshotClass for CloneStrategySnapshot.
	// If not set, a VolumeSnapshotClass is chosen according to the provisioner.
	SnapshotClass *string `json:"snapshotClass,omitempty"`
}

// StorageProfileStatus provides the most recently observed status of the StorageProfile.
type StorageProfileStatus struct {
	// The StorageClass name for which capabilities are defined.
	StorageClass *string `json:"storageClass,omitempty"`
	// The StorageClass provisioner plugin name.
	Provisioner *string `json:"provisioner,omitempty"`
	// CloneStrategy defines the preferred method for cloning a volume.
	CloneStrategy *cdiv1.CDICloneStrategy `json:"cloneStrategy,omitempty"`
	// ClaimPropertySets computed from the spec and detected in the system.
	// +kubebuilder:validation:MaxItems=8
	ClaimPropertySets []cdiv1.ClaimPropertySet `json:"claimPropertySets,omitempty"`
	// DataImportCronSourceFormat defines the format of the DataImportCron-created disk image sources.
	DataImportCronSourceFormat *cdiv1.DataImportCronSourceFormat `json:"dataImportCronSourceFormat,omitempty"`
	// SnapshotClass is optional specific VolumeSnapshotClass for CloneStrategySnapshot.
	// If not set, a VolumeSnapshotClass is chosen according to the provisioner.
	SnapshotClass *string `json:"snapshotClass,omitempty"`
}

// StorageProfileList provides the needed parameters to request a list of StorageProfiles.
//
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type StorageProfileList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []StorageProfile `json:"items"`
}
