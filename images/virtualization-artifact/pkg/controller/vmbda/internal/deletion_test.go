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
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmbdacondition"
)

var _ = Describe("DeletionHandler", func() {
	It("sets Deleting condition while waiting for block device detach", func() {
		scheme := runtime.NewScheme()
		Expect(v1alpha2.AddToScheme(scheme)).To(Succeed())
		Expect(virtv1.AddToScheme(scheme)).To(Succeed())

		now := metav1.Now()
		vmbda := &v1alpha2.VirtualMachineBlockDeviceAttachment{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "vmbda",
				Namespace:         "default",
				DeletionTimestamp: &now,
			},
			Spec: v1alpha2.VirtualMachineBlockDeviceAttachmentSpec{
				VirtualMachineName: "vm",
				BlockDeviceRef: v1alpha2.VMBDAObjectRef{
					Kind: v1alpha2.VMBDAObjectRefKindVirtualDisk,
					Name: "vd",
				},
			},
		}
		vm := &v1alpha2.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "vm",
				Namespace: "default",
			},
		}
		kvvm := &virtv1.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "vm",
				Namespace: "default",
			},
		}

		client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(vm, kvvm).Build()
		handler := NewDeletionHandler(vmbdaUnplug{
			isAttached: func(*v1alpha2.VirtualMachine, *virtv1.VirtualMachine, *v1alpha2.VirtualMachineBlockDeviceAttachment) bool {
				return true
			},
			unplugDisk: func(context.Context, *virtv1.VirtualMachine, string) error {
				return nil
			},
		}, client)

		result, err := handler.Handle(context.Background(), vmbda)

		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(BeZero())
		Expect(vmbda.Finalizers).To(ContainElement(v1alpha2.FinalizerVMBDACleanup))

		cond, ok := conditions.GetCondition(vmbdacondition.DeletingType, vmbda.Status.Conditions)
		Expect(ok).To(BeTrue())
		Expect(cond.Status).To(Equal(metav1.ConditionFalse))
		Expect(cond.Reason).To(Equal(vmbdacondition.DeletionCleanupPending.String()))
		Expect(cond.Message).To(Equal("Waiting for block device VirtualDisk/vd to detach from VirtualMachine vm."))
	})
})

type vmbdaUnplug struct {
	isAttached func(*v1alpha2.VirtualMachine, *virtv1.VirtualMachine, *v1alpha2.VirtualMachineBlockDeviceAttachment) bool
	unplugDisk func(context.Context, *virtv1.VirtualMachine, string) error
}

func (u vmbdaUnplug) IsAttached(vm *v1alpha2.VirtualMachine, kvvm *virtv1.VirtualMachine, vmbda *v1alpha2.VirtualMachineBlockDeviceAttachment) bool {
	return u.isAttached(vm, kvvm, vmbda)
}

func (u vmbdaUnplug) UnplugDisk(ctx context.Context, kvvm *virtv1.VirtualMachine, diskName string) error {
	return u.unplugDisk(ctx, kvvm, diskName)
}
