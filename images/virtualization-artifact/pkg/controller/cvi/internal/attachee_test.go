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

	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/cvicondition"
)

var _ = Describe("AttacheeHandler", func() {
	newVirtualMachine := func(namespace, name string) *v1alpha2.VirtualMachine {
		return &v1alpha2.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
			Status: v1alpha2.VirtualMachineStatus{
				Phase: v1alpha2.MachineRunning,
				BlockDeviceRefs: []v1alpha2.BlockDeviceStatusRef{
					{Kind: v1alpha2.ClusterImageDevice, Name: "cvi"},
				},
			},
		}
	}

	newClient := func(objs ...client.Object) client.Client {
		scheme := runtime.NewScheme()
		Expect(v1alpha2.AddToScheme(scheme)).To(Succeed())

		return fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build()
	}

	It("names the VirtualMachine with its namespace and protects the image from deletion", func() {
		cvi := &v1alpha2.ClusterVirtualImage{
			ObjectMeta: metav1.ObjectMeta{Name: "cvi"},
		}

		handler := NewAttacheeHandler(newClient(newVirtualMachine("default", "vm-a")))
		_, err := handler.Handle(context.Background(), cvi)
		Expect(err).NotTo(HaveOccurred())

		cond, ok := conditions.GetCondition(cvicondition.InUseType, cvi.Status.Conditions)
		Expect(ok).To(BeTrue())
		Expect(cond.Status).To(Equal(metav1.ConditionTrue))
		Expect(cond.Reason).To(Equal(cvicondition.AttachedToVirtualMachine.String()))
		Expect(cond.Message).To(Equal(`The ClusterVirtualImage is in use by the VirtualMachine "default/vm-a"; detach it or stop the VirtualMachine to release the ClusterVirtualImage.`))
		Expect(cvi.Finalizers).To(ContainElement(v1alpha2.FinalizerCVIProtection))
	})

	It("reports VirtualMachines from every namespace sorted by name", func() {
		cvi := &v1alpha2.ClusterVirtualImage{
			ObjectMeta: metav1.ObjectMeta{Name: "cvi"},
		}

		handler := NewAttacheeHandler(newClient(newVirtualMachine("prod", "vm-b"), newVirtualMachine("default", "vm-a")))
		_, err := handler.Handle(context.Background(), cvi)
		Expect(err).NotTo(HaveOccurred())

		cond, _ := conditions.GetCondition(cvicondition.InUseType, cvi.Status.Conditions)
		Expect(cond.Message).To(Equal(`The ClusterVirtualImage is in use by 2 VirtualMachines; detach it or stop them to release the ClusterVirtualImage. In use by: "default/vm-a", "prod/vm-b".`))
	})

	It("sets NotInUse and releases the protection when no VirtualMachine uses the image", func() {
		cvi := &v1alpha2.ClusterVirtualImage{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "cvi",
				Finalizers: []string{v1alpha2.FinalizerCVIProtection},
			},
		}

		handler := NewAttacheeHandler(newClient())
		_, err := handler.Handle(context.Background(), cvi)
		Expect(err).NotTo(HaveOccurred())

		cond, ok := conditions.GetCondition(cvicondition.InUseType, cvi.Status.Conditions)
		Expect(ok).To(BeTrue())
		Expect(cond.Status).To(Equal(metav1.ConditionFalse))
		Expect(cond.Reason).To(Equal(cvicondition.NotInUse.String()))
		Expect(cvi.Finalizers).NotTo(ContainElement(v1alpha2.FinalizerCVIProtection))
	})
})
