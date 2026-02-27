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
	"log/slog"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apiruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/indexer"
	"github.com/deckhouse/virtualization-controller/pkg/controller/reconciler"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/usbdevice/internal/state"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	virtv1alpha2 "github.com/deckhouse/virtualization/api/client/generated/clientset/versioned/typed/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/usbdevicecondition"
	subv1alpha2 "github.com/deckhouse/virtualization/api/subresources/v1alpha2"
)

type mockVMInterface struct {
	virtv1alpha2.VirtualMachineInterface
}

func (m *mockVMInterface) RemoveResourceClaim(_ context.Context, _ string, _ subv1alpha2.VirtualMachineRemoveResourceClaim) error {
	return nil
}

var _ = Describe("DeletionHandler", func() {
	var ctx context.Context

	type testCase struct {
		deleting         bool
		attached         bool
		withVM           bool
		withMultipleVMs  bool
		expectRequeue    bool
		finalizerPresent bool
		expectError      bool
		vmNotFound       bool
	}

	BeforeEach(func() {
		ctx = logger.ToContext(context.TODO(), slog.Default())
	})

	DescribeTable("Handle",
		func(tc testCase) {
			scheme := apiruntime.NewScheme()
			Expect(v1alpha2.AddToScheme(scheme)).To(Succeed())

			now := metav1.Now()
			usb := &v1alpha2.USBDevice{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "usb-device-1",
					Namespace:         "default",
					DeletionTimestamp: &now,
					Finalizers:        []string{v1alpha2.FinalizerUSBDeviceCleanup},
				},
			}
			if !tc.deleting {
				usb.DeletionTimestamp = nil
				usb.Finalizers = nil
			}

			condStatus := metav1.ConditionFalse
			condReason := string(usbdevicecondition.Available)
			if tc.attached {
				condStatus = metav1.ConditionTrue
				condReason = string(usbdevicecondition.AttachedToVirtualMachine)
			}
			usb.Status.Conditions = []metav1.Condition{{Type: string(usbdevicecondition.AttachedType), Status: condStatus, Reason: condReason}}

			objects := []client.Object{usb}

			vms := make([]client.Object, 0)
			if tc.withVM {
				vm := &v1alpha2.VirtualMachine{
					ObjectMeta: metav1.ObjectMeta{Name: "test-vm", Namespace: "default"},
					Spec:       v1alpha2.VirtualMachineSpec{USBDevices: []v1alpha2.USBDeviceSpecRef{{Name: "usb-device-1"}}},
					Status:     v1alpha2.VirtualMachineStatus{USBDevices: []v1alpha2.USBDeviceStatusRef{{Name: "usb-device-1", Attached: true}}},
				}
				if tc.vmNotFound {
					vm = nil
				}
				if vm != nil {
					vms = append(vms, vm)
				}
			}

			if tc.withMultipleVMs {
				vms = append(vms, &v1alpha2.VirtualMachine{
					ObjectMeta: metav1.ObjectMeta{Name: "test-vm-2", Namespace: "default"},
					Spec:       v1alpha2.VirtualMachineSpec{USBDevices: []v1alpha2.USBDeviceSpecRef{{Name: "usb-device-1"}}},
					Status:     v1alpha2.VirtualMachineStatus{USBDevices: []v1alpha2.USBDeviceStatusRef{{Name: "usb-device-1", Attached: true}}},
				})
			}

			objects = append(objects, vms...)

			vmObj, vmField, vmExtractValue := indexer.IndexVMByUSBDevice()
			cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(objects...).WithIndex(vmObj, vmField, vmExtractValue).Build()

			res := reconciler.NewResource(
				types.NamespacedName{Name: usb.Name, Namespace: usb.Namespace},
				cl,
				func() *v1alpha2.USBDevice { return &v1alpha2.USBDevice{} },
				func(obj *v1alpha2.USBDevice) v1alpha2.USBDeviceStatus { return obj.Status },
			)
			Expect(res.Fetch(ctx)).To(Succeed())

			st := state.New(cl, res)
			vmMock := &mockVMInterface{}
			virtClientMock := &service.VirtClientMock{
				VirtualMachinesFunc: func(namespace string) virtv1alpha2.VirtualMachineInterface {
					return vmMock
				},
			}
			h := NewDeletionHandler(virtClientMock)
			result, err := h.Handle(ctx, st)

			if tc.expectError {
				Expect(err).To(HaveOccurred())
			} else {
				Expect(err).NotTo(HaveOccurred())
			}

			if tc.expectRequeue {
				Expect(result.RequeueAfter).To(BeNumerically(">", 0))
			} else {
				Expect(result).To(Equal(reconcile.Result{}))
			}

			if tc.finalizerPresent {
				Expect(res.Changed().GetFinalizers()).To(ContainElement(v1alpha2.FinalizerUSBDeviceCleanup))
			} else {
				Expect(res.Changed().GetFinalizers()).NotTo(ContainElement(v1alpha2.FinalizerUSBDeviceCleanup))
			}
		},
		Entry("not deleting adds finalizer", testCase{finalizerPresent: true}),
		Entry("deleting not attached removes finalizer", testCase{deleting: true, finalizerPresent: false}),
		Entry("deleting attached requeues", testCase{deleting: true, attached: true, withVM: true, expectRequeue: true, finalizerPresent: true}),
		Entry("deleting with multiple VMs requeues", testCase{deleting: true, attached: true, withMultipleVMs: true, expectRequeue: true, finalizerPresent: true}),
		Entry("deleting with VM not found removes finalizer", testCase{deleting: true, attached: true, withVM: true, vmNotFound: true, finalizerPresent: false}),
	)
})
