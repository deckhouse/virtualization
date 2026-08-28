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

package adapter

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"

	storagev1alpha1 "github.com/deckhouse/state-snapshotter/api/storage/v1alpha1"
	"github.com/deckhouse/state-snapshotter/pkg/snapshotsdk"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

// adapterCases exercises both concrete adapters against the shared snapshotsdk.SnapshotAdapter contract
// (they share every helper function in vmsnapshot_adapter.go), so the round-trip/condition-getter behavior
// only needs to be proven once per case, not once per type.
func adapterCases() []struct {
	name string
	new  func() snapshotsdk.SnapshotAdapter
} {
	return []struct {
		name string
		new  func() snapshotsdk.SnapshotAdapter
	}{
		{"VirtualMachineSnapshotAdapter", func() snapshotsdk.SnapshotAdapter {
			return &VirtualMachineSnapshotAdapter{VMS: &v1alpha2.VirtualMachineSnapshot{}}
		}},
		{"VirtualDiskSnapshotAdapter", func() snapshotsdk.SnapshotAdapter {
			return &VirtualDiskSnapshotAdapter{VDS: &v1alpha2.VirtualDiskSnapshot{}}
		}},
	}
}

func TestDomainCaptureStateRoundTrip(t *testing.T) {
	for _, tc := range adapterCases() {
		t.Run(tc.name, func(t *testing.T) {
			a := tc.new()

			empty := a.GetDomainCaptureState()
			if empty.Phase != "" || empty.ManifestCaptureRequestName != "" || len(empty.ChildrenSnapshotRefs) != 0 {
				t.Fatalf("expected zero-value state before any Set, got %+v", empty)
			}
			if empty.ExcludedRefs == nil {
				t.Fatal("ExcludedRefs must be a non-nil empty slice on a fresh object (SDK contract: always present)")
			}

			want := snapshotsdk.DomainCaptureState{
				ManifestCaptureRequestName: "mcr-1",
				VolumeCaptureRequestName:   "vcr-1",
				Phase:                      snapshotsdk.PhasePlanned,
				Reason:                     "SomeReason",
				Message:                    "some message",
				ChildrenSnapshotRefs: []storagev1alpha1.SnapshotChildRef{
					{APIVersion: v1alpha2.SchemeGroupVersion.String(), Kind: v1alpha2.VirtualDiskSnapshotKind, Name: "child-1"},
				},
				ExcludedRefs: []snapshotsdk.ExcludedObjectRef{
					{APIVersion: "v1", Kind: "Secret", Name: "excluded-1"},
				},
			}
			a.SetDomainCaptureState(want)

			got := a.GetDomainCaptureState()
			if got.ManifestCaptureRequestName != want.ManifestCaptureRequestName ||
				got.VolumeCaptureRequestName != want.VolumeCaptureRequestName ||
				got.Phase != want.Phase ||
				got.Reason != want.Reason ||
				got.Message != want.Message {
				t.Fatalf("scalar fields did not round-trip: got %+v, want %+v", got, want)
			}
			if len(got.ChildrenSnapshotRefs) != 1 || got.ChildrenSnapshotRefs[0] != want.ChildrenSnapshotRefs[0] {
				t.Fatalf("ChildrenSnapshotRefs did not round-trip: got %+v", got.ChildrenSnapshotRefs)
			}
			if len(got.ExcludedRefs) != 1 || got.ExcludedRefs[0] != want.ExcludedRefs[0] {
				t.Fatalf("ExcludedRefs did not round-trip: got %+v", got.ExcludedRefs)
			}
		})
	}
}

func TestSnapshotSourceRoundTrip(t *testing.T) {
	for _, tc := range adapterCases() {
		t.Run(tc.name, func(t *testing.T) {
			a := tc.new()

			if got := a.GetSnapshotSource(); got != nil {
				t.Fatalf("expected nil source before any Set, got %+v", got)
			}

			want := &snapshotsdk.SnapshotSource{
				APIVersion: v1alpha2.SchemeGroupVersion.String(),
				Kind:       v1alpha2.VirtualMachineKind,
				Name:       "vm-1",
				Namespace:  "ns-1",
				UID:        types.UID("uid-1"),
			}
			a.SetSnapshotSource(want)

			got := a.GetSnapshotSource()
			if got == nil || *got != *want {
				t.Fatalf("got %+v, want %+v", got, want)
			}
		})
	}
}

