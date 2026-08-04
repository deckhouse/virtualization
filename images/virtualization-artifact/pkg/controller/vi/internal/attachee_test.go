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
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vicondition"
)

var _ = Describe("AttacheeHandler", func() {
	newVirtualMachine := func(name string) *v1alpha2.VirtualMachine {
		return &v1alpha2.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: "default",
			},
			Status: v1alpha2.VirtualMachineStatus{
				Phase: v1alpha2.MachineRunning,
				BlockDeviceRefs: []v1alpha2.BlockDeviceStatusRef{
					{Kind: v1alpha2.ImageDevice, Name: "vi"},
				},
			},
		}
	}

	newClient := func(objs ...client.Object) client.Client {
		scheme := runtime.NewScheme()
		Expect(v1alpha2.AddToScheme(scheme)).To(Succeed())

		return fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build()
	}

	It("names the VirtualMachine that holds the image and protects it from deletion", func() {
		vi := &v1alpha2.VirtualImage{
			ObjectMeta: metav1.ObjectMeta{Name: "vi", Namespace: "default", Generation: 3},
		}

		handler := NewAttacheeHandler(newClient(newVirtualMachine("vm-a")))
		_, err := handler.Handle(context.Background(), vi)
		Expect(err).NotTo(HaveOccurred())

		cond, ok := conditions.GetCondition(vicondition.InUseType, vi.Status.Conditions)
		Expect(ok).To(BeTrue())
		Expect(cond.Status).To(Equal(metav1.ConditionTrue))
		Expect(cond.Reason).To(Equal(vicondition.AttachedToVirtualMachine.String()))
		Expect(cond.ObservedGeneration).To(Equal(int64(3)))
		Expect(cond.Message).To(Equal(`The VirtualImage is in use by the VirtualMachine "vm-a"; detach it or stop the VirtualMachine to release the VirtualImage.`))
		Expect(vi.Finalizers).To(ContainElement(v1alpha2.FinalizerVIProtection))
	})

	It("reports every VirtualMachine sorted by name", func() {
		vi := &v1alpha2.VirtualImage{
			ObjectMeta: metav1.ObjectMeta{Name: "vi", Namespace: "default"},
		}

		handler := NewAttacheeHandler(newClient(newVirtualMachine("vm-c"), newVirtualMachine("vm-a"), newVirtualMachine("vm-b")))
		_, err := handler.Handle(context.Background(), vi)
		Expect(err).NotTo(HaveOccurred())

		cond, _ := conditions.GetCondition(vicondition.InUseType, vi.Status.Conditions)
		Expect(cond.Message).To(Equal(`The VirtualImage is in use by 3 VirtualMachines; detach it or stop them to release the VirtualImage. In use by: "vm-a", "vm-b", "vm-c".`))
	})

	It("sets NotInUse and releases the protection when no VirtualMachine uses the image", func() {
		vi := &v1alpha2.VirtualImage{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "vi",
				Namespace:  "default",
				Finalizers: []string{v1alpha2.FinalizerVIProtection},
			},
		}

		handler := NewAttacheeHandler(newClient())
		_, err := handler.Handle(context.Background(), vi)
		Expect(err).NotTo(HaveOccurred())

		cond, ok := conditions.GetCondition(vicondition.InUseType, vi.Status.Conditions)
		Expect(ok).To(BeTrue())
		Expect(cond.Status).To(Equal(metav1.ConditionFalse))
		Expect(cond.Reason).To(Equal(vicondition.NotInUse.String()))
		Expect(cond.Message).To(BeEmpty())
		Expect(vi.Finalizers).NotTo(ContainElement(v1alpha2.FinalizerVIProtection))
	})
})
