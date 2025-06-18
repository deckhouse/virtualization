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

	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

func IndexVDByVDSnapshot() (obj client.Object, field string, extractValue client.IndexerFunc) {
	return &virtv2.VirtualDisk{}, IndexFieldVDByVDSnapshot, func(object client.Object) []string {
		vd, ok := object.(*virtv2.VirtualDisk)
		if !ok || vd == nil {
			return nil
		}

		if vd.Spec.DataSource == nil || vd.Spec.DataSource.Type != virtv2.DataSourceTypeObjectRef {
			return nil
		}

		if vd.Spec.DataSource.ObjectRef == nil || vd.Spec.DataSource.ObjectRef.Kind != virtv2.VirtualDiskObjectRefKindVirtualDiskSnapshot {
			return nil
		}

		return []string{vd.Spec.DataSource.ObjectRef.Name}
	}
}

func IndexVDByStorageClass() (obj client.Object, field string, extractValue client.IndexerFunc) {
	return &virtv2.VirtualDisk{}, IndexFieldVDByStorageClass, func(object client.Object) []string {
		vd, ok := object.(*virtv2.VirtualDisk)
		if !ok || vd == nil {
			return nil
		}

		switch {
		case vd.Status.StorageClassName != "":
			return []string{vd.Status.StorageClassName}
		case vd.Spec.PersistentVolumeClaim.StorageClass != nil:
			return []string{*vd.Spec.PersistentVolumeClaim.StorageClass}
		default:
			return []string{DefaultStorageClass}
		}
	}
}
