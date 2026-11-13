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
	"reflect"
	"slices"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/controller/unified-snapshotter/internal/accessclaim"
	coordinationv1alpha1 "github.com/deckhouse/virtualization/api/coordination/v1alpha1"
	"github.com/deckhouse/virtualization/api/coordination/v1alpha1/vmaccondition"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdscondition"
)

var vds1 = ctrl.Request{NamespacedName: types.NamespacedName{Namespace: testNamespace, Name: "vds1"}}

func reconcileVDS(t *testing.T, r *Reconciler) {
	t.Helper()

	if _, err := r.Reconcile(context.Background(), vds1); err != nil {
		t.Fatal(err)
	}
}

func getClaim(t *testing.T, c client.Client, name string) *coordinationv1alpha1.VirtualMachineAccessClaim {
	t.Helper()

	claim, err := accessclaim.Get(context.Background(), c, c, testNamespace, name)
	if err != nil {
		t.Fatal(err)
	}

	return claim
}

// attachedDisk is a bound disk attached to vm1, with vm1 itself.
func attachedDisk(attachedTo ...string) []client.Object {
	vd := &v1alpha2.VirtualDisk{
		ObjectMeta: metav1.ObjectMeta{Name: "vd1", Namespace: testNamespace, UID: "vd1-uid"},
		Status:     v1alpha2.VirtualDiskStatus{Target: v1alpha2.DiskTarget{PersistentVolumeClaim: "pvc1"}},
	}
	for _, name := range attachedTo {
		vd.Status.AttachedToVirtualMachines = append(vd.Status.AttachedToVirtualMachines, v1alpha2.AttachedVirtualMachine{Name: name})
	}

	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: "pvc1", Namespace: testNamespace, UID: "pvc1-uid"},
		Status:     corev1.PersistentVolumeClaimStatus{Phase: corev1.ClaimBound},
	}
	vm := &v1alpha2.VirtualMachine{ObjectMeta: metav1.ObjectMeta{Name: "vm1", Namespace: testNamespace, UID: "vm1-uid"}}

	return []client.Object{vd, pvc, vm}
}

func standaloneVDS(requiredConsistency bool) *v1alpha2.VirtualDiskSnapshot {
	return &v1alpha2.VirtualDiskSnapshot{
		ObjectMeta: metav1.ObjectMeta{
			Name: "vds1", Namespace: testNamespace, UID: "vds1-uid",
			Annotations: map[string]string{v1alpha2.AnnUseUnifiedSnapshotter: ""},
		},
		Spec:   v1alpha2.VirtualDiskSnapshotSpec{VirtualDiskName: "vd1", RequiredConsistency: requiredConsistency},
		Status: v1alpha2.VirtualDiskSnapshotStatus{Phase: v1alpha2.VirtualDiskSnapshotPhasePending},
	}
}

func claimOf(vds *v1alpha2.VirtualDiskSnapshot, phase coordinationv1alpha1.AccessClaimPhase, reason, message string) *coordinationv1alpha1.VirtualMachineAccessClaim {
	claim := &coordinationv1alpha1.VirtualMachineAccessClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:            accessclaim.Name(vds),
			Namespace:       testNamespace,
			OwnerReferences: []metav1.OwnerReference{*metav1.NewControllerRef(vds, v1alpha2.SchemeGroupVersion.WithKind(v1alpha2.VirtualDiskSnapshotKind))},
		},
		Spec: coordinationv1alpha1.VirtualMachineAccessClaimSpec{
			VirtualMachine: coordinationv1alpha1.VirtualMachineAccessClaimTarget{Name: "vm1", UID: "vm1-uid"},
			Access:         []coordinationv1alpha1.AccessMode{coordinationv1alpha1.AccessModeSnapshot, coordinationv1alpha1.AccessModeFreeze},
			FreezePolicy:   ptr.To(coordinationv1alpha1.FreezePolicyRequired),
			Timeout:        metav1.Duration{Duration: accessclaim.Timeout},
		},
		Status: coordinationv1alpha1.VirtualMachineAccessClaimStatus{Phase: phase},
	}
	if reason != "" {
		claim.Status.Conditions = []metav1.Condition{{Type: vmaccondition.GrantedType.String(), Reason: reason, Message: message, Status: metav1.ConditionFalse}}
	}

	return claim
}

