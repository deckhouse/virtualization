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

package vmsnapshot

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/deckhouse/deckhouse/pkg/log"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

func newFullTestReconciler(t *testing.T, objs ...client.Object) *Reconciler {
	t.Helper()
	c := fake.NewClientBuilder().WithScheme(newTestScheme(t)).WithObjects(objs...).WithStatusSubresource(&v1alpha2.VirtualMachineSnapshot{}).Build()
	return &Reconciler{Client: c, APIReader: c, Freezer: service.NewSnapshotService(nil, c, nil), Log: log.NewNop()}
}

func getVMS(t *testing.T, c client.Client) *v1alpha2.VirtualMachineSnapshot {
	t.Helper()
	vms := &v1alpha2.VirtualMachineSnapshot{}
	if err := c.Get(context.Background(), types.NamespacedName{Namespace: testNamespace, Name: "vms1"}, vms); err != nil {
		t.Fatalf("get VirtualMachineSnapshot: %v", err)
	}
	return vms
}

func TestReconcile_DeletionTimestampIsANoOp(t *testing.T) {
	vms := &v1alpha2.VirtualMachineSnapshot{
		ObjectMeta: metav1.ObjectMeta{
			Name: "vms1", Namespace: testNamespace,
			Annotations:       map[string]string{v1alpha2.AnnUseUnifiedSnapshotter: ""},
			DeletionTimestamp: ptr.To(metav1.Now()),
			Finalizers:        []string{"kubernetes"},
		},
	}
	r := newFullTestReconciler(t, vms)

	res, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Namespace: testNamespace, Name: "vms1"}})
	if err != nil {
		t.Fatal(err)
	}
	if res != (ctrl.Result{}) {
		t.Fatalf("expected a no-op result, got %+v", res)
	}
}

func TestReconcile_MissingAnnotationIsANoOp(t *testing.T) {
	vms := &v1alpha2.VirtualMachineSnapshot{
		ObjectMeta: metav1.ObjectMeta{Name: "vms1", Namespace: testNamespace},
	}
	r := newFullTestReconciler(t, vms)

	res, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Namespace: testNamespace, Name: "vms1"}})
	if err != nil {
		t.Fatal(err)
	}
	if res != (ctrl.Result{}) {
		t.Fatalf("expected a no-op result, got %+v", res)
	}
	if getVMS(t, r.Client).Status.Phase != "" {
		t.Fatal("expected phase to stay empty when the annotation is absent")
	}
}

func TestReconcile_BootstrapsPendingPhase(t *testing.T) {
	vms := &v1alpha2.VirtualMachineSnapshot{
		ObjectMeta: metav1.ObjectMeta{Name: "vms1", Namespace: testNamespace, Annotations: map[string]string{v1alpha2.AnnUseUnifiedSnapshotter: ""}},
		Spec:       v1alpha2.VirtualMachineSnapshotSpec{VirtualMachineName: "vm1"},
	}
	r := newFullTestReconciler(t, vms)

	res, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Namespace: testNamespace, Name: "vms1"}})
	if err != nil {
		t.Fatal(err)
	}
	if res != (ctrl.Result{Requeue: true}) {
		t.Fatalf("expected Requeue, got %+v", res)
	}
	if got := getVMS(t, r.Client).Status.Phase; got != v1alpha2.VirtualMachineSnapshotPhasePending {
		t.Fatalf("got phase %q, want Pending", got)
	}
}

func TestReconcile_WaitsWhenSourceVirtualMachineIsMissing(t *testing.T) {
	vms := &v1alpha2.VirtualMachineSnapshot{
		ObjectMeta: metav1.ObjectMeta{Name: "vms1", Namespace: testNamespace, Annotations: map[string]string{v1alpha2.AnnUseUnifiedSnapshotter: ""}},
		Spec:       v1alpha2.VirtualMachineSnapshotSpec{VirtualMachineName: "missing-vm"},
		Status:     v1alpha2.VirtualMachineSnapshotStatus{Phase: v1alpha2.VirtualMachineSnapshotPhasePending},
	}
	r := newFullTestReconciler(t, vms)

	res, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Namespace: testNamespace, Name: "vms1"}})
	if err != nil {
		t.Fatal(err)
	}
	if res.RequeueAfter != requeueAfter {
		t.Fatalf("got RequeueAfter %v, want %v", res.RequeueAfter, requeueAfter)
	}

	got := getVMS(t, r.Client)
	if got.Status.CaptureState == nil || got.Status.CaptureState.DomainSpecificController == nil {
		t.Fatal("expected DomainSpecificController capture state to be published")
	}
	if got.Status.CaptureState.DomainSpecificController.Message == "" {
		t.Fatal("expected a waiting message explaining the missing VirtualMachine")
	}
}

