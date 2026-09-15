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
	"errors"
	"testing"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	virtv1 "kubevirt.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	"github.com/deckhouse/deckhouse/pkg/log"
	"github.com/deckhouse/state-snapshotter/pkg/snapshotsdk"
	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmscondition"
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

// The unified controllers are registered only where UNIFIED_SNAPSHOTTER_PRESENT is set, so an object
// nobody pinned is theirs: taking a snapshot through them no longer needs an opt-in annotation.
func TestReconcile_UnpinnedObjectIsTakenOver(t *testing.T) {
	vms := &v1alpha2.VirtualMachineSnapshot{
		ObjectMeta: metav1.ObjectMeta{Name: "vms1", Namespace: testNamespace},
	}
	r := newFullTestReconciler(t, vms)

	if _, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Namespace: testNamespace, Name: "vms1"}}); err != nil {
		t.Fatal(err)
	}
	if got := getVMS(t, r.Client).Status.Phase; got != v1alpha2.VirtualMachineSnapshotPhasePending {
		t.Fatalf("expected the reconciler to take an unpinned object over, got phase %q", got)
	}
}

// Mirror image of the built-in controller's guard: exactly one of the two drives any object.
func TestReconcile_BuiltInPinIsANoOp(t *testing.T) {
	vms := &v1alpha2.VirtualMachineSnapshot{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "vms1",
			Namespace:   testNamespace,
			Annotations: map[string]string{v1alpha2.AnnUseBuiltInSnapshotter: ""},
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
	if getVMS(t, r.Client).Status.Phase != "" {
		t.Fatal("expected phase to stay empty for an object pinned to the built-in mechanism")
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
	r := newFullTestReconciler(t,
		&v1alpha2.VirtualDisk{ObjectMeta: metav1.ObjectMeta{Name: "disk1", Namespace: testNamespace, UID: types.UID("disk-uid-1")}},
		&v1alpha2.VirtualDisk{ObjectMeta: metav1.ObjectMeta{Name: "disk2", Namespace: testNamespace, UID: types.UID("disk-uid-2")}},
	)
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

	children, excluded, err := r.planChildren(context.Background(), vms, vm)
	if err != nil {
		t.Fatalf("planChildren: %v", err)
	}
	if len(excluded) != 0 {
		t.Fatalf("expected nothing excluded, got %+v", excluded)
	}
	if len(children) != 2 {
		t.Fatalf("expected exactly 2 disk children, got %d: %+v", len(children), children)
	}

	wantNames := map[string]bool{
		snapshotsdk.ChildSnapshotName(vms.UID, types.UID("disk-uid-1")): false,
		snapshotsdk.ChildSnapshotName(vms.UID, types.UID("disk-uid-2")): false,
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
		// No mechanism annotation: a child resolves its mechanism from this parent's captureState, so
		// that a controller restart or an upgrade mid-capture cannot separate the two.
		if len(vds.Annotations) != 0 {
			t.Fatalf("expected child %q to carry no mechanism annotation, got %v", vds.Name, vds.Annotations)
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

func TestChildrenSettled(t *testing.T) {
	oneChild := []v1alpha2.VirtualDiskSnapshot{{
		ObjectMeta: metav1.ObjectMeta{Name: "child", Namespace: testNamespace},
	}}

	tests := []struct {
		name     string
		children []v1alpha2.VirtualDiskSnapshot
		cs       snapshotsdk.CoreCaptureState
		want     bool
	}{
		// A machine with no disks, or one whose every disk was vetoed, declares no children — the core
		// leaves the latch nil for it, and waiting on that would hold the freeze until the deadline.
		// childVirtualDiskSnapshots is what makes this emptiness trustworthy.
		{name: "no children, nil latch", want: true},
		{name: "child pending", children: oneChild, cs: snapshotsdk.CoreCaptureState{}, want: false},
		{name: "child not settled", children: oneChild, cs: snapshotsdk.CoreCaptureState{ChildrenSettled: ptr.To(false)}, want: false},
		{name: "child settled", children: oneChild, cs: snapshotsdk.CoreCaptureState{ChildrenSettled: ptr.To(true)}, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := childrenSettled(tt.children, tt.cs); got != tt.want {
				t.Errorf("childrenSettled() = %v, want %v", got, tt.want)
			}
		})
	}
}

// planningGateVM is a VirtualMachine whose manifest target set cannot be resolved: it references a
// hotplugged VirtualMachineBlockDeviceAttachment that does not exist, so planManifestTargets returns
// errSourceNotReady. It carries no disks, so planChildren yields an empty (and therefore never frozen-
// growth) child set.
func planningGateVM() *v1alpha2.VirtualMachine {
	return &v1alpha2.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{Name: "vm1", Namespace: testNamespace},
		Status: v1alpha2.VirtualMachineStatus{
			BlockDeviceRefs: []v1alpha2.BlockDeviceStatusRef{{
				Kind:                                    v1alpha2.ImageDevice,
				Name:                                    "cdrom",
				Hotplugged:                              true,
				VirtualMachineBlockDeviceAttachmentName: "missing-vmbda",
			}},
		},
	}
}

func planningGateVMS(mcrName string, manifestCaptured *bool) *v1alpha2.VirtualMachineSnapshot {
	vms := &v1alpha2.VirtualMachineSnapshot{
		ObjectMeta: metav1.ObjectMeta{
			Name: "vms1", Namespace: testNamespace,
			Annotations: map[string]string{v1alpha2.AnnUseUnifiedSnapshotter: ""},
			Finalizers:  []string{v1alpha2.FinalizerVMSnapshotCleanup},
		},
		Spec: v1alpha2.VirtualMachineSnapshotSpec{VirtualMachineName: "vm1"},
		Status: v1alpha2.VirtualMachineSnapshotStatus{
			Phase: v1alpha2.VirtualMachineSnapshotPhaseInProgress,
			CaptureState: &v1alpha2.UnifiedSnapshotterCaptureState{
				DomainSpecificController: &v1alpha2.UnifiedSnapshotterDomainCaptureState{
					Phase:                      v1alpha2.UnifiedSnapshotterPhase(snapshotsdk.PhasePlanned),
					ManifestCaptureRequestName: mcrName,
				},
				CommonController: &v1alpha2.UnifiedSnapshotterCommonCaptureState{
					ManifestCaptured: manifestCaptured,
				},
			},
		},
	}
	return vms
}

// TestReconcile_ManifestLegIsNotRePlannedOnceEstablished pins the ManifestCaptureNeeded gate. The MCR's
// Targets are immutable once created, so a reference that disappears AFTER the plan was committed must
// not be re-derived — before the gate, the re-derivation returned errSourceNotReady and waitForSource
// turned it into a terminal GraphPlanningFailed, destroying a capture whose legs were already done.
func TestReconcile_ManifestLegIsNotRePlannedOnceEstablished(t *testing.T) {
	vms := planningGateVMS("nss-mcr-established", ptr.To(true))
	r := newFullTestReconciler(t, vms, planningGateVM())

	if _, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Namespace: testNamespace, Name: "vms1"}}); err != nil {
		t.Fatal(err)
	}

	got := getVMS(t, r.Client)
	if d := got.Status.CaptureState.DomainSpecificController; d.Phase == v1alpha2.UnifiedSnapshotterPhase(snapshotsdk.PhaseFailed) {
		t.Fatalf("capture was failed after the manifest leg was established: reason=%q message=%q", d.Reason, d.Message)
	}
	if got.Status.Phase == v1alpha2.VirtualMachineSnapshotPhaseFailed {
		t.Fatalf("VMS phase went Failed, want the established capture to survive")
	}
}

