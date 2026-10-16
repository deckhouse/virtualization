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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdcondition"
)

var _ = Describe("VirtualDiskWatcher", func() {
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

	DescribeTable("diskChangeMattersToAttachments",
		func(old, cur *v1alpha2.VirtualDisk, want bool) {
			Expect(diskChangeMattersToAttachments(old, cur)).To(Equal(want))
		},
		Entry("nothing changed", disk("vm-a", vdcondition.AttachedToVirtualMachine), disk("vm-a", vdcondition.AttachedToVirtualMachine), false),
		Entry("the disk is given to another VM", disk("vm-a", vdcondition.AttachedToVirtualMachine), disk("vm-b", vdcondition.AttachedToVirtualMachine), true),
		Entry("the disk is released", disk("vm-a", vdcondition.AttachedToVirtualMachine), disk("", vdcondition.NotInUse), true),
		Entry("an image creation released the disk", disk("", vdcondition.UsedForImageCreation), disk("", vdcondition.NotInUse), true),
	)
})
