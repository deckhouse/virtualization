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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/testutil"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/reconciler"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vm/internal/state"
	"github.com/deckhouse/virtualization-controller/pkg/featuregates"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
)

var _ = Describe("NetworkInterfaceHandler", func() {
	const (
		name      = "vm"
		namespace = "vms"

		podName           = "test-pod"
		podUID  types.UID = "test-pod-uid"
	)

	var (
		ctx        = testutil.ContextBackgroundWithNoOpLogger()
		fakeClient client.WithWatch
		resource   *reconciler.Resource[*virtv2.VirtualMachine, virtv2.VirtualMachineStatus]
		vmState    state.VirtualMachineState
		vm         *virtv2.VirtualMachine
		vmPod      *corev1.Pod
	)

	BeforeEach(func() {
		vmPod = &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:        podName,
				Namespace:   namespace,
				Labels:      map[string]string{virtv1.VirtualMachineNameLabel: name},
				UID:         podUID,
				Annotations: map[string]string{},
			},
			Spec: corev1.PodSpec{},
		}

		vm = &virtv2.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
				UID:       "test-uid",
			},
			Spec:   virtv2.VirtualMachineSpec{},
			Status: virtv2.VirtualMachineStatus{},
		}
	})

	AfterEach(func() {
		fakeClient = nil
		resource = nil
		vmState = nil
		vm = nil
		vmPod = nil
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
		gate, _, setFromMap, err := featuregates.New()
		Expect(err).NotTo(HaveOccurred())
		Expect(setFromMap(map[string]bool{string(featuregates.SDN): true})).To(Succeed())

		h := NewNetworkInterfaceHandler(gate)
		_, err = h.Handle(ctx, vmState)
		Expect(err).NotTo(HaveOccurred())
		err = resource.Update(context.Background())
		Expect(err).NotTo(HaveOccurred())
	}

	Describe("Condition presence and absence scenarios", func() {
		Describe("NetworkSpec is nil", func() {
			It("Condition should have status 'Unknown'", func() {
				fakeClient, resource, vmState = setupEnvironment(vm, vmPod)
				reconcile()

				newVM := &virtv2.VirtualMachine{}
				err := fakeClient.Get(ctx, client.ObjectKeyFromObject(vm), newVM)
				Expect(err).NotTo(HaveOccurred())

				cond, exists := conditions.GetCondition(vmcondition.TypeNetworkReady, newVM.Status.Conditions)
				Expect(exists).To(BeTrue())
				Expect(cond.Status).To(Equal(metav1.ConditionUnknown))
				Expect(cond.Reason).To(Equal(conditions.ReasonUnknown.String()))
				Expect(newVM.Status.Networks).To(BeNil())
			})
		})

		Describe("NetworkSpec have only 'Main' interface", func() {
			It("Network status is not exist; Condition should have status 'False'", func() {
				networkSpec := []virtv2.NetworksSpec{
					{
						Type: virtv2.NetworksTypeMain,
					},
				}
				vm.Spec.Networks = networkSpec
				fakeClient, resource, vmState = setupEnvironment(vm, vmPod)
				reconcile()

				newVM := &virtv2.VirtualMachine{}
				err := fakeClient.Get(ctx, client.ObjectKeyFromObject(vm), newVM)
				Expect(err).NotTo(HaveOccurred())

				cond, exists := conditions.GetCondition(vmcondition.TypeNetworkReady, newVM.Status.Conditions)
				Expect(exists).To(BeTrue())
				Expect(cond.Status).To(Equal(metav1.ConditionUnknown))
				Expect(cond.Reason).To(Equal(conditions.ReasonUnknown.String()))
				Expect(newVM.Status.Networks).NotTo(BeNil())
			})
		})

		Describe("NetworkSpec have many interfaces", func() {
			It("Network status is not exist; Condition should have status 'False'", func() {
				mac1 := newMACAddress("test-mac-address1", "aa:bb:cc:dd:ee:ff", virtv2.VirtualMachineMACAddressPhaseAttached, name)
				networkSpec := []virtv2.NetworksSpec{
					{
						Type: virtv2.NetworksTypeMain,
					},
					{
						Type: virtv2.NetworksTypeNetwork,
						Name: "test-network",
					},
				}
				vm.Spec.Networks = networkSpec
				fakeClient, resource, vmState = setupEnvironment(vm, vmPod, mac1)
				reconcile()

				newVM := &virtv2.VirtualMachine{}
				err := fakeClient.Get(ctx, client.ObjectKeyFromObject(vm), newVM)
				Expect(err).NotTo(HaveOccurred())

				cond, exists := conditions.GetCondition(vmcondition.TypeNetworkReady, newVM.Status.Conditions)
				Expect(exists).To(BeTrue())
				Expect(cond.Status).To(Equal(metav1.ConditionFalse))
				Expect(cond.Reason).To(Equal(vmcondition.ReasonNetworkNotReady.String()))
				Expect(newVM.Status.Networks).NotTo(BeNil())
			})

			It("Network status is exist; Condition should have status 'True'", func() {
				mac1 := newMACAddress("test-mac-address1", "aa:bb:cc:dd:ee:ff", virtv2.VirtualMachineMACAddressPhaseAttached, name)
				networkSpec := []virtv2.NetworksSpec{
					{
						Type: virtv2.NetworksTypeMain,
					},
					{
						Type: virtv2.NetworksTypeNetwork,
						Name: "test-network",
					},
				}
				vm.Spec.Networks = networkSpec
				vmPod.Annotations[annotations.AnnNetworksStatus] = `
				[
					{
					  "type": "Network",
					  "name": "test-network",
					  "ifName": "veth_nsadfsdaf",
					  "mac": "aa:bb:cc:dd:ee:ff",
					  "conditions": [
						{
						  "message": "",
						  "reason": "InterfaceConfiguredSuccessfully",
						  "status": "True",
						  "type": "Configured",
						  "lastTransitionTime": "2025-06-02T13:03:13Z"
						},
						{
						  "message": "",
						  "reason": "Up",
						  "status": "True",
						  "type": "Negotiated",
						  "lastTransitionTime": "2025-06-02T13:03:13Z"
						}
					  ]
					}
				]`
				fakeClient, resource, vmState = setupEnvironment(vm, vmPod, mac1)
				reconcile()

				newVM := &virtv2.VirtualMachine{}
				err := fakeClient.Get(ctx, client.ObjectKeyFromObject(vm), newVM)
				Expect(err).NotTo(HaveOccurred())

				cond, exists := conditions.GetCondition(vmcondition.TypeNetworkReady, newVM.Status.Conditions)
				Expect(exists).To(BeTrue())
				Expect(cond.Status).To(Equal(metav1.ConditionTrue))
				Expect(cond.Reason).To(Equal(vmcondition.ReasonNetworkReady.String()))
				Expect(newVM.Status.Networks).NotTo(BeNil())
			})

			It("Network status is exist; Condition should have status 'False'", func() {
				mac1 := newMACAddress("test-mac-address1", "aa:bb:cc:dd:ee:ff", virtv2.VirtualMachineMACAddressPhaseAttached, name)
				networkSpec := []virtv2.NetworksSpec{
					{
						Type: virtv2.NetworksTypeMain,
					},
					{
						Type: virtv2.NetworksTypeNetwork,
						Name: "test-network",
					},
				}
				vm.Spec.Networks = networkSpec
				vmPod.Annotations[annotations.AnnNetworksStatus] = `
				[
					{
					  "type": "Network",
					  "name": "test-network",
					  "ifName": "veth_nsadfsdaf",
					  "mac": "aa:bb:cc:dd:ee:ff",
					  "ipAddress": "10.2.3.4",
					  "conditions": [
						{
						  "message": "message with configuration error",
						  "reason": "ConfigurationError",
						  "status": "False",
						  "type": "Configured",
						  "lastTransitionTime": "2025-06-02T13:03:13Z"
						},
						{
						  "message": "message with negotiation error",
						  "reason": "NoCarrier",
						  "status": "False",
						  "type": "Negotiated",
						  "lastTransitionTime": "2025-06-02T13:03:13Z"
						}
					  ]
					}
				]`
				fakeClient, resource, vmState = setupEnvironment(vm, vmPod, mac1)
				reconcile()

				newVM := &virtv2.VirtualMachine{}
				err := fakeClient.Get(ctx, client.ObjectKeyFromObject(vm), newVM)
				Expect(err).NotTo(HaveOccurred())

				cond, exists := conditions.GetCondition(vmcondition.TypeNetworkReady, newVM.Status.Conditions)
				Expect(exists).To(BeTrue())
				Expect(cond.Status).To(Equal(metav1.ConditionFalse))
				Expect(cond.Reason).To(Equal(vmcondition.ReasonNetworkNotReady.String()))
				Expect(newVM.Status.Networks).NotTo(BeNil())
			})
		})
	})
})
