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

package internal

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	storev1 "k8s.io/api/storage/v1"
	cdiv1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"

	"github.com/deckhouse/virtualization-controller/pkg/controller/supplements"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vi/internal/source"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

//go:generate moq -rm -out mock.go . DiskService Sources

type Sources interface {
	Changed(ctx context.Context, vi *virtv2.VirtualImage) bool
	For(dsType virtv2.DataSourceType) (source.Handler, bool)
	CleanUp(ctx context.Context, vd *virtv2.VirtualImage) (bool, error)
}

type DiskService interface {
	GetStorageClass(ctx context.Context, storageClassName *string) (*storev1.StorageClass, error)
	GetStorageProfile(ctx context.Context, name string) (*cdiv1.StorageProfile, error)
	GetPersistentVolumeClaim(ctx context.Context, sup *supplements.Generator) (*corev1.PersistentVolumeClaim, error)
}
