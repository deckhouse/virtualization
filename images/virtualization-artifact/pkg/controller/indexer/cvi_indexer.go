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
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

func IndexCVIByVDSnapshot() (obj client.Object, field string, extractValue client.IndexerFunc) {
	return &v1alpha2.ClusterVirtualImage{}, IndexFieldCVIByVDSnapshot, func(object client.Object) []string {
		cvi, ok := object.(*v1alpha2.ClusterVirtualImage)
		if !ok || cvi == nil {
			return nil
		}

		if cvi.Spec.DataSource.Type != v1alpha2.DataSourceTypeObjectRef {
			return nil
		}

		if cvi.Spec.DataSource.ObjectRef == nil || cvi.Spec.DataSource.ObjectRef.Kind != v1alpha2.ClusterVirtualImageObjectRefKindVirtualDiskSnapshot {
			return nil
		}

		key := types.NamespacedName{
			Namespace: cvi.Spec.DataSource.ObjectRef.Namespace,
			Name:      cvi.Spec.DataSource.ObjectRef.Name,
		}

		return []string{key.String()}
	}
}

func IndexCVIByReadyPhase() (obj client.Object, field string, extractValue client.IndexerFunc) {
	return &v1alpha2.ClusterVirtualImage{}, IndexFieldCVIByPhase, func(object client.Object) []string {
		cvi, ok := object.(*v1alpha2.ClusterVirtualImage)
		if !ok || cvi == nil {
			return nil
		}

		if cvi.Status.Phase == v1alpha2.ImageReady {
			return []string{ReadyDVCRImage}
		}

		return nil
	}
}
