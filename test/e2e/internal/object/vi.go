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
)

func NewHTTPVIUbuntu(name, namespace string, opts ...vi.Option) *v1alpha2.VirtualImage {
	baseOpts := []vi.Option{
		vi.WithName(name),
		vi.WithStorage(v1alpha2.StorageContainerRegistry),
		vi.WithNamespace(namespace),
		vi.WithDataSourceHTTP(
			ImageURLUbuntu,
			nil,
			nil,
		),
		vi.WithStorage(v1alpha2.StorageContainerRegistry),
	}
	baseOpts = append(baseOpts, opts...)
	return vi.New(baseOpts...)
}

func NewGeneratedHTTPVIUbuntu(prefix, namespace string, opts ...vi.Option) *v1alpha2.VirtualImage {
	baseOpts := []vi.Option{
		vi.WithGenerateName(prefix),
		vi.WithNamespace(namespace),
		vi.WithDataSourceHTTP(
			ImageURLUbuntu,
			nil,
			nil,
		),
		vi.WithStorage(v1alpha2.StorageContainerRegistry),
	}
	baseOpts = append(baseOpts, opts...)
	return vi.New(baseOpts...)
}

func NewContainerImageVI(name, namespace string, opts ...vi.Option) *v1alpha2.VirtualImage {
	baseOpts := []vi.Option{
		vi.WithName(name),
		vi.WithNamespace(namespace),
		vi.WithStorage(v1alpha2.StorageContainerRegistry),
		vi.WithDataSourceContainerImage(ImageURLContainerImage, v1alpha2.ImagePullSecretName{}, nil),
	}
	baseOpts = append(baseOpts, opts...)
	return vi.New(baseOpts...)
}

func NewGeneratedContainerImageVI(prefix, namespace string, opts ...vi.Option) *v1alpha2.VirtualImage {
	baseOpts := []vi.Option{
		vi.WithGenerateName(prefix),
		vi.WithNamespace(namespace),
		vi.WithStorage(v1alpha2.StorageContainerRegistry),
		vi.WithDataSourceContainerImage(ImageURLContainerImage, v1alpha2.ImagePullSecretName{}, nil),
	}
	baseOpts = append(baseOpts, opts...)
	return vi.New(baseOpts...)
}