// TestReconcile_ManifestLegStillPlannedWhileNeeded is the negative half of the gate: with no MCR name
// published the leg does still need planning, so the missing reference must be surfaced as a planning
// wait/failure rather than silently skipped.
func TestReconcile_ManifestLegStillPlannedWhileNeeded(t *testing.T) {
	vms := planningGateVMS("", nil)
	r := newFullTestReconciler(t, vms, planningGateVM())

	if _, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Namespace: testNamespace, Name: "vms1"}}); err != nil {
		t.Fatal(err)
	}

	d := getVMS(t, r.Client).Status.CaptureState.DomainSpecificController
	if d.Message == "" {
		t.Fatal("expected the unresolvable manifest target to be reported")
	}
}

// TestReconcile_TransientChildrenErrorIsRetriedNotFatal pins the sentinel-based classification of
// EnsureChildren failures. Only ErrChildrenSetFrozen (and a Conflict, which requeues) is a reason to burn
// the snapshot; anything else — an API timeout, a not-yet-propagated RBAC rule — must go back on the
// workqueue. Before the split, every non-Conflict error became a terminal GraphPlanningFailed.
func TestReconcile_TransientChildrenErrorIsRetriedNotFatal(t *testing.T) {
	vms := &v1alpha2.VirtualMachineSnapshot{
		ObjectMeta: metav1.ObjectMeta{
			Name: "vms1", Namespace: testNamespace,
			Annotations: map[string]string{v1alpha2.AnnUseUnifiedSnapshotter: ""},
			Finalizers:  []string{v1alpha2.FinalizerVMSnapshotCleanup},
		},
		Spec:   v1alpha2.VirtualMachineSnapshotSpec{VirtualMachineName: "vm1"},
		Status: v1alpha2.VirtualMachineSnapshotStatus{Phase: v1alpha2.VirtualMachineSnapshotPhaseInProgress},
	}
	vm := &v1alpha2.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{Name: "vm1", Namespace: testNamespace},
		Status: v1alpha2.VirtualMachineStatus{
			BlockDeviceRefs: []v1alpha2.BlockDeviceStatusRef{{Kind: v1alpha2.DiskDevice, Name: "vd1"}},
		},
	}
	vd := &v1alpha2.VirtualDisk{ObjectMeta: metav1.ObjectMeta{Name: "vd1", Namespace: testNamespace, UID: "vd1-uid"}}

	boom := apierrors.NewInternalError(errors.New("etcd is having a moment"))
	c := fake.NewClientBuilder().
		WithScheme(newTestScheme(t)).
		WithObjects(vms, vm, vd).
		WithStatusSubresource(&v1alpha2.VirtualMachineSnapshot{}).
		WithInterceptorFuncs(interceptor.Funcs{
			Create: func(ctx context.Context, cl client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
				if _, ok := obj.(*v1alpha2.VirtualDiskSnapshot); ok {
					return boom
				}
				return cl.Create(ctx, obj, opts...)
			},
		}).
		Build()
	r := &Reconciler{Client: c, APIReader: c, Freezer: service.NewSnapshotService(nil, c, nil), Log: log.NewNop()}

	_, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Namespace: testNamespace, Name: "vms1"}})
	if err == nil {
		t.Fatal("expected the transient error to be returned so the workqueue retries it")
	}

	got := getVMS(t, c)
	if got.Status.Phase == v1alpha2.VirtualMachineSnapshotPhaseFailed {
		t.Fatal("VMS phase went Failed on a transient error")
	}
	if cs := got.Status.CaptureState; cs != nil && cs.DomainSpecificController != nil &&
		cs.DomainSpecificController.Phase == v1alpha2.UnifiedSnapshotterPhase(snapshotsdk.PhaseFailed) {
		t.Fatalf("capture was failed on a transient error: reason=%q", cs.DomainSpecificController.Reason)
	}
}

