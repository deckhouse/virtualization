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
	"errors"
	"log/slog"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/vm/internal/state"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

var _ = Describe("USBDeviceDetachHandler", func() {
	var ctx context.Context
	var mockVirtCl *mockVirtClient
	var handler *USBDeviceDetachHandler
	var vmState state.VirtualMachineState

	BeforeEach(func() {
		ctx = logger.ToContext(context.TODO(), slog.Default())
		mockVirtCl = newMockVirtClient()
	})

	DescribeTable("Handle detach matrix",
		func(inSpec, hasAttachedStatus, usbReady bool, expectDetachCalls int, expectFirstDetachName string) {
			spec := []v1alpha2.USBDeviceSpecRef{}
			if inSpec {
				spec = append(spec, v1alpha2.USBDeviceSpecRef{Name: "usb-device-1"})
			}

			status := []v1alpha2.USBDeviceStatusRef{}
			if hasAttachedStatus {
				status = append(status, v1alpha2.USBDeviceStatusRef{Name: "usb-device-1", Attached: true})
			}

			vm := &v1alpha2.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{Name: "test-vm", Namespace: "default", UID: types.UID("vm-uid")},
				Spec:       v1alpha2.VirtualMachineSpec{USBDevices: spec},
				Status:     v1alpha2.VirtualMachineStatus{Phase: v1alpha2.MachineRunning, USBDevices: status},
			}

			objs := []client.Object{}
			if inSpec {
				conds := []metav1.Condition{}
				vendor := ""
				if usbReady {
					vendor = "1234"
					conds = append(conds, metav1.Condition{Type: "Ready", Status: metav1.ConditionTrue})
				}
				objs = append(objs, &v1alpha2.USBDevice{
					ObjectMeta: metav1.ObjectMeta{Name: "usb-device-1", Namespace: "default"},
					Status: v1alpha2.USBDeviceStatus{
						Attributes: v1alpha2.NodeUSBDeviceAttributes{VendorID: vendor, ProductID: "5678"},
						NodeName:   "node-1",
						Conditions: conds,
					},
				})
			}

			fakeClient, _, st := setupEnvironment(vm, objs...)
			vmState = st
			handler = NewUSBDeviceDetachHandler(fakeClient, mockVirtCl)

			result, err := handler.Handle(ctx, vmState)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(reconcile.Result{}))

			_ = mockVirtCl.VirtualMachines("default")
			calls := mockVirtCl.vmClients["default"].removeResourceClaimCalls
			Expect(calls).To(HaveLen(expectDetachCalls))
			if expectDetachCalls > 0 {
				Expect(calls[0].Name).To(Equal(expectFirstDetachName))
			}
		},
		Entry("removed from spec", false, true, true, 1, "usb-device-1"),
		Entry("in spec and ready", true, true, true, 0, ""),
		Entry("in spec but not ready", true, true, false, 1, "usb-device-1"),
		Entry("no attached status means no detach", true, false, true, 0, ""),
	)

	DescribeTable("detach failure handling",
		func(removeErr error, expectErr bool) {
			vm := &v1alpha2.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{Name: "test-vm", Namespace: "default", UID: types.UID("vm-uid")},
				Spec:       v1alpha2.VirtualMachineSpec{USBDevices: []v1alpha2.USBDeviceSpecRef{}},
				Status: v1alpha2.VirtualMachineStatus{
					Phase:      v1alpha2.MachineRunning,
					USBDevices: []v1alpha2.USBDeviceStatusRef{{Name: "usb-device-1", Attached: true}},
				},
			}

			fakeClient, _, st := setupEnvironment(vm)
			vmState = st
			handler = NewUSBDeviceDetachHandler(fakeClient, mockVirtCl)

			mockVM := mockVirtCl.VirtualMachines("default").(*mockVirtualMachines)
			mockVM.removeResourceClaimErr = removeErr

			_, err := handler.Handle(ctx, vmState)
			if expectErr {
				Expect(err).To(HaveOccurred())
			} else {
				Expect(err).NotTo(HaveOccurred())
			}
		},
		Entry("returns error", errors.New("boom"), true),
		Entry("ignores not found", nil, false),
	)
})
