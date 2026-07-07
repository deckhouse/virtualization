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

package config

import (
	"os"
	"testing"

	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestResolveDefaultStorageClass(t *testing.T) {
	scList := &storagev1.StorageClassList{Items: []storagev1.StorageClass{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "linstor-thin-r1",
				Annotations: map[string]string{
					"storageclass.kubernetes.io/is-default-class": "true",
				},
			},
			Provisioner: "replicated.csi.storage.deckhouse.io",
		},
		{
			ObjectMeta:  metav1.ObjectMeta{Name: "sds-local-thin-wffc"},
			Provisioner: "local.csi.storage.deckhouse.io",
		},
	}}

	t.Run("uses default when env is unset", func(t *testing.T) {
		unsetStorageClassNameEnv(t)

		got, err := ResolveDefaultStorageClass(scList)
		if err != nil {
			t.Fatalf("ResolveDefaultStorageClass() error = %v", err)
		}
		if got == nil || got.Name != "linstor-thin-r1" {
			t.Fatalf("ResolveDefaultStorageClass() = %#v, want linstor-thin-r1", got)
		}
	})

	t.Run("uses env override", func(t *testing.T) {
		unsetStorageClassNameEnv(t)
		t.Setenv(StorageClassNameEnv, "sds-local-thin-wffc")

		got, err := ResolveDefaultStorageClass(scList)
		if err != nil {
			t.Fatalf("ResolveDefaultStorageClass() with env error = %v", err)
		}
		if got == nil || got.Name != "sds-local-thin-wffc" {
			t.Fatalf("ResolveDefaultStorageClass() with env = %#v, want sds-local-thin-wffc", got)
		}
	})

	t.Run("errors on missing env override", func(t *testing.T) {
		unsetStorageClassNameEnv(t)
		t.Setenv(StorageClassNameEnv, "missing-sc")

		if _, err := ResolveDefaultStorageClass(scList); err == nil {
			t.Fatal("ResolveDefaultStorageClass() with missing env SC expected error")
		}
	})

	t.Run("errors on empty env override", func(t *testing.T) {
		unsetStorageClassNameEnv(t)
		t.Setenv(StorageClassNameEnv, "")

		if _, err := ResolveDefaultStorageClass(scList); err == nil {
			t.Fatal("ResolveDefaultStorageClass() with empty env SC expected error")
		}
	})

	t.Run("returns nil without default and env", func(t *testing.T) {
		unsetStorageClassNameEnv(t)
		scListWithoutDefault := &storagev1.StorageClassList{Items: []storagev1.StorageClass{{
			ObjectMeta:  metav1.ObjectMeta{Name: "sds-local-thin-wffc"},
			Provisioner: "local.csi.storage.deckhouse.io",
		}}}

		got, err := ResolveDefaultStorageClass(scListWithoutDefault)
		if err != nil {
			t.Fatalf("ResolveDefaultStorageClass() without default error = %v", err)
		}
		if got != nil {
			t.Fatalf("ResolveDefaultStorageClass() without default = %#v, want nil", got)
		}
	})
}

func unsetStorageClassNameEnv(t *testing.T) {
	t.Helper()

	oldValue, wasSet := os.LookupEnv(StorageClassNameEnv)
	if wasSet {
		t.Setenv(StorageClassNameEnv, oldValue)
	} else {
		t.Setenv(StorageClassNameEnv, "")
	}
	if err := os.Unsetenv(StorageClassNameEnv); err != nil {
		t.Fatalf("failed to unset %s: %v", StorageClassNameEnv, err)
	}
}
