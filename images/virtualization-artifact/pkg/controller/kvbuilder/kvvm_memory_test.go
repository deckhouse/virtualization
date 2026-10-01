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

package kvbuilder

import (
	"testing"

	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	virtv1 "kubevirt.io/api/core/v1"
)

// TestSetMemoryMaxGuest runs with the default feature gates, where HotplugMemoryWithLiveMigration is disabled.
func TestSetMemoryMaxGuest(t *testing.T) {
	maxGuest := resource.NewQuantity(MaxMemorySizeForHotplug, resource.BinarySI)

	tests := []struct {
		name       string
		status     virtv1.VirtualMachinePrintableStatus
		memorySize string
		// expectedMaxGuest is nil when maxGuest should be left unset.
		expectedMaxGuest *resource.Quantity
	}{
		{
			name:             "keep maxGuest on a Running VM",
			status:           virtv1.VirtualMachineStatusRunning,
			memorySize:       "2Gi",
			expectedMaxGuest: maxGuest,
		},
		{
			name:             "keep maxGuest on a Migrating VM",
			status:           virtv1.VirtualMachineStatusMigrating,
			memorySize:       "2Gi",
			expectedMaxGuest: maxGuest,
		},
		{
			name:             "keep maxGuest on a Running VM resized below the hotplug threshold",
			status:           virtv1.VirtualMachineStatusRunning,
			memorySize:       "512Mi",
			expectedMaxGuest: maxGuest,
		},
		{
			name:             "drop maxGuest on a Stopped VM when the feature is disabled",
			status:           virtv1.VirtualMachineStatusStopped,
			memorySize:       "2Gi",
			expectedMaxGuest: nil,
		},
		{
			name:             "mark maxGuest for removal on a Stopped VM resized below the hotplug threshold",
			status:           virtv1.VirtualMachineStatusStopped,
			memorySize:       "512Mi",
			expectedMaxGuest: resource.NewQuantity(0, resource.BinarySI),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			kvvm := &virtv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
				Spec: virtv1.VirtualMachineSpec{
					Template: &virtv1.VirtualMachineInstanceTemplateSpec{},
				},
				Status: virtv1.VirtualMachineStatus{PrintableStatus: tt.status},
			}
			kvvm.Spec.Template.Spec.Domain.Memory = &virtv1.Memory{MaxGuest: resource.NewQuantity(MaxMemorySizeForHotplug, resource.BinarySI)}

			b := NewKVVM(kvvm, KVVMOptions{})
			b.SetMemory(resource.MustParse(tt.memorySize))

			memory := b.Resource.Spec.Template.Spec.Domain.Memory
			if memory == nil || memory.Guest == nil || memory.Guest.Cmp(resource.MustParse(tt.memorySize)) != 0 {
				t.Fatalf("expected guest memory %s, got %+v", tt.memorySize, memory)
			}
			switch {
			case tt.expectedMaxGuest == nil && memory.MaxGuest != nil:
				t.Fatalf("expected maxGuest unset, got %s", memory.MaxGuest.String())
			case tt.expectedMaxGuest != nil && memory.MaxGuest == nil:
				t.Fatalf("expected maxGuest %s, got unset", tt.expectedMaxGuest.String())
			case tt.expectedMaxGuest != nil && memory.MaxGuest.Cmp(*tt.expectedMaxGuest) != 0:
				t.Fatalf("expected maxGuest %s, got %s", tt.expectedMaxGuest.String(), memory.MaxGuest.String())
			}
		})
	}
}
