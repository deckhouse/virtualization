/*
Copyright 2025 Flant JSC

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

package handler

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	vmopbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vmop"
	"github.com/deckhouse/virtualization-controller/pkg/common/testutil"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

var _ = Describe("TestCancelHandler", func() {
	const (
		namespace = "default"
		vmName    = "test-vm"
	)

	var (
		ctx        = testutil.ContextBackgroundWithNoOpLogger()
		fakeClient client.Client
	)

	AfterEach(func() {
		fakeClient = nil
	})

	newVD := func(testName string, storageClassChanged, migrating bool) *v1alpha2.VirtualDisk {
		return newTestVD(testName, namespace, vmName, storageClassChanged, true, migrating)
	}

	newVMOP := func(name string, phase v1alpha2.VMOPPhase) *v1alpha2.VirtualMachineOperation {
		return newTestVMOP(name, namespace, vmName, phase)
	}

	It("should skip when storage class changed", func() {
		vd := newVD("vd-storage-changed", true, true)
		vmop := newVMOP("volume-migration-0", v1alpha2.VMOPPhaseInProgress)
		fakeClient = setupEnvironment(vd, vmop)

		h := NewCancelHandler(fakeClient)
		result, err := h.Handle(ctx, vd)

		Expect(err).NotTo(HaveOccurred())
		Expect(result.IsZero()).To(BeTrue())

		// Check that no VMOPs were deleted
		vmopList := &v1alpha2.VirtualMachineOperationList{}
		err = fakeClient.List(ctx, vmopList, client.InNamespace(namespace))
		Expect(err).NotTo(HaveOccurred())
		Expect(vmopList.Items).To(HaveLen(1))
	})

	It("should skip when not migrating", func() {
		vd := newVD("vd-not-migrating", false, false)
		vmop := newVMOP("volume-migration-0", v1alpha2.VMOPPhaseInProgress)
		fakeClient = setupEnvironment(vd, vmop)

		h := NewCancelHandler(fakeClient)
		result, err := h.Handle(ctx, vd)

		Expect(err).NotTo(HaveOccurred())
		Expect(result.IsZero()).To(BeTrue())

		// Check that no VMOPs were deleted
		vmopList := &v1alpha2.VirtualMachineOperationList{}
		err = fakeClient.List(ctx, vmopList, client.InNamespace(namespace))
		Expect(err).NotTo(HaveOccurred())
		Expect(vmopList.Items).To(HaveLen(1))
	})

	It("should skip when no active VMOPs", func() {
		vd := newVD("vd-no-vmops", false, true)
		fakeClient = setupEnvironment(vd)

		h := NewCancelHandler(fakeClient)
		result, err := h.Handle(ctx, vd)

		Expect(err).NotTo(HaveOccurred())
		Expect(result.IsZero()).To(BeTrue())

		// Check that no VMOPs exist
		vmopList := &v1alpha2.VirtualMachineOperationList{}
		err = fakeClient.List(ctx, vmopList, client.InNamespace(namespace))
		Expect(err).NotTo(HaveOccurred())
		Expect(vmopList.Items).To(HaveLen(0))
	})

	It("should skip when VMOP not in progress", func() {
		vd := newVD("vd-vmop-not-progress", false, true)
		vmop := newVMOP("volume-migration-0", v1alpha2.VMOPPhaseCompleted)
		fakeClient = setupEnvironment(vd, vmop)

		h := NewCancelHandler(fakeClient)
		result, err := h.Handle(ctx, vd)

		Expect(err).NotTo(HaveOccurred())
		Expect(result.IsZero()).To(BeTrue())

		// Check that no VMOPs were deleted
		vmopList := &v1alpha2.VirtualMachineOperationList{}
		err = fakeClient.List(ctx, vmopList, client.InNamespace(namespace))
		Expect(err).NotTo(HaveOccurred())
		Expect(vmopList.Items).To(HaveLen(1))
	})

	It("should skip when VMOP not migration type", func() {
		vd := newVD("vd-vmop-not-migration", false, true)
		vmop := vmopbuilder.New(
			vmopbuilder.WithGenerateName("other-operation-"),
			vmopbuilder.WithNamespace(namespace),
			vmopbuilder.WithType(v1alpha2.VMOPTypeStart),
			vmopbuilder.WithVirtualMachine(vmName),
		)
		fakeClient = setupEnvironment(vd, vmop)

		h := NewCancelHandler(fakeClient)
		result, err := h.Handle(ctx, vd)

		Expect(err).NotTo(HaveOccurred())
		Expect(result.IsZero()).To(BeTrue())

		// Check that no VMOPs were deleted
		vmopList := &v1alpha2.VirtualMachineOperationList{}
		err = fakeClient.List(ctx, vmopList, client.InNamespace(namespace))
		Expect(err).NotTo(HaveOccurred())
		Expect(vmopList.Items).To(HaveLen(1))
	})

	It("should skip when VMOP without volume migration annotation", func() {
		vd := newVD("vd-vmop-no-annotation", false, true)
		vmop := vmopbuilder.New(
			vmopbuilder.WithGenerateName("volume-migration-"),
			vmopbuilder.WithNamespace(namespace),
			vmopbuilder.WithType(v1alpha2.VMOPTypeEvict),
			vmopbuilder.WithVirtualMachine(vmName),
		)
		fakeClient = setupEnvironment(vd, vmop)

		h := NewCancelHandler(fakeClient)
		result, err := h.Handle(ctx, vd)

		Expect(err).NotTo(HaveOccurred())
		Expect(result.IsZero()).To(BeTrue())

		// Check that no VMOPs were deleted
		vmopList := &v1alpha2.VirtualMachineOperationList{}
		err = fakeClient.List(ctx, vmopList, client.InNamespace(namespace))
		Expect(err).NotTo(HaveOccurred())
		Expect(vmopList.Items).To(HaveLen(1))
	})

	It("should delete active migration VMOP", func() {
		vd := newVD("vd-active-migration", false, true)
		vmop := newVMOP("volume-migration-0", v1alpha2.VMOPPhaseInProgress)
		fakeClient = setupEnvironment(vd, vmop)

		h := NewCancelHandler(fakeClient)
		result, err := h.Handle(ctx, vd)

		Expect(err).NotTo(HaveOccurred())
		Expect(result.IsZero()).To(BeTrue())

		// Check that the VMOP was deleted
		vmopList := &v1alpha2.VirtualMachineOperationList{}
		err = fakeClient.List(ctx, vmopList, client.InNamespace(namespace))
		Expect(err).NotTo(HaveOccurred())
		Expect(vmopList.Items).To(HaveLen(0))
	})

	It("should delete only active migration VMOP from multiple VMOPs", func() {
		vd := newVD("vd-multiple-vmops", false, true)
		activeVMOP := newVMOP("volume-migration-0", v1alpha2.VMOPPhaseInProgress)
		completedVMOP := newVMOP("volume-migration-1", v1alpha2.VMOPPhaseCompleted)
		otherVMOP := vmopbuilder.New(
			vmopbuilder.WithName("other-operation"),
			vmopbuilder.WithNamespace(namespace),
			vmopbuilder.WithType(v1alpha2.VMOPTypeStart),
			vmopbuilder.WithVirtualMachine(vmName),
		)
		fakeClient = setupEnvironment(vd, activeVMOP, completedVMOP, otherVMOP)

		h := NewCancelHandler(fakeClient)
		result, err := h.Handle(ctx, vd)

		Expect(err).NotTo(HaveOccurred())
		Expect(result.IsZero()).To(BeTrue())

		// Check that only the active migration VMOP was deleted
		vmopList := &v1alpha2.VirtualMachineOperationList{}
		err = fakeClient.List(ctx, vmopList, client.InNamespace(namespace))
		Expect(err).NotTo(HaveOccurred())
		Expect(vmopList.Items).To(HaveLen(2))
	})

	Describe("getActiveVolumeMigration", func() {
		It("should return nil when no VMOPs exist", func() {
			vd := newVD("vd-no-vmops-test", false, true)
			fakeClient = setupEnvironment(vd)
			handler := NewCancelHandler(fakeClient)

			vmop, err := handler.getActiveVolumeMigration(ctx, types.NamespacedName{Name: vmName, Namespace: namespace})
			Expect(err).NotTo(HaveOccurred())
			Expect(vmop).To(BeNil())
		})

		It("should return the active migration VMOP", func() {
			vd := newVD("vd-active-vmop-test", false, true)
			activeVMOP := newVMOP("volume-migration-0", v1alpha2.VMOPPhaseInProgress)
			completedVMOP1 := newVMOP("volume-migration-1", v1alpha2.VMOPPhaseCompleted)
			completedVMOP2 := newVMOP("volume-migration-2", v1alpha2.VMOPPhaseCompleted)
			completedVMOP3 := newVMOP("volume-migration-3", v1alpha2.VMOPPhaseCompleted)
			otherVMOP := vmopbuilder.New(
				vmopbuilder.WithName("other-operation"),
				vmopbuilder.WithNamespace(namespace),
				vmopbuilder.WithType(v1alpha2.VMOPTypeStart),
				vmopbuilder.WithVirtualMachine(vmName),
			)
			fakeClient = setupEnvironment(vd, activeVMOP, completedVMOP1, completedVMOP2, completedVMOP3, otherVMOP)
			handler := NewCancelHandler(fakeClient)

			vmop, err := handler.getActiveVolumeMigration(ctx, types.NamespacedName{Name: vmName, Namespace: namespace})
			Expect(err).NotTo(HaveOccurred())
			Expect(vmop).NotTo(BeNil())
			Expect(vmop.Name).To(Equal(activeVMOP.Name))
		})

		It("should return nil when no active migration VMOPs exist", func() {
			vd := newVD("vd-completed-vmop-test", false, true)
			completedVMOP1 := newVMOP("volume-migration-1", v1alpha2.VMOPPhaseCompleted)
			completedVMOP2 := newVMOP("volume-migration-2", v1alpha2.VMOPPhaseCompleted)
			completedVMOP3 := newVMOP("volume-migration-3", v1alpha2.VMOPPhaseCompleted)
			otherVMOP := vmopbuilder.New(
				vmopbuilder.WithName("other-operation"),
				vmopbuilder.WithNamespace(namespace),
				vmopbuilder.WithType(v1alpha2.VMOPTypeStart),
				vmopbuilder.WithVirtualMachine(vmName),
			)
			fakeClient = setupEnvironment(vd, completedVMOP1, completedVMOP2, completedVMOP3, otherVMOP)
			handler := NewCancelHandler(fakeClient)

			vmop, err := handler.getActiveVolumeMigration(ctx, types.NamespacedName{Name: vmName, Namespace: namespace})
			Expect(err).NotTo(HaveOccurred())
			Expect(vmop).To(BeNil())
		})
	})
})
