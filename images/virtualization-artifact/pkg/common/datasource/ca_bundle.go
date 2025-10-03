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

package datasource

import (
	"k8s.io/apimachinery/pkg/types"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type CABundle struct {
	Type           v1alpha2.DataSourceType
	HTTP           *v1alpha2.DataSourceHTTP
	ContainerImage *ContainerRegistry
}

type ContainerRegistry struct {
	Image           string
	ImagePullSecret types.NamespacedName
	CABundle        []byte
}

func NewCABundleForCVMI(ds v1alpha2.ClusterVirtualImageDataSource) *CABundle {
	switch ds.Type {
	case v1alpha2.DataSourceTypeHTTP:
		return &CABundle{
			Type: ds.Type,
			HTTP: ds.HTTP,
		}
	case v1alpha2.DataSourceTypeContainerImage:
		return &CABundle{
			Type: ds.Type,
			ContainerImage: &ContainerRegistry{
				Image: ds.ContainerImage.Image,
				ImagePullSecret: types.NamespacedName{
					Name:      ds.ContainerImage.ImagePullSecret.Name,
					Namespace: ds.ContainerImage.ImagePullSecret.Namespace,
				},
				CABundle: ds.ContainerImage.CABundle,
			},
		}
	}

	return &CABundle{Type: ds.Type}
}

func NewCABundleForVMI(namespace string, ds v1alpha2.VirtualImageDataSource) *CABundle {
	switch ds.Type {
	case v1alpha2.DataSourceTypeHTTP:
		return &CABundle{
			Type: ds.Type,
			HTTP: ds.HTTP,
		}
	case v1alpha2.DataSourceTypeContainerImage:
		return &CABundle{
			Type: ds.Type,
			ContainerImage: &ContainerRegistry{
				Image: ds.ContainerImage.Image,
				ImagePullSecret: types.NamespacedName{
					Name:      ds.ContainerImage.ImagePullSecret.Name,
					Namespace: namespace,
				},
				CABundle: ds.ContainerImage.CABundle,
			},
		}
	}

	return &CABundle{Type: ds.Type}
}

func NewCABundleForVMD(namespace string, ds *v1alpha2.VirtualDiskDataSource) *CABundle {
	switch ds.Type {
	case v1alpha2.DataSourceTypeHTTP:
		return &CABundle{
			Type: ds.Type,
			HTTP: ds.HTTP,
		}
	case v1alpha2.DataSourceTypeContainerImage:
		return &CABundle{
			Type: ds.Type,
			ContainerImage: &ContainerRegistry{
				Image: ds.ContainerImage.Image,
				ImagePullSecret: types.NamespacedName{
					Name:      ds.ContainerImage.ImagePullSecret.Name,
					Namespace: namespace,
				},
				CABundle: ds.ContainerImage.CABundle,
			},
		}
	}

	return &CABundle{Type: ds.Type}
}

func (ds *CABundle) HasCABundle() bool {
	return len(ds.GetCABundle()) > 0
}

func (ds *CABundle) GetCABundle() string {
	if ds == nil {
		return ""
	}
	switch ds.Type {
	case v1alpha2.DataSourceTypeHTTP:
		if ds.HTTP != nil {
			return string(ds.HTTP.CABundle)
		}
	case v1alpha2.DataSourceTypeContainerImage:
		if ds.ContainerImage != nil {
			return string(ds.ContainerImage.CABundle)
		}
	}
	return ""
}

func (ds *CABundle) GetContainerImage() *ContainerRegistry {
	return ds.ContainerImage
}
