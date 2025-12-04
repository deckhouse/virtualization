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

package service

import (
	"context"
	"sort"

	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/types"
	cdiv1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	"github.com/deckhouse/virtualization-controller/pkg/controller/supplements"
)

type BaseStorageClassService struct {
	client client.Client
}

func NewBaseStorageClassService(client client.Client) *BaseStorageClassService {
	return &BaseStorageClassService{client: client}
}

func (s BaseStorageClassService) GetDefaultStorageClass(ctx context.Context) (*storagev1.StorageClass, error) {
	var scs storagev1.StorageClassList
	err := s.client.List(ctx, &scs, &client.ListOptions{})
	if err != nil {
		return nil, err
	}

	var defaultClasses []*storagev1.StorageClass
	for idx := range scs.Items {
		if scs.Items[idx].Annotations[annotations.AnnDefaultStorageClass] == "true" {
			defaultClasses = append(defaultClasses, &scs.Items[idx])
		}
	}

	if len(defaultClasses) == 0 {
		return nil, ErrDefaultStorageClassNotFound
	}

	// Primary sort by creation timestamp, newest first.
	// Secondary sort by class name, ascending order.
	sort.Slice(defaultClasses, func(i, j int) bool {
		if defaultClasses[i].CreationTimestamp.UnixNano() == defaultClasses[j].CreationTimestamp.UnixNano() {
			return defaultClasses[i].Name < defaultClasses[j].Name
		}
		return defaultClasses[i].CreationTimestamp.UnixNano() > defaultClasses[j].CreationTimestamp.UnixNano()
	})

	return defaultClasses[0], nil
}

func (s BaseStorageClassService) GetStorageClass(ctx context.Context, scName string) (*storagev1.StorageClass, error) {
	return object.FetchObject(ctx, types.NamespacedName{Name: scName}, s.client, &storagev1.StorageClass{})
}

func (s BaseStorageClassService) GetPersistentVolumeClaim(ctx context.Context, sup supplements.Generator) (*corev1.PersistentVolumeClaim, error) {
	return supplements.FetchSupplement(ctx, s.client, sup, supplements.SupplementPVC, &corev1.PersistentVolumeClaim{})
}

func (s BaseStorageClassService) IsStorageClassDeprecated(sc *storagev1.StorageClass) bool {
	return sc != nil && sc.Labels["module"] == "local-path-provisioner"
}

func (s BaseStorageClassService) GetStorageProfile(ctx context.Context, name string) (*cdiv1.StorageProfile, error) {
	return object.FetchObject(ctx, types.NamespacedName{Name: name}, s.client, &cdiv1.StorageProfile{})
}