func TestPatchStatus_OnlyTouchesOwnedFields(t *testing.T) {
	vms := &v1alpha2.VirtualMachineSnapshot{
		ObjectMeta: metav1.ObjectMeta{Name: "vms1", Namespace: testNamespace},
		Status: v1alpha2.VirtualMachineSnapshotStatus{
			// A core/SDK-owned field this controller must never clobber via its own patch.
			CaptureState: &v1alpha2.UnifiedSnapshotterCaptureState{
				CommonController: &v1alpha2.UnifiedSnapshotterCommonCaptureState{ManifestCaptured: ptr.To(true)},
			},
		},
	}
	r := newFullTestReconciler(t, vms)

	vms.Status.Phase = v1alpha2.VirtualMachineSnapshotPhaseReady
	vms.Status.Consistent = ptr.To(true)
	if err := r.patchStatus(context.Background(), vms); err != nil {
		t.Fatal(err)
	}

	got := getVMS(t, r.Client)
	if got.Status.Phase != v1alpha2.VirtualMachineSnapshotPhaseReady {
		t.Fatalf("got phase %q, want Ready", got.Status.Phase)
	}
	if got.Status.Consistent == nil || !*got.Status.Consistent {
		t.Fatal("expected Consistent to be patched to true")
	}
	if got.Status.CaptureState == nil || got.Status.CaptureState.CommonController == nil || got.Status.CaptureState.CommonController.ManifestCaptured == nil || !*got.Status.CaptureState.CommonController.ManifestCaptured {
		t.Fatal("expected the core-owned captureState.commonController to survive the domain controller's own status patch untouched")
	}
}

func TestPlanChildren(t *testing.T) {
	r := &Reconciler{}
	vms := &v1alpha2.VirtualMachineSnapshot{
		ObjectMeta: metav1.ObjectMeta{Name: "vms1", Namespace: testNamespace, UID: types.UID("vms-uid-1")},
		Spec:       v1alpha2.VirtualMachineSnapshotSpec{RequiredConsistency: true},
	}
	vm := &v1alpha2.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{Name: "vm1", Namespace: testNamespace},
		Status: v1alpha2.VirtualMachineStatus{
			BlockDeviceRefs: []v1alpha2.BlockDeviceStatusRef{
				{Kind: v1alpha2.DiskDevice, Name: "disk1"},
				{Kind: v1alpha2.DiskDevice, Name: "disk2"},
				{Kind: v1alpha2.ImageDevice, Name: "image1"}, // not a disk: must be skipped
			},
		},
	}

	children := r.planChildren(vms, vm)
	if len(children) != 2 {
		t.Fatalf("expected exactly 2 disk children, got %d: %+v", len(children), children)
	}

	wantNames := map[string]bool{
		"disk1-" + string(vms.UID): false,
		"disk2-" + string(vms.UID): false,
	}
	for _, c := range children {
		vds, ok := c.Object.(*v1alpha2.VirtualDiskSnapshot)
		if !ok {
			t.Fatalf("expected a *v1alpha2.VirtualDiskSnapshot child, got %T", c.Object)
		}
		if _, ok := wantNames[vds.Name]; !ok {
			t.Fatalf("unexpected child name %q", vds.Name)
		}
		wantNames[vds.Name] = true
		if _, ok := vds.Annotations[v1alpha2.AnnUseUnifiedSnapshotter]; !ok {
			t.Fatalf("expected child %q to carry the unified-snapshotter annotation so it's driven by this same controller family", vds.Name)
		}
		if !vds.Spec.RequiredConsistency {
			t.Fatalf("expected child %q to inherit RequiredConsistency from the parent spec", vds.Name)
		}
	}
	for name, seen := range wantNames {
		if !seen {
			t.Fatalf("expected a child named %q, none was produced", name)
		}
	}
}
