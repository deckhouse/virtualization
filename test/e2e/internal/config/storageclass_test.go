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

func TestResolveWFFCStorageClass(t *testing.T) {
	wffc := storagev1.VolumeBindingWaitForFirstConsumer
	immediate := storagev1.VolumeBindingImmediate
	provisioner := "replicated.csi.storage.deckhouse.io"

	scList := &storagev1.StorageClassList{Items: []storagev1.StorageClass{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "rv-thin-r1",
				Annotations: map[string]string{
					"storageclass.kubernetes.io/is-default-class": "true",
				},
			},
			Provisioner:         provisioner,
			VolumeBindingMode:   &immediate,
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "rv-thin-r1-wffc"},
			Provisioner:       provisioner,
			VolumeBindingMode: &wffc,
		},
	}}

	got, err := ResolveWFFCStorageClass(scList)
	if err != nil {
		t.Fatalf("ResolveWFFCStorageClass() error = %v", err)
	}
	if got == nil || got.Name != "rv-thin-r1-wffc" {
		t.Fatalf("ResolveWFFCStorageClass() = %#v, want rv-thin-r1-wffc", got)
	}

	scList.Items[0].VolumeBindingMode = &wffc
	scList.Items[0].Annotations = map[string]string{
		"storageclass.kubernetes.io/is-default-class": "true",
	}
	scList.Items[1].VolumeBindingMode = &immediate

	got, err = ResolveWFFCStorageClass(scList)
	if err != nil {
		t.Fatalf("ResolveWFFCStorageClass() error = %v", err)
	}
	if got == nil || got.Name != "rv-thin-r1" {
		t.Fatalf("ResolveWFFCStorageClass() = %#v, want rv-thin-r1", got)
	}

	t.Setenv(WFFCStorageClassEnv, "rv-thin-r1-wffc")
	got, err = ResolveWFFCStorageClass(scList)
	if err != nil {
		t.Fatalf("ResolveWFFCStorageClass() with env error = %v", err)
	}
	if got == nil || got.Name != "rv-thin-r1-wffc" {
		t.Fatalf("ResolveWFFCStorageClass() with env = %#v, want rv-thin-r1-wffc", got)
	}

	t.Setenv(WFFCStorageClassEnv, "missing-sc")
	if _, err := ResolveWFFCStorageClass(scList); err == nil {
		t.Fatal("ResolveWFFCStorageClass() with missing env SC expected error")
	}

	os.Unsetenv(WFFCStorageClassEnv)
	scList.Items[0].Annotations = nil
	if got, err := ResolveWFFCStorageClass(scList); err != nil || got != nil {
		t.Fatalf("ResolveWFFCStorageClass() without default = (%#v, %v), want (nil, nil)", got, err)
	}
}

func TestResolveImmediateStorageClass(t *testing.T) {
	wffc := storagev1.VolumeBindingWaitForFirstConsumer
	immediate := storagev1.VolumeBindingImmediate
	provisioner := "replicated.csi.storage.deckhouse.io"

	scList := &storagev1.StorageClassList{Items: []storagev1.StorageClass{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "rv-thin-r1-wffc",
				Annotations: map[string]string{
					"storageclass.kubernetes.io/is-default-class": "true",
				},
			},
			Provisioner:         provisioner,
			VolumeBindingMode:   &wffc,
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "rv-thin-r1"},
			Provisioner:       provisioner,
			VolumeBindingMode: &immediate,
		},
	}}

	got, err := ResolveImmediateStorageClass(scList)
	if err != nil {
		t.Fatalf("ResolveImmediateStorageClass() error = %v", err)
	}
	if got == nil || got.Name != "rv-thin-r1" {
		t.Fatalf("ResolveImmediateStorageClass() = %#v, want rv-thin-r1", got)
	}

	scList.Items[0].VolumeBindingMode = &immediate
	scList.Items[0].Name = "rv-thin-r1"
	scList.Items[1].VolumeBindingMode = &wffc
	scList.Items[1].Name = "rv-thin-r1-wffc"

	got, err = ResolveImmediateStorageClass(scList)
	if err != nil {
		t.Fatalf("ResolveImmediateStorageClass() error = %v", err)
	}
	if got == nil || got.Name != "rv-thin-r1" {
		t.Fatalf("ResolveImmediateStorageClass() = %#v, want rv-thin-r1", got)
	}
}
