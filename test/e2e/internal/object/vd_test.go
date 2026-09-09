/*
Copyright 2026 Flant JSC

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
	"testing"

	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/utils/ptr"

	"github.com/deckhouse/virtualization-controller/pkg/builder/vd"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/test/e2e/internal/config"
)

func TestVirtualDiskStorageClassOverride(t *testing.T) {
	size := ptr.To(resource.MustParse("64Mi"))

	disks := func() map[string]*v1alpha2.VirtualDisk {
		return map[string]*v1alpha2.VirtualDisk{
			"from CVI":   NewVDFromCVI("vd", "ns", "cvi"),
			"from VI":    NewVDFromVI("vd", "ns", &v1alpha2.VirtualImage{}),
			"from HTTP":  NewHTTPVDCustomBIOS("vd", "ns"),
			"blank":      NewBlankVD("vd", "ns", nil, size),
			"blank opts": NewBlankVD("vd", "ns", nil, size, vd.WithSize(size)),
			"raw builder": NewVD(
				vd.WithName("vd"),
				vd.WithNamespace("ns"),
				vd.WithSize(size),
			),
		}
	}

	t.Run("StorageClass is taken from the env override", func(t *testing.T) {
		t.Setenv(config.StorageClassNameEnv, "env-sc")

		for name, disk := range disks() {
			sc := disk.Spec.PersistentVolumeClaim.StorageClass
			if sc == nil {
				t.Errorf("%s: StorageClass is not set", name)
				continue
			}
			if *sc != "env-sc" {
				t.Errorf("%s: StorageClass is %q, want %q", name, *sc, "env-sc")
			}
		}
	})

	t.Run("StorageClass is left empty for the cluster default", func(t *testing.T) {
		t.Setenv(config.StorageClassNameEnv, "")

		for name, disk := range disks() {
			if sc := disk.Spec.PersistentVolumeClaim.StorageClass; sc != nil && *sc != "" {
				t.Errorf("%s: StorageClass is %q, want empty", name, *sc)
			}
		}
	})

	t.Run("explicit StorageClass wins over the env override", func(t *testing.T) {
		t.Setenv(config.StorageClassNameEnv, "env-sc")

		explicit := map[string]*v1alpha2.VirtualDisk{
			"blank": NewBlankVD("vd", "ns", ptr.To("linstor-thin-r2"), size),
			"from CVI": NewVDFromCVI("vd", "ns", "cvi",
				vd.WithStorageClass(ptr.To("linstor-thin-r2")),
			),
		}

		for name, disk := range explicit {
			sc := disk.Spec.PersistentVolumeClaim.StorageClass
			if sc == nil || *sc != "linstor-thin-r2" {
				t.Errorf("%s: StorageClass is %v, want %q", name, sc, "linstor-thin-r2")
			}
		}
	})
}
