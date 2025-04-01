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
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vicondition"
)

func IndexVIByVDSnapshot(ctx context.Context, mgr manager.Manager) error {
	return mgr.GetFieldIndexer().IndexField(ctx, &virtv2.VirtualImage{}, IndexFieldVIByVDSnapshot, func(object client.Object) []string {
		vi, ok := object.(*virtv2.VirtualImage)
		if !ok || vi == nil {
			return nil
		}

		if vi.Spec.DataSource.Type != virtv2.DataSourceTypeObjectRef {
			return nil
		}

		if vi.Spec.DataSource.ObjectRef == nil || vi.Spec.DataSource.ObjectRef.Kind != virtv2.VirtualImageObjectRefKindVirtualDiskSnapshot {
			return nil
		}

		return []string{vi.Spec.DataSource.ObjectRef.Name}
	})
}

func IndexVIByStorageClass(ctx context.Context, mgr manager.Manager) error {
	return mgr.GetFieldIndexer().IndexField(ctx, &virtv2.VirtualImage{}, IndexFieldVIByStorageClass, func(object client.Object) []string {
		vi, ok := object.(*virtv2.VirtualImage)
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
	})
}

func IndexVIByNotReadyStorageClass(ctx context.Context, mgr manager.Manager) error {
	return mgr.GetFieldIndexer().IndexField(ctx, &virtv2.VirtualImage{}, IndexFieldVIByNotReadyStorageClass, func(object client.Object) []string {
		vi, ok := object.(*virtv2.VirtualImage)
		if !ok || vi == nil {
			return nil
		}

		for _, condition := range vi.Status.Conditions {
			if condition.Type == string(vicondition.StorageClassReadyType) && condition.Status == metav1.ConditionTrue {
				return []string{condition.Type}
			}
		}

		return nil
	})
}
