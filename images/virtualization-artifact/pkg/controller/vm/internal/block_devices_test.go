/*
Copyright 2024 Flant JSC

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

package internal

import (
	"log/slog"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

func TestBlockDeviceHandler(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "BlockDeviceHandler Suite")
}

var _ = Describe("BlockDeviceHandler", func() {
	var h *BlockDeviceHandler
	var logger *slog.Logger
	var vm *virtv2.VirtualMachine
	var vi *virtv2.VirtualImage
	var cvi *virtv2.ClusterVirtualImage
	var vdFoo *virtv2.VirtualDisk
	var vdBar *virtv2.VirtualDisk

	getBlockDevicesState := func(vi *virtv2.VirtualImage, cvi *virtv2.ClusterVirtualImage, vdFoo, vdBar *virtv2.VirtualDisk) BlockDevicesState {
		return BlockDevicesState{
			VIByName:  map[string]*virtv2.VirtualImage{vi.Name: vi},
			CVIByName: map[string]*virtv2.ClusterVirtualImage{cvi.Name: cvi},
			VDByName:  map[string]*virtv2.VirtualDisk{vdFoo.Name: vdFoo, vdBar.Name: vdBar},
		}
	}

	BeforeEach(func() {
		logger = slog.Default()
		h = NewBlockDeviceHandler(nil, &EventRecorderMock{
			EventFunc: func(_ runtime.Object, _, _, _ string) {},
		})
		vi = &virtv2.VirtualImage{
			ObjectMeta: metav1.ObjectMeta{Name: "vi-01"},
			Status:     virtv2.VirtualImageStatus{Phase: virtv2.ImageReady},
		}
		cvi = &virtv2.ClusterVirtualImage{
			ObjectMeta: metav1.ObjectMeta{Name: "cvi-01"},
			Status:     virtv2.ClusterVirtualImageStatus{Phase: virtv2.ImageReady},
		}
		vdFoo = &virtv2.VirtualDisk{
			ObjectMeta: metav1.ObjectMeta{Name: "vd-foo"},
			Status: virtv2.VirtualDiskStatus{
				Phase:  virtv2.DiskReady,
				Target: virtv2.DiskTarget{PersistentVolumeClaim: "pvc-foo"},
			},
		}
		vdBar = &virtv2.VirtualDisk{
			ObjectMeta: metav1.ObjectMeta{Name: "vd-bar"},
			Status: virtv2.VirtualDiskStatus{
				Phase:  virtv2.DiskReady,
				Target: virtv2.DiskTarget{PersistentVolumeClaim: "pvc-bar"},
			},
		}
		vm = &virtv2.VirtualMachine{
			Spec: virtv2.VirtualMachineSpec{
				BlockDeviceRefs: []virtv2.BlockDeviceSpecRef{
					{Name: vi.Name, Kind: virtv2.ImageDevice},
					{Name: cvi.Name, Kind: virtv2.ClusterImageDevice},
					{Name: vdFoo.Name, Kind: virtv2.DiskDevice},
					{Name: vdBar.Name, Kind: virtv2.DiskDevice},
				},
			},
		}
	})

	Context("VirtualMachine is nil", func() {
		It("Not ready, cannot start, no warnings", func() {
			ready, canStart, warnings := h.countReadyBlockDevices(nil, BlockDevicesState{}, logger)
			Expect(ready).To(Equal(0))
			Expect(canStart).To(BeFalse())
			Expect(warnings).To(BeNil())
		})
	})

	Context("BlockDevices are ready", func() {
		It("Ready, can start, no warnings", func() {
			state := getBlockDevicesState(vi, cvi, vdFoo, vdBar)
			ready, canStart, warnings := h.countReadyBlockDevices(vm, state, logger)
			Expect(ready).To(Equal(4))
			Expect(canStart).To(BeTrue())
			Expect(warnings).To(BeNil())
		})
	})

	Context("Image is not ready", func() {
		It("VirtualImage not ready: cannot start, no warnings", func() {
			vi.Status.Phase = virtv2.ImagePending
			state := getBlockDevicesState(vi, cvi, vdFoo, vdBar)
			ready, canStart, warnings := h.countReadyBlockDevices(vm, state, logger)
			Expect(ready).To(Equal(3))
			Expect(canStart).To(BeFalse())
			Expect(warnings).To(BeNil())
		})

		It("ClusterVirtualImage not ready: cannot start, no warnings", func() {
			cvi.Status.Phase = virtv2.ImagePending
			state := getBlockDevicesState(vi, cvi, vdFoo, vdBar)
			ready, canStart, warnings := h.countReadyBlockDevices(vm, state, logger)
			Expect(ready).To(Equal(3))
			Expect(canStart).To(BeFalse())
			Expect(warnings).To(BeNil())
		})
	})

	Context("Cannot attach VirtualDisk ", func() {
		It("VirtualDisk is attached to different virtual machine", func() {
			vdFoo.Status.AttachedToVirtualMachines = []virtv2.AttachedVirtualMachine{
				{Name: "different"},
			}
			state := getBlockDevicesState(vi, cvi, vdFoo, vdBar)
			ready, canStart, warnings := h.countReadyBlockDevices(vm, state, logger)
			Expect(ready).To(Equal(3))
			Expect(canStart).To(BeFalse())
			Expect(warnings).NotTo(BeEmpty())
		})

		It("VirtualDisk is attached to multiple virtual machines", func() {
			vdFoo.Status.AttachedToVirtualMachines = []virtv2.AttachedVirtualMachine{
				{Name: vm.Name},
				{Name: "different"},
			}
			state := getBlockDevicesState(vi, cvi, vdFoo, vdBar)
			ready, canStart, warnings := h.countReadyBlockDevices(vm, state, logger)
			Expect(ready).To(Equal(3))
			Expect(canStart).To(BeFalse())
			Expect(warnings).ToNot(BeEmpty())
		})
	})

	Context("VirtualDisk is not ready", func() {
		It("VirtualDisk's target pvc is not yet created", func() {
			vdFoo.Status.Phase = virtv2.DiskProvisioning
			vdFoo.Status.Target.PersistentVolumeClaim = ""
			state := getBlockDevicesState(vi, cvi, vdFoo, vdBar)
			ready, canStart, warnings := h.countReadyBlockDevices(vm, state, logger)
			Expect(ready).To(Equal(3))
			Expect(canStart).To(BeFalse())
			Expect(warnings).To(BeNil())
		})

		It("VirtualDisk's target pvc is created", func() {
			vdFoo.Status.Phase = virtv2.DiskProvisioning
			state := getBlockDevicesState(vi, cvi, vdFoo, vdBar)
			ready, canStart, warnings := h.countReadyBlockDevices(vm, state, logger)
			Expect(ready).To(Equal(3))
			Expect(canStart).To(BeTrue())
			Expect(warnings).ToNot(BeEmpty())
		})
	})
})
