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
	"slices"
	"testing"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	virtv1 "kubevirt.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/deckhouse/state-snapshotter/pkg/snapshotsdk"
	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/vmaccess"
	"github.com/deckhouse/virtualization-controller/pkg/controller/accessclaim"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	coordinationv1alpha1 "github.com/deckhouse/virtualization/api/coordination/v1alpha1"
	"github.com/deckhouse/virtualization/api/coordination/v1alpha1/vmaccondition"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmscondition"
)

var vms1 = ctrl.Request{NamespacedName: types.NamespacedName{Namespace: testNamespace, Name: "vms1"}}

func reconcileOnce(t *testing.T, r *Reconciler) {
	t.Helper()

	if _, err := r.Reconcile(context.Background(), vms1); err != nil {
		t.Fatal(err)
	}
}

func getClaim(t *testing.T, c client.Client, vms *v1alpha2.VirtualMachineSnapshot) *coordinationv1alpha1.VirtualMachineAccessClaim {
	t.Helper()

	claim, err := accessclaim.Get(context.Background(), c, c, vms.Namespace, accessclaim.Name(vms))
	if err != nil {
		t.Fatal(err)
	}

	return claim
}

// settledSnapshot is a snapshot whose manifests and only disk are captured, with the access claimed.
func settledSnapshot() *v1alpha2.VirtualMachineSnapshot {
	return &v1alpha2.VirtualMachineSnapshot{
		ObjectMeta: metav1.ObjectMeta{
			Name: "vms1", Namespace: testNamespace, UID: "vms1-uid",
			Annotations: map[string]string{v1alpha2.AnnUseUnifiedSnapshotter: ""},
			Finalizers:  []string{v1alpha2.FinalizerVMSnapshotCleanup},
		},
		Spec: v1alpha2.VirtualMachineSnapshotSpec{VirtualMachineName: "vm1", RequiredConsistency: true},
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
}

func readyChild() *v1alpha2.VirtualDiskSnapshot {
	return &v1alpha2.VirtualDiskSnapshot{
		ObjectMeta: metav1.ObjectMeta{Name: "child", Namespace: testNamespace},
		Status:     v1alpha2.VirtualDiskSnapshotStatus{Phase: v1alpha2.VirtualDiskSnapshotPhaseReady},
	}
}

// Nothing is planned before the coordinator decides, so no disk is captured before the access is granted.
func TestReconcile_RequestsAccessBeforePlanning(t *testing.T) {
	for name, tc := range map[string]struct {
		requiredConsistency bool
		access              []coordinationv1alpha1.AccessMode
		policy              *coordinationv1alpha1.FreezePolicy
		waitReason          vmscondition.VirtualMachineSnapshotReadyReason
	}{
		"consistency required": {
			requiredConsistency: true,
			access:              []coordinationv1alpha1.AccessMode{coordinationv1alpha1.AccessModeSnapshot, coordinationv1alpha1.AccessModeFreeze},
			policy:              ptr.To(coordinationv1alpha1.FreezePolicyRequired),
			waitReason:          vmscondition.FileSystemFreezing,
		},
		"consistency not required": {
			requiredConsistency: false,
			access:              []coordinationv1alpha1.AccessMode{coordinationv1alpha1.AccessModeSnapshot},
			waitReason:          vmscondition.Snapshotting,
		},
	} {
		t.Run(name, func(t *testing.T) {
			vms := &v1alpha2.VirtualMachineSnapshot{
				ObjectMeta: metav1.ObjectMeta{
					Name: "vms1", Namespace: testNamespace, UID: "vms1-uid",
					Annotations: map[string]string{v1alpha2.AnnUseUnifiedSnapshotter: ""},
					Finalizers:  []string{v1alpha2.FinalizerVMSnapshotCleanup},
				},
				Spec:   v1alpha2.VirtualMachineSnapshotSpec{VirtualMachineName: "vm1", RequiredConsistency: tc.requiredConsistency},
				Status: v1alpha2.VirtualMachineSnapshotStatus{Phase: v1alpha2.VirtualMachineSnapshotPhasePending},
			}
			vm := &v1alpha2.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{Name: "vm1", Namespace: testNamespace, UID: "vm1-uid"},
				Status: v1alpha2.VirtualMachineStatus{BlockDeviceRefs: []v1alpha2.BlockDeviceStatusRef{{
					Kind: v1alpha2.DiskDevice, Name: "vd1",
				}}},
			}
			vd := &v1alpha2.VirtualDisk{ObjectMeta: metav1.ObjectMeta{Name: "vd1", Namespace: testNamespace, UID: "vd1-uid"}}
			r := newFullTestReconciler(t, vms, vm, vd)

			reconcileOnce(t, r)

			claim := getClaim(t, r.Client, vms)
			if claim == nil {
				t.Fatal("no access claim was created")
			}

			if claim.Spec.VirtualMachine != (coordinationv1alpha1.VirtualMachineAccessClaimTarget{Name: "vm1", UID: "vm1-uid"}) {
				t.Errorf("claim targets %+v", claim.Spec.VirtualMachine)
			}

			if !slices.Equal(claim.Spec.Access, tc.access) || !equalPolicy(claim.Spec.FreezePolicy, tc.policy) {
				t.Errorf("claim access = %v, policy = %v; want %v, %v", claim.Spec.Access, claim.Spec.FreezePolicy, tc.access, tc.policy)
			}

			if claim.Spec.Timeout.Duration != accessclaim.Timeout {
				t.Errorf("claim timeout = %s, want %s", claim.Spec.Timeout.Duration, accessclaim.Timeout)
			}

			if owner := metav1.GetControllerOf(claim); owner == nil || owner.UID != vms.UID {
				t.Errorf("claim controller owner = %+v, want the snapshot", owner)
			}

			if d := getVMS(t, r.Client).Status.CaptureState.DomainSpecificController; d.Reason != string(tc.waitReason) {
				t.Errorf("waiting reason = %q, want %q", d.Reason, tc.waitReason)
			}

			children := &v1alpha2.VirtualDiskSnapshotList{}
			if err := r.Client.List(context.Background(), children); err != nil {
				t.Fatal(err)
			}

			if len(children.Items) != 0 {
				t.Errorf("%d children were planned before the access was granted", len(children.Items))
			}
		})
	}
}

