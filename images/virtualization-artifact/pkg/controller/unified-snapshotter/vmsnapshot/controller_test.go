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
	"strings"
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
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/deckhouse/deckhouse/pkg/log"
	"github.com/deckhouse/state-snapshotter/pkg/snapshotsdk"
	"github.com/deckhouse/virtualization-controller/pkg/controller/indexer"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/unified-snapshotter/internal/accessclaim"
	coordinationv1alpha1 "github.com/deckhouse/virtualization/api/coordination/v1alpha1"
	"github.com/deckhouse/virtualization/api/coordination/v1alpha1/vmaccondition"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmscondition"
)

func newFullTestReconciler(t *testing.T, objs ...client.Object) *Reconciler {
	t.Helper()
	// The same field indexes the manager registers. Without them a List by MatchingFields fails
	// instead of returning the snapshots, and the freeze code paths that survey a machine's other
	// snapshots would be exercised against an error rather than against their real input.
	c := fake.NewClientBuilder().
		WithScheme(newTestScheme(t)).
		WithObjects(objs...).
		WithStatusSubresource(&v1alpha2.VirtualMachineSnapshot{}, &coordinationv1alpha1.VirtualMachineAccessClaim{}).
		WithIndex(indexer.IndexVMSnapshotByVM()).
		WithIndex(indexer.IndexVDSnapshotByVD()).
		Build()

	return &Reconciler{
		Client:    c,
		APIReader: c,
		Unfreezer: &fakeUnfreezer{},
		Log:       log.NewNop(),
	}
}

// fakeUnfreezer counts the unfreeze calls.
type fakeUnfreezer struct {
	calls int
}

func (f *fakeUnfreezer) Unfreeze(context.Context, string, string, types.UID) error {
	f.calls++
	return nil
}

// accessFixture is a running virtual machine and its snapshot, whose access claim is in phase with the given
// Granted condition.
func accessFixture(t *testing.T, phase coordinationv1alpha1.AccessClaimPhase, reason, message string, objs ...client.Object) (*Reconciler, ctrl.Request) {
	t.Helper()

	vms := &v1alpha2.VirtualMachineSnapshot{
		ObjectMeta: metav1.ObjectMeta{
			Name: "vms1", Namespace: testNamespace, UID: "vms-uid",
			Annotations:       map[string]string{v1alpha2.AnnUseUnifiedSnapshotter: ""},
			Finalizers:        []string{v1alpha2.FinalizerVMSnapshotCleanup},
			CreationTimestamp: metav1.NewTime(time.Now().Add(-time.Minute)),
		},
		Spec: v1alpha2.VirtualMachineSnapshotSpec{VirtualMachineName: "vm1", RequiredConsistency: true},
		Status: v1alpha2.VirtualMachineSnapshotStatus{
			Phase: v1alpha2.VirtualMachineSnapshotPhaseInProgress,
		},
	}
	vm := &v1alpha2.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{
			Name: "vm1", Namespace: testNamespace, UID: "vm-uid",
			CreationTimestamp: metav1.NewTime(time.Now().Add(-time.Hour)),
		},
	}

	r := newFullTestReconciler(t, append(objs, vms, vm, testClaim(vms, phase, reason, message))...)

	return r, ctrl.Request{NamespacedName: types.NamespacedName{Namespace: testNamespace, Name: "vms1"}}
}

// testClaim is the access claim of vms in phase with the given Granted condition.
func testClaim(vms *v1alpha2.VirtualMachineSnapshot, phase coordinationv1alpha1.AccessClaimPhase, reason, message string) *coordinationv1alpha1.VirtualMachineAccessClaim {
	claim := &coordinationv1alpha1.VirtualMachineAccessClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:            accessclaim.Name(vms),
			Namespace:       vms.Namespace,
			OwnerReferences: []metav1.OwnerReference{*metav1.NewControllerRef(vms, v1alpha2.SchemeGroupVersion.WithKind(v1alpha2.VirtualMachineSnapshotKind))},
		},
		Spec: coordinationv1alpha1.VirtualMachineAccessClaimSpec{
			VirtualMachine: coordinationv1alpha1.VirtualMachineAccessClaimTarget{Name: "vm1", UID: "vm-uid"},
			Access:         []coordinationv1alpha1.AccessMode{coordinationv1alpha1.AccessModeSnapshot, coordinationv1alpha1.AccessModeFreeze},
			FreezePolicy:   ptr.To(coordinationv1alpha1.FreezePolicyRequired),
			Timeout:        metav1.Duration{Duration: accessclaim.Timeout},
		},
		Status: coordinationv1alpha1.VirtualMachineAccessClaimStatus{Phase: phase},
	}

	if reason != "" {
		status := metav1.ConditionFalse
		if phase == coordinationv1alpha1.AccessClaimPhaseGranted {
			status = metav1.ConditionTrue
		}

		claim.Status.Conditions = []metav1.Condition{{
			Type:    vmaccondition.GrantedType.String(),
			Status:  status,
			Reason:  reason,
			Message: message,
		}}
	}

	return claim
}

