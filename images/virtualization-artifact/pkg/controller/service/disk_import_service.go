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

package service

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// PVCImportSource describes where a target PersistentVolumeClaim should pull
// its data from when populated by PersistentVolumeClaimService.Import. Either
// Registry or PVC may be set; both nil is valid for blank PVCs that do not
// need any pre-population.
type PVCImportSource struct {
	Registry *PVCImportSourceRegistry
	PVC      *PVCImportSourcePVC
}

// PVCImportSourceRegistry points at a DVCR registry image populated by an
// upstream uploader/importer pod.
type PVCImportSourceRegistry struct {
	URL           string
	Secret        string
	CertConfigMap string
}

// PVCImportSourcePVC points at another PersistentVolumeClaim used as the
// clone source.
type PVCImportSourcePVC struct {
	Name      string
	Namespace string
}

// NewPVCRegistryImportSource builds a PVCImportSource that points at a DVCR
// registry image (used by Upload, HTTP, Registry and ObjectRef CVI/VI data
// sources).
func NewPVCRegistryImportSource(url, secret, certConfigMap string) *PVCImportSource {
	return &PVCImportSource{
		Registry: &PVCImportSourceRegistry{
			URL:           url,
			Secret:        secret,
			CertConfigMap: certConfigMap,
		},
	}
}

// NewPVCPVCImportSource builds a PVCImportSource that points at another PVC
// (used when cloning from a VirtualDisk).
func NewPVCPVCImportSource(name, namespace string) *PVCImportSource {
	return &PVCImportSource{
		PVC: &PVCImportSourcePVC{
			Name:      name,
			Namespace: namespace,
		},
	}
}

// ownerReferenceForObject builds a controller OwnerReference pointing at
// owner. It is used by services that create child resources (PVCs, snapshots)
// owned by a VirtualDisk or VirtualImage.
func ownerReferenceForObject(obj client.Object) metav1.OwnerReference {
	gvk := obj.GetObjectKind().GroupVersionKind()
	return metav1.OwnerReference{
		APIVersion:         gvk.GroupVersion().String(),
		Kind:               gvk.Kind,
		Name:               obj.GetName(),
		UID:                obj.GetUID(),
		Controller:         ptr.To(true),
		BlockOwnerDeletion: ptr.To(true),
	}
}