func equalPolicy(a, b *coordinationv1alpha1.FreezePolicy) bool {
	if a == nil || b == nil {
		return a == b
	}

	return *a == *b
}

// A claim the coordinator refuses fails the snapshot with the coordinator's reason and message, which are
// written for the snapshot.
func TestReconcile_RefusedAccessFailsTheSnapshot(t *testing.T) {
	const message = `The virtual machine "vm1" is in use by the VirtualMachineSnapshot "daily". Retry after it completes.`

	r, _ := accessFixture(t, coordinationv1alpha1.AccessClaimPhaseDenied, string(vmaccondition.VirtualMachineBusy), message)

	reconcileOnce(t, r)

	got := getVMS(t, r.Client)
	if got.Status.Phase != v1alpha2.VirtualMachineSnapshotPhaseFailed {
		t.Fatalf("phase = %q, want Failed", got.Status.Phase)
	}

	d := got.Status.CaptureState.DomainSpecificController
	if d.Reason != string(vmaccondition.VirtualMachineBusy) || d.Message != message {
		t.Errorf("domain state = %+v, want the coordinator's reason and message", d)
	}

	if got.Status.Consistent == nil || *got.Status.Consistent {
		t.Errorf("consistent = %v, want false", got.Status.Consistent)
	}
}

// Every disk captured: the snapshot releases its access and waits for the verdict instead of turning
// Ready on its own.
func TestReconcile_ReleasesTheAccessOnceEveryDiskIsCaptured(t *testing.T) {
	vms := settledSnapshot()
	vm := &v1alpha2.VirtualMachine{ObjectMeta: metav1.ObjectMeta{Name: "vm1", Namespace: testNamespace}}
	r := newFullTestReconciler(t, vms, vm, readyChild(), grantedClaim(vms))

	reconcileOnce(t, r)

	if !getClaim(t, r.Client, vms).Spec.Release {
		t.Error("the access was not released")
	}

	if got := getVMS(t, r.Client); got.Status.Phase != v1alpha2.VirtualMachineSnapshotPhaseInProgress {
		t.Errorf("phase = %q, want InProgress until the verdict", got.Status.Phase)
	}
}