// grantedClaim is the access claim of vms with the guest frozen for it.
func grantedClaim(vms *v1alpha2.VirtualMachineSnapshot) *coordinationv1alpha1.VirtualMachineAccessClaim {
	return testClaim(vms, coordinationv1alpha1.AccessClaimPhaseGranted, string(vmaccondition.Frozen), "The filesystem is frozen.")
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

	if res.RequeueAfter == 0 {
		t.Fatalf("expected RequeueAfter, got %+v", res)
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
	r := newFullTestReconciler(t, vms, planningGateVM(), grantedClaim(vms))

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
	r := newFullTestReconciler(t, vms, planningGateVM(), grantedClaim(vms))

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
		WithObjects(vms, vm, vd, grantedClaim(vms)).
		WithStatusSubresource(&v1alpha2.VirtualMachineSnapshot{}, &coordinationv1alpha1.VirtualMachineAccessClaim{}).
		WithInterceptorFuncs(interceptor.Funcs{
			Create: func(ctx context.Context, cl client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
				if _, ok := obj.(*v1alpha2.VirtualDiskSnapshot); ok {
					return boom
				}

				return cl.Create(ctx, obj, opts...)
			},
		}).
		Build()
	r := &Reconciler{Client: c, APIReader: c, Log: log.NewNop()}

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
	r := newFullTestReconciler(t, vms, vm, child, grantedClaim(vms))

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

// planningStartedAt starts the bounded planning budget when planning first became possible: after the
// snapshot and its VirtualMachine exist and the access is granted.
func TestPlanningStartedAt(t *testing.T) {
	base := time.Now().Add(-time.Hour).Truncate(time.Second)
	vms := &v1alpha2.VirtualMachineSnapshot{
		ObjectMeta: metav1.ObjectMeta{CreationTimestamp: metav1.NewTime(base)},
	}

	tests := []struct {
		name  string
		vm    *v1alpha2.VirtualMachine
		claim *coordinationv1alpha1.VirtualMachineAccessClaim
		want  time.Time
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
		{
			name: "access granted after a long freeze confirmation",
			vm:   &v1alpha2.VirtualMachine{ObjectMeta: metav1.ObjectMeta{CreationTimestamp: metav1.NewTime(base.Add(-time.Hour))}},
			claim: &coordinationv1alpha1.VirtualMachineAccessClaim{
				Status: coordinationv1alpha1.VirtualMachineAccessClaimStatus{GrantedAt: ptr.To(metav1.NewTime(base.Add(4 * time.Minute)))},
			},
			want: base.Add(4 * time.Minute),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := planningStartedAt(vms, tt.vm, tt.claim); !got.Equal(tt.want) {
				t.Errorf("planningStartedAt() = %v, want %v", got, tt.want)
			}
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

// The finalizer list cannot be merged — a patch replaces it wholesale — so it is written under an
// optimistic lock and retried against a fresh read. What must never happen is that a finalizer added by
// somebody else, foregroundDeletion among them, disappears because this controller wrote from a stale
// copy.
func TestPatchFinalizer_KeepsFinalizersAddedMeanwhile(t *testing.T) {
	vms := &v1alpha2.VirtualMachineSnapshot{
		ObjectMeta: metav1.ObjectMeta{
			Name: "vms1", Namespace: testNamespace,
			Annotations: map[string]string{v1alpha2.AnnUseUnifiedSnapshotter: ""},
		},
		Spec: v1alpha2.VirtualMachineSnapshotSpec{VirtualMachineName: "vm1"},
	}
	r := newFullTestReconciler(t, vms)
	ctx := context.Background()

	live := getVMS(t, r.Client)

	live.Finalizers = append(live.Finalizers, "foregroundDeletion")
	if err := r.Client.Update(ctx, live); err != nil {
		t.Fatalf("simulate a concurrent finalizer: %v", err)
	}

	if err := r.patchFinalizer(ctx, vms, controllerutil.AddFinalizer); err != nil {
		t.Fatalf("add the cleanup finalizer from a stale copy: %v", err)
	}

	stored := getVMS(t, r.Client)
	if !controllerutil.ContainsFinalizer(stored, v1alpha2.FinalizerVMSnapshotCleanup) {
		t.Error("the cleanup finalizer was not added")
	}

	if !controllerutil.ContainsFinalizer(stored, "foregroundDeletion") {
		t.Error("the foreignly-added finalizer was dropped; cascading deletion would break")
	}
}

// Removing is the same write in reverse, and must leave the other finalizers alone.
func TestPatchFinalizer_RemovesOnlyItsOwn(t *testing.T) {
	vms := &v1alpha2.VirtualMachineSnapshot{
		ObjectMeta: metav1.ObjectMeta{
			Name: "vms1", Namespace: testNamespace,
			Annotations: map[string]string{v1alpha2.AnnUseUnifiedSnapshotter: ""},
			Finalizers:  []string{v1alpha2.FinalizerVMSnapshotCleanup, "someone.else/keep-me"},
		},
		Spec: v1alpha2.VirtualMachineSnapshotSpec{VirtualMachineName: "vm1"},
	}
	r := newFullTestReconciler(t, vms)

	if err := r.patchFinalizer(context.Background(), vms, controllerutil.RemoveFinalizer); err != nil {
		t.Fatalf("remove the cleanup finalizer: %v", err)
	}

	stored := getVMS(t, r.Client)
	if controllerutil.ContainsFinalizer(stored, v1alpha2.FinalizerVMSnapshotCleanup) {
		t.Error("the cleanup finalizer is still there")
	}

	if !controllerutil.ContainsFinalizer(stored, "someone.else/keep-me") {
		t.Error("somebody else's finalizer was removed too")
	}
}

// A snapshot can be deleted at any moment — by a user, or by an e2e teardown — including after this
// reconcile has already read it and started writing. Every write after that point comes back NotFound,
// and returning it logged a failure and scheduled a retry for an object that is never coming back.
//
// The object is therefore removed from under the reconcile rather than before it: deleting it up front
// would be caught by the first read's own guard and would never reach the code under test.
func TestReconcile_StopsQuietlyWhenTheSnapshotIsDeletedMidReconcile(t *testing.T) {
	vms := &v1alpha2.VirtualMachineSnapshot{
		ObjectMeta: metav1.ObjectMeta{
			Name: "vms1", Namespace: testNamespace,
			Annotations: map[string]string{v1alpha2.AnnUseUnifiedSnapshotter: ""},
		},
		Spec: v1alpha2.VirtualMachineSnapshotSpec{VirtualMachineName: "vm1", RequiredConsistency: true},
	}
	vm := &v1alpha2.VirtualMachine{ObjectMeta: metav1.ObjectMeta{Name: "vm1", Namespace: testNamespace}}

	gone := apierrors.NewNotFound(
		v1alpha2.SchemeGroupVersion.WithResource(v1alpha2.VirtualMachineSnapshotResource).GroupResource(), "vms1")

	// The first read hands the snapshot over; everything after it finds the snapshot gone, which is what
	// a deletion mid-reconcile looks like from in here.
	var reads int

	c := fake.NewClientBuilder().
		WithScheme(newTestScheme(t)).
		WithObjects(vms, vm).
		WithStatusSubresource(&v1alpha2.VirtualMachineSnapshot{}, &coordinationv1alpha1.VirtualMachineAccessClaim{}).
		WithInterceptorFuncs(interceptor.Funcs{
			Get: func(ctx context.Context, cl client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
				if _, ours := obj.(*v1alpha2.VirtualMachineSnapshot); ours {
					if reads++; reads > 1 {
						return gone
					}
				}

				return cl.Get(ctx, key, obj, opts...)
			},
			SubResourcePatch: func(_ context.Context, _ client.Client, _ string, obj client.Object, _ client.Patch, _ ...client.SubResourcePatchOption) error {
				if _, ours := obj.(*v1alpha2.VirtualMachineSnapshot); ours {
					return gone
				}

				return nil
			},
		}).
		Build()

	r := &Reconciler{Client: c, APIReader: c, Log: log.NewNop()}
	req := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: testNamespace, Name: "vms1"}}

	res, err := r.Reconcile(context.Background(), req)
	if err != nil {
		t.Fatalf("Reconcile returned %v; a snapshot deleted mid-reconcile leaves nothing to reconcile", err)
	}

	if res.RequeueAfter != 0 {
		t.Errorf("result = %+v, want no requeue for an object that will not come back", res)
	}

	if reads < 2 {
		t.Fatalf("the snapshot was read %d time(s); the reconcile never got past its first read, so nothing was exercised", reads)
	}
}

// A reconcile that only waits must not write. The domain status is cleared on the way in and refilled
// by the waiting branch, so the object ends each pass with the same content but a new
// resourceVersion — and since the controller watches its own object, every write re-enqueues the next
// reconcile at once, which is a write storm for as long as the wait lasts.
func TestReconcile_AWaitingPassDoesNotRewriteTheDomainStatus(t *testing.T) {
	r, req := accessFixture(t, coordinationv1alpha1.AccessClaimPhasePending, string(vmaccondition.Freezing), "Waiting for the guest to confirm the filesystem freeze.")
	ctx := context.Background()

	if _, err := r.Reconcile(ctx, req); err != nil {
		t.Fatal(err)
	}

	settled := getVMS(t, r.Client)

	d := settled.Status.CaptureState.DomainSpecificController
	if d == nil || d.Reason != string(vmscondition.FileSystemFreezing) {
		t.Fatalf("the first pass did not park in the access wait: %+v", d)
	}

	before := settled.ResourceVersion

	if _, err := r.Reconcile(ctx, req); err != nil {
		t.Fatal(err)
	}

	after := getVMS(t, r.Client).ResourceVersion

	if before != after {
		t.Errorf("a second waiting pass wrote the snapshot (resourceVersion %s -> %s); "+
			"nothing changed, so it must not write at all", before, after)
	}
}

// The parent must carry captureState at the moment it creates a child, not merely by the end of the
// pass. A child VirtualDiskSnapshot with no capture state of its own routes itself by the parent's, so
// a child born while the parent has none is handed to the built-in controller, which reads a
// spec.virtualDiskName the unified children do not have and fails them — taking the whole
// VirtualMachineSnapshot down with it. Asserting the end state cannot see this: a later Planned write
// fills captureState in either case, so the ordering is checked where it is decided, on Create.
func TestReconcile_TheParentCarriesCaptureStateWhenAChildIsCreated(t *testing.T) {
	vms := &v1alpha2.VirtualMachineSnapshot{
		ObjectMeta: metav1.ObjectMeta{
			Name: "vms1", Namespace: testNamespace,
			Finalizers:        []string{v1alpha2.FinalizerVMSnapshotCleanup},
			CreationTimestamp: metav1.NewTime(time.Now().Add(-time.Minute)),
		},
		Spec:   v1alpha2.VirtualMachineSnapshotSpec{VirtualMachineName: "vm1", RequiredConsistency: true},
		Status: v1alpha2.VirtualMachineSnapshotStatus{Phase: v1alpha2.VirtualMachineSnapshotPhasePending},
	}
	vm := &v1alpha2.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{
			Name: "vm1", Namespace: testNamespace,
			CreationTimestamp: metav1.NewTime(time.Now().Add(-time.Minute)),
		},
		Status: v1alpha2.VirtualMachineStatus{
			BlockDeviceRefs: []v1alpha2.BlockDeviceStatusRef{
				{Kind: v1alpha2.DiskDevice, Name: "vd1"},
			},
		},
	}
	vd := &v1alpha2.VirtualDisk{
		ObjectMeta: metav1.ObjectMeta{Name: "vd1", Namespace: testNamespace, UID: "vd1-uid"},
		Status:     v1alpha2.VirtualDiskStatus{Phase: v1alpha2.DiskReady},
	}
	// Already frozen with no request pending: the freeze block is skipped entirely, so no waiting
	// branch writes a status on the way to the children.
	kvvmi := &virtv1.VirtualMachineInstance{
		ObjectMeta: metav1.ObjectMeta{Name: "vm1", Namespace: testNamespace},
		Status: virtv1.VirtualMachineInstanceStatus{
			Phase:          virtv1.Running,
			FSFreezeStatus: service.FSFrozen,
		},
	}

	var orphanedChildren, createdChildren []string

	c := fake.NewClientBuilder().
		WithScheme(newTestScheme(t)).
		WithObjects(vms, vm, vd, kvvmi, grantedClaim(vms)).
		WithStatusSubresource(&v1alpha2.VirtualMachineSnapshot{}, &coordinationv1alpha1.VirtualMachineAccessClaim{}).
		WithInterceptorFuncs(interceptor.Funcs{
			Create: func(ctx context.Context, cl client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
				if vds, ok := obj.(*v1alpha2.VirtualDiskSnapshot); ok {
					createdChildren = append(createdChildren, vds.Name)

					parent := &v1alpha2.VirtualMachineSnapshot{}
					if err := cl.Get(ctx, types.NamespacedName{Namespace: testNamespace, Name: "vms1"}, parent); err == nil {
						if parent.Status.CaptureState == nil {
							orphanedChildren = append(orphanedChildren, vds.Name)
						}
					}
				}

				return cl.Create(ctx, obj, opts...)
			},
		}).
		Build()
	r := &Reconciler{Client: c, APIReader: c, Log: log.NewNop()}

	req := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: testNamespace, Name: "vms1"}}
	if _, err := r.Reconcile(context.Background(), req); err != nil {
		t.Fatal(err)
	}

	if len(createdChildren) == 0 {
		t.Fatal("no child was created, so the ordering this test is about was never exercised")
	}

	if len(orphanedChildren) > 0 {
		t.Errorf("%d child(ren) were created while the parent had no captureState: %v; "+
			"they would route to the built-in controller", len(orphanedChildren), orphanedChildren)
	}
}

