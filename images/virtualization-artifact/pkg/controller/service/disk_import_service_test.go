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

package service

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	cdiv1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

func TestDiskServiceStartObjectRefDiskImportCreatesTargetPVC(t *testing.T) {
	ctx := context.Background()
	c := fake.NewClientBuilder().WithScheme(diskImportTestScheme(t)).Build()
	svc := NewDiskService(c, nil, nil, "test", DiskImporterConfig{Image: "disk-importer:latest", PullPolicy: string(corev1.PullIfNotPresent), Verbose: "3"})

	url := "docker://registry.example/image:tag"
	secret := "auth"
	cert := "ca"
	vd := diskImportTestVD()
	sc := &storagev1.StorageClass{ObjectMeta: metav1.ObjectMeta{
		Name: "fast",
		Annotations: map[string]string{
			annotations.AnnVirtualDiskVolumeMode: string(corev1.PersistentVolumeFilesystem),
			annotations.AnnVirtualDiskAccessMode: string(corev1.ReadWriteOnce),
		},
	}}

	err := svc.StartObjectRefDiskImport(ctx, resource.MustParse("1Gi"), sc, &cdiv1.DataVolumeSource{
		Registry: &cdiv1.DataVolumeSourceRegistry{
			URL:           &url,
			SecretRef:     &secret,
			CertConfigMap: &cert,
		},
	}, vd, nil)
	if err != nil {
		t.Fatalf("StartObjectRefDiskImport failed: %v", err)
	}

	pvc := &corev1.PersistentVolumeClaim{}
	if err := c.Get(ctx, types.NamespacedName{Name: vd.Status.Target.PersistentVolumeClaim, Namespace: vd.Namespace}, pvc); err != nil {
		t.Fatalf("target pvc not found: %v", err)
	}
	if got := pvc.Annotations[AnnObjectRefImportSource]; got != sourceRegistry {
		t.Fatalf("unexpected source annotation: %q", got)
	}
	if got := pvc.Annotations[AnnObjectRefImportEndpoint]; got != url {
		t.Fatalf("unexpected endpoint annotation: %q", got)
	}
	if got := pvc.Annotations[AnnObjectRefImportPhase]; got != string(corev1.PodPending) {
		t.Fatalf("unexpected import phase annotation: %q", got)
	}
	if len(pvc.OwnerReferences) != 1 || pvc.OwnerReferences[0].Kind != v1alpha2.VirtualDiskKind {
		t.Fatalf("target pvc owner reference not set: %#v", pvc.OwnerReferences)
	}
}

func TestDiskServiceEnsureObjectRefDiskImportIsResumable(t *testing.T) {
	ctx := context.Background()
	vd := diskImportTestVD()
	target := diskImportTargetPVC(vd)
	c := fake.NewClientBuilder().WithScheme(diskImportTestScheme(t)).WithObjects(target).Build()
	svc := NewDiskService(c, nil, nil, "test", DiskImporterConfig{Image: "disk-importer:latest", PullPolicy: string(corev1.PullIfNotPresent), Verbose: "3"})

	phase, err := svc.EnsureObjectRefDiskImport(ctx, target, &cdiv1.DataVolumeSource{Registry: &cdiv1.DataVolumeSourceRegistry{}}, vd, nil)
	if err != nil {
		t.Fatalf("EnsureObjectRefDiskImport failed: %v", err)
	}
	if phase != corev1.PodPending {
		t.Fatalf("unexpected phase after first ensure: %s", phase)
	}

	for _, key := range []types.NamespacedName{
		{Name: target.Name + "-scratch", Namespace: target.Namespace},
		{Name: target.Name, Namespace: target.Namespace},
	} {
		obj := &corev1.PersistentVolumeClaim{}
		if key.Name == target.Name {
			pod := &corev1.Pod{}
			if err := c.Get(ctx, key, pod); err != nil {
				t.Fatalf("pod %s not found: %v", key.Name, err)
			}
			continue
		}
		if err := c.Get(ctx, key, obj); err != nil {
			t.Fatalf("scratch pvc %s not found: %v", key.Name, err)
		}
	}

	pod := &corev1.Pod{}
	if err := c.Get(ctx, types.NamespacedName{Name: target.Name, Namespace: target.Namespace}, pod); err != nil {
		t.Fatalf("get pod: %v", err)
	}
	pod.Status.Phase = corev1.PodSucceeded
	if err := c.Status().Update(ctx, pod); err != nil {
		t.Fatalf("update pod status: %v", err)
	}
	if err := c.Get(ctx, client.ObjectKeyFromObject(target), target); err != nil {
		t.Fatalf("refresh target: %v", err)
	}

	phase, err = svc.EnsureObjectRefDiskImport(ctx, target, &cdiv1.DataVolumeSource{Registry: &cdiv1.DataVolumeSourceRegistry{}}, vd, nil)
	if err != nil {
		t.Fatalf("EnsureObjectRefDiskImport after pod success failed: %v", err)
	}
	if phase != corev1.PodSucceeded {
		t.Fatalf("unexpected final phase: %s", phase)
	}
	if err := c.Get(ctx, types.NamespacedName{Name: target.Name + "-scratch", Namespace: target.Namespace}, &corev1.PersistentVolumeClaim{}); client.IgnoreNotFound(err) == nil && err == nil {
		t.Fatalf("scratch pvc still exists")
	}
	if err := c.Get(ctx, types.NamespacedName{Name: target.Name, Namespace: target.Namespace}, &corev1.Pod{}); client.IgnoreNotFound(err) == nil && err == nil {
		t.Fatalf("import pod still exists")
	}
}

func diskImportTestScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := storagev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := v1alpha2.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	return scheme
}

func diskImportTestVD() *v1alpha2.VirtualDisk {
	return &v1alpha2.VirtualDisk{
		TypeMeta: metav1.TypeMeta{APIVersion: v1alpha2.SchemeGroupVersion.String(), Kind: v1alpha2.VirtualDiskKind},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "disk",
			Namespace: "default",
			UID:       "22222222-2222-2222-2222-222222222222",
		},
		Status: v1alpha2.VirtualDiskStatus{Target: v1alpha2.DiskTarget{PersistentVolumeClaim: "d8v-vd-22222222-2222-2222-2222-222222222222-abcde"}},
	}
}

func diskImportTargetPVC(vd *v1alpha2.VirtualDisk) *corev1.PersistentVolumeClaim {
	return &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      vd.Status.Target.PersistentVolumeClaim,
			Namespace: vd.Namespace,
			UID:       "33333333-3333-3333-3333-333333333333",
			Annotations: map[string]string{
				AnnObjectRefImportEndpoint:  "docker://registry.example/image:tag",
				AnnObjectRefImportImageSize: "1Gi",
			},
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: v1alpha2.SchemeGroupVersion.String(),
				Kind:       v1alpha2.VirtualDiskKind,
				Name:       vd.Name,
				UID:        vd.UID,
				Controller: ptr.To(true),
			}},
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			StorageClassName: ptr.To("fast"),
			AccessModes:      []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			VolumeMode:       ptr.To(corev1.PersistentVolumeFilesystem),
			Resources: corev1.VolumeResourceRequirements{Requests: corev1.ResourceList{
				corev1.ResourceStorage: resource.MustParse("1Gi"),
			}},
		},
		Status: corev1.PersistentVolumeClaimStatus{Phase: corev1.ClaimBound},
	}
}