// The verdict is the only source of consistency, and it settles the snapshot even once its virtual
// machine is gone.
func TestReconcile_TheVerdictMakesTheSnapshotReady(t *testing.T) {
	for name, tc := range map[string]struct {
		frozen     bool
		vmiUID     types.UID
		vmExists   bool
		consistent bool
	}{
		"frozen session":                      {frozen: true, vmiUID: "vmi-uid", vmExists: true, consistent: true},
		"running machine without a freeze":    {frozen: false, vmiUID: "vmi-uid", vmExists: true, consistent: false},
		"machine stopped for all the session": {frozen: false, vmiUID: "", vmExists: true, consistent: true},
		"virtual machine gone":                {frozen: true, vmiUID: "vmi-uid", vmExists: false, consistent: true},
	} {
		t.Run(name, func(t *testing.T) {
			vms := settledSnapshot()
			claim := testClaim(vms, coordinationv1alpha1.AccessClaimPhaseCompleted, string(vmaccondition.Released), "")
			claim.Spec.Release = true
			claim.Status.GrantedAt = ptr.To(metav1.Now())
			claim.Status.Frozen = tc.frozen
			claim.Status.VirtualMachineInstanceUID = tc.vmiUID

			objs := []client.Object{vms, readyChild(), claim}
			if tc.vmExists {
				objs = append(objs, &v1alpha2.VirtualMachine{ObjectMeta: metav1.ObjectMeta{Name: "vm1", Namespace: testNamespace}})
			}

			r := newFullTestReconciler(t, objs...)

			reconcileOnce(t, r)

			got := getVMS(t, r.Client)
			if got.Status.Phase != v1alpha2.VirtualMachineSnapshotPhaseReady {
				t.Fatalf("phase = %q, want Ready", got.Status.Phase)
			}

			if got.Status.Consistent == nil || *got.Status.Consistent != tc.consistent {
				t.Errorf("consistent = %v, want %v", got.Status.Consistent, tc.consistent)
			}

			if d := got.Status.CaptureState.DomainSpecificController; d.Phase != v1alpha2.UnifiedSnapshotterPhase(snapshotsdk.PhaseFinished) {
				t.Errorf("domain phase = %q, want Finished", d.Phase)
			}
		})
	}
}

// The verdict comes before the unfreeze goes through: the snapshot waits for the guest to be unfrozen, and
// warns when the guest does not confirm it.
func TestReconcile_TheSnapshotWaitsForTheUnfreeze(t *testing.T) {
	vms := settledSnapshot()
	claim := testClaim(vms, coordinationv1alpha1.AccessClaimPhaseCompleted, string(vmaccondition.Released), "")
	claim.Spec.Release = true
	claim.Status.Frozen = true
	claim.Status.VirtualMachineInstanceUID = "vmi-uid"
	claim.Status.Conditions = append(claim.Status.Conditions, metav1.Condition{
		Type: vmaccondition.UnfrozenType.String(), Status: metav1.ConditionFalse, Reason: string(vmaccondition.Unfreezing),
	})
	vm := &v1alpha2.VirtualMachine{ObjectMeta: metav1.ObjectMeta{Name: "vm1", Namespace: testNamespace}}
	r := newFullTestReconciler(t, vms, readyChild(), claim, vm)

	var warnings []string

	r.Recorder = &eventrecord.EventRecorderLoggerMock{
		EventFunc: func(_ client.Object, eventtype, reason, _ string) {
			if eventtype == corev1.EventTypeWarning {
				warnings = append(warnings, reason)
			}
		},
	}

	reconcileOnce(t, r)

	if got := getVMS(t, r.Client); got.Status.Phase != v1alpha2.VirtualMachineSnapshotPhaseInProgress {
		t.Fatalf("phase = %q, want InProgress while the guest is being unfrozen", got.Status.Phase)
	}

	unfreezing := getClaim(t, r.Client, vms)
	unfreezing.Status.Conditions[len(unfreezing.Status.Conditions)-1].Reason = string(vmaccondition.UnfreezeNotConfirmed)

	unfreezing.Status.Conditions[len(unfreezing.Status.Conditions)-1].Message = "The guest did not confirm the unfreeze."
	if err := r.Client.Status().Update(context.Background(), unfreezing); err != nil {
		t.Fatal(err)
	}

	reconcileOnce(t, r)

	if got := getVMS(t, r.Client); got.Status.Phase != v1alpha2.VirtualMachineSnapshotPhaseReady {
		t.Fatalf("phase = %q, want Ready", got.Status.Phase)
	}

	if len(warnings) != 1 || warnings[0] != v1alpha2.ReasonFilesystemUnfreezeNotConfirmed {
		t.Errorf("warnings = %v, want one %s", warnings, v1alpha2.ReasonFilesystemUnfreezeNotConfirmed)
	}
}

