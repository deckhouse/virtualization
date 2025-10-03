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

package internal

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	vmbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vm"
	"github.com/deckhouse/virtualization-controller/pkg/common/testutil"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/reconciler"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vm/internal/state"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
)

var _ = Describe("TestFirmwareHandler", func() {
	const (
		name          = "vm-firmware"
		namespace     = "default1"
		expectedImage = "image:latest"
	)

	var (
		ctx        = testutil.ContextBackgroundWithNoOpLogger()
		fakeClient client.WithWatch
		resource   *reconciler.Resource[*v1alpha2.VirtualMachine, v1alpha2.VirtualMachineStatus]
		vmState    state.VirtualMachineState
	)

	AfterEach(func() {
		fakeClient = nil
		resource = nil
		vmState = nil
	})

	newVM := func() *v1alpha2.VirtualMachine {
		return vmbuilder.NewEmpty(name, namespace)
	}

	newKVVMI := func(image string) *virtv1.VirtualMachineInstance {
		kvvmi := newEmptyKVVMI(name, namespace)
		kvvmi.Status.LauncherContainerImageVersion = image
		return kvvmi
	}

	reconcile := func() {
		h := NewFirmwareHandler(expectedImage)
		_, err := h.Handle(testutil.ContextBackgroundWithNoOpLogger(), vmState)
		Expect(err).NotTo(HaveOccurred())
		err = resource.Update(context.Background())
		Expect(err).NotTo(HaveOccurred())
	}

	DescribeTable("Condition TypeFirmwareUpToDate should be in expected state",
		func(vm *v1alpha2.VirtualMachine, kvvmi *virtv1.VirtualMachineInstance, expectedStatus metav1.ConditionStatus, expectedReason vmcondition.FirmwareUpToDateReason, expectedExistence bool) {
			fakeClient, resource, vmState = setupEnvironment(vm, kvvmi)
			reconcile()

			newVM := &v1alpha2.VirtualMachine{}
			err := fakeClient.Get(ctx, client.ObjectKeyFromObject(vm), newVM)
			Expect(err).NotTo(HaveOccurred())

			upToDate, exists := conditions.GetCondition(vmcondition.TypeFirmwareUpToDate, newVM.Status.Conditions)
			Expect(exists).To(Equal(expectedExistence))
			if exists {
				Expect(upToDate.Status).To(Equal(expectedStatus))
				Expect(upToDate.Reason).To(Equal(expectedReason.String()))
			}
		},
		Entry("Should be up to date", newVM(), newKVVMI(expectedImage), metav1.ConditionTrue, vmcondition.ReasonFirmwareUpToDate, false),
		Entry("Should be up to date because kvvmi is not exists", newVM(), nil, metav1.ConditionTrue, vmcondition.ReasonFirmwareUpToDate, false),
		Entry("Should be out of date 1", newVM(), newKVVMI("other-image-1"), metav1.ConditionFalse, vmcondition.ReasonFirmwareOutOfDate, true),
		Entry("Should be out of date 2", newVM(), newKVVMI("other-image-2"), metav1.ConditionFalse, vmcondition.ReasonFirmwareOutOfDate, true),
		Entry("Should be out of date 3", newVM(), newKVVMI("other-image-3"), metav1.ConditionFalse, vmcondition.ReasonFirmwareOutOfDate, true),
		Entry("Should be out of date 4", newVM(), newKVVMI("other-image-4"), metav1.ConditionFalse, vmcondition.ReasonFirmwareOutOfDate, true),
		Entry("Should be out of date 5", newVM(), newKVVMI("other-image-5"), metav1.ConditionFalse, vmcondition.ReasonFirmwareOutOfDate, true),
	)

	DescribeTable("Condition TypeFirmwareUpToDate should be in the expected state considering the VM phase",
		func(vm *v1alpha2.VirtualMachine, phase v1alpha2.MachinePhase, kvvmi *virtv1.VirtualMachineInstance, expectedStatus metav1.ConditionStatus, expectedExistence bool) {
			vm.Status.Phase = phase
			fakeClient, resource, vmState = setupEnvironment(vm, kvvmi)
			reconcile()
			newVM := &v1alpha2.VirtualMachine{}
			err := fakeClient.Get(ctx, client.ObjectKeyFromObject(vm), newVM)
			Expect(err).NotTo(HaveOccurred())
			upToDate, exists := conditions.GetCondition(vmcondition.TypeFirmwareUpToDate, newVM.Status.Conditions)
			Expect(exists).To(Equal(expectedExistence))
			if exists {
				Expect(upToDate.Status).To(Equal(expectedStatus))
			}
		},
		Entry("Running phase, condition should not be set", newVM(), v1alpha2.MachineRunning, newKVVMI(expectedImage), metav1.ConditionUnknown, false),
		Entry("Running phase, condition should be set", newVM(), v1alpha2.MachineRunning, newKVVMI("other-image-1"), metav1.ConditionFalse, true),

		Entry("Migrating phase, condition should not be set", newVM(), v1alpha2.MachineMigrating, newKVVMI(expectedImage), metav1.ConditionUnknown, false),
		Entry("Migrating phase, condition should be set", newVM(), v1alpha2.MachineMigrating, newKVVMI("other-image-1"), metav1.ConditionFalse, true),

		Entry("Stopping phase, condition should not be set", newVM(), v1alpha2.MachineStopping, newKVVMI(expectedImage), metav1.ConditionUnknown, false),
		Entry("Stopping phase, condition should be set", newVM(), v1alpha2.MachineStopping, newKVVMI("other-image-1"), metav1.ConditionFalse, true),

		Entry("Pending phase, condition should not be set", newVM(), v1alpha2.MachinePending, newKVVMI(expectedImage), metav1.ConditionUnknown, false),
		Entry("Pending phase, condition should not be set", newVM(), v1alpha2.MachinePending, newKVVMI("other-image-1"), metav1.ConditionUnknown, false),

		Entry("Starting phase, condition should not be set", newVM(), v1alpha2.MachineStarting, newKVVMI(expectedImage), metav1.ConditionUnknown, false),
		Entry("Starting phase, condition should not be set", newVM(), v1alpha2.MachineStarting, newKVVMI("other-image-1"), metav1.ConditionUnknown, false),

		Entry("Stopped phase, condition should not be set", newVM(), v1alpha2.MachineStopped, newKVVMI(expectedImage), metav1.ConditionUnknown, false),
		Entry("Stopped phase, condition should not be set", newVM(), v1alpha2.MachineStopped, newKVVMI("other-image-1"), metav1.ConditionUnknown, false),
	)
})
