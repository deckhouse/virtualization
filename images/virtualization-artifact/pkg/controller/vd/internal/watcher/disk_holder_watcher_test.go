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

package watcher

import (
	"context"
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/common/testutil"
	"github.com/deckhouse/virtualization-controller/pkg/controller/kvbuilder"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

func TestChangedVolumeNames(t *testing.T) {
	old := map[string]struct{}{"vd-kept": {}, "vd-left": {}}
	cur := map[string]struct{}{"vd-kept": {}, "vd-came": {}}

	changed := changedVolumeNames(old, cur)

	if len(changed) != 2 {
		t.Fatalf("changed = %v, want vd-left and vd-came", changed)
	}
	for _, name := range []string{"vd-left", "vd-came"} {
		if _, ok := changed[name]; !ok {
			t.Errorf("changed = %v, want %s", changed, name)
		}
	}
}

func TestDisksByVolume(t *testing.T) {
	long := "disk-" + strings.Repeat("x", 70)
	vd := func(name string) *v1alpha2.VirtualDisk {
		return &v1alpha2.VirtualDisk{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "ns"}}
	}
	c, err := testutil.NewFakeClientWithObjects(vd("short"), vd(long), vd("other"))
	if err != nil {
		t.Fatal(err)
	}

	// A shortened volume name cannot be mapped back to the disk, only derived from it.
	names := map[string]struct{}{kvbuilder.GenerateVDDiskName("short"): {}, kvbuilder.GenerateVDDiskName(long): {}}
	got := disksByVolume(context.Background(), c, "ns", names)

	want := map[types.NamespacedName]bool{{Namespace: "ns", Name: "short"}: true, {Namespace: "ns", Name: long}: true}
	if len(got) != len(want) {
		t.Fatalf("requests = %v, want %v", got, want)
	}
	for _, req := range got {
		if !want[req.NamespacedName] {
			t.Errorf("unexpected request %v", req)
		}
	}
}

func TestEnqueueAttachedDisk(t *testing.T) {
	vmbda := func(kind v1alpha2.VMBDAObjectRefKind) *v1alpha2.VirtualMachineBlockDeviceAttachment {
		return &v1alpha2.VirtualMachineBlockDeviceAttachment{
			ObjectMeta: metav1.ObjectMeta{Name: "vmbda", Namespace: "ns"},
			Spec:       v1alpha2.VirtualMachineBlockDeviceAttachmentSpec{BlockDeviceRef: v1alpha2.VMBDAObjectRef{Kind: kind, Name: "disk"}},
		}
	}

	got := enqueueAttachedDisk(context.Background(), vmbda(v1alpha2.VMBDAObjectRefKindVirtualDisk))
	want := []reconcile.Request{{NamespacedName: types.NamespacedName{Namespace: "ns", Name: "disk"}}}
	if len(got) != 1 || got[0] != want[0] {
		t.Errorf("requests = %v, want %v", got, want)
	}

	if got := enqueueAttachedDisk(context.Background(), vmbda(v1alpha2.VMBDAObjectRefKindVirtualImage)); len(got) != 0 {
		t.Errorf("requests for an image attachment = %v, want none", got)
	}
}

func TestAttachmentChangeMattersToDisk(t *testing.T) {
	pending := func(vmReady metav1.ConditionStatus) *v1alpha2.VirtualMachineBlockDeviceAttachment {
		return &v1alpha2.VirtualMachineBlockDeviceAttachment{Status: v1alpha2.VirtualMachineBlockDeviceAttachmentStatus{
			Phase:      v1alpha2.BlockDeviceAttachmentPhasePending,
			Conditions: []metav1.Condition{{Type: "VirtualMachineReady", Status: vmReady}},
		}}
	}

	if attachmentChangeMattersToDisk(pending(metav1.ConditionFalse), pending(metav1.ConditionFalse)) {
		t.Error("an unchanged attachment must not wake the disk")
	}
	// The VM of a Pending attachment has started: the disk must elect its owner again.
	if !attachmentChangeMattersToDisk(pending(metav1.ConditionFalse), pending(metav1.ConditionTrue)) {
		t.Error("a started VM of a Pending attachment must wake the disk")
	}
	attached := pending(metav1.ConditionTrue)
	attached.Status.Phase = v1alpha2.BlockDeviceAttachmentPhaseAttached
	if !attachmentChangeMattersToDisk(pending(metav1.ConditionTrue), attached) {
		t.Error("a phase change must wake the disk")
	}
}
