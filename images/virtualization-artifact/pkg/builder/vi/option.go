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

package vi

import (
	"github.com/deckhouse/virtualization-controller/pkg/builder/meta"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type Option func(vi *v1alpha2.VirtualImage)

var (
	WithName         = meta.WithName[*v1alpha2.VirtualImage]
	WithGenerateName = meta.WithGenerateName[*v1alpha2.VirtualImage]
	WithNamespace    = meta.WithNamespace[*v1alpha2.VirtualImage]
	WithLabel        = meta.WithLabel[*v1alpha2.VirtualImage]
	WithLabels       = meta.WithLabels[*v1alpha2.VirtualImage]
	WithAnnotation   = meta.WithAnnotation[*v1alpha2.VirtualImage]
	WithAnnotations  = meta.WithAnnotations[*v1alpha2.VirtualImage]
)

func WithPhase(phase v1alpha2.ImagePhase) func(vi *v1alpha2.VirtualImage) {
	return func(vi *v1alpha2.VirtualImage) {
		vi.Status.Phase = phase
	}
}

func WithCDROM(cdrom bool) func(vi *v1alpha2.VirtualImage) {
	return func(vi *v1alpha2.VirtualImage) {
		vi.Status.CDROM = cdrom
	}
}

func WithDataSourceHTTP(url string, checksum *v1alpha2.Checksum, caBundle []byte) Option {
	return func(vi *v1alpha2.VirtualImage) {
		vi.Spec.DataSource = v1alpha2.VirtualImageDataSource{
			Type: v1alpha2.DataSourceTypeHTTP,
			HTTP: &v1alpha2.DataSourceHTTP{
				URL:      url,
				Checksum: checksum,
				CABundle: caBundle,
			},
		}
	}
}

func WithDataSourceContainerImage(image string, imagePullSecret v1alpha2.ImagePullSecretName, caBundle []byte) Option {
	return func(vi *v1alpha2.VirtualImage) {
		vi.Spec.DataSource = v1alpha2.VirtualImageDataSource{
			Type: v1alpha2.DataSourceTypeContainerImage,
			ContainerImage: &v1alpha2.VirtualImageContainerImage{
				Image:           image,
				ImagePullSecret: imagePullSecret,
				CABundle:        caBundle,
			},
		}
	}
}

func WithDataSourceObjectRef(kind v1alpha2.VirtualImageObjectRefKind, name, namespace string) Option {
	return func(vi *v1alpha2.VirtualImage) {
		vi.Spec.DataSource = v1alpha2.VirtualImageDataSource{
			Type: v1alpha2.DataSourceTypeObjectRef,
			ObjectRef: &v1alpha2.VirtualImageObjectRef{
				Kind: kind,
				Name: name,
			},
		}
	}
}

func WithDatasource(datasource v1alpha2.VirtualImageDataSource) func(vi *v1alpha2.VirtualImage) {
	return func(vi *v1alpha2.VirtualImage) {
		vi.Spec.DataSource = datasource
	}
}

func WithStorage(storage v1alpha2.StorageType) func(vi *v1alpha2.VirtualImage) {
	return func(vi *v1alpha2.VirtualImage) {
		vi.Spec.Storage = storage
	}
}
