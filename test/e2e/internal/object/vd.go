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
	"github.com/deckhouse/virtualization/test/e2e/internal/config"
)

// NewVD builds a VirtualDisk and pins every disk the suites create to a single
// StorageClass: the STORAGE_CLASS_NAME override when it is set, otherwise nothing
// is written to the spec and the cluster default StorageClass applies. A
// StorageClass chosen explicitly by the caller always wins.
//
// Tests build their VirtualDisks with it instead of the bare vd.New builder, so the
// override does not have to be repeated in every suite.
func NewVD(opts ...vd.Option) *v1alpha2.VirtualDisk {
	disk := vd.New(opts...)
	OverrideStorageClass(disk)
	return disk
}

// OverrideStorageClass sets the STORAGE_CLASS_NAME StorageClass on a VirtualDisk
// that has none.
func OverrideStorageClass(disk *v1alpha2.VirtualDisk) {
	if disk == nil {
		return
	}
	if sc := disk.Spec.PersistentVolumeClaim.StorageClass; sc != nil && *sc != "" {
		return
	}

	if !config.IsStorageClassNameOverridden() {
		return
	}
	scName := config.StorageClassNameOverride()
	disk.Spec.PersistentVolumeClaim.StorageClass = &scName
}

func NewVDFromVI(name, namespace string, vi *v1alpha2.VirtualImage, opts ...vd.Option) *v1alpha2.VirtualDisk {
	baseOpts := []vd.Option{
		vd.WithName(name),
		vd.WithNamespace(namespace),
		vd.WithDataSourceObjectRefFromVI(vi),
	}
	baseOpts = append(baseOpts, opts...)
	return NewVD(baseOpts...)
}

func NewBlankVD(name, namespace string, storageClass *string, size *resource.Quantity, opts ...vd.Option) *v1alpha2.VirtualDisk {
	baseOpts := []vd.Option{
		vd.WithName(name),
		vd.WithNamespace(namespace),
		vd.WithPersistentVolumeClaim(storageClass, size),
	}
	baseOpts = append(baseOpts, opts...)
	return NewVD(baseOpts...)
}

func NewHTTPVDAlpineBIOS(name, namespace string, opts ...vd.Option) *v1alpha2.VirtualDisk {
	baseOpts := []vd.Option{
		vd.WithName(name),
		vd.WithNamespace(namespace),
		vd.WithDataSourceHTTP(&v1alpha2.DataSourceHTTP{
			URL: ImageURLAlpineBIOS,
		}),
	}
	baseOpts = append(baseOpts, opts...)
	return NewVD(baseOpts...)
}

// NewHTTPVDCustomBIOS builds a VirtualDisk sourced over HTTP from the custom
// custom image. Used by the blockdevice suite; the AlpineBIOS variant is left
// for the other suites that rely on it.
func NewHTTPVDCustomBIOS(name, namespace string, opts ...vd.Option) *v1alpha2.VirtualDisk {
	baseOpts := []vd.Option{
		vd.WithName(name),
		vd.WithNamespace(namespace),
		vd.WithDataSourceHTTP(&v1alpha2.DataSourceHTTP{
			URL: ImageURLCustomBIOS,
		}),
	}
	baseOpts = append(baseOpts, opts...)
	return NewVD(baseOpts...)
}

func NewVDFromCVI(name, namespace, cviName string, opts ...vd.Option) *v1alpha2.VirtualDisk {
	baseOpts := []vd.Option{
		vd.WithName(name),
		vd.WithNamespace(namespace),
		vd.WithDataSourceObjectRef(v1alpha2.VirtualDiskObjectRefKindClusterVirtualImage, cviName),
	}
	baseOpts = append(baseOpts, opts...)
	return NewVD(baseOpts...)
}
