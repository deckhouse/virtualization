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
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdcondition"
)

func TestDiskChangeMattersToAttachments(t *testing.T) {
	disk := func(owner string, inUseReason vdcondition.InUseReason) *v1alpha2.VirtualDisk {
		vd := &v1alpha2.VirtualDisk{Status: v1alpha2.VirtualDiskStatus{
			Phase:      v1alpha2.DiskReady,
			Conditions: []metav1.Condition{{Type: vdcondition.InUseType.String(), Reason: inUseReason.String()}},
		}}
		if owner != "" {
			vd.Status.AttachedToVirtualMachines = []v1alpha2.AttachedVirtualMachine{{Name: owner, Mounted: true}}
		}
		return vd
	}

	tests := []struct {
		name     string
		old, cur *v1alpha2.VirtualDisk
		want     bool
	}{
		{"nothing changed", disk("vm-a", vdcondition.AttachedToVirtualMachine), disk("vm-a", vdcondition.AttachedToVirtualMachine), false},
		{"the disk is given to another VM", disk("vm-a", vdcondition.AttachedToVirtualMachine), disk("vm-b", vdcondition.AttachedToVirtualMachine), true},
		{"the disk is released", disk("vm-a", vdcondition.AttachedToVirtualMachine), disk("", vdcondition.NotInUse), true},
		{"an image creation released the disk", disk("", vdcondition.UsedForImageCreation), disk("", vdcondition.NotInUse), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := diskChangeMattersToAttachments(tt.old, tt.cur); got != tt.want {
				t.Errorf("diskChangeMattersToAttachments() = %v, want %v", got, tt.want)
			}
		})
	}
}
