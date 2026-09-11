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

package handler

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apiruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/deckhouse/virtualization-controller/pkg/controller/nodepcidevice/internal/state"
	"github.com/deckhouse/virtualization-controller/pkg/controller/reconciler"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/nodepcidevicecondition"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/pcidevicecondition"
)

var _ = Describe("AttachedHandler", func() {
	DescribeTable("Handle",
		func(assignedNamespace string, pciDevice *v1alpha2.PCIDevice, expectedStatus metav1.ConditionStatus, expectedReason, expectedMessage string) {
			scheme := apiruntime.NewScheme()
			Expect(v1alpha2.AddToScheme(scheme)).To(Succeed())

			node := &v1alpha2.NodePCIDevice{
				ObjectMeta: metav1.ObjectMeta{Name: "pci-device-1", Generation: 1},
				Spec:       v1alpha2.NodePCIDeviceSpec{AssignedNamespace: assignedNamespace},
			}

			objects := []client.Object{node}
			if pciDevice != nil {
				objects = append(objects, pciDevice)
			}

			cl := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(objects...).
				Build()

			res := reconciler.NewResource(
				types.NamespacedName{Name: node.Name},
				cl,
				func() *v1alpha2.NodePCIDevice { return &v1alpha2.NodePCIDevice{} },
				func(obj *v1alpha2.NodePCIDevice) v1alpha2.NodePCIDeviceStatus { return obj.Status },
			)
			Expect(res.Fetch(context.Background())).To(Succeed())

			h := NewAttachedHandler(cl)
			st := state.New(cl, res)
			_, err := h.Handle(context.Background(), st)
			Expect(err).NotTo(HaveOccurred())

			attached := meta.FindStatusCondition(res.Changed().Status.Conditions, string(nodepcidevicecondition.AttachedType))
			Expect(attached).NotTo(BeNil())
			Expect(attached.Status).To(Equal(expectedStatus))
			Expect(attached.Reason).To(Equal(expectedReason))
			Expect(attached.Message).To(Equal(expectedMessage))
		},
		Entry("unassigned device is not attached", "", nil, metav1.ConditionFalse, string(nodepcidevicecondition.AttachedAvailable), "Device is not assigned to any namespace and is not attached to a virtual machine."),
		Entry("missing PCIDevice returns available", "test-ns", nil, metav1.ConditionFalse, string(nodepcidevicecondition.AttachedAvailable), "Corresponding PCIDevice test-ns/pci-device-1 not found."),
		Entry("mirrors attached PCIDevice condition", "test-ns", &v1alpha2.PCIDevice{
			ObjectMeta: metav1.ObjectMeta{Name: "pci-device-1", Namespace: "test-ns"},
			Status: v1alpha2.PCIDeviceStatus{Conditions: []metav1.Condition{{
				Type:    string(pcidevicecondition.AttachedType),
				Status:  metav1.ConditionTrue,
				Reason:  string(pcidevicecondition.AttachedToVirtualMachine),
				Message: "Device is attached to VirtualMachine test-ns/vm-1.",
			}}},
		}, metav1.ConditionTrue, string(nodepcidevicecondition.AttachedToVirtualMachine), "Device is attached to VirtualMachine test-ns/vm-1."),
		Entry("mirrors available PCIDevice condition", "test-ns", &v1alpha2.PCIDevice{
			ObjectMeta: metav1.ObjectMeta{Name: "pci-device-1", Namespace: "test-ns"},
			Status: v1alpha2.PCIDeviceStatus{Conditions: []metav1.Condition{{
				Type:    string(pcidevicecondition.AttachedType),
				Status:  metav1.ConditionFalse,
				Reason:  string(pcidevicecondition.Available),
				Message: "Device is available for attachment.",
			}}},
		}, metav1.ConditionFalse, string(nodepcidevicecondition.AttachedAvailable), "Device is available for attachment."),
		Entry("missing attached condition falls back to available", "test-ns", &v1alpha2.PCIDevice{
			ObjectMeta: metav1.ObjectMeta{Name: "pci-device-1", Namespace: "test-ns"},
		}, metav1.ConditionFalse, string(nodepcidevicecondition.AttachedAvailable), "Waiting for the attachment status of PCIDevice test-ns/pci-device-1."),
	)
})
