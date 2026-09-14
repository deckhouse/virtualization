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
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

// The collector does not filter by snapshot mechanism, so a snapshot the state-snapshotter core planned
// reaches it too. Such a snapshot carries spec.sourceRef and no spec.virtualMachineName, so reading the latter
// exported an empty virtual_machine label and broke every dashboard that joins a snapshot to its machine.
func TestNewDataMetric_ResolvesTheMachineFromEitherSpecShape(t *testing.T) {
	tests := []struct {
		name string
		spec v1alpha2.VirtualMachineSnapshotSpec
		want string
	}{
		{
			name: "user-created snapshot",
			spec: v1alpha2.VirtualMachineSnapshotSpec{VirtualMachineName: "vm1"},
			want: "vm1",
		},
		{
			name: "core-planned snapshot",
			spec: v1alpha2.VirtualMachineSnapshotSpec{SourceRef: &v1alpha2.UnifiedSnapshotterSpecSourceRef{
				APIVersion: v1alpha2.SchemeGroupVersion.String(),
				Kind:       v1alpha2.VirtualMachineKind,
				Name:       "vm1",
			}},
			want: "vm1",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newDataMetric(&v1alpha2.VirtualMachineSnapshot{
				ObjectMeta: metav1.ObjectMeta{Name: "vms1", Namespace: "ns"},
				Spec:       tt.spec,
			})
			if m.VirtualMachine != tt.want {
				t.Errorf("VirtualMachine = %q, want %q", m.VirtualMachine, tt.want)
			}
		})
	}
}