// A snapshot whose spec stops resolving must still be failed through the normal path: it first gets its
// Pending bootstrap (the cluster default handed it to this controller), and only then goes Failed with
// reasonInvalidSource. Failing it before the bootstrap left it without a phase and out of
// reconcileCaptureFailed's reach.
func TestReconcile_UnresolvableSourceFailsAfterBootstrap(t *testing.T) {
	vms := &v1alpha2.VirtualMachineSnapshot{
		ObjectMeta: metav1.ObjectMeta{Name: "vms1", Namespace: testNamespace},
		Spec: v1alpha2.VirtualMachineSnapshotSpec{
			// A sourceRef of a kind this controller does not capture: SourceVirtualMachineName() is "".
			SourceRef: &v1alpha2.UnifiedSnapshotterSpecSourceRef{
				APIVersion: v1alpha2.SchemeGroupVersion.String(),
				Kind:       "Pod",
				Name:       "not-a-vm",
			},
		},
	}
	r := newFullTestReconciler(t, vms)
	req := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: testNamespace, Name: "vms1"}}

	if _, err := r.Reconcile(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	if got := getVMS(t, r.Client).Status.Phase; got != v1alpha2.VirtualMachineSnapshotPhasePending {
		t.Fatalf("first reconcile should bootstrap the phase, got %q", got)
	}

	if _, err := r.Reconcile(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	got := getVMS(t, r.Client)
	if got.Status.Phase != v1alpha2.VirtualMachineSnapshotPhaseFailed {
		t.Fatalf("second reconcile should fail the capture, got phase %q", got.Status.Phase)
	}
	d := got.Status.CaptureState.DomainSpecificController
	if d.Phase != v1alpha2.UnifiedSnapshotterPhase(snapshotsdk.PhaseFailed) || d.Reason != reasonInvalidSource {
		t.Fatalf("got domain phase %q reason %q, want Failed/%s", d.Phase, d.Reason, reasonInvalidSource)
	}
}

// cleanupSource is what keeps a wedged snapshot recoverable: spec is mutable, so it can stop naming the
// VirtualMachine whose filesystem this snapshot froze, and status.sourceRef is the only remaining record
// of it.
func TestCleanupSource_FallsBackToTheRecordedSource(t *testing.T) {
	vm := &v1alpha2.VirtualMachine{ObjectMeta: metav1.ObjectMeta{Name: "vm1", Namespace: testNamespace}}
	recorded := func(kind string) *v1alpha2.UnifiedSnapshotterSourceRef {
		return &v1alpha2.UnifiedSnapshotterSourceRef{
			APIVersion: v1alpha2.SchemeGroupVersion.String(),
			Kind:       kind,
			Name:       "vm1",
			Namespace:  testNamespace,
		}
	}

	tests := []struct {
		name     string
		spec     v1alpha2.VirtualMachineSnapshotSpec
		status   *v1alpha2.UnifiedSnapshotterSourceRef
		wantName string
	}{
		{name: "spec resolves", spec: v1alpha2.VirtualMachineSnapshotSpec{VirtualMachineName: "vm1"}, wantName: "vm1"},
		{name: "spec empty, status records the VM", status: recorded(v1alpha2.VirtualMachineKind), wantName: "vm1"},
		{name: "spec empty, nothing recorded", wantName: ""},
		{name: "recorded ref is a foreign kind", status: recorded("Pod"), wantName: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vms := &v1alpha2.VirtualMachineSnapshot{
				ObjectMeta: metav1.ObjectMeta{Name: "vms1", Namespace: testNamespace},
				Spec:       tt.spec,
				Status:     v1alpha2.VirtualMachineSnapshotStatus{SourceRef: tt.status},
			}
			r := newFullTestReconciler(t, vms, vm)

			got, _, err := r.cleanupSource(context.Background(), vms)
			if err != nil {
				t.Fatal(err)
			}
			switch {
			case tt.wantName == "" && got != nil:
				t.Fatalf("expected no VirtualMachine to unfreeze, got %q", got.Name)
			case tt.wantName != "" && got == nil:
				t.Fatalf("expected to resolve VirtualMachine %q, got none", tt.wantName)
			case got != nil && got.Name != tt.wantName:
				t.Fatalf("resolved %q, want %q", got.Name, tt.wantName)
			}
		})
	}
}

