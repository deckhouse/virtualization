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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmmaccondition"
)

var _ = Describe("DeletionHandler", func() {
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
				Name:              "vmmac",
				Namespace:         "ns",
				CreationTimestamp: metav1.NewTime(time.Now().Add(-time.Minute)),
			},
			Status: v1alpha2.VirtualMachineMACAddressStatus{Address: "f6:e1:74:94:12:34"},
		}
		vm = &v1alpha2.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{Name: "vm", Namespace: "ns"},
		}

		recorderMock = &eventrecord.EventRecorderLoggerMock{
			EventFunc:  func(_ client.Object, _, _, _ string) {},
			EventfFunc: func(_ client.Object, _, _, _ string, _ ...interface{}) {},
		}
	})

	reconcile := func() (deleted bool) {
		k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(vmmac.DeepCopy(), vm).Build()
		_, err := NewAttachedHandler(recorderMock, k8sClient).Handle(context.Background(), vmmac)
		Expect(err).NotTo(HaveOccurred())
		_, err = NewDeletionHandler(k8sClient).Handle(context.Background(), vmmac)
		Expect(err).NotTo(HaveOccurred())
		err = k8sClient.Get(context.Background(), client.ObjectKeyFromObject(vmmac), &v1alpha2.VirtualMachineMACAddress{})
		return k8serrors.IsNotFound(err)
	}

	attachedReason := func() string {
		cond, _ := conditions.GetCondition(vmmaccondition.AttachedType, vmmac.Status.Conditions)
		return cond.Reason
	}

	It("keeps a new vmmac whose first reconcile is late", func() {
		Expect(reconcile()).To(BeFalse())
		Expect(attachedReason()).To(Equal(vmmaccondition.VirtualMachineNotFound.String()))
	})

	It("deletes an address once its virtual machine no longer uses it", func() {
		vm.Status.Networks = []v1alpha2.NetworksStatus{{Type: v1alpha2.NetworksTypeClusterNetwork, Name: "net", VirtualMachineMACAddressName: vmmac.Name}}
		Expect(reconcile()).To(BeFalse())
		Expect(attachedReason()).To(Equal(vmmaccondition.Attached.String()))

		vm.Status.Networks = nil
		Expect(reconcile()).To(BeTrue())
		Expect(attachedReason()).To(Equal(vmmaccondition.Detached.String()))
	})

	It("still deletes a detached address on the next reconcile", func() {
		vm.Status.Networks = []v1alpha2.NetworksStatus{{Type: v1alpha2.NetworksTypeClusterNetwork, Name: "net", VirtualMachineMACAddressName: vmmac.Name}}
		Expect(reconcile()).To(BeFalse())
		vm.Status.Networks = nil
		Expect(reconcile()).To(BeTrue())

		Expect(vmmac.Status.VirtualMachine).To(BeEmpty())
		Expect(reconcile()).To(BeTrue())
	})
})