// A capture failing on its own side releases the access at once, so the guest is not held frozen until the
// session timeout.
func TestReconcile_AFailedCaptureReleasesTheAccess(t *testing.T) {
	vms := settledSnapshot()
	vm := &v1alpha2.VirtualMachine{ObjectMeta: metav1.ObjectMeta{Name: "vm1", Namespace: testNamespace}}
	child := readyChild()
	child.Status.Phase = v1alpha2.VirtualDiskSnapshotPhaseFailed
	r := newFullTestReconciler(t, vms, vm, child, grantedClaim(vms))

	reconcileOnce(t, r)

	if got := getVMS(t, r.Client); got.Status.Phase != v1alpha2.VirtualMachineSnapshotPhaseFailed {
		t.Fatalf("phase = %q, want Failed", got.Status.Phase)
	}

	if !getClaim(t, r.Client, vms).Spec.Release {
		t.Error("the access was not released")
	}
}

// startedBeforeUpdateFixture is a snapshot an older version of the controller started, with no access
// claim, for vm1 whose instance is kvvmi. lock, when set, is the access lock on the internal virtual machine.
func startedBeforeUpdateFixture(t *testing.T, vms *v1alpha2.VirtualMachineSnapshot, kvvmi *virtv1.VirtualMachineInstance, lock *vmaccess.Lock) (*Reconciler, *fakeUnfreezer) {
	t.Helper()

	vm := &v1alpha2.VirtualMachine{ObjectMeta: metav1.ObjectMeta{Name: "vm1", Namespace: testNamespace}}

	kvvm := &virtv1.VirtualMachine{ObjectMeta: metav1.ObjectMeta{Name: "vm1", Namespace: testNamespace}}
	if lock != nil {
		if err := vmaccess.WriteLock(kvvm, lock); err != nil {
			t.Fatal(err)
		}
	}

	r := newFullTestReconciler(t, vms, vm, kvvm, kvvmi, readyChild())
	unfreezer := &fakeUnfreezer{}
	r.Unfreezer = unfreezer

	return r, unfreezer
}

// runningInstance is the running instance of vm1 with the given filesystem freeze status and filesystem
// request.
func runningInstance(fsFreezeStatus, request string) *virtv1.VirtualMachineInstance {
	kvvmi := &virtv1.VirtualMachineInstance{
		ObjectMeta: metav1.ObjectMeta{Name: "vm1", Namespace: testNamespace},
		Status:     virtv1.VirtualMachineInstanceStatus{Phase: virtv1.Running, FSFreezeStatus: fsFreezeStatus},
	}
	if request != "" {
		kvvmi.Annotations = map[string]string{annotations.AnnVMFilesystemRequest: request}
	}

	return kvvmi
}

func filesystemRequest(t *testing.T, c client.Client) (string, bool) {
	t.Helper()

	kvvmi := &virtv1.VirtualMachineInstance{}
	if err := c.Get(context.Background(), types.NamespacedName{Namespace: testNamespace, Name: "vm1"}, kvvmi); err != nil {
		t.Fatal(err)
	}

	request, ok := kvvmi.Annotations[annotations.AnnVMFilesystemRequest]

	return request, ok
}

