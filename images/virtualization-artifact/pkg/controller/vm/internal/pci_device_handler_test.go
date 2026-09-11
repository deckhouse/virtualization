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

package internal

import (
	"context"
	"log/slog"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apiruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/reconciler"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vm/internal/state"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/pcidevicecondition"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
)

var _ = Describe("PCIDevicesReady condition", func() {
	const (
		namespace  = "default"
		vmName     = "vm-pci"
		deviceName = "pci-device-1"
	)

	var ctx context.Context

	BeforeEach(func() {
		ctx = logger.ToContext(context.TODO(), slog.Default())
	})

	scheme := apiruntime.NewScheme()
	Expect(v1alpha2.AddToScheme(scheme)).To(Succeed())
	Expect(virtv1.AddToScheme(scheme)).To(Succeed())

	newVM := func(refs ...string) *v1alpha2.VirtualMachine {
		vm := &v1alpha2.VirtualMachine{ObjectMeta: metav1.ObjectMeta{Name: vmName, Namespace: namespace}}
		for _, ref := range refs {
			vm.Spec.PCIDevices = append(vm.Spec.PCIDevices, v1alpha2.PCIDeviceSpecRef{Name: ref})
		}
		return vm
	}

	readyDevice := func() *v1alpha2.PCIDevice {
		return &v1alpha2.PCIDevice{
			ObjectMeta: metav1.ObjectMeta{Name: deviceName, Namespace: namespace},
			Status: v1alpha2.PCIDeviceStatus{
				NodeName:   "node-1",
				Attributes: v1alpha2.NodePCIDeviceAttributes{PCIAddress: "0000:03:00.0"},
				Conditions: []metav1.Condition{{
					Type:   pcidevicecondition.ReadyType.String(),
					Status: metav1.ConditionTrue,
					Reason: pcidevicecondition.Ready.String(),
				}},
			},
		}
	}

	run := func(vm *v1alpha2.VirtualMachine, objects ...client.Object) []metav1.Condition {
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(append(objects, vm)...).Build()
		vmResource := reconciler.NewResource(types.NamespacedName{Name: vmName, Namespace: namespace}, fakeClient, vmFactoryByVM(vm), vmStatusGetter)
		Expect(vmResource.Fetch(ctx)).To(Succeed())

		vmState := state.New(fakeClient, vmResource)
		_, err := NewPCIDeviceHandler(fakeClient).Handle(ctx, vmState)
		Expect(err).NotTo(HaveOccurred())
		return vmState.VirtualMachine().Changed().Status.Conditions
	}

	It("does not set the condition without PCI devices", func() {
		_, found := conditions.GetCondition(vmcondition.TypePCIDevicesReady, run(newVM()))
		Expect(found).To(BeFalse())
	})

	It("is true when every device is ready", func() {
		cond, found := conditions.GetCondition(vmcondition.TypePCIDevicesReady, run(newVM(deviceName), readyDevice()))
		Expect(found).To(BeTrue())
		Expect(cond.Status).To(Equal(metav1.ConditionTrue))
		Expect(cond.Reason).To(Equal(vmcondition.ReasonPCIDevicesReady.String()))
	})

	It("explains a device that is missing from the namespace", func() {
		cond, _ := conditions.GetCondition(vmcondition.TypePCIDevicesReady, run(newVM(deviceName)))
		Expect(cond.Status).To(Equal(metav1.ConditionFalse))
		Expect(cond.Reason).To(Equal(vmcondition.ReasonPCIDevicesNotReady.String()))
		Expect(cond.Message).To(ContainSubstring(`PCIDevice "pci-device-1" was not found in the namespace`))
	})

	It("explains a device that is being deleted", func() {
		device := readyDevice()
		now := metav1.Now()
		device.DeletionTimestamp = &now
		device.Finalizers = []string{v1alpha2.FinalizerPCIDeviceCleanup}

		cond, _ := conditions.GetCondition(vmcondition.TypePCIDevicesReady, run(newVM(deviceName), device))
		Expect(cond.Status).To(Equal(metav1.ConditionFalse))
		Expect(cond.Message).To(ContainSubstring(`PCIDevice "pci-device-1" is being deleted`))
	})

	It("passes the readiness message of the device through", func() {
		device := readyDevice()
		device.Status.Conditions[0].Status = metav1.ConditionFalse
		device.Status.Conditions[0].Reason = pcidevicecondition.NotFound.String()
		device.Status.Conditions[0].Message = "Device is absent on the host."

		cond, _ := conditions.GetCondition(vmcondition.TypePCIDevicesReady, run(newVM(deviceName), device))
		Expect(cond.Status).To(Equal(metav1.ConditionFalse))
		Expect(cond.Message).To(Equal(`PCIDevice "pci-device-1" is not ready: Device is absent on the host.`))
	})
})
