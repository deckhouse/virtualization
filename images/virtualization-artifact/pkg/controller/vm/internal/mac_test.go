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
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/testutil"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/netmanager"
	"github.com/deckhouse/virtualization-controller/pkg/controller/reconciler"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vm/internal/state"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
)

var _ = Describe("MACHandler", func() {
	const (
		name      = "vm"
		namespace = "vms"
	)

	var (
		ctx        = testutil.ContextBackgroundWithNoOpLogger()
		fakeClient client.WithWatch
		resource   *reconciler.Resource[*virtv2.VirtualMachine, virtv2.VirtualMachineStatus]
		vmState    state.VirtualMachineState
		vm         *virtv2.VirtualMachine
		recorder   *eventrecord.EventRecorderLoggerMock
	)

	BeforeEach(func() {
		vm = &virtv2.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
				UID:       "test-uid",
			},
			Spec:   virtv2.VirtualMachineSpec{},
			Status: virtv2.VirtualMachineStatus{},
		}
		recorder = &eventrecord.EventRecorderLoggerMock{
			EventFunc:       func(_ client.Object, _, _, _ string) {},
			EventfFunc:      func(_ client.Object, _, _, _ string, _ ...interface{}) {},
			WithLoggingFunc: func(logger eventrecord.InfoLogger) eventrecord.EventRecorderLogger { return recorder },
		}
	})

	AfterEach(func() {
		fakeClient = nil
		resource = nil
		vmState = nil
		vm = nil
		recorder = nil
	})

	newMACAddress := func(name, address string, phase virtv2.VirtualMachineMACAddressPhase, attachedVM string) *virtv2.VirtualMachineMACAddress {
		mac := &virtv2.VirtualMachineMACAddress{
			TypeMeta: metav1.TypeMeta{
				Kind:       "VirtualMachineMACAddress",
				APIVersion: "virtualization.deckhouse.io/v1alpha2",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
				Labels: map[string]string{
					annotations.LabelVirtualMachineUID: string(vm.UID),
				},
			},
			Status: virtv2.VirtualMachineMACAddressStatus{
				Address: address,
			},
		}
		if phase != "" {
			mac.Status.Phase = phase
		}
		if attachedVM != "" {
			mac.Status.VirtualMachine = attachedVM
		}
		return mac
	}

	reconcile := func() {
		h := NewMACHandler(netmanager.NewMACManager(), fakeClient, recorder)
		_, err := h.Handle(ctx, vmState)
		Expect(err).NotTo(HaveOccurred())
		err = resource.Update(context.Background())
		Expect(err).NotTo(HaveOccurred())
	}

	Describe("Condition presence and absence scenarios", func() {
		Describe("NetworkSpec is nil", func() {
			It("Condition 'MACAddressReady' should have status 'True'", func() {
				fakeClient, resource, vmState = setupEnvironment(vm)
				reconcile()

				newVM := &virtv2.VirtualMachine{}
				err := fakeClient.Get(ctx, client.ObjectKeyFromObject(vm), newVM)
				Expect(err).NotTo(HaveOccurred())

				cond, exists := conditions.GetCondition(vmcondition.TypeMACAddressReady, newVM.Status.Conditions)
				Expect(exists).To(BeTrue())
				Expect(cond.Status).To(Equal(metav1.ConditionTrue))
				Expect(cond.Reason).To(Equal(vmcondition.ReasonMACAddressReady.String()))
				Expect(cond.Message).To(Equal(""))
			})
		})

		Describe("NetworkSpec have only 'Main' interface", func() {
			It("Condition 'MACAddressReady' should have status 'True'", func() {
				networkSpec := []virtv2.NetworksSpec{
					{
						Type: virtv2.NetworksTypeMain,
					},
				}
				vm.Spec.Networks = networkSpec
				fakeClient, resource, vmState = setupEnvironment(vm)
				reconcile()

				newVM := &virtv2.VirtualMachine{}
				err := fakeClient.Get(ctx, client.ObjectKeyFromObject(vm), newVM)
				Expect(err).NotTo(HaveOccurred())

				cond, exists := conditions.GetCondition(vmcondition.TypeMACAddressReady, newVM.Status.Conditions)
				Expect(exists).To(BeTrue())
				Expect(cond.Status).To(Equal(metav1.ConditionTrue))
				Expect(cond.Reason).To(Equal(vmcondition.ReasonMACAddressReady.String()))
				Expect(cond.Message).To(Equal(""))
			})
		})

		Describe("NetworkSpec have many interfaces", func() {
			It("One macAddress exist - Condition 'MACAddressReady' should have status 'False'", func() {
				networkSpec := []virtv2.NetworksSpec{
					{
						Type: virtv2.NetworksTypeMain,
					},
					{
						Type: virtv2.NetworksTypeNetwork,
						Name: "test-network1",
					},
					{
						Type: virtv2.NetworksTypeNetwork,
						Name: "test-network2",
					},
				}

				macAddress1 := newMACAddress("test-mac-address1", "aa:bb:cc:dd:ee:ff", "", "")

				vm.Spec.Networks = networkSpec
				fakeClient, resource, vmState = setupEnvironment(vm, macAddress1)
				reconcile()

				newVM := &virtv2.VirtualMachine{}
				err := fakeClient.Get(ctx, client.ObjectKeyFromObject(vm), newVM)
				Expect(err).NotTo(HaveOccurred())

				cond, exists := conditions.GetCondition(vmcondition.TypeMACAddressReady, newVM.Status.Conditions)
				Expect(exists).To(BeTrue())
				Expect(cond.Status).To(Equal(metav1.ConditionFalse))
				Expect(cond.Reason).To(Equal(vmcondition.ReasonMACAddressNotReady.String()))
				Expect(cond.Message).NotTo(Equal(""))
			})
		})

		It("One ready macAddress - Condition 'MACAddressReady' should have status 'False'", func() {
			networkSpec := []virtv2.NetworksSpec{
				{
					Type: virtv2.NetworksTypeMain,
				},
				{
					Type: virtv2.NetworksTypeNetwork,
					Name: "test-network1",
				},
				{
					Type: virtv2.NetworksTypeNetwork,
					Name: "test-network2",
				},
			}

			macAddress1 := newMACAddress("test-mac-address1", "aa:bb:cc:dd:ee:ff", virtv2.VirtualMachineMACAddressPhaseAttached, name)
			macAddress2 := newMACAddress("test-mac-address2", "aa:bb:cc:dd:ee:ef", "", "")

			vm.Spec.Networks = networkSpec
			fakeClient, resource, vmState = setupEnvironment(vm, macAddress1, macAddress2)
			reconcile()

			newVM := &virtv2.VirtualMachine{}
			err := fakeClient.Get(ctx, client.ObjectKeyFromObject(vm), newVM)
			Expect(err).NotTo(HaveOccurred())

			cond, exists := conditions.GetCondition(vmcondition.TypeMACAddressReady, newVM.Status.Conditions)
			Expect(exists).To(BeTrue())
			Expect(cond.Status).To(Equal(metav1.ConditionFalse))
			Expect(cond.Reason).To(Equal(vmcondition.ReasonMACAddressNotReady.String()))
			Expect(cond.Message).NotTo(Equal(""))
		})

		It("two ready macAddresses - Condition 'MACAddressReady' should have status 'True'", func() {
			networkSpec := []virtv2.NetworksSpec{
				{
					Type: virtv2.NetworksTypeMain,
				},
				{
					Type: virtv2.NetworksTypeNetwork,
					Name: "test-network1",
				},
				{
					Type: virtv2.NetworksTypeNetwork,
					Name: "test-network2",
				},
			}

			macAddress1 := newMACAddress("test-mac-address1", "aa:bb:cc:dd:ee:ff", virtv2.VirtualMachineMACAddressPhaseAttached, name)
			macAddress2 := newMACAddress("test-mac-address2", "aa:bb:cc:dd:ee:ef", virtv2.VirtualMachineMACAddressPhaseAttached, name)

			vm.Spec.Networks = networkSpec
			fakeClient, resource, vmState = setupEnvironment(vm, macAddress1, macAddress2)
			reconcile()

			newVM := &virtv2.VirtualMachine{}
			err := fakeClient.Get(ctx, client.ObjectKeyFromObject(vm), newVM)
			Expect(err).NotTo(HaveOccurred())

			cond, exists := conditions.GetCondition(vmcondition.TypeMACAddressReady, newVM.Status.Conditions)
			Expect(exists).To(BeTrue())
			Expect(cond.Status).To(Equal(metav1.ConditionTrue))
			Expect(cond.Reason).To(Equal(vmcondition.ReasonMACAddressReady.String()))
			Expect(cond.Message).To(Equal(""))
		})
	})
})
