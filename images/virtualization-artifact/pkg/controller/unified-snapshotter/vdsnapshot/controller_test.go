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

package vdsnapshot

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apiruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	storagev1alpha1 "github.com/deckhouse/state-snapshotter/api/storage/v1alpha1"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

const testNamespace = "ns"

func newTestScheme(t *testing.T) *apiruntime.Scheme {
	t.Helper()
	scheme := apiruntime.NewScheme()
	for _, f := range []func(*apiruntime.Scheme) error{
		clientgoscheme.AddToScheme,
		v1alpha2.AddToScheme,
		storagev1alpha1.AddToScheme,
	} {
		if err := f(scheme); err != nil {
			t.Fatal(err)
		}
	}
	return scheme
}

func newTestReconciler(t *testing.T, objs ...client.Object) *Reconciler {
	t.Helper()
	c := fake.NewClientBuilder().WithScheme(newTestScheme(t)).WithObjects(objs...).WithStatusSubresource(&v1alpha2.VirtualDiskSnapshot{}).Build()
	return &Reconciler{Client: c, APIReader: c}
}

func getVDS(t *testing.T, c client.Client) *v1alpha2.VirtualDiskSnapshot {
	t.Helper()
	vds := &v1alpha2.VirtualDiskSnapshot{}
	if err := c.Get(context.Background(), types.NamespacedName{Namespace: testNamespace, Name: "vds1"}, vds); err != nil {
		t.Fatalf("get VirtualDiskSnapshot: %v", err)
	}
	return vds
}

func TestReconcile_DeletionTimestampIsANoOp(t *testing.T) {
	vds := &v1alpha2.VirtualDiskSnapshot{
		ObjectMeta: metav1.ObjectMeta{
			Name: "vds1", Namespace: testNamespace,
			Annotations:       map[string]string{v1alpha2.AnnUseUnifiedSnapshotter: ""},
			DeletionTimestamp: ptr.To(metav1.Now()),
			Finalizers:        []string{"kubernetes"},
		},
	}
	r := newTestReconciler(t, vds)

	res, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Namespace: testNamespace, Name: "vds1"}})
	if err != nil {
		t.Fatal(err)
	}
	if res != (ctrl.Result{}) {
		t.Fatalf("expected a no-op result, got %+v", res)
	}
}

func TestReconcile_MissingAnnotationIsANoOp(t *testing.T) {
	vds := &v1alpha2.VirtualDiskSnapshot{
		ObjectMeta: metav1.ObjectMeta{Name: "vds1", Namespace: testNamespace},
	}
	r := newTestReconciler(t, vds)

	res, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Namespace: testNamespace, Name: "vds1"}})
	if err != nil {
		t.Fatal(err)
	}
	if res != (ctrl.Result{}) {
		t.Fatalf("expected a no-op result, got %+v", res)
	}
	if getVDS(t, r.Client).Status.Phase != "" {
		t.Fatal("expected phase to stay empty when the annotation is absent")
	}
}

func TestReconcile_BootstrapsPendingPhase(t *testing.T) {
	vds := &v1alpha2.VirtualDiskSnapshot{
		ObjectMeta: metav1.ObjectMeta{Name: "vds1", Namespace: testNamespace, Annotations: map[string]string{v1alpha2.AnnUseUnifiedSnapshotter: ""}},
		Spec:       v1alpha2.VirtualDiskSnapshotSpec{VirtualDiskName: "vd1"},
	}
	r := newTestReconciler(t, vds)

	res, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Namespace: testNamespace, Name: "vds1"}})
	if err != nil {
		t.Fatal(err)
	}
	if res != (ctrl.Result{Requeue: true}) {
		t.Fatalf("expected Requeue, got %+v", res)
	}
	if got := getVDS(t, r.Client).Status.Phase; got != v1alpha2.VirtualDiskSnapshotPhasePending {
		t.Fatalf("got phase %q, want Pending", got)
	}
}