// The refs write EnsureChildren fail-closed dropped (its documented TOCTOU belt) leaves the child CRs
// created but status.childrenSnapshotRefs empty. Read as "childless", that made this controller unfreeze
// the guest, declare the snapshot consistent and mark it Ready while the children were still capturing.
// The empty set must therefore be answered from the cluster, by ownerRef.
func TestChildVirtualDiskSnapshots_EmptyRefsFallBackToOwnership(t *testing.T) {
	vms := &v1alpha2.VirtualMachineSnapshot{
		ObjectMeta: metav1.ObjectMeta{Name: "vms1", Namespace: testNamespace, UID: "vms1-uid"},
	}
	owned := &v1alpha2.VirtualDiskSnapshot{
		ObjectMeta: metav1.ObjectMeta{
			Name: "child-of-vms1", Namespace: testNamespace,
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: v1alpha2.SchemeGroupVersion.String(),
				Kind:       v1alpha2.VirtualMachineSnapshotKind,
				Name:       "vms1",
				UID:        "vms1-uid",
			}},
		},
	}
	foreign := &v1alpha2.VirtualDiskSnapshot{
		ObjectMeta: metav1.ObjectMeta{
			Name: "child-of-someone-else", Namespace: testNamespace,
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: v1alpha2.SchemeGroupVersion.String(),
				Kind:       v1alpha2.VirtualMachineSnapshotKind,
				Name:       "vms2",
				UID:        "vms2-uid",
			}},
		},
	}
	standalone := &v1alpha2.VirtualDiskSnapshot{
		ObjectMeta: metav1.ObjectMeta{Name: "standalone", Namespace: testNamespace},
	}
	r := newFullTestReconciler(t, vms, owned, foreign, standalone)

	got, err := r.childVirtualDiskSnapshots(context.Background(), vms)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Name != "child-of-vms1" {
		names := make([]string, 0, len(got))
		for _, c := range got {
			names = append(names, c.Name)
		}
		t.Fatalf("resolved %v, want exactly [child-of-vms1]", names)
	}
	if childrenSettled(got, snapshotsdk.CoreCaptureState{}) {
		t.Fatal("an unpublished but existing child must not read as settled")
	}
	if childrenAreConsistent(got) {
		t.Fatal("a child that has not vouched for its disk must not read as consistent")
	}
}

// A childless node stays childless: the ownership fallback must not invent children out of unrelated
// VirtualDiskSnapshots living in the same namespace, or every disk-less VM snapshot would hold its
// freeze until the settle deadline.
func TestChildVirtualDiskSnapshots_ChildlessStaysChildless(t *testing.T) {
	vms := &v1alpha2.VirtualMachineSnapshot{
		ObjectMeta: metav1.ObjectMeta{Name: "vms1", Namespace: testNamespace, UID: "vms1-uid"},
	}
	unrelated := &v1alpha2.VirtualDiskSnapshot{
		ObjectMeta: metav1.ObjectMeta{Name: "unrelated", Namespace: testNamespace},
	}
	r := newFullTestReconciler(t, vms, unrelated)

	got, err := r.childVirtualDiskSnapshots(context.Background(), vms)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("expected no children, got %d", len(got))
	}
	if !childrenSettled(got, snapshotsdk.CoreCaptureState{}) {
		t.Fatal("a genuinely childless node must read as settled")
	}
}

