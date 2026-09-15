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

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apiruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	storagev1alpha1 "github.com/deckhouse/state-snapshotter/api/storage/v1alpha1"
	snapshotterv1alpha1 "github.com/deckhouse/state-snapshotter/api/v1alpha1"
	foundationv1alpha1 "github.com/deckhouse/storage-foundation/api/v1alpha1"
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
		snapshotterv1alpha1.AddToScheme,
		storagev1alpha1.AddToScheme,
		foundationv1alpha1.AddToScheme,
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

// The unified controllers are registered only where UNIFIED_SNAPSHOTTER_PRESENT is set, so an object
// nobody pinned is theirs: taking a snapshot through them no longer needs an opt-in annotation.
func TestReconcile_UnpinnedObjectIsTakenOver(t *testing.T) {
	vds := &v1alpha2.VirtualDiskSnapshot{
		ObjectMeta: metav1.ObjectMeta{Name: "vds1", Namespace: testNamespace},
	}
	r := newTestReconciler(t, vds)

	if _, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Namespace: testNamespace, Name: "vds1"}}); err != nil {
		t.Fatal(err)
	}
	if got := getVDS(t, r.Client).Status.Phase; got != v1alpha2.VirtualDiskSnapshotPhasePending {
		t.Fatalf("expected the reconciler to take an unpinned object over, got phase %q", got)
	}
}

// Mirror image of the built-in controller's guard: exactly one of the two drives any object.
func TestReconcile_BuiltInPinIsANoOp(t *testing.T) {
	vds := &v1alpha2.VirtualDiskSnapshot{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "vds1",
			Namespace:   testNamespace,
			Annotations: map[string]string{v1alpha2.AnnUseBuiltInSnapshotter: ""},
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
	if getVDS(t, r.Client).Status.Phase != "" {
		t.Fatal("expected phase to stay empty for an object pinned to the built-in mechanism")
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

// A disk attached to several running machines cannot be frozen unambiguously, so a consistency mandate
// cannot be honored.
func TestReconcile_MultipleAttachedVirtualMachines(t *testing.T) {
	coreOwner := metav1.OwnerReference{
		APIVersion: "state-snapshotter.deckhouse.io/v1alpha1",
		Kind:       "Snapshot",
		Name:       "namespace-snapshot",
		UID:        "snap-uid",
	}

	tests := []struct {
		name       string
		owners     []metav1.OwnerReference
		wantFailed bool
	}{
		{
			name:       "asked for by a user",
			wantFailed: true,
		},
		{
			name:       "planned by the core for a namespace snapshot",
			owners:     []metav1.OwnerReference{coreOwner},
			wantFailed: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vd := &v1alpha2.VirtualDisk{
				ObjectMeta: metav1.ObjectMeta{Name: "vd1", Namespace: testNamespace},
				Status: v1alpha2.VirtualDiskStatus{
					Target: v1alpha2.DiskTarget{PersistentVolumeClaim: "pvc1"},
					AttachedToVirtualMachines: []v1alpha2.AttachedVirtualMachine{
						{Name: "vm1"}, {Name: "vm2"},
					},
				},
			}
			pvc := &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{Name: "pvc1", Namespace: testNamespace},
				Status:     corev1.PersistentVolumeClaimStatus{Phase: corev1.ClaimBound},
			}
			vds := &v1alpha2.VirtualDiskSnapshot{
				ObjectMeta: metav1.ObjectMeta{
					Name: "vds1", Namespace: testNamespace,
					Annotations:     map[string]string{v1alpha2.AnnUseUnifiedSnapshotter: ""},
					OwnerReferences: tt.owners,
				},
				Spec: v1alpha2.VirtualDiskSnapshotSpec{
					VirtualDiskName:     "vd1",
					RequiredConsistency: true,
				},
				Status: v1alpha2.VirtualDiskSnapshotStatus{Phase: v1alpha2.VirtualDiskSnapshotPhasePending},
			}
			r := newTestReconciler(t, vd, pvc, vds)

			if _, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Namespace: testNamespace, Name: "vds1"}}); err != nil {
				t.Fatal(err)
			}

			got := getVDS(t, r.Client)
			failed := got.Status.Phase == v1alpha2.VirtualDiskSnapshotPhaseFailed
			if failed != tt.wantFailed {
				d := got.Status.CaptureState.DomainSpecificController
				t.Fatalf("failed = %v, want %v (phase %q, reason %q, message %q)",
					failed, tt.wantFailed, got.Status.Phase, d.Reason, d.Message)
			}
		})
	}
}

// An import-mode snapshot has no disk to capture — spec forbids naming one — so this controller only
// reports the core's progress. See the vmsnapshot controller test for the shared reasoning.
func TestReconcile_ImportModeOnlyMirrorsTheCoreProgress(t *testing.T) {
	tests := []struct {
		name      string
		bound     string
		ready     bool
		wantPhase v1alpha2.VirtualDiskSnapshotPhase
	}{
		{
			name:      "not bound yet",
			wantPhase: v1alpha2.VirtualDiskSnapshotPhasePending,
		},
		{
			name:      "bound, being assembled",
			bound:     "content-1",
			wantPhase: v1alpha2.VirtualDiskSnapshotPhaseInProgress,
		},
		{
			name:      "the core says the content is ready",
			bound:     "content-1",
			ready:     true,
			wantPhase: v1alpha2.VirtualDiskSnapshotPhaseReady,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vds := &v1alpha2.VirtualDiskSnapshot{
				ObjectMeta: metav1.ObjectMeta{Name: "vds1", Namespace: testNamespace},
				Spec:       v1alpha2.VirtualDiskSnapshotSpec{Mode: v1alpha2.UnifiedSnapshotterModeImport},
				Status:     v1alpha2.VirtualDiskSnapshotStatus{BoundSnapshotContentName: tt.bound},
			}
			if tt.ready {
				vds.Status.Conditions = []metav1.Condition{{
					Type:   v1alpha2.UnifiedSnapshotterConditionReady,
					Status: metav1.ConditionTrue,
					Reason: "Imported",
				}}
			}
			r := newTestReconciler(t, vds)

			if _, err := r.Reconcile(context.Background(), ctrl.Request{
				NamespacedName: types.NamespacedName{Namespace: testNamespace, Name: "vds1"},
			}); err != nil {
				t.Fatal(err)
			}

			got := getVDS(t, r.Client)
			if got.Status.Phase != tt.wantPhase {
				t.Errorf("phase = %q, want %q", got.Status.Phase, tt.wantPhase)
			}
			if got.Status.Consistent != nil {
				t.Errorf("consistent = %v, want it left unset on an import", *got.Status.Consistent)
			}
			if got.Status.CaptureState != nil {
				t.Errorf("captureState = %+v; an import must not enter the capture state machine", got.Status.CaptureState)
			}
			// These describe the disk as it was at capture time and default a NEW disk cloned from this
			// snapshot. Nothing reachable from here knows them, and a guess would be read as a fact.
			if got.Status.StorageClassName != "" || got.Status.PersistentVolumeClaimSize != "" {
				t.Errorf("storageClassName = %q, persistentVolumeClaimSize = %q; both must stay unset on an import",
					got.Status.StorageClassName, got.Status.PersistentVolumeClaimSize)
			}
		})
	}
}
