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

package object

import (
	"github.com/deckhouse/virtualization-controller/pkg/builder/vi"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/test/e2e/internal/config"
)

// NewVI builds a VirtualImage and, when it is backed by a PersistentVolumeClaim,
// pins it to the same StorageClass as the disks: see NewVD.
func NewVI(opts ...vi.Option) *v1alpha2.VirtualImage {
	image := vi.New(opts...)
	OverrideImageStorageClass(image)
	return image
}

// OverrideImageStorageClass sets the STORAGE_CLASS_NAME StorageClass on a
// PVC-backed VirtualImage that has none.
func OverrideImageStorageClass(image *v1alpha2.VirtualImage) {
	if image == nil || image.Spec.Storage != v1alpha2.StoragePersistentVolumeClaim {
		return
	}
	if sc := image.Spec.PersistentVolumeClaim.StorageClass; sc != nil && *sc != "" {
		return
	}

	if !config.IsStorageClassNameOverridden() {
		return
	}
	scName := config.StorageClassNameOverride()
	image.Spec.PersistentVolumeClaim.StorageClass = &scName
}

func NewGeneratedHTTPVIAlpineBIOS(prefix, namespace string, opts ...vi.Option) *v1alpha2.VirtualImage {
	baseOpts := []vi.Option{
		vi.WithGenerateName(prefix),
		vi.WithNamespace(namespace),
		vi.WithDataSourceHTTP(
			ImageURLAlpineBIOS, nil, nil,
		),
		vi.WithStorage(v1alpha2.StorageContainerRegistry),
	}
	baseOpts = append(baseOpts, opts...)
	return NewVI(baseOpts...)
}

// NewGeneratedHTTPVICustomBIOS builds a VirtualImage sourced over HTTP from the
// custom image. Used by the blockdevice suite; the AlpineBIOS variant is
// left for the other suites that rely on it.
func NewGeneratedHTTPVICustomBIOS(prefix, namespace string, opts ...vi.Option) *v1alpha2.VirtualImage {
	baseOpts := []vi.Option{
		vi.WithGenerateName(prefix),
		vi.WithNamespace(namespace),
		vi.WithDataSourceHTTP(
			ImageURLCustomBIOS, nil, nil,
		),
		vi.WithStorage(v1alpha2.StorageContainerRegistry),
	}
	baseOpts = append(baseOpts, opts...)
	return NewVI(baseOpts...)
}

func NewGeneratedHTTPVIAlpineBIOSPerf(prefix, namespace string, opts ...vi.Option) *v1alpha2.VirtualImage {
	baseOpts := []vi.Option{
		vi.WithGenerateName(prefix),
		vi.WithNamespace(namespace),
		vi.WithDataSourceHTTP(
			ImageURLAlpineBIOSPerf, nil, nil,
		),
		vi.WithStorage(v1alpha2.StorageContainerRegistry),
	}
	baseOpts = append(baseOpts, opts...)
	return NewVI(baseOpts...)
}

func NewGeneratedContainerImageVI(prefix, namespace string, opts ...vi.Option) *v1alpha2.VirtualImage {
	baseOpts := []vi.Option{
		vi.WithGenerateName(prefix),
		vi.WithNamespace(namespace),
		vi.WithStorage(v1alpha2.StorageContainerRegistry),
		vi.WithDataSourceContainerImage(ImageURLContainerImage, v1alpha2.ImagePullSecretName{}, nil),
	}
	baseOpts = append(baseOpts, opts...)
	return NewVI(baseOpts...)
}

func NewGeneratedVIFromCVI(prefix, namespace, cviName string, opts ...vi.Option) *v1alpha2.VirtualImage {
	baseOpts := []vi.Option{
		vi.WithGenerateName(prefix),
		vi.WithNamespace(namespace),
		vi.WithStorage(v1alpha2.StorageContainerRegistry),
		vi.WithDataSourceObjectRef(v1alpha2.VirtualImageObjectRefKindClusterVirtualImage, cviName),
	}
	baseOpts = append(baseOpts, opts...)
	return NewVI(baseOpts...)
}