// A snapshot creates its claim before it plans any child or leaves Pending, so one past that point without
// a claim, or one that asked for a freeze, was started before the module update: it fails, and the guest it
// may have left frozen is unfrozen whatever fsFreezeStatus reads. The filesystem request it may have left
// stays for the coordinator to remove.
func TestReconcile_FailsASnapshotStartedWithoutAClaim(t *testing.T) {
	for name, tc := range map[string]struct {
		mutate func(*v1alpha2.VirtualMachineSnapshot)
		kvvmi  *virtv1.VirtualMachineInstance
	}{
		"in progress, frozen":             {mutate: func(*v1alpha2.VirtualMachineSnapshot) {}, kvvmi: runningInstance(service.FSFrozen, "")},
		"in progress, status reads empty": {mutate: func(*v1alpha2.VirtualMachineSnapshot) {}, kvvmi: runningInstance("", "")},
		"in progress, freeze request":     {mutate: func(*v1alpha2.VirtualMachineSnapshot) {}, kvvmi: runningInstance("", service.RequestFSFreeze)},
		"pending with children": {
			mutate: func(vms *v1alpha2.VirtualMachineSnapshot) {
				vms.Status.Phase = v1alpha2.VirtualMachineSnapshotPhasePending
			},
			kvvmi: runningInstance(service.FSFrozen, ""),
		},
		"pending, froze before planning": {
			mutate: func(vms *v1alpha2.VirtualMachineSnapshot) {
				vms.Status.Phase = v1alpha2.VirtualMachineSnapshotPhasePending
				vms.Status.ChildrenSnapshotRefs = nil
				vms.Status.CaptureState = nil
				vms.Annotations[annFSFreezeRequestedAt] = "2026-11-01T00:00:00Z"
			},
			kvvmi: runningInstance(service.FSFrozen, ""),
		},
	} {
		t.Run(name, func(t *testing.T) {
			vms := settledSnapshot()
			tc.mutate(vms)
			r, unfreezer := startedBeforeUpdateFixture(t, vms, tc.kvvmi, nil)

			reconcileOnce(t, r)

			got := getVMS(t, r.Client)
			if got.Status.Phase != v1alpha2.VirtualMachineSnapshotPhaseFailed {
				t.Fatalf("phase = %q, want Failed", got.Status.Phase)
			}

			if d := got.Status.CaptureState.DomainSpecificController; d.Reason != string(vmscondition.AbortedByModuleUpdate) {
				t.Errorf("domain reason = %q, want AbortedByModuleUpdate", d.Reason)
			}

			if got.Status.Consistent == nil || *got.Status.Consistent {
				t.Errorf("consistent = %v, want false", got.Status.Consistent)
			}

			if unfreezer.calls != 1 {
				t.Errorf("unfreezes = %d, want 1", unfreezer.calls)
			}

			wantRequest, wantOK := tc.kvvmi.Annotations[annotations.AnnVMFilesystemRequest]
			if request, ok := filesystemRequest(t, r.Client); request != wantRequest || ok != wantOK {
				t.Errorf("filesystem request = %q (%v), want %q (%v): the coordinator removes it", request, ok, wantRequest, wantOK)
			}

			if claim := getClaim(t, r.Client, got); claim != nil {
				t.Error("an access claim was created for a snapshot started before the update")
			}
		})
	}
}

// A capture failed before the module update was written before its guest was unfrozen, so the snapshot
// unfreezes the guest before it fails.
func TestReconcile_ACaptureFailedWithoutAClaimUnfreezes(t *testing.T) {
	vms := settledSnapshot()
	vms.Status.CaptureState.DomainSpecificController.Phase = v1alpha2.UnifiedSnapshotterPhase(snapshotsdk.PhaseFailed)
	r, unfreezer := startedBeforeUpdateFixture(t, vms, runningInstance(service.FSFrozen, ""), nil)

	reconcileOnce(t, r)

	if got := getVMS(t, r.Client); got.Status.Phase != v1alpha2.VirtualMachineSnapshotPhaseFailed {
		t.Fatalf("phase = %q, want Failed", got.Status.Phase)
	}

	if unfreezer.calls != 1 {
		t.Errorf("unfreezes = %d, want 1", unfreezer.calls)
	}
}

// A guest under a coordinator session is frozen for that session, which unfreezes it.
func TestReconcile_ASnapshotStartedWithoutAClaimLeavesASessionFreezeAlone(t *testing.T) {
	r, unfreezer := startedBeforeUpdateFixture(t, settledSnapshot(), runningInstance(service.FSFrozen, ""), &vmaccess.Lock{ClaimUID: "other-claim"})

	reconcileOnce(t, r)

	if got := getVMS(t, r.Client); got.Status.Phase != v1alpha2.VirtualMachineSnapshotPhaseFailed {
		t.Fatalf("phase = %q, want Failed", got.Status.Phase)
	}

	if unfreezer.calls != 0 {
		t.Error("the freeze of a coordinator session was released")
	}
}

// A stopped virtual machine has no guest to unfreeze.
func TestReconcile_ASnapshotStartedWithoutAClaimOfAStoppedMachineFails(t *testing.T) {
	kvvmi := runningInstance("", "")
	kvvmi.Status.Phase = virtv1.Succeeded
	r, unfreezer := startedBeforeUpdateFixture(t, settledSnapshot(), kvvmi, nil)

	reconcileOnce(t, r)

	if got := getVMS(t, r.Client); got.Status.Phase != v1alpha2.VirtualMachineSnapshotPhaseFailed {
		t.Fatalf("phase = %q, want Failed", got.Status.Phase)
	}

	if unfreezer.calls != 0 {
		t.Errorf("unfreezes = %d, want 0", unfreezer.calls)
	}
}

