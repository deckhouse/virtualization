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

package internal

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/deckhouse/virtualization-controller/pkg/controller/supplements"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vd/internal/source"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

//go:generate moq -rm -out mock.go . Handler Sources DiskService StorageClassService

type Handler = source.Handler

type Sources interface {
	Changed(_ context.Context, vi *virtv2.VirtualDisk) bool
	Get(dsType virtv2.DataSourceType) (source.Handler, bool)
	CleanUp(ctx context.Context, vd *virtv2.VirtualDisk) (bool, error)
}

type DiskService interface {
	Resize(ctx context.Context, pvc *corev1.PersistentVolumeClaim, newSize resource.Quantity) error
	GetPersistentVolumeClaim(ctx context.Context, sup *supplements.Generator) (*corev1.PersistentVolumeClaim, error)
}

type StorageClassService interface {
	IsStorageClassAllowed(sc string) bool
	GetModuleStorageClass(ctx context.Context) (*storagev1.StorageClass, error)
	GetDefaultStorageClass(ctx context.Context) (*storagev1.StorageClass, error)
	GetStorageClass(ctx context.Context, sc string) (*storagev1.StorageClass, error)
	GetPersistentVolumeClaim(ctx context.Context, sup *supplements.Generator) (*corev1.PersistentVolumeClaim, error)
	IsStorageClassDeprecated(sc *storagev1.StorageClass) bool
}