// A snapshot taken on its own asks for access to the machine its disk is attached to and captures nothing
// before it is granted. requiredConsistency decides whether the access includes the freeze.
func TestReconcile_AStandaloneSnapshotRequestsAccess(t *testing.T) {
	for name, tc := range map[string]struct {
		requiredConsistency bool
		access              []coordinationv1alpha1.AccessMode
		policy              *coordinationv1alpha1.FreezePolicy
	}{
		"consistency required": {
			requiredConsistency: true,
			access:              []coordinationv1alpha1.AccessMode{coordinationv1alpha1.AccessModeSnapshot, coordinationv1alpha1.AccessModeFreeze},
			policy:              ptr.To(coordinationv1alpha1.FreezePolicyRequired),
		},
		"consistency not required": {
			requiredConsistency: false,
			access:              []coordinationv1alpha1.AccessMode{coordinationv1alpha1.AccessModeSnapshot},
		},
	} {
		t.Run(name, func(t *testing.T) {
			vds := standaloneVDS(tc.requiredConsistency)
			r := newTestReconciler(t, append(attachedDisk("vm1"), vds)...)

			reconcileVDS(t, r)

			claim := getClaim(t, r.Client, accessclaim.Name(vds))
			if claim == nil {
				t.Fatal("no access claim was created")
			}

			if !slices.Equal(claim.Spec.Access, tc.access) || !reflect.DeepEqual(claim.Spec.FreezePolicy, tc.policy) {
				t.Errorf("claim access = %v, policy = %v; want %v, %v", claim.Spec.Access, claim.Spec.FreezePolicy, tc.access, tc.policy)
			}

			if claim.Spec.VirtualMachine.UID != "vm1-uid" {
				t.Errorf("claim targets %+v", claim.Spec.VirtualMachine)
			}

			got := getVDS(t, r.Client)
			if d := got.Status.CaptureState.DomainSpecificController; d.VolumeCaptureRequestName != "" {
				t.Errorf("the data capture started before the access was granted: %+v", d)
			}
		})
	}
}

// A disk attached to no machine needs no access.
func TestReconcile_ADetachedDiskNeedsNoAccess(t *testing.T) {
	vds := standaloneVDS(true)
	r := newTestReconciler(t, append(attachedDisk(), vds)...)

	reconcileVDS(t, r)

	if claim := getClaim(t, r.Client, accessclaim.Name(vds)); claim != nil {
		t.Error("an access claim was created for a detached disk")
	}

	got := getVDS(t, r.Client)
	if !startedOnUnattachedDisk(got) {
		t.Error("the snapshot is not marked as started on an unattached disk")
	}

	if got.Status.Consistent != nil {
		t.Errorf("consistent = %v before the capture is finished, want it unset", *got.Status.Consistent)
	}

	if d := got.Status.CaptureState.DomainSpecificController; d.VolumeCaptureRequestName == "" {
		t.Error("the data capture did not start")
	}
}

