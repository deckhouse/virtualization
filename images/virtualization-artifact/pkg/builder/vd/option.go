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

package vd

import (
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/utils/ptr"

	"github.com/deckhouse/virtualization-controller/pkg/builder/meta"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type Option func(vd *v1alpha2.VirtualDisk)

var (
	WithName         = meta.WithName[*v1alpha2.VirtualDisk]
	WithNamespace    = meta.WithNamespace[*v1alpha2.VirtualDisk]
	WithGenerateName = meta.WithGenerateName[*v1alpha2.VirtualDisk]
	WithLabel        = meta.WithLabel[*v1alpha2.VirtualDisk]
	WithLabels       = meta.WithLabels[*v1alpha2.VirtualDisk]
	WithAnnotation   = meta.WithAnnotation[*v1alpha2.VirtualDisk]
	WithAnnotations  = meta.WithAnnotations[*v1alpha2.VirtualDisk]
	WithFinalizer    = meta.WithFinalizer[*v1alpha2.VirtualDisk]
)

func WithDatasource(datasource *v1alpha2.VirtualDiskDataSource) func(vd *v1alpha2.VirtualDisk) {
	return func(vd *v1alpha2.VirtualDisk) {
		vd.Spec.DataSource = datasource
	}
}
func WithDataSourceHTTP(url string, checksum *v1alpha2.Checksum, caBundle []byte) Option {
	return func(vd *v1alpha2.VirtualDisk) {
		vd.Spec.DataSource = &v1alpha2.VirtualDiskDataSource{
			Type: v1alpha2.DataSourceTypeHTTP,
			HTTP: &v1alpha2.DataSourceHTTP{
				URL:      url,
				Checksum: checksum,
				CABundle: caBundle,
			},
		}
	}
}

func WithDataSourceContainerImage(image, imagePullSecretName string, caBundle []byte) Option {
	return func(vd *v1alpha2.VirtualDisk) {
		vd.Spec.DataSource = &v1alpha2.VirtualDiskDataSource{
			Type: v1alpha2.DataSourceTypeContainerImage,
			ContainerImage: &v1alpha2.VirtualDiskContainerImage{
				Image: image,
				ImagePullSecret: v1alpha2.ImagePullSecretName{
					Name: imagePullSecretName,
				},
				CABundle: caBundle,
			},
		}
	}
}

func WithDataSourceObjectRef(kind v1alpha2.VirtualDiskObjectRefKind, name string) Option {
	return func(vd *v1alpha2.VirtualDisk) {
		vd.Spec.DataSource = &v1alpha2.VirtualDiskDataSource{
			Type: v1alpha2.DataSourceTypeObjectRef,
			ObjectRef: &v1alpha2.VirtualDiskObjectRef{
				Kind: kind,
				Name: name,
			},
		}
	}
}

func WithDataSourceObjectRefFromCVI(cvi *v1alpha2.ClusterVirtualImage) Option {
	return WithDataSourceObjectRef(v1alpha2.VirtualDiskObjectRefKindClusterVirtualImage, cvi.Name)
}

func WithDataSourceObjectRefFromVI(vi *v1alpha2.VirtualImage) Option {
	return WithDataSourceObjectRef(v1alpha2.VirtualDiskObjectRefKindVirtualImage, vi.Name)
}

func WithPersistentVolumeClaim(storageClass *string, size *resource.Quantity) Option {
	return func(vd *v1alpha2.VirtualDisk) {
		vd.Spec.PersistentVolumeClaim = v1alpha2.VirtualDiskPersistentVolumeClaim{
			StorageClass: storageClass,
			Size:         size,
		}
	}
}

func WithStorageClass(storageClass string) Option {
	return func(vd *v1alpha2.VirtualDisk) {
		vd.Spec.PersistentVolumeClaim.StorageClass = ptr.To(storageClass)
	}
}

func WithSize(size *resource.Quantity) Option {
	return func(vd *v1alpha2.VirtualDisk) {
		vd.Spec.PersistentVolumeClaim.Size = size
	}
}