// A published ref whose object is gone must not shrink the set: it cannot report itself terminal, and it
// cannot vouch for its disk. It also must not stall the wait forever — with no child carrying a creation
// timestamp the deadline falls back to this node's own age.
func TestChildVirtualDiskSnapshots_MissingPublishedChildStillCounts(t *testing.T) {
	vms := &v1alpha2.VirtualMachineSnapshot{
		ObjectMeta: metav1.ObjectMeta{
			Name: "vms1", Namespace: testNamespace, UID: "vms1-uid",
			CreationTimestamp: metav1.NewTime(time.Now().Add(-2 * childrenSettleDeadline)),
		},
		Status: v1alpha2.VirtualMachineSnapshotStatus{
			ChildrenSnapshotRefs: []v1alpha2.UnifiedSnapshotterChildRef{{
				APIVersion: v1alpha2.SchemeGroupVersion.String(),
				Kind:       v1alpha2.VirtualDiskSnapshotKind,
				Name:       "vanished",
			}},
		},
	}
	r := newFullTestReconciler(t, vms)

	got, err := r.childVirtualDiskSnapshots(context.Background(), vms)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Name != "vanished" {
		t.Fatalf("expected the vanished child to still count, got %+v", got)
	}
	if childrenSettled(got, snapshotsdk.CoreCaptureState{}) {
		t.Fatal("a vanished child must not read as settled")
	}
	waited, pending := childrenWaitProgress(vms, got)
	if waited <= childrenSettleDeadline {
		t.Fatalf("waited %v, expected the node's own age to end the wait", waited)
	}
	if len(pending) != 1 || pending[0] != "vanished" {
		t.Fatalf("pending = %v, want [vanished]", pending)
	}
}

// Once the node has declared barrier 1 the child set is frozen, and re-deriving it can only be a no-op or
// a terminal ErrChildrenSetFrozen. This pins that it is not re-derived at all: the source disk is gone, so
// planChildren would return errSourceNotReady and burn an already-captured snapshot.
func TestReconcile_ChildrenAreNotRePlannedOnceFrozen(t *testing.T) {
	vms := &v1alpha2.VirtualMachineSnapshot{
		ObjectMeta: metav1.ObjectMeta{
			Name: "vms1", Namespace: testNamespace, UID: "vms1-uid",
			Annotations: map[string]string{v1alpha2.AnnUseUnifiedSnapshotter: ""},
			Finalizers:  []string{v1alpha2.FinalizerVMSnapshotCleanup},
		},
		Spec: v1alpha2.VirtualMachineSnapshotSpec{VirtualMachineName: "vm1"},
		Status: v1alpha2.VirtualMachineSnapshotStatus{
			Phase: v1alpha2.VirtualMachineSnapshotPhaseInProgress,
			CaptureState: &v1alpha2.UnifiedSnapshotterCaptureState{
				DomainSpecificController: &v1alpha2.UnifiedSnapshotterDomainCaptureState{
					Phase:                      v1alpha2.UnifiedSnapshotterPhase(snapshotsdk.PhasePlanned),
					ManifestCaptureRequestName: "nss-mcr-established",
				},
				CommonController: &v1alpha2.UnifiedSnapshotterCommonCaptureState{
					ManifestCaptured: ptr.To(true),
					ChildrenSettled:  ptr.To(true),
				},
			},
			ChildrenSnapshotRefs: []v1alpha2.UnifiedSnapshotterChildRef{{
				APIVersion: v1alpha2.SchemeGroupVersion.String(),
				Kind:       v1alpha2.VirtualDiskSnapshotKind,
				Name:       "child",
			}},
		},
	}
	// The VM still lists a disk device, but the VirtualDisk itself is gone: planChildren would fail here.
	vm := &v1alpha2.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{Name: "vm1", Namespace: testNamespace},
		Status: v1alpha2.VirtualMachineStatus{
			BlockDeviceRefs: []v1alpha2.BlockDeviceStatusRef{{Kind: v1alpha2.DiskDevice, Name: "vanished-disk"}},
		},
	}
	child := &v1alpha2.VirtualDiskSnapshot{
		ObjectMeta: metav1.ObjectMeta{Name: "child", Namespace: testNamespace},
		Status: v1alpha2.VirtualDiskSnapshotStatus{
			Phase:      v1alpha2.VirtualDiskSnapshotPhaseReady,
			Consistent: ptr.To(true),
		},
	}
	r := newFullTestReconciler(t, vms, vm, child)

	if _, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Namespace: testNamespace, Name: "vms1"}}); err != nil {
		t.Fatal(err)
	}

	got := getVMS(t, r.Client)
	if d := got.Status.CaptureState.DomainSpecificController; d.Phase == v1alpha2.UnifiedSnapshotterPhase(snapshotsdk.PhaseFailed) {
		t.Fatalf("frozen plan was re-derived and failed: reason=%q message=%q", d.Reason, d.Message)
	}
	if got.Status.Phase == v1alpha2.VirtualMachineSnapshotPhaseFailed {
		t.Fatal("VMS phase went Failed, want the captured snapshot to survive a disk that vanished after the barrier")
	}
}

