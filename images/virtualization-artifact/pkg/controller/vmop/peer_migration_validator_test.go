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

package vmop

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
)

var _ = Describe("checkPeerMigratable", func() {
	running := func() *v1alpha2.VirtualMachine {
		return &v1alpha2.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{Name: "vm"},
			Status: v1alpha2.VirtualMachineStatus{
				Phase:           v1alpha2.MachineRunning,
				BlockDeviceRefs: []v1alpha2.BlockDeviceStatusRef{{Kind: v1alpha2.DiskDevice, Name: "root", Hotplugged: true}},
			},
		}
	}

	It("accepts a running VM with VirtualDisks, hot-plugged as every disk is", func() {
		Expect(checkPeerMigratable(running())).To(Succeed())
	})

	DescribeTable("refuses migration until the snapshot or freeze finishes", func(condition vmcondition.Type, cause string) {
		vm := running()
		vm.Status.Conditions = []metav1.Condition{{Type: condition.String(), Status: metav1.ConditionTrue}}
		Expect(checkPeerMigratable(vm)).To(MatchError(And(ContainSubstring(cause), ContainSubstring("retry the migration"))))
		vm.Status.Conditions[0].Status = metav1.ConditionFalse
		Expect(checkPeerMigratable(vm)).To(Succeed())
		vm.Status.Conditions = nil
		Expect(checkPeerMigratable(vm)).To(Succeed())
	},
		Entry("snapshotting", vmcondition.TypeSnapshotting, "snapshot is pending or in progress"),
		Entry("filesystem frozen", vmcondition.TypeFilesystemFrozen, "filesystem is frozen"),
	)

	DescribeTable("refuses",
		func(change func(vm *v1alpha2.VirtualMachine)) {
			vm := running()
			change(vm)
			Expect(checkPeerMigratable(vm)).NotTo(Succeed())
		},
		Entry("a stopped VM", func(vm *v1alpha2.VirtualMachine) { vm.Status.Phase = v1alpha2.MachineStopped }),
		Entry("a pool replica", func(vm *v1alpha2.VirtualMachine) {
			vm.OwnerReferences = []metav1.OwnerReference{{Kind: v1alpha2.VirtualMachinePoolKind, Name: "pool"}}
		}),
		Entry("a USB device", func(vm *v1alpha2.VirtualMachine) { vm.Spec.USBDevices = []v1alpha2.USBDeviceSpecRef{{Name: "u"}} }),
		Entry("a disk of a VirtualMachineBlockDeviceAttachment", func(vm *v1alpha2.VirtualMachine) {
			vm.Status.BlockDeviceRefs = append(vm.Status.BlockDeviceRefs, v1alpha2.BlockDeviceStatusRef{Kind: v1alpha2.DiskDevice, Name: "d", Hotplugged: true, VirtualMachineBlockDeviceAttachmentName: "d"})
		}),
		Entry("an image", func(vm *v1alpha2.VirtualMachine) {
			vm.Status.BlockDeviceRefs = append(vm.Status.BlockDeviceRefs, v1alpha2.BlockDeviceStatusRef{Kind: v1alpha2.ImageDevice, Name: "iso"})
		}),
	)
})

var _ = Describe("checkPeerMigrationType", func() {
	vmop := func(operationType v1alpha2.VMOPType, peer string) *v1alpha2.VirtualMachineOperation {
		op := &v1alpha2.VirtualMachineOperation{
			Spec: v1alpha2.VirtualMachineOperationSpec{Type: operationType},
		}
		if peer != "" {
			op.Spec.Migrate = &v1alpha2.VirtualMachineOperationMigrateSpec{ProjectPeerName: peer}
		}

		return op
	}

	It("accepts projectPeerName for Migrate", func() {
		Expect(checkPeerMigrationType(vmop(v1alpha2.VMOPTypeMigrate, "cluster-b"))).To(Succeed())
	})

	It("accepts a migration within the cluster", func() {
		Expect(checkPeerMigrationType(vmop(v1alpha2.VMOPTypeEvict, ""))).To(Succeed())
	})

	It("refuses projectPeerName for Evict", func() {
		Expect(checkPeerMigrationType(vmop(v1alpha2.VMOPTypeEvict, "cluster-b"))).NotTo(Succeed())
	})
})