// A snapshot in progress or failed without a claim may have been started before the module update, so
// deleting it unfreezes the guest it may have left frozen.
func TestReconcileDeletion_UnfreezesForASnapshotStartedWithoutAClaim(t *testing.T) {
	for name, phase := range map[string]v1alpha2.VirtualMachineSnapshotPhase{
		"in progress": v1alpha2.VirtualMachineSnapshotPhaseInProgress,
		"failed":      v1alpha2.VirtualMachineSnapshotPhaseFailed,
	} {
		t.Run(name, func(t *testing.T) {
			vms := settledSnapshot()
			vms.Status.Phase = phase
			now := metav1.Now()
			vms.DeletionTimestamp = &now
			r, unfreezer := startedBeforeUpdateFixture(t, vms, runningInstance(service.FSFrozen, ""), nil)

			reconcileOnce(t, r)

			if unfreezer.calls != 1 {
				t.Errorf("unfreezes = %d, want 1", unfreezer.calls)
			}

			got := &v1alpha2.VirtualMachineSnapshot{}

			err := r.Client.Get(context.Background(), vms1.NamespacedName, got)
			if err == nil && controllerutil.ContainsFinalizer(got, v1alpha2.FinalizerVMSnapshotCleanup) {
				t.Error("the finalizer was not removed")
			}
		})
	}
}

// A snapshot deleted before its phase was bootstrapped is new and has frozen nothing, unless it carries the
// freeze request of the controller before the module update: deleting such a snapshot unfreezes the guest.
func TestReconcileDeletion_OfASnapshotWithoutAPhase(t *testing.T) {
	for name, tc := range map[string]struct {
		freezeRequested bool
		wantUnfreezes   int
	}{
		"new snapshot":                   {freezeRequested: false, wantUnfreezes: 0},
		"freeze requested before update": {freezeRequested: true, wantUnfreezes: 1},
	} {
		t.Run(name, func(t *testing.T) {
			vms := settledSnapshot()

			vms.Status = v1alpha2.VirtualMachineSnapshotStatus{}
			if tc.freezeRequested {
				vms.Annotations[annFSFreezeRequestedAt] = "2026-11-01T00:00:00Z"
			}

			now := metav1.Now()
			vms.DeletionTimestamp = &now
			r, unfreezer := startedBeforeUpdateFixture(t, vms, runningInstance(service.FSFrozen, ""), nil)

			reconcileOnce(t, r)

			if unfreezer.calls != tc.wantUnfreezes {
				t.Errorf("unfreezes = %d, want %d", unfreezer.calls, tc.wantUnfreezes)
			}
		})
	}
}

// Consistency comes from the claim only, so a capture finished before the module update is not consistent.
func TestReconcile_ACaptureFinishedWithoutAClaimIsNotConsistent(t *testing.T) {
	vms := settledSnapshot()
	vms.Status.CaptureState.DomainSpecificController.Phase = v1alpha2.UnifiedSnapshotterPhase(snapshotsdk.PhaseFinished)
	child := readyChild()
	child.Status.Consistent = ptr.To(true)
	r := newFullTestReconciler(t, vms, child)

	reconcileOnce(t, r)

	got := getVMS(t, r.Client)
	if got.Status.Phase != v1alpha2.VirtualMachineSnapshotPhaseReady {
		t.Fatalf("phase = %q, want Ready", got.Status.Phase)
	}

	if got.Status.Consistent == nil || *got.Status.Consistent {
		t.Errorf("consistent = %v, want false", got.Status.Consistent)
	}
}

// Deleting a snapshot releases its access; the claim goes away with the snapshot, and its own finalizer
// makes the coordinator unfreeze the guest first.
func TestReconcileDeletion_ReleasesTheAccess(t *testing.T) {
	vms := settledSnapshot()
	now := metav1.Now()
	vms.DeletionTimestamp = &now
	claim := grantedClaim(vms)
	r := newFullTestReconciler(t, vms, claim)

	reconcileOnce(t, r)

	if !getClaim(t, r.Client, vms).Spec.Release {
		t.Error("the access was not released")
	}

	err := r.Client.Get(context.Background(), vms1.NamespacedName, &v1alpha2.VirtualMachineSnapshot{})
	if !apierrors.IsNotFound(err) {
		t.Errorf("the snapshot is still there (%v); its finalizer should be gone", err)
	}
}

// The deadlines the controller bounds itself fire before the coordinator's, so the snapshot fails with a
// message that says what was waited on.
func TestClaimTimeoutCoversTheDeadlines(t *testing.T) {
	if planningDeadline+childrenSettleDeadline >= accessclaim.Timeout {
		t.Errorf("planning %s + children settle %s do not fit the claim timeout %s",
			planningDeadline, childrenSettleDeadline, accessclaim.Timeout)
	}
}