// The timeout message must say why a disk snapshot is stuck, not only which one: the domain-written
// detail wins, the core-owned Ready condition is the fallback, and terminal children are not blamed.
func TestPendingChildrenDetails(t *testing.T) {
	withDomain := v1alpha2.VirtualDiskSnapshot{
		ObjectMeta: metav1.ObjectMeta{Name: "a"},
		Status: v1alpha2.VirtualDiskSnapshotStatus{
			CaptureState: &v1alpha2.UnifiedSnapshotterCaptureState{
				DomainSpecificController: &v1alpha2.UnifiedSnapshotterDomainCaptureState{Reason: "DataCaptureRetrying", Message: "not Thin"},
			},
			Conditions: []metav1.Condition{{Type: v1alpha2.UnifiedSnapshotterConditionReady, Status: metav1.ConditionFalse, Reason: "DataCapturePending", Message: "waiting"}},
		},
	}
	withReady := v1alpha2.VirtualDiskSnapshot{
		ObjectMeta: metav1.ObjectMeta{Name: "b"},
		Status: v1alpha2.VirtualDiskSnapshotStatus{
			Conditions: []metav1.Condition{{Type: v1alpha2.UnifiedSnapshotterConditionReady, Status: metav1.ConditionFalse, Reason: "DataCapturePending", Message: "waiting"}},
		},
	}
	failed := v1alpha2.VirtualDiskSnapshot{
		ObjectMeta: metav1.ObjectMeta{Name: "c"},
		Status:     v1alpha2.VirtualDiskSnapshotStatus{Phase: v1alpha2.VirtualDiskSnapshotPhaseFailed},
	}
	silent := v1alpha2.VirtualDiskSnapshot{ObjectMeta: metav1.ObjectMeta{Name: "d"}}

	got := pendingChildrenDetails([]v1alpha2.VirtualDiskSnapshot{withReady, withDomain, failed, silent})

	want := `. VirtualDiskSnapshot "a": DataCaptureRetrying: not Thin; VirtualDiskSnapshot "b": DataCapturePending: waiting`
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}

	if got := pendingChildrenDetails([]v1alpha2.VirtualDiskSnapshot{silent, failed}); got != "" {
		t.Fatalf("got %q, want empty", got)
	}
}

