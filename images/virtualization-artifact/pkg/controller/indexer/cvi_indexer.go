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

package indexer

import (
	"context"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

func IndexCVIByVDSnapshot(ctx context.Context, mgr manager.Manager) error {
	return mgr.GetFieldIndexer().IndexField(ctx, &virtv2.ClusterVirtualImage{}, IndexFieldCVIByVDSnapshot, func(object client.Object) []string {
		cvi, ok := object.(*virtv2.ClusterVirtualImage)
		if !ok || cvi == nil {
			return nil
		}

		if cvi.Spec.DataSource.Type != virtv2.DataSourceTypeObjectRef {
			return nil
		}

		if cvi.Spec.DataSource.ObjectRef == nil || cvi.Spec.DataSource.ObjectRef.Kind != virtv2.ClusterVirtualImageObjectRefKindVirtualDiskSnapshot {
			return nil
		}

		key := types.NamespacedName{
			Namespace: cvi.Spec.DataSource.ObjectRef.Namespace,
			Name:      cvi.Spec.DataSource.ObjectRef.Name,
		}

		return []string{key.String()}
	})
}