// A snapshot without a claim past planning is settled by the mark: a disk that stayed unattached gives a
// consistent snapshot, a disk attached during the capture fails it, and a snapshot without the mark, started
// before the module update, fails whatever consistency it recorded then.
func TestReconcile_ASnapshotWithoutAClaimIsSettledByTheMark(t *testing.T) {
	for name, tc := range map[string]struct {
		marked     bool
		attachedTo []string
		phase      v1alpha2.VirtualDiskSnapshotPhase
		reason     vdscondition.VirtualDiskSnapshotReadyReason
		consistent bool
	}{
		"the disk stayed unattached":            {marked: true, phase: v1alpha2.VirtualDiskSnapshotPhaseReady, consistent: true},
		"the disk was attached":                 {marked: true, attachedTo: []string{"vm1"}, phase: v1alpha2.VirtualDiskSnapshotPhaseFailed, reason: vdscondition.VirtualMachineStateChanged},
		"the disk was attached to two machines": {marked: true, attachedTo: []string{"vm1", "vm2"}, phase: v1alpha2.VirtualDiskSnapshotPhaseFailed, reason: vdscondition.VirtualMachineStateChanged},
		"started before the module update":      {attachedTo: []string{"vm1"}, phase: v1alpha2.VirtualDiskSnapshotPhaseFailed, reason: vdscondition.AbortedByModuleUpdate},
	} {
		t.Run(name, func(t *testing.T) {
			vds := capturedVDS()
			if tc.marked {
				vds.Annotations[annotations.AnnStartedOnUnattachedDisk] = "true"
			} else {
				vds.Status.Consistent = ptr.To(true)
			}

			r := newTestReconciler(t, append(attachedDisk(tc.attachedTo...), vds)...)

			reconcileVDS(t, r)

			got := getVDS(t, r.Client)
			if got.Status.Phase != tc.phase {
				d := got.Status.CaptureState.DomainSpecificController
				t.Fatalf("phase = %q, want %q (reason %q, message %q)", got.Status.Phase, tc.phase, d.Reason, d.Message)
			}

			if tc.phase == v1alpha2.VirtualDiskSnapshotPhaseFailed {
				if d := got.Status.CaptureState.DomainSpecificController; d.Reason != string(tc.reason) {
					t.Errorf("reason = %q, want %q", d.Reason, tc.reason)
				}
			}

			if got.Status.Consistent == nil || *got.Status.Consistent != tc.consistent {
				t.Errorf("consistent = %v, want %v", got.Status.Consistent, tc.consistent)
			}

			if claim := getClaim(t, r.Client, accessclaim.Name(vds)); claim != nil {
				t.Error("an access claim was created past planning")
			}
		})
	}
}

func TestReconcile_RefusedAccessFailsTheStandaloneSnapshot(t *testing.T) {
	const message = `The virtual machine "vm1" is migrating. Retry after the migration completes.`

	vds := standaloneVDS(true)
	claim := claimOf(vds, coordinationv1alpha1.AccessClaimPhaseDenied, string(vmaccondition.VirtualMachineBusy), message)
	r := newTestReconciler(t, append(attachedDisk("vm1"), vds, claim)...)

	reconcileVDS(t, r)

	got := getVDS(t, r.Client)
	if got.Status.Phase != v1alpha2.VirtualDiskSnapshotPhaseFailed {
		t.Fatalf("phase = %q, want Failed", got.Status.Phase)
	}

	if d := got.Status.CaptureState.DomainSpecificController; d.Reason != string(vmaccondition.VirtualMachineBusy) || d.Message != message {
		t.Errorf("domain state = %+v, want the coordinator's reason and message", d)
	}
}

// capturedVDS is a snapshot taken on its own whose manifests and data are captured.
func capturedVDS() *v1alpha2.VirtualDiskSnapshot {
	vds := standaloneVDS(true)
	vds.Status.Phase = v1alpha2.VirtualDiskSnapshotPhaseInProgress
	vds.Status.CaptureState = &v1alpha2.UnifiedSnapshotterCaptureState{
		DomainSpecificController: &v1alpha2.UnifiedSnapshotterDomainCaptureState{
			Phase:                      v1alpha2.UnifiedSnapshotterPhasePlanned,
			VolumeCaptureRequestName:   "vcr1",
			ManifestCaptureRequestName: "mcr1",
		},
		CommonController: &v1alpha2.UnifiedSnapshotterCommonCaptureState{
			ManifestCaptured: ptr.To(true),
			DataCaptured:     ptr.To(true),
		},
	}
	vds.Status.Data = &v1alpha2.UnifiedSnapshotterDataBinding{}

	return vds
}

