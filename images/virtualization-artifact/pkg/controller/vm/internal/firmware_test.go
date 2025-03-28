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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/common/testutil"
	"github.com/deckhouse/virtualization-controller/pkg/controller/reconciler"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vm/internal/state"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
)

var _ = Describe("FirmwareHandler", func() {
	const (
		vmName        = "vm"
		vmNamespace   = "default"
		firmwareImage = "image"
	)

	var (
		ctx        = testutil.ContextBackgroundWithNoOpLogger()
		fakeClient client.WithWatch
		resource   *reconciler.Resource[*virtv2.VirtualMachine, virtv2.VirtualMachineStatus]
		vmState    state.VirtualMachineState
	)

	AfterEach(func() {
		fakeClient = nil
		resource = nil
		vmState = nil
	})

	reconcile := func() {
		h := NewFirmwareHandler(firmwareImage)
		h.firmwareMinSupportedVersion = "v0.70.0"
		h.firmwareVersion = "v0.99.1"

		_, err := h.Handle(ctx, vmState)
		Expect(err).NotTo(HaveOccurred())
		err = resource.Update(ctx)
		Expect(err).NotTo(HaveOccurred())
	}

	newVm := func(version string) *virtv2.VirtualMachine {
		vm := newEmptyVirtualMachine(vmName, vmNamespace)
		vm.Status.FirmwareVersion = version
		return vm
	}

	newVmWithCond := func(version string) *virtv2.VirtualMachine {
		vm := newVm(version)
		vm.Status.Conditions = append(vm.Status.Conditions, metav1.Condition{
			Type:   vmcondition.TypeFirmwareUpdateRequired.String(),
			Status: metav1.ConditionTrue,
		})
		return vm
	}

	newKVVMI := func(image string) *virtv1.VirtualMachineInstance {
		kvvmi := newEmptyKVVMI(vmName, vmNamespace)
		kvvmi.Status.LauncherContainerImageVersion = image
		return kvvmi
	}

	DescribeTable("Check condition FirmwareNeedUpdate", func(vm *virtv2.VirtualMachine, kvvmi *virtv1.VirtualMachineInstance, condExist, firmwareUpdated bool) {
		oldVersion := vm.Status.FirmwareVersion
		fakeClient, resource, vmState = setupEnvironment(vm, kvvmi)
		reconcile()

		newVM := &virtv2.VirtualMachine{}
		err := fakeClient.Get(ctx, client.ObjectKey{Name: vmName, Namespace: vmNamespace}, newVM)
		Expect(err).NotTo(HaveOccurred())

		if condExist {
			Expect(newVM.Status.Conditions).To(HaveLen(1))
			Expect(newVM.Status.Conditions[0].Type).To(Equal(vmcondition.TypeFirmwareUpdateRequired.String()))
			Expect(newVM.Status.Conditions[0].Status).To(Equal(metav1.ConditionTrue))
		}

		newVersion := newVM.Status.FirmwareVersion
		if firmwareUpdated {
			Expect(oldVersion).NotTo(Equal(newVersion))
		} else {
			Expect(oldVersion).To(Equal(newVersion))
		}
	},
		Entry("Condition should be removed because the firmware version is supported",
			newVmWithCond("v0.80.1"), newKVVMI(""), false, false),
		Entry("Condition should be removed, and the firmware should be updated because the firmware version is not supported, but the VM is stopped",
			newVmWithCond("v0.10.0"), nil, false, true),
		Entry("Condition should be added because the firmware version is not supported (older) and the VM is running",
			newVm("v0.10.0"), newKVVMI(""), true, false),
		Entry("Condition should be added because the firmware version is not supported (newer) and the VM is running",
			newVm("v1.10.0"), newKVVMI(""), true, false),
	)
})