// The wait for a missing VirtualMachine is unbounded on purpose — nothing is frozen yet — so the only
// thing that makes it diagnosable is what it publishes. A message alone is not enough: the reason is the
// machine-readable half an alert or a UI filter can key on, and conditions here are core-owned.
func TestReconcile_MissingVirtualMachineReportsWhyItWaits(t *testing.T) {
	vms := &v1alpha2.VirtualMachineSnapshot{
		ObjectMeta: metav1.ObjectMeta{
			Name: "vms1", Namespace: testNamespace,
			Annotations: map[string]string{v1alpha2.AnnUseUnifiedSnapshotter: ""},
			// Old enough that a budget anchored on this snapshot alone would already be spent.
			CreationTimestamp: metav1.NewTime(time.Now().Add(-time.Hour)),
		},
		Spec:   v1alpha2.VirtualMachineSnapshotSpec{VirtualMachineName: "not-yet-created"},
		Status: v1alpha2.VirtualMachineSnapshotStatus{Phase: v1alpha2.VirtualMachineSnapshotPhasePending},
	}
	r := newFullTestReconciler(t, vms)

	res, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Namespace: testNamespace, Name: "vms1"}})
	if err != nil {
		t.Fatal(err)
	}
	if res.RequeueAfter != requeueAfter {
		t.Fatalf("got RequeueAfter %v, want %v — the wait must not give up", res.RequeueAfter, requeueAfter)
	}

	got := getVMS(t, r.Client)
	if got.Status.Phase == v1alpha2.VirtualMachineSnapshotPhaseFailed {
		t.Fatal("an hour of waiting for a VirtualMachine must not fail the snapshot")
	}
	d := got.Status.CaptureState.DomainSpecificController
	if d.Phase != v1alpha2.UnifiedSnapshotterPhase(snapshotsdk.PhasePlanning) {
		t.Fatalf("domain phase = %q, want Planning", d.Phase)
	}
	if d.Reason != string(vmscondition.WaitingForTheVirtualMachine) {
		t.Fatalf("domain reason = %q, want %q", d.Reason, vmscondition.WaitingForTheVirtualMachine)
	}
	if d.Message == "" {
		t.Fatal("expected a message naming the missing VirtualMachine")
	}
}

// planningStartedAt is what keeps the bounded planning budget honest once the unbounded wait above has
// run: the budget starts when planning first became possible, not when the snapshot was created.
func TestPlanningStartedAt(t *testing.T) {
	base := time.Now().Add(-time.Hour).Truncate(time.Second)
	vms := &v1alpha2.VirtualMachineSnapshot{
		ObjectMeta: metav1.ObjectMeta{CreationTimestamp: metav1.NewTime(base)},
	}

	tests := []struct {
		name string
		vm   *v1alpha2.VirtualMachine
		want time.Time
	}{
		{name: "no virtual machine yet", vm: nil, want: base},
		{
			name: "machine predates the snapshot",
			vm:   &v1alpha2.VirtualMachine{ObjectMeta: metav1.ObjectMeta{CreationTimestamp: metav1.NewTime(base.Add(-time.Hour))}},
			want: base,
		},
		{
			name: "machine appeared after the snapshot",
			vm:   &v1alpha2.VirtualMachine{ObjectMeta: metav1.ObjectMeta{CreationTimestamp: metav1.NewTime(base.Add(30 * time.Minute))}},
			want: base.Add(30 * time.Minute),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := planningStartedAt(vms, tt.vm); !got.Equal(tt.want) {
				t.Errorf("planningStartedAt() = %v, want %v", got, tt.want)
			}
		})
	}
}