// Its disk captured, the snapshot releases its access and turns Ready with the consistency of the verdict.
func TestReconcile_AStandaloneSnapshotIsReadyWithTheVerdict(t *testing.T) {
	vds := capturedVDS()
	claim := claimOf(vds, coordinationv1alpha1.AccessClaimPhaseGranted, string(vmaccondition.Frozen), "")
	r := newTestReconciler(t, append(attachedDisk("vm1"), vds, claim)...)

	reconcileVDS(t, r)

	released := getClaim(t, r.Client, accessclaim.Name(vds))
	if !released.Spec.Release {
		t.Fatal("the access was not released")
	}

	if got := getVDS(t, r.Client); got.Status.Phase == v1alpha2.VirtualDiskSnapshotPhaseReady {
		t.Fatal("the snapshot turned Ready before the verdict")
	}

	released.Status.Phase = coordinationv1alpha1.AccessClaimPhaseCompleted
	released.Status.GrantedAt = ptr.To(metav1.Now())
	released.Status.Frozen = true

	released.Status.VirtualMachineInstanceUID = "vmi-uid"
	if err := r.Client.Status().Update(context.Background(), released); err != nil {
		t.Fatal(err)
	}

	reconcileVDS(t, r)

	got := getVDS(t, r.Client)
	if got.Status.Phase != v1alpha2.VirtualDiskSnapshotPhaseReady {
		t.Fatalf("phase = %q, want Ready", got.Status.Phase)
	}

	if got.Status.Consistent == nil || !*got.Status.Consistent {
		t.Errorf("consistent = %v, want true", got.Status.Consistent)
	}
}

// A captured snapshot does not hold on to its disk: once the verdict arrives it turns Ready even though the
// disk was deleted while it waited.
func TestReconcile_AVerdictSettlesTheSnapshotOfADeletedDisk(t *testing.T) {
	vds := capturedVDS()
	claim := claimOf(vds, coordinationv1alpha1.AccessClaimPhaseCompleted, string(vmaccondition.Released), "")
	claim.Spec.Release = true
	claim.Status.GrantedAt = ptr.To(metav1.Now())
	claim.Status.Frozen = true
	claim.Status.VirtualMachineInstanceUID = "vmi-uid"
	r := newTestReconciler(t, vds, claim)

	reconcileVDS(t, r)

	got := getVDS(t, r.Client)
	if got.Status.Phase != v1alpha2.VirtualDiskSnapshotPhaseReady {
		t.Fatalf("phase = %q, want Ready", got.Status.Phase)
	}

	if d := got.Status.CaptureState.DomainSpecificController; d.Phase != v1alpha2.UnifiedSnapshotterPhaseFinished {
		t.Errorf("domain phase = %q, want Finished", d.Phase)
	}

	if got.Status.Consistent == nil || !*got.Status.Consistent {
		t.Errorf("consistent = %v, want true", got.Status.Consistent)
	}
}

// A failed capture stays failed when its phase was not written before the access was released: the released
// claim completes, and its verdict must not turn the snapshot Ready.
func TestReconcile_AFailedCaptureIsNotMadeReadyByTheCompletedAccess(t *testing.T) {
	vds := capturedVDS()
	vds.Status.CaptureState.DomainSpecificController.Phase = v1alpha2.UnifiedSnapshotterPhaseFailed
	claim := claimOf(vds, coordinationv1alpha1.AccessClaimPhaseCompleted, string(vmaccondition.Released), "")
	claim.Spec.Release = true
	claim.Status.Frozen = true
	r := newTestReconciler(t, append(attachedDisk("vm1"), vds, claim)...)

	reconcileVDS(t, r)

	got := getVDS(t, r.Client)
	if got.Status.Phase != v1alpha2.VirtualDiskSnapshotPhaseFailed {
		t.Fatalf("phase = %q, want Failed", got.Status.Phase)
	}

	if d := got.Status.CaptureState.DomainSpecificController; d.Phase != v1alpha2.UnifiedSnapshotterPhaseFailed {
		t.Errorf("domain phase = %q, want Failed", d.Phase)
	}
}