// ChildrenSettled counts a failed child as settled, and the core bubbles the failure up to this
// snapshot's own Ready on its own schedule, so the node can see "settled" before it sees the failure. A
// failed disk must still never be published as a Ready virtual machine snapshot, whichever of the child's
// two failure markers (phase, domain capture phase) has landed yet.
func TestReconcile_FailedChildNeverYieldsAReadySnapshot(t *testing.T) {
	failedPhase := v1alpha2.UnifiedSnapshotterPhase(snapshotsdk.PhaseFailed)

	tests := map[string]v1alpha2.VirtualDiskSnapshotStatus{
		"phase only": {Phase: v1alpha2.VirtualDiskSnapshotPhaseFailed},
		"domain phase only": {
			Phase: v1alpha2.VirtualDiskSnapshotPhaseInProgress,
			CaptureState: &v1alpha2.UnifiedSnapshotterCaptureState{
				DomainSpecificController: &v1alpha2.UnifiedSnapshotterDomainCaptureState{
					Phase: failedPhase, Reason: "SnapshotsNotSupported", Message: "no snapshots here",
				},
			},
		},
	}
	for name, childStatus := range tests {
		t.Run(name, func(t *testing.T) {
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
			vm := &v1alpha2.VirtualMachine{ObjectMeta: metav1.ObjectMeta{Name: "vm1", Namespace: testNamespace}}
			child := &v1alpha2.VirtualDiskSnapshot{
				ObjectMeta: metav1.ObjectMeta{Name: "child", Namespace: testNamespace},
				Status:     childStatus,
			}
			r := newFullTestReconciler(t, vms, vm, child, grantedClaim(vms))

			if _, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Namespace: testNamespace, Name: "vms1"}}); err != nil {
				t.Fatal(err)
			}

			got := getVMS(t, r.Client)
			if got.Status.Phase != v1alpha2.VirtualMachineSnapshotPhaseFailed {
				t.Fatalf("phase = %q, want Failed", got.Status.Phase)
			}

			if d := got.Status.CaptureState.DomainSpecificController; d.Phase != failedPhase || !strings.Contains(d.Message, "[child]") {
				t.Fatalf("domain state = %+v, want Failed naming the child", d)
			}
		})
	}
}

