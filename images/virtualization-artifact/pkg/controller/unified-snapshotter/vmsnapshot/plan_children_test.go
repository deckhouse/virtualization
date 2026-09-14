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
	"fmt"
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	storagev1alpha1 "github.com/deckhouse/state-snapshotter/api/storage/v1alpha1"
	"github.com/deckhouse/state-snapshotter/pkg/snapshotsdk"
	"github.com/deckhouse/virtualization-controller/pkg/controller/unified-snapshotter/internal/annotation"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

func TestPlanChildren_AddressesDisksBySourceRef(t *testing.T) {
	r := newFullTestReconciler(t,
		&v1alpha2.VirtualDisk{ObjectMeta: metav1.ObjectMeta{Name: "disk-a", Namespace: testNamespace, UID: types.UID("disk-uid-a")}},
		&v1alpha2.VirtualDisk{ObjectMeta: metav1.ObjectMeta{Name: "disk-b", Namespace: testNamespace, UID: types.UID("disk-uid-b")}},
	)
	vms := &v1alpha2.VirtualMachineSnapshot{
		ObjectMeta: metav1.ObjectMeta{Name: "vms", Namespace: testNamespace, UID: types.UID("uid-1")},
		Spec:       v1alpha2.VirtualMachineSnapshotSpec{RequiredConsistency: true},
	}
	vm := &v1alpha2.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{Name: "vm", Namespace: testNamespace},
		Status: v1alpha2.VirtualMachineStatus{BlockDeviceRefs: []v1alpha2.BlockDeviceStatusRef{
			{Kind: v1alpha2.DiskDevice, Name: "disk-a"},
			{Kind: v1alpha2.ImageDevice, Name: "image-a"},
			{Kind: v1alpha2.DiskDevice, Name: "disk-b"},
		}},
	}

	specs, _, err := r.planChildren(context.Background(), vms, vm)
	if err != nil {
		t.Fatalf("planChildren: %v", err)
	}
	if got, want := len(specs), 2; got != want {
		t.Fatalf("planned %d children, want %d (images carry no data leg)", got, want)
	}
	for i, wantDisk := range []string{"disk-a", "disk-b"} {
		child, ok := specs[i].Object.(*v1alpha2.VirtualDiskSnapshot)
		if !ok {
			t.Fatalf("child %d is %T, want *VirtualDiskSnapshot", i, specs[i].Object)
		}
		if child.Spec.SourceRef == nil {
			t.Fatalf("child %d has no spec.sourceRef", i)
		}
		if child.Spec.SourceRef.Name != wantDisk ||
			child.Spec.SourceRef.Kind != v1alpha2.VirtualDiskKind ||
			child.Spec.SourceRef.APIVersion != v1alpha2.SchemeGroupVersion.String() {
			t.Errorf("child %d sourceRef = %+v, want %s %s/%s", i, *child.Spec.SourceRef,
				v1alpha2.SchemeGroupVersion.String(), v1alpha2.VirtualDiskKind, wantDisk)
		}
		// The two spec shapes are mutually exclusive (CEL-enforced), so the name field must stay empty.
		if child.Spec.VirtualDiskName != "" {
			t.Errorf("child %d also sets spec.virtualDiskName = %q; the CRD rejects both at once", i, child.Spec.VirtualDiskName)
		}
		if !child.Spec.RequiredConsistency {
			t.Errorf("child %d did not inherit requiredConsistency", i)
		}
		driven, err := annotation.DrivenByUnifiedVirtualDiskSnapshot(context.Background(), r.APIReader, child)
		if err != nil {
			t.Errorf("error detecting snapshot mechanism for child %d: %v", i, err)
		}
		if !driven {
			t.Errorf("child %d is not routed to the unified snapshotter", i)
		}

		if child.Namespace != testNamespace {
			t.Errorf("child %d namespace = %q, want %q", i, child.Namespace, testNamespace)
		}
	}
	if specs[0].Object.GetName() == specs[1].Object.GetName() {
		t.Error("children share a name; create-or-adopt would collapse them into one")
	}
}

// Children inherit the parent's consistency mandate verbatim.
func TestPlanChildren_ChildrenInheritTheConsistencyMandate(t *testing.T) {
	r := newFullTestReconciler(t,
		&v1alpha2.VirtualDisk{ObjectMeta: metav1.ObjectMeta{Name: "disk-a", Namespace: testNamespace, UID: types.UID("disk-uid-a")}},
	)
	vm := &v1alpha2.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{Name: "vm", Namespace: testNamespace},
		Status: v1alpha2.VirtualMachineStatus{BlockDeviceRefs: []v1alpha2.BlockDeviceStatusRef{
			{Kind: v1alpha2.DiskDevice, Name: "disk-a"},
		}},
	}

	owners := []struct {
		name   string
		owners []metav1.OwnerReference
	}{
		{
			name: "asked for by a user",
		},
		{
			name: "planned by the core for a namespace snapshot",
			owners: []metav1.OwnerReference{{
				APIVersion: "state-snapshotter.deckhouse.io/v1alpha1",
				Kind:       "Snapshot",
				Name:       "ns-snap-1",
			}},
		},
		{
			name: "owned by something else entirely",
			owners: []metav1.OwnerReference{{
				APIVersion: v1alpha2.SchemeGroupVersion.String(),
				Kind:       v1alpha2.VirtualMachineKind,
				Name:       "vm",
			}},
		},
	}

	for _, tt := range owners {
		for _, mandate := range []bool{true, false} {
			t.Run(fmt.Sprintf("%s/requiredConsistency=%v", tt.name, mandate), func(t *testing.T) {
				vms := &v1alpha2.VirtualMachineSnapshot{
					ObjectMeta: metav1.ObjectMeta{
						Name: "vms", Namespace: testNamespace, UID: types.UID("uid-1"),
						OwnerReferences: tt.owners,
					},
					Spec: v1alpha2.VirtualMachineSnapshotSpec{RequiredConsistency: mandate},
				}

				specs, _, err := r.planChildren(context.Background(), vms, vm)
				if err != nil {
					t.Fatalf("planChildren: %v", err)
				}
				if len(specs) != 1 {
					t.Fatalf("planned %d children, want 1", len(specs))
				}
				child := specs[0].Object.(*v1alpha2.VirtualDiskSnapshot)
				if got := child.Spec.RequiredConsistency; got != mandate {
					t.Errorf("child requiredConsistency = %v, want %v (inherited verbatim)", got, mandate)
				}
			})
		}
	}
}