// freezeStuckFixture is a running VirtualMachine whose guest has a freeze request pending and never
// confirmed: SyncFSFreezeRequest clears the annotation only once fsFreezeStatus reads "frozen", so both
// it and IsFrozen keep reporting ErrUntrustedFilesystemFrozenCondition.
func freezeStuckFixture(t *testing.T, age time.Duration, owners []metav1.OwnerReference) (*Reconciler, ctrl.Request) {
	t.Helper()

	vms := &v1alpha2.VirtualMachineSnapshot{
		ObjectMeta: metav1.ObjectMeta{
			Name: "vms1", Namespace: testNamespace,
			Annotations:       map[string]string{v1alpha2.AnnUseUnifiedSnapshotter: ""},
			Finalizers:        []string{v1alpha2.FinalizerVMSnapshotCleanup},
			OwnerReferences:   owners,
			CreationTimestamp: metav1.NewTime(time.Now().Add(-age)),
		},
		Spec: v1alpha2.VirtualMachineSnapshotSpec{VirtualMachineName: "vm1", RequiredConsistency: true},
		Status: v1alpha2.VirtualMachineSnapshotStatus{
			Phase: v1alpha2.VirtualMachineSnapshotPhaseInProgress,
		},
	}
	vm := &v1alpha2.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{
			Name: "vm1", Namespace: testNamespace,
			CreationTimestamp: metav1.NewTime(time.Now().Add(-age)),
		},
	}
	kvvmi := &virtv1.VirtualMachineInstance{
		ObjectMeta: metav1.ObjectMeta{
			Name: "vm1", Namespace: testNamespace,
			Annotations: map[string]string{annotations.AnnVMFilesystemRequest: service.RequestFSFreeze},
		},
		Status: virtv1.VirtualMachineInstanceStatus{Phase: virtv1.Running},
	}

	r := newFullTestReconciler(t, vms, vm, kvvmi)

	return r, ctrl.Request{NamespacedName: types.NamespacedName{Namespace: testNamespace, Name: "vms1"}}
}

var coreSnapshotOwner = []metav1.OwnerReference{{
	APIVersion: "state-snapshotter.deckhouse.io/v1alpha1",
	Kind:       "Snapshot",
	Name:       "namespace-snapshot",
	UID:        "snap-uid",
}}

// Inside the budget the node waits, and it must say what it waits for. Before the published reason the
// only user-visible signal was the core's ManifestCapturePending, which points at the wrong leg.
func TestReconcile_FreezeInFlightReportsWhyItWaits(t *testing.T) {
	r, req := freezeStuckFixture(t, time.Second, nil)

	res, err := r.Reconcile(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if res.RequeueAfter != requeueAfter {
		t.Fatalf("got RequeueAfter %v, want %v", res.RequeueAfter, requeueAfter)
	}

	d := getVMS(t, r.Client).Status.CaptureState.DomainSpecificController
	if d.Reason != string(vmscondition.FileSystemFreezing) {
		t.Fatalf("domain reason = %q, want %q", d.Reason, vmscondition.FileSystemFreezing)
	}
	if d.Message == "" {
		t.Fatal("expected a message naming the virtual machine being waited on")
	}
}

// assertFreezeRequestRetired checks that the abandoned request annotation is gone. Leaving it behind
// would make every later SyncFSFreezeRequest fail with the same sentinel — including releaseFreeze's on
// the way out, which would hold the snapshot short of its terminal phase.
func assertFreezeRequestRetired(t *testing.T, c client.Client) {
	t.Helper()

	kvvmi := &virtv1.VirtualMachineInstance{}
	if err := c.Get(context.Background(), types.NamespacedName{Namespace: testNamespace, Name: "vm1"}, kvvmi); err != nil {
		t.Fatalf("get VirtualMachineInstance: %v", err)
	}
	if req, ok := kvvmi.Annotations[annotations.AnnVMFilesystemRequest]; ok {
		t.Fatalf("freeze request %q was left pending on the guest", req)
	}
}

// The capture fails if spent the wait budget.
// A freeze that the guest never confirms fails the snapshot. The
// abandoned freeze request is retired either way, so it does not wedge the next freeze round-trip.
func TestReconcile_UnconfirmedFreezeFailsTheCapture(t *testing.T) {
	tests := []struct {
		name   string
		owners []metav1.OwnerReference
	}{
		{
			name: "asked for by a user",
		},
		{
			name:   "planned by the core for a namespace snapshot",
			owners: coreSnapshotOwner,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, req := freezeStuckFixture(t, 2*freezeConfirmDeadline, tt.owners)

			if _, err := r.Reconcile(context.Background(), req); err != nil {
				t.Fatal(err)
			}

			d := getVMS(t, r.Client).Status.CaptureState.DomainSpecificController
			if d.Reason != string(vmscondition.PotentiallyInconsistent) {
				t.Fatalf("domain reason = %q, want %q", d.Reason, vmscondition.PotentiallyInconsistent)
			}
			assertFreezeRequestRetired(t, r.Client)
		})
	}
}

