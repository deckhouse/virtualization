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
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
)

var _ = Describe("USBDeviceMigrationHandler", func() {
	var ctx context.Context
	var mockVirtCl *mockVirtClient
	var handler *USBDeviceMigrationHandler
	var vmState state.VirtualMachineState

	BeforeEach(func() {
		ctx = logger.ToContext(context.TODO(), slog.Default())
		mockVirtCl = newMockVirtClient()
	})

	DescribeTable("Handle migration matrix",
		func(reason string, vmopPhase v1alpha2.VMOPPhase, expectDetach bool) {
			vm := &v1alpha2.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{Name: "test-vm", Namespace: "default", UID: types.UID("vm-uid")},
				Status: v1alpha2.VirtualMachineStatus{
					USBDevices: []v1alpha2.USBDeviceStatusRef{{Name: "usb-device-1", Attached: true}},
					Conditions: []metav1.Condition{{
						Type:   string(vmcondition.TypeMigratable),
						Status: metav1.ConditionFalse,
						Reason: reason,
					}},
				},
			}

			objs := []client.Object{}
			if vmopPhase != "" {
				objs = append(objs, &v1alpha2.VirtualMachineOperation{
					ObjectMeta: metav1.ObjectMeta{Name: "vmop-1", Namespace: "default"},
					Spec:       v1alpha2.VirtualMachineOperationSpec{VirtualMachine: "test-vm", Type: v1alpha2.VMOPTypeMigrate},
					Status:     v1alpha2.VirtualMachineOperationStatus{Phase: vmopPhase},
				})
			}

			fakeClient, _, st := setupEnvironment(vm, objs...)
			vmState = st
			handler = NewUSBDeviceMigrationHandler(fakeClient, mockVirtCl)

			_, err := handler.Handle(ctx, vmState)
			Expect(err).NotTo(HaveOccurred())

			_ = mockVirtCl.VirtualMachines("default")
			calls := len(mockVirtCl.vmClients["default"].removeResourceClaimCalls)
			if expectDetach {
				Expect(calls).To(Equal(1))
			} else {
				Expect(calls).To(Equal(0))
			}
		},
		Entry("usb migration reason + pending vmop", vmcondition.ReasonUSBShouldBeMigrating.String(), v1alpha2.VMOPPhasePending, true),
		Entry("usb migration reason + inprogress vmop", vmcondition.ReasonUSBShouldBeMigrating.String(), v1alpha2.VMOPPhaseInProgress, false),
		Entry("other migratable reason", vmcondition.ReasonMigratable.String(), v1alpha2.VMOPPhasePending, false),
	)

	It("should detach all USB devices when migration is pending", func() {
		vm := &v1alpha2.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{Name: "test-vm", Namespace: "default", UID: types.UID("vm-uid")},
			Status: v1alpha2.VirtualMachineStatus{
				USBDevices: []v1alpha2.USBDeviceStatusRef{{Name: "usb-device-1", Attached: true, Hotplugged: true}, {Name: "usb-device-2", Attached: true}},
				Conditions: []metav1.Condition{{
					Type:   string(vmcondition.TypeMigratable),
					Status: metav1.ConditionFalse,
					Reason: vmcondition.ReasonUSBShouldBeMigrating.String(),
				}},
			},
		}
		vmop := &v1alpha2.VirtualMachineOperation{
			ObjectMeta: metav1.ObjectMeta{Name: "vmop-1", Namespace: "default"},
			Spec:       v1alpha2.VirtualMachineOperationSpec{VirtualMachine: "test-vm", Type: v1alpha2.VMOPTypeMigrate},
			Status:     v1alpha2.VirtualMachineOperationStatus{Phase: v1alpha2.VMOPPhasePending},
		}

		fakeClient, vmResource, st := setupEnvironment(vm, vmop)
		vmState = st
		handler = NewUSBDeviceMigrationHandler(fakeClient, mockVirtCl)

		result, err := handler.Handle(ctx, vmState)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(Equal(reconcile.Result{}))

		mockVM := mockVirtCl.vmClients["default"]
		Expect(mockVM.removeResourceClaimCalls).To(HaveLen(2))
		Expect(vmResource.Changed().Status.USBDevices[0].Attached).To(BeFalse())
		Expect(vmResource.Changed().Status.USBDevices[0].Hotplugged).To(BeFalse())
	})

	It("should return error when detach fails during migration", func() {
		vm := &v1alpha2.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{Name: "test-vm", Namespace: "default", UID: types.UID("vm-uid")},
			Status: v1alpha2.VirtualMachineStatus{
				USBDevices: []v1alpha2.USBDeviceStatusRef{{Name: "usb-device-1", Attached: true}},
				Conditions: []metav1.Condition{{
					Type:   string(vmcondition.TypeMigratable),
					Status: metav1.ConditionFalse,
					Reason: vmcondition.ReasonUSBShouldBeMigrating.String(),
				}},
			},
		}
		vmop := &v1alpha2.VirtualMachineOperation{
			ObjectMeta: metav1.ObjectMeta{Name: "vmop-1", Namespace: "default"},
			Spec:       v1alpha2.VirtualMachineOperationSpec{VirtualMachine: "test-vm", Type: v1alpha2.VMOPTypeMigrate},
			Status:     v1alpha2.VirtualMachineOperationStatus{Phase: v1alpha2.VMOPPhasePending},
		}

		fakeClient, _, st := setupEnvironment(vm, vmop)
		vmState = st
		handler = NewUSBDeviceMigrationHandler(fakeClient, mockVirtCl)

		mockVM := mockVirtCl.VirtualMachines("default").(*mockVirtualMachines)
		mockVM.removeResourceClaimErr = errors.New("boom")

		_, err := handler.Handle(ctx, vmState)
		Expect(err).To(HaveOccurred())
	})
})
