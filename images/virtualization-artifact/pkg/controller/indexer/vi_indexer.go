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

package indexer

import (
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

func IndexVIByVDSnapshot() (obj client.Object, field string, extractValue client.IndexerFunc) {
	return &v1alpha2.VirtualImage{}, IndexFieldVIByVDSnapshot, func(object client.Object) []string {
		vi, ok := object.(*v1alpha2.VirtualImage)
		if !ok || vi == nil {
			return nil
		}

		if vi.Spec.DataSource.Type != v1alpha2.DataSourceTypeObjectRef {
			return nil
		}

		if vi.Spec.DataSource.ObjectRef == nil || vi.Spec.DataSource.ObjectRef.Kind != v1alpha2.VirtualImageObjectRefKindVirtualDiskSnapshot {
			return nil
		}

		return []string{vi.Spec.DataSource.ObjectRef.Name}
	}
}

func IndexVIByStorageClass() (obj client.Object, field string, extractValue client.IndexerFunc) {
	return &v1alpha2.VirtualImage{}, IndexFieldVIByStorageClass, func(object client.Object) []string {
		vi, ok := object.(*v1alpha2.VirtualImage)
		if !ok || vi == nil {
			return nil
		}

		switch {
		case vi.Status.StorageClassName != "":
			return []string{vi.Status.StorageClassName}
		case vi.Spec.PersistentVolumeClaim.StorageClass != nil:
			return []string{*vi.Spec.PersistentVolumeClaim.StorageClass}
		default:
			return []string{DefaultStorageClass}
		}
	}
}

func IndexVIByPhaseAndStorage() (obj client.Object, field string, extractValue client.IndexerFunc) {
	return &v1alpha2.VirtualImage{}, IndexFieldVIByPhaseAndStorage, func(object client.Object) []string {
		vi, ok := object.(*v1alpha2.VirtualImage)
		if !ok || vi == nil {
			return nil
		}

		if vi.Status.Phase == v1alpha2.ImageReady && vi.Spec.Storage == v1alpha2.StorageContainerRegistry {
			return []string{ReadyDVCRImage}
		}

		return nil
	}
}
