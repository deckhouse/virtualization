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
	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/deckhouse/virtualization-controller/pkg/builder/vd"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

func NewGeneratedVDFromCVI(prefix, namespace string, cvi *v1alpha2.ClusterVirtualImage) *v1alpha2.VirtualDisk {
	return vd.New(
		vd.WithGenerateName(prefix),
		vd.WithNamespace(namespace),
		vd.WithDataSourceObjectRefFromCVI(cvi),
	)
}

func NewVDFromCVI(name, namespace string, cvi *v1alpha2.ClusterVirtualImage) *v1alpha2.VirtualDisk {
	return vd.New(
		vd.WithName(name),
		vd.WithNamespace(namespace),
		vd.WithDataSourceObjectRefFromCVI(cvi),
	)
}

func NewGeneratedVDFromVI(prefix, namespace string, vi *v1alpha2.VirtualImage) *v1alpha2.VirtualDisk {
	return vd.New(
		vd.WithGenerateName(prefix),
		vd.WithNamespace(namespace),
		vd.WithDataSourceObjectRefFromVI(vi),
	)
}

func NewVDFromVI(name, namespace string, vi *v1alpha2.VirtualImage) *v1alpha2.VirtualDisk {
	return vd.New(
		vd.WithName(name),
		vd.WithNamespace(namespace),
		vd.WithDataSourceObjectRefFromVI(vi),
	)
}

func NewBlankVD(name, namespace string, storageClass *string, size *resource.Quantity) *v1alpha2.VirtualDisk {
	return vd.New(
		vd.WithName(name),
		vd.WithNamespace(namespace),
		vd.WithPersistentVolumeClaim(storageClass, size),
	)
}

func NewGeneratedHTTPVDUbuntu(prefix, namespace string) *v1alpha2.VirtualDisk {
	return vd.New(
		vd.WithGenerateName(prefix),
		vd.WithNamespace(namespace),
		vd.WithDataSourceHTTP(&v1alpha2.DataSourceHTTP{
			URL: UbuntuHTTP,
		}),
	)
}
