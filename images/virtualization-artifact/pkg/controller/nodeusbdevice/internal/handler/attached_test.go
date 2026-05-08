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

	"github.com/deckhouse/virtualization-controller/pkg/controller/nodeusbdevice/internal/state"
	"github.com/deckhouse/virtualization-controller/pkg/controller/reconciler"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/nodeusbdevicecondition"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/usbdevicecondition"
)

var _ = Describe("AttachedHandler", func() {
	DescribeTable("Handle",
		func(assignedNamespace string, usbDevice *v1alpha2.USBDevice, expectedStatus metav1.ConditionStatus, expectedReason, expectedMessage string) {
			scheme := apiruntime.NewScheme()
			Expect(v1alpha2.AddToScheme(scheme)).To(Succeed())

			node := &v1alpha2.NodeUSBDevice{
				ObjectMeta: metav1.ObjectMeta{Name: "usb-device-1", Generation: 1},
				Spec:       v1alpha2.NodeUSBDeviceSpec{AssignedNamespace: assignedNamespace},
			}

			objects := []client.Object{node}
			if usbDevice != nil {
				objects = append(objects, usbDevice)
			}

			cl := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(objects...).
				Build()

			res := reconciler.NewResource(
				types.NamespacedName{Name: node.Name},
				cl,
				func() *v1alpha2.NodeUSBDevice { return &v1alpha2.NodeUSBDevice{} },
				func(obj *v1alpha2.NodeUSBDevice) v1alpha2.NodeUSBDeviceStatus { return obj.Status },
			)
			Expect(res.Fetch(context.Background())).To(Succeed())

			h := NewAttachedHandler(cl)
			st := state.New(cl, res)
			_, err := h.Handle(context.Background(), st)
			Expect(err).NotTo(HaveOccurred())

			attached := meta.FindStatusCondition(res.Changed().Status.Conditions, string(nodeusbdevicecondition.AttachedType))
			Expect(attached).NotTo(BeNil())
			Expect(attached.Status).To(Equal(expectedStatus))
			Expect(attached.Reason).To(Equal(expectedReason))
			Expect(attached.Message).To(Equal(expectedMessage))
		},
		Entry("unassigned device is not attached", "", nil, metav1.ConditionFalse, string(nodeusbdevicecondition.AttachedAvailable), "Device is not assigned to any namespace and is not attached to a virtual machine."),
		Entry("missing USBDevice returns available", "test-ns", nil, metav1.ConditionFalse, string(nodeusbdevicecondition.AttachedAvailable), "Corresponding USBDevice test-ns/usb-device-1 not found."),
		Entry("mirrors attached USBDevice condition", "test-ns", &v1alpha2.USBDevice{
			ObjectMeta: metav1.ObjectMeta{Name: "usb-device-1", Namespace: "test-ns"},
			Status: v1alpha2.USBDeviceStatus{Conditions: []metav1.Condition{{
				Type:    string(usbdevicecondition.AttachedType),
				Status:  metav1.ConditionTrue,
				Reason:  string(usbdevicecondition.AttachedToVirtualMachine),
				Message: "Device is attached to VirtualMachine test-ns/vm-1.",
			}}},
		}, metav1.ConditionTrue, string(nodeusbdevicecondition.AttachedToVirtualMachine), "Device is attached to VirtualMachine test-ns/vm-1."),
		Entry("mirrors detached for migration", "test-ns", &v1alpha2.USBDevice{
			ObjectMeta: metav1.ObjectMeta{Name: "usb-device-1", Namespace: "test-ns"},
			Status: v1alpha2.USBDeviceStatus{Conditions: []metav1.Condition{{
				Type:    string(usbdevicecondition.AttachedType),
				Status:  metav1.ConditionFalse,
				Reason:  string(usbdevicecondition.DetachedForMigration),
				Message: "Device was detached for migration.",
			}}},
		}, metav1.ConditionFalse, string(nodeusbdevicecondition.DetachedForMigration), "Device was detached for migration."),
		Entry("mirrors no free port condition", "test-ns", &v1alpha2.USBDevice{
			ObjectMeta: metav1.ObjectMeta{Name: "usb-device-1", Namespace: "test-ns"},
			Status: v1alpha2.USBDeviceStatus{Conditions: []metav1.Condition{{
				Type:    string(usbdevicecondition.AttachedType),
				Status:  metav1.ConditionFalse,
				Reason:  string(usbdevicecondition.NoFreeUSBIPPort),
				Message: "No free USBIP ports are available.",
			}}},
		}, metav1.ConditionFalse, string(nodeusbdevicecondition.NoFreeUSBIPPort), "No free USBIP ports are available."),
		Entry("missing attached condition falls back to available", "test-ns", &v1alpha2.USBDevice{
			ObjectMeta: metav1.ObjectMeta{Name: "usb-device-1", Namespace: "test-ns"},
		}, metav1.ConditionFalse, string(nodeusbdevicecondition.AttachedAvailable), "Attached condition not found in USBDevice test-ns/usb-device-1."),
	)
})
