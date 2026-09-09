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

	"k8s.io/utils/ptr"

	"github.com/deckhouse/virtualization-controller/pkg/builder/vi"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/test/e2e/internal/config"
)

func TestVirtualImageStorageClassOverride(t *testing.T) {
	t.Run("PVC-backed image takes the env override", func(t *testing.T) {
		t.Setenv(config.StorageClassNameEnv, "env-sc")

		image := NewVI(
			vi.WithName("vi"),
			vi.WithNamespace("ns"),
			vi.WithStorage(v1alpha2.StoragePersistentVolumeClaim),
		)

		sc := image.Spec.PersistentVolumeClaim.StorageClass
		if sc == nil || *sc != "env-sc" {
			t.Errorf("StorageClass is %v, want %q", sc, "env-sc")
		}
	})

	t.Run("registry-backed image is left alone", func(t *testing.T) {
		t.Setenv(config.StorageClassNameEnv, "env-sc")

		image := NewGeneratedHTTPVICustomBIOS("vi-", "ns")

		if sc := image.Spec.PersistentVolumeClaim.StorageClass; sc != nil {
			t.Errorf("StorageClass is %q, want none for a %s image", *sc, image.Spec.Storage)
		}
	})

	t.Run("explicit StorageClass wins over the env override", func(t *testing.T) {
		t.Setenv(config.StorageClassNameEnv, "env-sc")

		image := NewVI(
			vi.WithName("vi"),
			vi.WithNamespace("ns"),
			vi.WithStorage(v1alpha2.StoragePersistentVolumeClaim),
		)
		// The vi builder has no StorageClass option: the suites set the field
		// directly, as blockdevice does.
		image.Spec.PersistentVolumeClaim.StorageClass = ptr.To("linstor-thin-r2")
		OverrideImageStorageClass(image)

		sc := image.Spec.PersistentVolumeClaim.StorageClass
		if sc == nil || *sc != "linstor-thin-r2" {
			t.Errorf("StorageClass is %v, want %q", sc, "linstor-thin-r2")
		}
	})
}