// A capture the core failed stays failed once its released claim completes: the claim verdict only
// settles consistency and never turns a failed capture into a Ready snapshot.
func TestReconcile_CoreFailedCaptureStaysFailedAfterClaimCompletes(t *testing.T) {
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
			},
			Conditions: []metav1.Condition{{
				Type: v1alpha2.UnifiedSnapshotterConditionReady, Status: metav1.ConditionFalse,
				Reason: "ManifestCheckpointFailed", Message: "the checkpoint failed",
			}},
		},
	}
	vm := &v1alpha2.VirtualMachine{ObjectMeta: metav1.ObjectMeta{Name: "vm1", Namespace: testNamespace}}
	r := newFullTestReconciler(t, vms, vm, grantedClaim(vms))
	req := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: testNamespace, Name: "vms1"}}

	if _, err := r.Reconcile(context.Background(), req); err != nil {
		t.Fatal(err)
	}

	claim := &coordinationv1alpha1.VirtualMachineAccessClaim{}
	if err := r.Client.Get(context.Background(), types.NamespacedName{Namespace: testNamespace, Name: accessclaim.Name(vms)}, claim); err != nil {
		t.Fatal(err)
	}

	if !claim.Spec.Release {
		t.Fatal("the claim of a failed capture must be released")
	}

	claim.Status.Phase = coordinationv1alpha1.AccessClaimPhaseCompleted

	claim.Status.Frozen = true
	if err := r.Client.Status().Update(context.Background(), claim); err != nil {
		t.Fatal(err)
	}

	if _, err := r.Reconcile(context.Background(), req); err != nil {
		t.Fatal(err)
	}

	got := getVMS(t, r.Client)
	if got.Status.Phase != v1alpha2.VirtualMachineSnapshotPhaseFailed {
		t.Fatalf("phase = %q, want Failed", got.Status.Phase)
	}

	if d := got.Status.CaptureState.DomainSpecificController; d.Phase != v1alpha2.UnifiedSnapshotterPhase(snapshotsdk.PhaseFailed) {
		t.Fatalf("domain phase = %q, want Failed", d.Phase)
	}

	if got.Status.Consistent == nil || *got.Status.Consistent {
		t.Fatalf("consistent = %v, want false", got.Status.Consistent)
	}
}