// An import-mode snapshot has no capture to drive, so this controller must not start one: no finalizer,
// no source lookup, no planning. All it does is report what the core has got to.
func TestReconcile_ImportModeOnlyMirrorsTheCoreProgress(t *testing.T) {
	tests := []struct {
		name      string
		bound     string
		ready     bool
		wantPhase v1alpha2.VirtualMachineSnapshotPhase
	}{
		{
			name:      "not bound yet",
			wantPhase: v1alpha2.VirtualMachineSnapshotPhasePending,
		},
		{
			name:      "bound, being assembled",
			bound:     "content-1",
			wantPhase: v1alpha2.VirtualMachineSnapshotPhaseInProgress,
		},
		{
			name:      "the core says the content is ready",
			bound:     "content-1",
			ready:     true,
			wantPhase: v1alpha2.VirtualMachineSnapshotPhaseReady,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vms := &v1alpha2.VirtualMachineSnapshot{
				ObjectMeta: metav1.ObjectMeta{Name: "vms1", Namespace: testNamespace},
				Spec:       v1alpha2.VirtualMachineSnapshotSpec{Mode: v1alpha2.UnifiedSnapshotterModeImport},
				Status:     v1alpha2.VirtualMachineSnapshotStatus{BoundSnapshotContentName: tt.bound},
			}
			if tt.ready {
				vms.Status.Conditions = []metav1.Condition{{
					Type:   v1alpha2.UnifiedSnapshotterConditionReady,
					Status: metav1.ConditionTrue,
					Reason: "Imported",
				}}
			}
			r := newFullTestReconciler(t, vms)

			if _, err := r.Reconcile(context.Background(), ctrl.Request{
				NamespacedName: types.NamespacedName{Namespace: testNamespace, Name: "vms1"},
			}); err != nil {
				t.Fatal(err)
			}

			got := getVMS(t, r.Client)
			if got.Status.Phase != tt.wantPhase {
				t.Errorf("phase = %q, want %q", got.Status.Phase, tt.wantPhase)
			}
			if len(got.Finalizers) != 0 {
				t.Errorf("finalizers = %v; an import freezes nothing, so it needs no cleanup finalizer", got.Finalizers)
			}
			// Consistency is a fact about the capture the archive came from, which this controller
			// cannot read. Claiming either answer would misdescribe it.
			if got.Status.Consistent != nil {
				t.Errorf("consistent = %v, want it left unset on an import", *got.Status.Consistent)
			}
			if got.Status.CaptureState != nil {
				t.Errorf("captureState = %+v; an import must not enter the capture state machine", got.Status.CaptureState)
			}
		})
	}
}

// The upload subresource is what records an import's children, and the phase mirror must republish them
// under the name users read (status.virtualDiskSnapshotNames) just as a capture does.
//
// The upload lands while the phase is already InProgress and does not move it, so this starts from
// exactly that state: a reconcile that compared only the phase would return early and publish nothing.
func TestReconcile_ImportModeRepublishesTheUploadedChildren(t *testing.T) {
	vms := &v1alpha2.VirtualMachineSnapshot{
		ObjectMeta: metav1.ObjectMeta{Name: "vms1", Namespace: testNamespace},
		Spec:       v1alpha2.VirtualMachineSnapshotSpec{Mode: v1alpha2.UnifiedSnapshotterModeImport},
		Status: v1alpha2.VirtualMachineSnapshotStatus{
			Phase:                    v1alpha2.VirtualMachineSnapshotPhaseInProgress,
			BoundSnapshotContentName: "content-1",
			ChildrenSnapshotRefs: []v1alpha2.UnifiedSnapshotterChildRef{
				{APIVersion: v1alpha2.SchemeGroupVersion.String(), Kind: v1alpha2.VirtualDiskSnapshotKind, Name: "vds-b"},
				{APIVersion: v1alpha2.SchemeGroupVersion.String(), Kind: v1alpha2.VirtualDiskSnapshotKind, Name: "vds-a"},
			},
		},
	}
	r := newFullTestReconciler(t, vms)

	if _, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Namespace: testNamespace, Name: "vms1"},
	}); err != nil {
		t.Fatal(err)
	}

	got := getVMS(t, r.Client)
	want := []string{"vds-a", "vds-b"}
	if len(got.Status.VirtualDiskSnapshotNames) != len(want) {
		t.Fatalf("virtualDiskSnapshotNames = %v, want %v", got.Status.VirtualDiskSnapshotNames, want)
	}
	for i := range want {
		if got.Status.VirtualDiskSnapshotNames[i] != want[i] {
			t.Errorf("virtualDiskSnapshotNames = %v, want %v", got.Status.VirtualDiskSnapshotNames, want)
			break
		}
	}
}
