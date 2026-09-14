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

package annotation

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

func annotatedUnified() metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:        "snap",
		Namespace:   "ns",
		Annotations: map[string]string{v1alpha2.AnnUseUnifiedSnapshotter: ""},
	}
}

func annotatedBuiltIn() metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:        "snap",
		Namespace:   "ns",
		Annotations: map[string]string{v1alpha2.AnnUseBuiltInSnapshotter: ""},
	}
}

func vmSourceRef() *v1alpha2.UnifiedSnapshotterSpecSourceRef {
	return &v1alpha2.UnifiedSnapshotterSpecSourceRef{
		APIVersion: v1alpha2.SchemeGroupVersion.String(),
		Kind:       v1alpha2.VirtualMachineKind,
		Name:       "vm",
	}
}

const (
	unified = true
	builtIn = false
)

func TestShouldHandle(t *testing.T) {
	tests := []struct {
		name string
		obj  client.Object
		want bool
	}{
		{
			name: "VirtualMachineSnapshot with the unified annotation",
			obj:  &v1alpha2.VirtualMachineSnapshot{ObjectMeta: annotatedUnified(), Spec: v1alpha2.VirtualMachineSnapshotSpec{VirtualMachineName: "vm"}},
			want: unified,
		},
		{
			name: "VirtualMachineSnapshot with the built-in annotation",
			obj:  &v1alpha2.VirtualMachineSnapshot{ObjectMeta: annotatedBuiltIn(), Spec: v1alpha2.VirtualMachineSnapshotSpec{VirtualMachineName: "vm"}},
			want: builtIn,
		},
		{
			name: "VirtualMachineSnapshot planned by the core (spec.sourceRef, no annotation)",
			obj:  &v1alpha2.VirtualMachineSnapshot{Spec: v1alpha2.VirtualMachineSnapshotSpec{SourceRef: vmSourceRef()}},
			want: unified,
		},
		{
			name: "plain user VirtualMachineSnapshot stays with the default mechanism (unified)",
			obj:  &v1alpha2.VirtualMachineSnapshot{Spec: v1alpha2.VirtualMachineSnapshotSpec{VirtualMachineName: "vm"}},
			want: unified,
		},
		{
			name: "VirtualDiskSnapshot planned by the core",
			obj: &v1alpha2.VirtualDiskSnapshot{Spec: v1alpha2.VirtualDiskSnapshotSpec{SourceRef: &v1alpha2.UnifiedSnapshotterSpecSourceRef{
				APIVersion: v1alpha2.SchemeGroupVersion.String(),
				Kind:       v1alpha2.VirtualDiskKind,
				Name:       "disk",
			}}},
			want: unified,
		},
		{
			name: "VirtualDiskSnapshot with the unified annotation",
			obj:  &v1alpha2.VirtualDiskSnapshot{ObjectMeta: annotatedUnified(), Spec: v1alpha2.VirtualDiskSnapshotSpec{VirtualDiskName: "disk"}},
			want: unified,
		},
		{
			name: "VirtualDiskSnapshot with the built-in annotation",
			obj:  &v1alpha2.VirtualDiskSnapshot{ObjectMeta: annotatedBuiltIn(), Spec: v1alpha2.VirtualDiskSnapshotSpec{VirtualDiskName: "disk"}},
			want: builtIn,
		},
		{
			name: "plain user VirtualDiskSnapshot stays with the default mechanism (unified)",
			obj:  &v1alpha2.VirtualDiskSnapshot{Spec: v1alpha2.VirtualDiskSnapshotSpec{VirtualDiskName: "disk"}},
			want: unified,
		},
		{
			name: "an unrelated kind is never claimed",
			obj:  &v1alpha2.VirtualMachine{ObjectMeta: annotatedUnified()},
			want: builtIn,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ShouldHandle().Create(event.CreateEvent{Object: tt.obj}); got != tt.want {
				t.Errorf("predicate Create() = %v, want %v", got, tt.want)
			}
		})
	}
}

// A sourceRef pointing at a kind this controller cannot capture resolves to an empty name, which the
// reconcilers treat as a permanent spec fault instead of waiting for an object that will never appear.
func TestSourceNameRejectsForeignKind(t *testing.T) {
	vms := &v1alpha2.VirtualMachineSnapshot{Spec: v1alpha2.VirtualMachineSnapshotSpec{
		SourceRef: &v1alpha2.UnifiedSnapshotterSpecSourceRef{
			APIVersion: v1alpha2.SchemeGroupVersion.String(),
			Kind:       v1alpha2.VirtualDiskKind,
			Name:       "disk",
		},
	}}
	if got := vms.SourceVirtualMachineName(); got != "" {
		t.Errorf("SourceVirtualMachineName() = %q, want empty", got)
	}

	vds := &v1alpha2.VirtualDiskSnapshot{Spec: v1alpha2.VirtualDiskSnapshotSpec{
		SourceRef: &v1alpha2.UnifiedSnapshotterSpecSourceRef{
			APIVersion: "apps/v1",
			Kind:       v1alpha2.VirtualDiskKind,
			Name:       "disk",
		},
	}}
	if got := vds.SourceVirtualDiskName(); got != "" {
		t.Errorf("SourceVirtualDiskName() = %q, want empty", got)
	}
}
