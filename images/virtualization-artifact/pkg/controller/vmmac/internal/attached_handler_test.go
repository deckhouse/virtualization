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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmmaccondition"
)

var _ = Describe("AttachedHandler", func() {
	const macAddress = "f6:e1:74:94:12:34"

	var (
		scheme       *runtime.Scheme
		vmmac        *v1alpha2.VirtualMachineMACAddress
		vm           *v1alpha2.VirtualMachine
		recorderMock *eventrecord.EventRecorderLoggerMock
	)

	BeforeEach(func() {
		scheme = runtime.NewScheme()
		Expect(v1alpha2.AddToScheme(scheme)).To(Succeed())

		vmmac = &v1alpha2.VirtualMachineMACAddress{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "vmmac",
				Namespace: "ns",
				Labels:    map[string]string{annotations.LabelVirtualMachineUID: "vm-uid"},
			},
			Status: v1alpha2.VirtualMachineMACAddressStatus{Address: macAddress},
		}
		vm = &v1alpha2.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{Name: "vm", Namespace: "ns", UID: "vm-uid"},
		}

		recorderMock = &eventrecord.EventRecorderLoggerMock{
			EventFunc:  func(_ client.Object, _, _, _ string) {},
			EventfFunc: func(_ client.Object, _, _, _ string, _ ...interface{}) {},
		}
	})

	handle := func(objs ...client.Object) {
		k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(append(objs, vm)...).Build()
		_, err := NewAttachedHandler(recorderMock, k8sClient).Handle(context.Background(), vmmac)
		Expect(err).NotTo(HaveOccurred())
	}

	attached := func() metav1.ConditionStatus {
		cond, _ := conditions.GetCondition(vmmaccondition.AttachedType, vmmac.Status.Conditions)
		return cond.Status
	}

	It("is attached to the virtual machine whose status names it", func() {
		vm.Status.Networks = []v1alpha2.NetworksStatus{{Type: v1alpha2.NetworksTypeClusterNetwork, Name: "net", VirtualMachineMACAddressName: vmmac.Name}}
		handle()
		Expect(attached()).To(Equal(metav1.ConditionTrue))
		Expect(vmmac.Status.VirtualMachine).To(Equal(vm.Name))
	})

	It("stays attached while an interface of the virtual machine is on its address without its name", func() {
		vm.Status.Networks = []v1alpha2.NetworksStatus{{Type: v1alpha2.NetworksTypeClusterNetwork, Name: "net", MAC: macAddress}}
		handle()
		Expect(attached()).To(Equal(metav1.ConditionTrue))
		Expect(vmmac.Status.VirtualMachine).To(Equal(vm.Name))
	})

	It("is not attached once the virtual machine neither names it nor uses its address", func() {
		vm.Status.Networks = []v1alpha2.NetworksStatus{{Type: v1alpha2.NetworksTypeClusterNetwork, Name: "net", MAC: "f6:e1:74:94:12:35"}}
		handle()
		Expect(attached()).To(Equal(metav1.ConditionFalse))
	})

	It("is not attached to another virtual machine on its address", func() {
		other := &v1alpha2.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{Name: "other", Namespace: "ns", UID: "other-uid"},
			Status: v1alpha2.VirtualMachineStatus{
				Networks: []v1alpha2.NetworksStatus{{Type: v1alpha2.NetworksTypeClusterNetwork, Name: "net", MAC: macAddress}},
			},
		}
		handle(other)
		Expect(attached()).To(Equal(metav1.ConditionFalse))
	})

	It("is not attached by an empty address", func() {
		vmmac.Status.Address = ""
		vm.Status.Networks = []v1alpha2.NetworksStatus{{Type: v1alpha2.NetworksTypeClusterNetwork, Name: "net"}}
		handle()
		Expect(attached()).To(Equal(metav1.ConditionFalse))
	})
})
