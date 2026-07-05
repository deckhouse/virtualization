/*
Copyright 2025 Flant JSC

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

package rewrite

import (
	"k8s.io/apimachinery/pkg/runtime/schema"
	virtv1 "kubevirt.io/api/core/v1"

	storagev1alpha1 "github.com/deckhouse/virtualization-controller/pkg/apis/storage/v1alpha1"
)

func rewriteVirtualizationV1(resource string) schema.GroupVersionResource {
	return schema.GroupVersionResource{
		Group:    "internal.virtualization.deckhouse.io",
		Version:  "v1",
		Resource: resource,
	}
}

func rewriteInternalVirtualizationResource(resource string) string {
	return "internalvirtualization" + resource
}

// StorageProfile is the module-owned storageprofiles.storage.virtualization.deckhouse.io
// resource maintained by virtualization-controller instead of CDI.
type StorageProfile struct {
	*storagev1alpha1.StorageProfile `json:",inline"`
}

func (StorageProfile) GVR() schema.GroupVersionResource {
	return schema.GroupVersionResource{
		Group:    "storage.virtualization.deckhouse.io",
		Version:  "v1alpha1",
		Resource: "storageprofiles",
	}
}

type VirtualMachineInstanceMigration struct {
	*virtv1.VirtualMachineInstanceMigration
}

func (VirtualMachineInstanceMigration) GVR() schema.GroupVersionResource {
	resource := rewriteInternalVirtualizationResource("virtualmachineinstancemigrations")
	return rewriteVirtualizationV1(resource)
}

type VirtualMachineInstance struct {
	*virtv1.VirtualMachineInstance
}

func (VirtualMachineInstance) GVR() schema.GroupVersionResource {
	resource := rewriteInternalVirtualizationResource("virtualmachineinstances")
	return rewriteVirtualizationV1(resource)
}