func TestReadyConditionGetters(t *testing.T) {
	for _, tc := range adapterCases() {
		t.Run(tc.name, func(t *testing.T) {
			a := tc.new()

			if status := a.ReadyStatus(); status != "" {
				t.Fatalf("expected empty ReadyStatus with no conditions, got %q", status)
			}
			if reason := a.ReadyReason(); reason != "" {
				t.Fatalf("expected empty ReadyReason with no conditions, got %q", reason)
			}
			if msg := a.ReadyMessage(); msg != "" {
				t.Fatalf("expected empty ReadyMessage with no conditions, got %q", msg)
			}

			conditions := []metav1.Condition{
				{Type: "SomeOtherType", Status: metav1.ConditionTrue, Reason: "Irrelevant"},
				{Type: v1alpha2.UnifiedSnapshotterConditionReady, Status: metav1.ConditionFalse, Reason: "VolumeCaptureFailed", Message: "boom"},
			}
			switch obj := a.Object().(type) {
			case *v1alpha2.VirtualMachineSnapshot:
				obj.Status.Conditions = conditions
			case *v1alpha2.VirtualDiskSnapshot:
				obj.Status.Conditions = conditions
			}

			if status := a.ReadyStatus(); status != metav1.ConditionFalse {
				t.Fatalf("got ReadyStatus %q, want %q", status, metav1.ConditionFalse)
			}
			if reason := a.ReadyReason(); reason != "VolumeCaptureFailed" {
				t.Fatalf("got ReadyReason %q, want VolumeCaptureFailed", reason)
			}
			if msg := a.ReadyMessage(); msg != "boom" {
				t.Fatalf("got ReadyMessage %q, want boom", msg)
			}
		})
	}
}

func TestCoreCaptureStateFromStatus(t *testing.T) {
	t.Run("nil CaptureState maps to zero value", func(t *testing.T) {
		a := &VirtualMachineSnapshotAdapter{VMS: &v1alpha2.VirtualMachineSnapshot{}}
		got := a.CoreCaptureState()
		if got.ManifestCaptured != nil || got.DataCaptured != nil || got.ChildrenSettled != nil {
			t.Fatalf("expected all-nil latches, got %+v", got)
		}
	})

	t.Run("nil CommonController maps to zero value", func(t *testing.T) {
		a := &VirtualMachineSnapshotAdapter{VMS: &v1alpha2.VirtualMachineSnapshot{
			Status: v1alpha2.VirtualMachineSnapshotStatus{CaptureState: &v1alpha2.UnifiedSnapshotterCaptureState{}},
		}}
		got := a.CoreCaptureState()
		if got.ManifestCaptured != nil || got.DataCaptured != nil || got.ChildrenSettled != nil {
			t.Fatalf("expected all-nil latches, got %+v", got)
		}
	})

	t.Run("mirrors CommonController latches including ChildrenSettled", func(t *testing.T) {
		a := &VirtualMachineSnapshotAdapter{VMS: &v1alpha2.VirtualMachineSnapshot{
			Status: v1alpha2.VirtualMachineSnapshotStatus{CaptureState: &v1alpha2.UnifiedSnapshotterCaptureState{
				CommonController: &v1alpha2.UnifiedSnapshotterCommonCaptureState{
					ManifestCaptured: ptr.To(true),
					DataCaptured:     ptr.To(false),
					ChildrenSettled:  ptr.To(true),
				},
			}},
		}}
		got := a.CoreCaptureState()
		if got.ManifestCaptured == nil || !*got.ManifestCaptured {
			t.Fatalf("ManifestCaptured did not mirror: %+v", got)
		}
		if got.DataCaptured == nil || *got.DataCaptured {
			t.Fatalf("DataCaptured did not mirror: %+v", got)
		}
		if got.ChildrenSettled == nil || !*got.ChildrenSettled {
			t.Fatalf("ChildrenSettled did not mirror: %+v", got)
		}
	})

	t.Run("VirtualDiskSnapshotAdapter mirrors the same way", func(t *testing.T) {
		a := &VirtualDiskSnapshotAdapter{VDS: &v1alpha2.VirtualDiskSnapshot{
			Status: v1alpha2.VirtualDiskSnapshotStatus{CaptureState: &v1alpha2.UnifiedSnapshotterCaptureState{
				CommonController: &v1alpha2.UnifiedSnapshotterCommonCaptureState{
					ManifestCaptured: ptr.To(true),
					DataCaptured:     ptr.To(true),
				},
			}},
		}}
		got := a.CoreCaptureState()
		if !got.AllLegsCaptured() {
			t.Fatalf("expected AllLegsCaptured, got %+v", got)
		}
	})
}
