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

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/testutil"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/netmanager"
	"github.com/deckhouse/virtualization-controller/pkg/controller/reconciler"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vm/internal/state"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
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
		resource   *reconciler.Resource[*v1alpha2.VirtualMachine, v1alpha2.VirtualMachineStatus]
		vmState    state.VirtualMachineState
		vm         *v1alpha2.VirtualMachine
		recorder   *eventrecord.EventRecorderLoggerMock
	)

	BeforeEach(func() {
		vm = &v1alpha2.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
				UID:       "test-uid",
			},
			Spec: v1alpha2.VirtualMachineSpec{},
			Status: v1alpha2.VirtualMachineStatus{
				Phase: v1alpha2.MachinePending,
			},
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

	newMACAddress := func(name, address string, phase v1alpha2.VirtualMachineMACAddressPhase, attachedVM string) *v1alpha2.VirtualMachineMACAddress {
		mac := &v1alpha2.VirtualMachineMACAddress{
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
			Status: v1alpha2.VirtualMachineMACAddressStatus{
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
		It("should create only one additional VMMAC when one additional network is added", func() {
			vm.Spec.Networks = []v1alpha2.NetworksSpec{
				{Type: v1alpha2.NetworksTypeMain},
				{Type: v1alpha2.NetworksTypeNetwork, Name: "test-network1"},
				{Type: v1alpha2.NetworksTypeNetwork, Name: "test-network2"},
			}

			macAddress1 := newMACAddress("test-mac-address1", "aa:bb:cc:dd:ee:ff", "", "")
			fakeClient, resource, vmState = setupEnvironment(vm, macAddress1)
			reconcile()

			vmmacList := &v1alpha2.VirtualMachineMACAddressList{}
			err := fakeClient.List(ctx, vmmacList, client.InNamespace(namespace))
			Expect(err).NotTo(HaveOccurred())
			Expect(vmmacList.Items).To(HaveLen(2))
		})

		It("takes the MAC addresses of a received VM from the domain of the source", func() {
			const sourceMAC = "aa:bb:cc:dd:ee:01"
			vm.Annotations = map[string]string{annotations.AnnVMMigrationReceiver: "uid-1"}
			vm.Spec.Networks = []v1alpha2.NetworksSpec{
				{Type: v1alpha2.NetworksTypeMain},
				{Type: v1alpha2.NetworksTypeNetwork, Name: "net-a"},
			}
			fakeClient, resource, vmState = setupEnvironment(vm)
			reconcile()
			vmmacList := &v1alpha2.VirtualMachineMACAddressList{}
			Expect(fakeClient.List(ctx, vmmacList, client.InNamespace(namespace))).To(Succeed())
			Expect(vmmacList.Items).To(BeEmpty(), "no address is allocated before the domain of the source is here")

			kvvm := &virtv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
				Spec: virtv1.VirtualMachineSpec{Template: &virtv1.VirtualMachineInstanceTemplateSpec{Spec: virtv1.VirtualMachineInstanceSpec{
					Domain: virtv1.DomainSpec{Devices: virtv1.Devices{Interfaces: []virtv1.Interface{{Name: "veth_n1", MacAddress: sourceMAC}}}},
				}}},
			}
			fakeClient, resource, vmState = setupEnvironment(vm, kvvm)
			reconcile()
			Expect(fakeClient.List(ctx, vmmacList, client.InNamespace(namespace))).To(Succeed())
			Expect(vmmacList.Items).To(HaveLen(1))
			Expect(vmmacList.Items[0].Spec.Address).To(Equal(sourceMAC))
		})

		DescribeTable("should create a VMMAC only for a network replacing one still attached",
			func(additionalNetworks []v1alpha2.NetworksSpec, netAInVMI bool, expectedVMMACs int) {
				const netAMAC = "aa:bb:cc:dd:ee:ff"
				vm.Spec.Networks = append([]v1alpha2.NetworksSpec{{Type: v1alpha2.NetworksTypeMain}}, additionalNetworks...)
				vm.Status.Networks = []v1alpha2.NetworksStatus{
					{Type: v1alpha2.NetworksTypeMain},
					{Type: v1alpha2.NetworksTypeNetwork, Name: "net-a", MAC: netAMAC},
				}
				kvvmi := newEmptyKVVMI(name, namespace)
				if netAInVMI {
					kvvmi.Spec.Domain.Devices.Interfaces = []virtv1.Interface{{Name: "veth_n1", MacAddress: netAMAC}}
				}

				fakeClient, resource, vmState = setupEnvironment(vm, kvvmi, newMACAddress("net-a-mac", netAMAC, v1alpha2.VirtualMachineMACAddressPhaseAttached, name))
				reconcile()

				vmmacList := &v1alpha2.VirtualMachineMACAddressList{}
				Expect(fakeClient.List(ctx, vmmacList, client.InNamespace(namespace))).To(Succeed())
				Expect(vmmacList.Items).To(HaveLen(expectedVMMACs))
			},
			Entry("a new network while the old one is still attached",
				[]v1alpha2.NetworksSpec{{Type: v1alpha2.NetworksTypeClusterNetwork, Name: "net-b"}}, true, 2),
			Entry("a new network once the old one is detached",
				[]v1alpha2.NetworksSpec{{Type: v1alpha2.NetworksTypeClusterNetwork, Name: "net-b"}}, false, 1),
			Entry("a network that stays attached",
				[]v1alpha2.NetworksSpec{{Type: v1alpha2.NetworksTypeNetwork, Name: "net-a"}}, true, 1),
			Entry("a network detached without a replacement", nil, true, 1),
		)

		Describe("NetworkSpec is nil", func() {
			It("Condition 'MACAddressReady' should have status 'True'", func() {
				fakeClient, resource, vmState = setupEnvironment(vm)
				reconcile()

				newVM := &v1alpha2.VirtualMachine{}
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
				networkSpec := []v1alpha2.NetworksSpec{
					{
						Type: v1alpha2.NetworksTypeMain,
					},
				}
				vm.Spec.Networks = networkSpec
				fakeClient, resource, vmState = setupEnvironment(vm)
				reconcile()

				newVM := &v1alpha2.VirtualMachine{}
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
				networkSpec := []v1alpha2.NetworksSpec{
					{
						Type: v1alpha2.NetworksTypeMain,
					},
					{
						Type: v1alpha2.NetworksTypeNetwork,
						Name: "test-network1",
					},
					{
						Type: v1alpha2.NetworksTypeNetwork,
						Name: "test-network2",
					},
				}

				macAddress1 := newMACAddress("test-mac-address1", "aa:bb:cc:dd:ee:ff", "", "")

				vm.Spec.Networks = networkSpec
				fakeClient, resource, vmState = setupEnvironment(vm, macAddress1)
				reconcile()

				newVM := &v1alpha2.VirtualMachine{}
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
			networkSpec := []v1alpha2.NetworksSpec{
				{
					Type: v1alpha2.NetworksTypeMain,
				},
				{
					Type: v1alpha2.NetworksTypeNetwork,
					Name: "test-network1",
				},
				{
					Type: v1alpha2.NetworksTypeNetwork,
					Name: "test-network2",
				},
			}

			macAddress1 := newMACAddress("test-mac-address1", "aa:bb:cc:dd:ee:ff", v1alpha2.VirtualMachineMACAddressPhaseAttached, name)
			macAddress2 := newMACAddress("test-mac-address2", "aa:bb:cc:dd:ee:ef", "", "")

			vm.Spec.Networks = networkSpec
			fakeClient, resource, vmState = setupEnvironment(vm, macAddress1, macAddress2)
			reconcile()

			newVM := &v1alpha2.VirtualMachine{}
			err := fakeClient.Get(ctx, client.ObjectKeyFromObject(vm), newVM)
			Expect(err).NotTo(HaveOccurred())

			cond, exists := conditions.GetCondition(vmcondition.TypeMACAddressReady, newVM.Status.Conditions)
			Expect(exists).To(BeTrue())
			Expect(cond.Status).To(Equal(metav1.ConditionFalse))
			Expect(cond.Reason).To(Equal(vmcondition.ReasonMACAddressNotReady.String()))
			Expect(cond.Message).NotTo(Equal(""))
		})

		It("two ready macAddresses - Condition 'MACAddressReady' should have status 'True'", func() {
			networkSpec := []v1alpha2.NetworksSpec{
				{
					Type: v1alpha2.NetworksTypeMain,
				},
				{
					Type: v1alpha2.NetworksTypeNetwork,
					Name: "test-network1",
				},
				{
					Type: v1alpha2.NetworksTypeNetwork,
					Name: "test-network2",
				},
			}

			macAddress1 := newMACAddress("test-mac-address1", "aa:bb:cc:dd:ee:ff", v1alpha2.VirtualMachineMACAddressPhaseAttached, name)
			macAddress2 := newMACAddress("test-mac-address2", "aa:bb:cc:dd:ee:ef", v1alpha2.VirtualMachineMACAddressPhaseAttached, name)

			vm.Spec.Networks = networkSpec
			fakeClient, resource, vmState = setupEnvironment(vm, macAddress1, macAddress2)
			reconcile()

			newVM := &v1alpha2.VirtualMachine{}
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
