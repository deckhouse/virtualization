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

package storageprofile

import (
	"context"
	"testing"

	vsv1 "github.com/kubernetes-csi/external-snapshotter/client/v6/apis/volumesnapshot/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	cdiv1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	storagev1alpha1 "github.com/deckhouse/virtualization-controller/pkg/apis/storage/v1alpha1"
)

func TestReconcileStorageProfileCreatesSnapshotStrategy(t *testing.T) {
	ctx := context.Background()
	scheme := storageProfileTestScheme(t)
	sc := &storagev1.StorageClass{Provisioner: "csi.example.com"}
	sc.Name = "fast"
	vsc := &vsv1.VolumeSnapshotClass{Driver: sc.Provisioner}
	vsc.Name = "snap-fast"
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(sc, vsc).Build()
	r := &Reconciler{client: c}

	_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: types.NamespacedName{Name: sc.Name}})
	if err != nil {
		t.Fatalf("reconcile failed: %v", err)
	}

	sp := &storagev1alpha1.StorageProfile{}
	if err := c.Get(ctx, types.NamespacedName{Name: sc.Name}, sp); err != nil {
		t.Fatalf("storageprofile not found: %v", err)
	}
	if sp.Status.CloneStrategy == nil || *sp.Status.CloneStrategy != cdiv1.CloneStrategySnapshot {
		t.Fatalf("unexpected clone strategy: %#v", sp.Status.CloneStrategy)
	}
	if sp.Status.SnapshotClass == nil || *sp.Status.SnapshotClass != vsc.Name {
		t.Fatalf("unexpected snapshot class: %#v", sp.Status.SnapshotClass)
	}
}

func TestReconcileStorageProfileUsesSnapshotForSDSReplicated(t *testing.T) {
	ctx := context.Background()
	scheme := storageProfileTestScheme(t)
	sc := &storagev1.StorageClass{Provisioner: SDSReplicatedCSIProvisioner}
	sc.Name = "rv-thin-r1"
	vsc := &vsv1.VolumeSnapshotClass{Driver: sc.Provisioner}
	vsc.Name = "sds-replicated-volume"
	snapshot := cdiv1.CloneStrategySnapshot
	existing := &storagev1alpha1.StorageProfile{
		ObjectMeta: metav1.ObjectMeta{Name: sc.Name},
		Spec: storagev1alpha1.StorageProfileSpec{
			CloneStrategy: &snapshot,
		},
	}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(sc, vsc, existing).Build()
	r := &Reconciler{client: c}

	_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: types.NamespacedName{Name: sc.Name}})
	if err != nil {
		t.Fatalf("reconcile failed: %v", err)
	}

	sp := &storagev1alpha1.StorageProfile{}
	if err := c.Get(ctx, types.NamespacedName{Name: sc.Name}, sp); err != nil {
		t.Fatalf("storageprofile not found: %v", err)
	}
	if sp.Spec.CloneStrategy == nil || *sp.Spec.CloneStrategy != cdiv1.CloneStrategySnapshot {
		t.Fatalf("spec clone strategy must stay snapshot: %#v", sp.Spec.CloneStrategy)
	}
	if sp.Status.CloneStrategy == nil || *sp.Status.CloneStrategy != cdiv1.CloneStrategySnapshot {
		t.Fatalf("unexpected clone strategy: %#v", sp.Status.CloneStrategy)
	}
	if sp.Status.SnapshotClass == nil || *sp.Status.SnapshotClass != vsc.Name {
		t.Fatalf("unexpected snapshot class: %#v", sp.Status.SnapshotClass)
	}
}

func TestReconcileStorageProfileHonorsStorageClassAnnotation(t *testing.T) {
	ctx := context.Background()
	scheme := storageProfileTestScheme(t)
	sc := &storagev1.StorageClass{
		Provisioner: "csi.example.com",
	}
	sc.Name = "fast"
	sc.Annotations = map[string]string{"cdi.kubevirt.io/clone-strategy": "csi-clone"}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(sc).Build()
	r := &Reconciler{client: c}

	_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: types.NamespacedName{Name: sc.Name}})
	if err != nil {
		t.Fatalf("reconcile failed: %v", err)
	}

	sp := &storagev1alpha1.StorageProfile{}
	if err := c.Get(ctx, types.NamespacedName{Name: sc.Name}, sp); err != nil {
		t.Fatalf("storageprofile not found: %v", err)
	}
	if sp.Status.CloneStrategy == nil || *sp.Status.CloneStrategy != cdiv1.CloneStrategyCsiClone {
		t.Fatalf("unexpected clone strategy: %#v", sp.Status.CloneStrategy)
	}
}

func storageProfileTestScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := storagev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := vsv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := storagev1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	return scheme
}