func TestReconcile_WaitsWhenSourceVirtualDiskIsMissing(t *testing.T) {
	vds := &v1alpha2.VirtualDiskSnapshot{
		ObjectMeta: metav1.ObjectMeta{Name: "vds1", Namespace: testNamespace, Annotations: map[string]string{v1alpha2.AnnUseUnifiedSnapshotter: ""}},
		Spec:       v1alpha2.VirtualDiskSnapshotSpec{VirtualDiskName: "missing-vd"},
		Status:     v1alpha2.VirtualDiskSnapshotStatus{Phase: v1alpha2.VirtualDiskSnapshotPhasePending},
	}
	r := newTestReconciler(t, vds)

	res, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Namespace: testNamespace, Name: "vds1"}})
	if err != nil {
		t.Fatal(err)
	}
	if res.RequeueAfter != requeueAfter {
		t.Fatalf("got RequeueAfter %v, want %v", res.RequeueAfter, requeueAfter)
	}

	got := getVDS(t, r.Client)
	if got.Status.CaptureState == nil || got.Status.CaptureState.DomainSpecificController == nil {
		t.Fatal("expected DomainSpecificController capture state to be published")
	}
	if got.Status.CaptureState.DomainSpecificController.Message == "" {
		t.Fatal("expected a waiting message explaining the missing VirtualDisk")
	}
}

func TestReconcile_WaitsWhenBackingPVCIsNotProvisionedYet(t *testing.T) {
	vd := &v1alpha2.VirtualDisk{
		ObjectMeta: metav1.ObjectMeta{Name: "vd1", Namespace: testNamespace},
		// Status.Target.PersistentVolumeClaim intentionally empty: not provisioned yet.
	}
	vds := &v1alpha2.VirtualDiskSnapshot{
		ObjectMeta: metav1.ObjectMeta{Name: "vds1", Namespace: testNamespace, Annotations: map[string]string{v1alpha2.AnnUseUnifiedSnapshotter: ""}},
		Spec:       v1alpha2.VirtualDiskSnapshotSpec{VirtualDiskName: "vd1"},
		Status:     v1alpha2.VirtualDiskSnapshotStatus{Phase: v1alpha2.VirtualDiskSnapshotPhasePending},
	}
	r := newTestReconciler(t, vd, vds)

	res, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Namespace: testNamespace, Name: "vds1"}})
	if err != nil {
		t.Fatal(err)
	}
	if res.RequeueAfter != requeueAfter {
		t.Fatalf("got RequeueAfter %v, want %v", res.RequeueAfter, requeueAfter)
	}

	got := getVDS(t, r.Client)
	if got.Status.CaptureState == nil || got.Status.CaptureState.DomainSpecificController == nil {
		t.Fatal("expected DomainSpecificController capture state to be published")
	}
	if got.Status.CaptureState.DomainSpecificController.Message == "" {
		t.Fatal("expected a waiting message explaining the unprovisioned PVC")
	}
}

func TestPatchStatus_OnlyTouchesOwnedFields(t *testing.T) {
	vds := &v1alpha2.VirtualDiskSnapshot{
		ObjectMeta: metav1.ObjectMeta{Name: "vds1", Namespace: testNamespace},
		Status: v1alpha2.VirtualDiskSnapshotStatus{
			// A core/SDK-owned field this controller must never clobber via its own patch.
			CaptureState: &v1alpha2.UnifiedSnapshotterCaptureState{
				CommonController: &v1alpha2.UnifiedSnapshotterCommonCaptureState{ManifestCaptured: ptr.To(true)},
			},
		},
	}
	r := newTestReconciler(t, vds)

	vds.Status.Phase = v1alpha2.VirtualDiskSnapshotPhaseReady
	vds.Status.Consistent = ptr.To(true)
	vds.Status.StorageClassName = "ssd"
	if err := r.patchStatus(context.Background(), vds); err != nil {
		t.Fatal(err)
	}

	got := getVDS(t, r.Client)
	if got.Status.Phase != v1alpha2.VirtualDiskSnapshotPhaseReady {
		t.Fatalf("got phase %q, want Ready", got.Status.Phase)
	}
	if got.Status.StorageClassName != "ssd" {
		t.Fatalf("got storageClassName %q, want ssd", got.Status.StorageClassName)
	}
	if got.Status.CaptureState == nil || got.Status.CaptureState.CommonController == nil || got.Status.CaptureState.CommonController.ManifestCaptured == nil || !*got.Status.CaptureState.CommonController.ManifestCaptured {
		t.Fatal("expected the core-owned captureState.commonController to survive the domain controller's own status patch untouched")
	}
}