// A capture the core declares failed turns the domain phase Failed before the access is released, so a pass
// that sees the completed claim still reads the capture as failed.
func TestReconcile_ACoreCaptureFailureFailsTheDomainPhaseBeforeTheRelease(t *testing.T) {
	vds := capturedVDS()
	vds.Status.CaptureState.CommonController.DataCaptured = ptr.To(false)
	vds.Status.Data = nil
	vds.Status.Conditions = []metav1.Condition{{
		Type:    v1alpha2.UnifiedSnapshotterConditionReady,
		Status:  metav1.ConditionFalse,
		Reason:  "VolumeCaptureFailed",
		Message: "boom",
	}}
	claim := claimOf(vds, coordinationv1alpha1.AccessClaimPhaseGranted, string(vmaccondition.Frozen), "")
	r := newTestReconciler(t, append(attachedDisk("vm1"), vds, claim)...)

	reconcileVDS(t, r)

	if released := getClaim(t, r.Client, accessclaim.Name(vds)); !released.Spec.Release {
		t.Fatal("the access was not released")
	}

	got := getVDS(t, r.Client)
	if got.Status.Phase != v1alpha2.VirtualDiskSnapshotPhaseFailed {
		t.Fatalf("phase = %q, want Failed", got.Status.Phase)
	}

	if d := got.Status.CaptureState.DomainSpecificController; d.Phase != v1alpha2.UnifiedSnapshotterPhaseFailed || d.Reason != "VolumeCaptureFailed" {
		t.Errorf("domain state = %+v, want Failed with the core's reason", d)
	}
}

func parentVMS(annotations map[string]string) *v1alpha2.VirtualMachineSnapshot {
	return &v1alpha2.VirtualMachineSnapshot{
		ObjectMeta: metav1.ObjectMeta{Name: "vms1", Namespace: testNamespace, UID: "vms1-uid", Annotations: annotations},
		Status: v1alpha2.VirtualMachineSnapshotStatus{
			CaptureState: &v1alpha2.UnifiedSnapshotterCaptureState{},
		},
	}
}

func childVDS(parent *v1alpha2.VirtualMachineSnapshot, phase v1alpha2.VirtualDiskSnapshotPhase) *v1alpha2.VirtualDiskSnapshot {
	vds := standaloneVDS(true)
	vds.Annotations = nil
	vds.OwnerReferences = []metav1.OwnerReference{*metav1.NewControllerRef(parent, v1alpha2.SchemeGroupVersion.WithKind(v1alpha2.VirtualMachineSnapshotKind))}
	vds.Status.Phase = phase
	vds.Status.CaptureState = &v1alpha2.UnifiedSnapshotterCaptureState{
		DomainSpecificController: &v1alpha2.UnifiedSnapshotterDomainCaptureState{Phase: v1alpha2.UnifiedSnapshotterPhasePlanned},
	}

	return vds
}

// A child is captured under its parent's access and takes its consistency from the parent once the parent
// is terminal: the parent records the verdict of the session, which ends only after the child is Ready.
func TestReconcile_AChildTakesItsConsistencyFromTheParent(t *testing.T) {
	for name, tc := range map[string]struct {
		phase      v1alpha2.VirtualMachineSnapshotPhase
		consistent *bool
		noParent   bool
		replaced   bool
		want       *bool
	}{
		"parent in progress":         {phase: v1alpha2.VirtualMachineSnapshotPhaseInProgress, want: nil},
		"parent ready, consistent":   {phase: v1alpha2.VirtualMachineSnapshotPhaseReady, consistent: ptr.To(true), want: ptr.To(true)},
		"parent ready, inconsistent": {phase: v1alpha2.VirtualMachineSnapshotPhaseReady, consistent: ptr.To(false), want: ptr.To(false)},
		"parent failed":              {phase: v1alpha2.VirtualMachineSnapshotPhaseFailed, consistent: ptr.To(false), want: ptr.To(false)},
		"parent gone":                {noParent: true, want: ptr.To(false)},
		"parent replaced":            {phase: v1alpha2.VirtualMachineSnapshotPhaseInProgress, replaced: true, want: ptr.To(false)},
	} {
		t.Run(name, func(t *testing.T) {
			parent := parentVMS(nil)
			child := childVDS(parent, v1alpha2.VirtualDiskSnapshotPhaseReady)
			objs := []client.Object{child}

			if !tc.noParent {
				parent.Status.Phase = tc.phase

				parent.Status.Consistent = tc.consistent
				if tc.replaced {
					parent.UID = "vms1-other-uid"
				}

				objs = append(objs, parent)
			}

			r := newTestReconciler(t, objs...)

			reconcileVDS(t, r)

			got := getVDS(t, r.Client).Status.Consistent
			if (got == nil) != (tc.want == nil) || (got != nil && *got != *tc.want) {
				t.Errorf("consistent = %v, want %v", got, tc.want)
			}
		})
	}
}

