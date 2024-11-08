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

	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type CABundle struct {
	Type           virtv2.DataSourceType
	HTTP           *virtv2.DataSourceHTTP
	ContainerImage *DataSourceContainerRegistry
}

type DataSourceContainerRegistry struct {
	Image           string
	ImagePullSecret types.NamespacedName
	CABundle        []byte
}

func NewCABundleForCVMI(ds virtv2.ClusterVirtualImageDataSource) *CABundle {
	return &CABundle{
		Type: ds.Type,
		HTTP: ds.HTTP,
		ContainerImage: &DataSourceContainerRegistry{
			Image: ds.ContainerImage.Image,
			ImagePullSecret: types.NamespacedName{
				Name:      ds.ContainerImage.ImagePullSecret.Name,
				Namespace: ds.ContainerImage.ImagePullSecret.Namespace,
			},
			CABundle: ds.ContainerImage.CABundle,
		},
	}
}

func NewCABundleForVMI(namespace string, ds virtv2.VirtualImageDataSource) *CABundle {
	return &CABundle{
		Type: ds.Type,
		HTTP: ds.HTTP,
		ContainerImage: &DataSourceContainerRegistry{
			Image: ds.ContainerImage.Image,
			ImagePullSecret: types.NamespacedName{
				Name:      ds.ContainerImage.Image,
				Namespace: namespace,
			},
			CABundle: ds.ContainerImage.CABundle,
		},
	}
}

func NewCABundleForVMD(namespace string, ds *virtv2.VirtualDiskDataSource) *CABundle {
	return &CABundle{
		Type: ds.Type,
		HTTP: ds.HTTP,
		ContainerImage: &DataSourceContainerRegistry{
			Image: ds.ContainerImage.Image,
			ImagePullSecret: types.NamespacedName{
				Name:      ds.ContainerImage.Image,
				Namespace: namespace,
			},
			CABundle: ds.ContainerImage.CABundle,
		},
	}
}

func (ds *CABundle) HasCABundle() bool {
	return len(ds.GetCABundle()) > 0
}

func (ds *CABundle) GetCABundle() string {
	if ds == nil {
		return ""
	}
	switch ds.Type {
	case virtv2.DataSourceTypeHTTP:
		if ds.HTTP != nil {
			return string(ds.HTTP.CABundle)
		}
	case virtv2.DataSourceTypeContainerImage:
		if ds.ContainerImage != nil {
			return string(ds.ContainerImage.CABundle)
		}
	}
	return ""
}

func (ds *CABundle) GetContainerImage() *DataSourceContainerRegistry {
	return ds.ContainerImage
}