func TestPlanChildren_HonoursTheExcludeVeto(t *testing.T) {
	vetoed := &v1alpha2.VirtualDisk{ObjectMeta: metav1.ObjectMeta{
		Name: "disk-vetoed", Namespace: testNamespace, UID: types.UID("disk-uid-vetoed"),
		Labels: map[string]string{storagev1alpha1.ExcludeLabelKey: ""},
	}}
	r := newFullTestReconciler(t,
		&v1alpha2.VirtualDisk{ObjectMeta: metav1.ObjectMeta{Name: "disk-kept", Namespace: testNamespace, UID: types.UID("disk-uid-kept")}},
		vetoed,
	)
	vms := &v1alpha2.VirtualMachineSnapshot{
		ObjectMeta: metav1.ObjectMeta{Name: "vms", Namespace: testNamespace, UID: types.UID("uid-1")},
	}
	vm := &v1alpha2.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{Name: "vm", Namespace: testNamespace},
		Status: v1alpha2.VirtualMachineStatus{BlockDeviceRefs: []v1alpha2.BlockDeviceStatusRef{
			{Kind: v1alpha2.DiskDevice, Name: "disk-kept"},
			{Kind: v1alpha2.DiskDevice, Name: "disk-vetoed"},
		}},
	}

	specs, excluded, err := r.planChildren(context.Background(), vms, vm)
	if err != nil {
		t.Fatalf("planChildren: %v", err)
	}

	if len(specs) != 1 {
		t.Fatalf("planned %d children, want 1 (the vetoed disk must not be captured)", len(specs))
	}
	child := specs[0].Object.(*v1alpha2.VirtualDiskSnapshot)
	if child.Spec.SourceRef.Name != "disk-kept" {
		t.Errorf("planned a child for %q, want disk-kept", child.Spec.SourceRef.Name)
	}

	if len(excluded) != 1 {
		t.Fatalf("reported %d excluded refs, want 1: %+v", len(excluded), excluded)
	}
	want := snapshotsdk.ExcludedObjectRef{
		APIVersion: v1alpha2.SchemeGroupVersion.String(),
		Kind:       v1alpha2.VirtualDiskKind,
		Name:       "disk-vetoed",
	}
	if excluded[0] != want {
		t.Errorf("excluded ref = %+v, want %+v", excluded[0], want)
	}
}

// A disk the machine's status lists but the API does not have leaves nothing to plan from — neither a veto
// to read nor a UID to name the child by — so planning waits for the two views to agree.
func TestPlanChildren_MissingDiskWaits(t *testing.T) {
	r := newFullTestReconciler(t)
	vms := &v1alpha2.VirtualMachineSnapshot{
		ObjectMeta: metav1.ObjectMeta{Name: "vms", Namespace: testNamespace, UID: types.UID("uid-1")},
	}
	vm := &v1alpha2.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{Name: "vm", Namespace: testNamespace},
		Status: v1alpha2.VirtualMachineStatus{BlockDeviceRefs: []v1alpha2.BlockDeviceStatusRef{
			{Kind: v1alpha2.DiskDevice, Name: "disk-gone"},
		}},
	}

	if _, _, err := r.planChildren(context.Background(), vms, vm); !errors.Is(err, errSourceNotReady) {
		t.Fatalf("err = %v, want errSourceNotReady", err)
	}
}

// The child name must not grow with the disk name: disk names are bounded only by Kubernetes' 253, and a
// name derived from one would push the child past that limit, where it cannot be created at all.
func TestPlanChildren_ChildNameIsBoundedRegardlessOfDiskName(t *testing.T) {
	longName := strings.Repeat("d", 253)
	r := newFullTestReconciler(t,
		&v1alpha2.VirtualDisk{ObjectMeta: metav1.ObjectMeta{
			Name: longName, Namespace: testNamespace, UID: types.UID("disk-uid-long"),
		}},
	)
	vms := &v1alpha2.VirtualMachineSnapshot{
		ObjectMeta: metav1.ObjectMeta{Name: "vms", Namespace: testNamespace, UID: types.UID("uid-1")},
	}
	vm := &v1alpha2.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{Name: "vm", Namespace: testNamespace},
		Status: v1alpha2.VirtualMachineStatus{BlockDeviceRefs: []v1alpha2.BlockDeviceStatusRef{
			{Kind: v1alpha2.DiskDevice, Name: longName},
		}},
	}

	specs, _, err := r.planChildren(context.Background(), vms, vm)
	if err != nil {
		t.Fatalf("planChildren: %v", err)
	}
	if len(specs) != 1 {
		t.Fatalf("planned %d children, want 1", len(specs))
	}
	if got := len(specs[0].Object.GetName()); got > 253 {
		t.Errorf("child name is %d characters, over the Kubernetes limit of 253", got)
	}
	want := snapshotsdk.ChildSnapshotName(vms.UID, types.UID("disk-uid-long"))
	if got := specs[0].Object.GetName(); got != want {
		t.Errorf("child name = %q, want the core's own scheme %q", got, want)
	}
}