// A child whose parent has no claim was started before the module update, with the parent: it fails whatever
// consistency it recorded then, as the parent fails and unfreezes the guest during the capture.
func TestReconcile_AChildOfAParentStartedBeforeTheUpdateFails(t *testing.T) {
	for name, tc := range map[string]struct {
		parentClaim bool
		phase       v1alpha2.VirtualDiskSnapshotPhase
	}{
		"the parent has a claim":  {parentClaim: true, phase: v1alpha2.VirtualDiskSnapshotPhaseInProgress},
		"the parent has no claim": {phase: v1alpha2.VirtualDiskSnapshotPhaseFailed},
	} {
		t.Run(name, func(t *testing.T) {
			parent := parentVMS(nil)
			parent.Status.Phase = v1alpha2.VirtualMachineSnapshotPhaseInProgress
			child := childVDS(parent, v1alpha2.VirtualDiskSnapshotPhaseInProgress)
			child.Status.Consistent = ptr.To(true)

			objs := append(attachedDisk("vm1"), parent, child)
			if tc.parentClaim {
				objs = append(objs, &coordinationv1alpha1.VirtualMachineAccessClaim{
					ObjectMeta: metav1.ObjectMeta{Name: accessclaim.Name(parent), Namespace: testNamespace},
				})
			}

			r := newTestReconciler(t, objs...)

			reconcileVDS(t, r)

			got := getVDS(t, r.Client)
			if got.Status.Phase != tc.phase {
				t.Fatalf("phase = %q, want %q", got.Status.Phase, tc.phase)
			}

			if tc.phase == v1alpha2.VirtualDiskSnapshotPhaseFailed {
				if d := got.Status.CaptureState.DomainSpecificController; d.Reason != string(vdscondition.AbortedByModuleUpdate) {
					t.Errorf("reason = %q, want %q", d.Reason, vdscondition.AbortedByModuleUpdate)
				}

				if got.Status.Consistent == nil || *got.Status.Consistent {
					t.Errorf("consistent = %v, want false", got.Status.Consistent)
				}
			}
		})
	}
}

// A snapshot holding its access fails instead of waiting for a disk that is gone: the session of a stopped
// virtual machine has no deadline, so the wait would hold the machine with no end.
func TestReconcile_ASnapshotHoldingItsAccessFailsOnALostDisk(t *testing.T) {
	for name, objs := range map[string]func() []client.Object{
		"the disk is deleted": func() []client.Object { return attachedDisk("vm1")[2:] },
		"the disk storage is deleted": func() []client.Object {
			objs := attachedDisk("vm1")
			return []client.Object{objs[0], objs[2]}
		},
	} {
		t.Run(name, func(t *testing.T) {
			vds := standaloneVDS(false)
			claim := claimOf(vds, coordinationv1alpha1.AccessClaimPhaseGranted, string(vmaccondition.NotRequired), "")
			claim.Status.GrantedAt = ptr.To(metav1.Now())
			r := newTestReconciler(t, append(objs(), vds, claim)...)

			reconcileVDS(t, r)

			got := getVDS(t, r.Client)
			if got.Status.Phase != v1alpha2.VirtualDiskSnapshotPhaseFailed {
				t.Fatalf("phase = %q, want Failed", got.Status.Phase)
			}

			if !getClaim(t, r.Client, accessclaim.Name(vds)).Spec.Release {
				t.Error("the access was not released")
			}
		})
	}
}
