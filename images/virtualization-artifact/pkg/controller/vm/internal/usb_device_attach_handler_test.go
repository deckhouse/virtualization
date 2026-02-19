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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	resourcev1 "k8s.io/api/resource/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/reconciler"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vm/internal/state"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	virtualizationv1alpha2 "github.com/deckhouse/virtualization/api/client/generated/clientset/versioned/typed/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	subv1alpha2 "github.com/deckhouse/virtualization/api/subresources/v1alpha2"
)

type mockVirtClient struct {
	vmClients map[string]*mockVirtualMachines
}

func newMockVirtClient() *mockVirtClient {
	return &mockVirtClient{vmClients: make(map[string]*mockVirtualMachines)}
}

func (m *mockVirtClient) VirtualMachines(namespace string) virtualizationv1alpha2.VirtualMachineInterface {
	if _, ok := m.vmClients[namespace]; !ok {
		m.vmClients[namespace] = &mockVirtualMachines{
			addResourceClaimCalls:    make([]subv1alpha2.VirtualMachineAddResourceClaim, 0),
			removeResourceClaimCalls: make([]subv1alpha2.VirtualMachineRemoveResourceClaim, 0),
		}
	}
	return m.vmClients[namespace]
}

type mockVirtualMachines struct {
	virtualizationv1alpha2.VirtualMachineInterface
	addResourceClaimCalls    []subv1alpha2.VirtualMachineAddResourceClaim
	removeResourceClaimCalls []subv1alpha2.VirtualMachineRemoveResourceClaim
	addResourceClaimErr      error
	removeResourceClaimErr   error
}

func (m *mockVirtualMachines) AddResourceClaim(_ context.Context, _ string, opts subv1alpha2.VirtualMachineAddResourceClaim) error {
	m.addResourceClaimCalls = append(m.addResourceClaimCalls, opts)
	return m.addResourceClaimErr
}

func (m *mockVirtualMachines) RemoveResourceClaim(_ context.Context, _ string, opts subv1alpha2.VirtualMachineRemoveResourceClaim) error {
	m.removeResourceClaimCalls = append(m.removeResourceClaimCalls, opts)
	return m.removeResourceClaimErr
}

var _ = Describe("USBDeviceAttachHandler", func() {
	var ctx context.Context
	var fakeClient client.WithWatch
	var mockVirtCl *mockVirtClient
	var handler *USBDeviceAttachHandler
	var vmState state.VirtualMachineState
	var vmResource *reconciler.Resource[*v1alpha2.VirtualMachine, v1alpha2.VirtualMachineStatus]

	BeforeEach(func() {
		ctx = logger.ToContext(context.TODO(), slog.Default())
		mockVirtCl = newMockVirtClient()
	})

	DescribeTable("Handle attach matrix",
		func(phase v1alpha2.MachinePhase, ready, withTemplate bool, attributeName string, expectAttached bool, expectAddCalls int, expectRequestName string) {
			vm := &v1alpha2.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{Name: "test-vm", Namespace: "default", UID: types.UID("vm-uid")},
				Spec:       v1alpha2.VirtualMachineSpec{USBDevices: []v1alpha2.USBDeviceSpecRef{{Name: "usb-device-1"}}},
				Status:     v1alpha2.VirtualMachineStatus{Phase: phase},
			}

			conds := []metav1.Condition{}
			vendor := ""
			if ready {
				conds = append(conds, metav1.Condition{Type: "Ready", Status: metav1.ConditionTrue})
				vendor = "1234"
			}

			usbDevice := &v1alpha2.USBDevice{
				ObjectMeta: metav1.ObjectMeta{Name: "usb-device-1", Namespace: "default"},
				Status: v1alpha2.USBDeviceStatus{
					Attributes: v1alpha2.NodeUSBDeviceAttributes{Name: attributeName, VendorID: vendor, ProductID: "5678"},
					NodeName:   "node-1",
					Conditions: conds,
				},
			}

			objs := []client.Object{usbDevice}
			if withTemplate {
				objs = append(objs, &resourcev1.ResourceClaimTemplate{ObjectMeta: metav1.ObjectMeta{Name: "usb-device-1-template", Namespace: "default"}})
			}

			fakeClient, vmResource, vmState = setupEnvironment(vm, objs...)
			handler = NewUSBDeviceAttachHandler(fakeClient, mockVirtCl)

			result, err := handler.Handle(ctx, vmState)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(reconcile.Result{}))
			Expect(vmResource.Changed().Status.USBDevices).To(HaveLen(1))
			Expect(vmResource.Changed().Status.USBDevices[0].Attached).To(Equal(expectAttached))

			_ = mockVirtCl.VirtualMachines("default")
			mockVM := mockVirtCl.vmClients["default"]
			Expect(mockVM.addResourceClaimCalls).To(HaveLen(expectAddCalls))
			if expectAddCalls > 0 {
				Expect(mockVM.addResourceClaimCalls[0].Name).To(Equal("usb-device-1"))
				Expect(mockVM.addResourceClaimCalls[0].RequestName).To(Equal(expectRequestName))
			}
		},
		Entry("ready + template", v1alpha2.MachineRunning, true, true, "usb-device-1", false, 1, "req-usb-device-1"),
		Entry("ready + no template", v1alpha2.MachineRunning, true, false, "usb-device-1", false, 0, ""),
		Entry("not ready + template", v1alpha2.MachineRunning, false, true, "usb-device-1", false, 0, ""),
		Entry("not ready + vm stopped", v1alpha2.MachineStopped, false, true, "", false, 0, ""),
		Entry("request name ignores attribute name", v1alpha2.MachineRunning, true, true, "usb-raw-name", false, 1, "req-usb-device-1"),
	)

	It("should handle missing USB device gracefully", func() {
		vm := &v1alpha2.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{Name: "test-vm", Namespace: "default", UID: types.UID("vm-uid")},
			Spec:       v1alpha2.VirtualMachineSpec{USBDevices: []v1alpha2.USBDeviceSpecRef{{Name: "missing-device"}}},
			Status:     v1alpha2.VirtualMachineStatus{Phase: v1alpha2.MachineStopped},
		}

		fakeClient, vmResource, vmState = setupEnvironment(vm)
		handler = NewUSBDeviceAttachHandler(fakeClient, mockVirtCl)

		_, err := handler.Handle(ctx, vmState)
		Expect(err).NotTo(HaveOccurred())
		Expect(vmResource.Changed().Status.USBDevices).To(HaveLen(1))
		Expect(vmResource.Changed().Status.USBDevices[0].Name).To(Equal("missing-device"))
		Expect(vmResource.Changed().Status.USBDevices[0].Attached).To(BeFalse())
	})

	It("should set address from KVVMI status when present", func() {
		vm := &v1alpha2.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{Name: "test-vm", Namespace: "default", UID: types.UID("vm-uid")},
			Spec:       v1alpha2.VirtualMachineSpec{USBDevices: []v1alpha2.USBDeviceSpecRef{{Name: "usb-device-1"}}},
			Status: v1alpha2.VirtualMachineStatus{
				Phase:      v1alpha2.MachineRunning,
				USBDevices: []v1alpha2.USBDeviceStatusRef{{Name: "usb-device-1", Attached: true}},
			},
		}
		usbDevice := &v1alpha2.USBDevice{
			ObjectMeta: metav1.ObjectMeta{Name: "usb-device-1", Namespace: "default"},
			Status: v1alpha2.USBDeviceStatus{
				Attributes: v1alpha2.NodeUSBDeviceAttributes{Name: "usb-device-1", VendorID: "1234", ProductID: "5678"},
				NodeName:   "node-1",
				Conditions: []metav1.Condition{{Type: "Ready", Status: metav1.ConditionTrue}},
			},
		}
		template := &resourcev1.ResourceClaimTemplate{ObjectMeta: metav1.ObjectMeta{Name: "usb-device-1-template", Namespace: "default"}}
		kvvmi := &virtv1.VirtualMachineInstance{
			ObjectMeta: metav1.ObjectMeta{Name: "test-vm", Namespace: "default"},
			Spec:       virtv1.VirtualMachineInstanceSpec{Domain: virtv1.DomainSpec{Devices: virtv1.Devices{HostDevices: []virtv1.HostDevice{{Name: "usb-device-1"}}}}},
			Status: virtv1.VirtualMachineInstanceStatus{DeviceStatus: &virtv1.DeviceStatus{HostDeviceStatuses: []virtv1.DeviceStatusInfo{{
				Name:                      "usb-device-1",
				DeviceResourceClaimStatus: &virtv1.DeviceResourceClaimStatus{Attributes: &virtv1.DeviceAttribute{USBAddress: &virtv1.USBAddress{Bus: 0, DeviceNumber: 2}}},
			}}}},
		}

		fakeClient, vmResource, vmState = setupEnvironment(vm, usbDevice, template, kvvmi)
		handler = NewUSBDeviceAttachHandler(fakeClient, mockVirtCl)

		_, err := handler.Handle(ctx, vmState)
		Expect(err).NotTo(HaveOccurred())
		Expect(vmResource.Changed().Status.USBDevices).To(HaveLen(1))
		Expect(vmResource.Changed().Status.USBDevices[0].Address).To(Equal(&v1alpha2.USBAddress{Bus: 0, Port: 2}))
	})

	DescribeTable("AddResourceClaim error handling",
		func(addErr error, expectAttached bool) {
			vm := &v1alpha2.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{Name: "test-vm", Namespace: "default", UID: types.UID("vm-uid")},
				Spec:       v1alpha2.VirtualMachineSpec{USBDevices: []v1alpha2.USBDeviceSpecRef{{Name: "usb-device-1"}}},
				Status:     v1alpha2.VirtualMachineStatus{Phase: v1alpha2.MachineRunning},
			}
			usbDevice := &v1alpha2.USBDevice{
				ObjectMeta: metav1.ObjectMeta{Name: "usb-device-1", Namespace: "default"},
				Status: v1alpha2.USBDeviceStatus{
					Attributes: v1alpha2.NodeUSBDeviceAttributes{Name: "usb-device-1", VendorID: "1234", ProductID: "5678"},
					NodeName:   "node-1",
					Conditions: []metav1.Condition{{Type: "Ready", Status: metav1.ConditionTrue}},
				},
			}
			template := &resourcev1.ResourceClaimTemplate{ObjectMeta: metav1.ObjectMeta{Name: "usb-device-1-template", Namespace: "default"}}

			fakeClient, vmResource, vmState = setupEnvironment(vm, usbDevice, template)
			handler = NewUSBDeviceAttachHandler(fakeClient, mockVirtCl)

			mockVM := mockVirtCl.VirtualMachines("default").(*mockVirtualMachines)
			mockVM.addResourceClaimErr = addErr

			_, err := handler.Handle(ctx, vmState)
			Expect(err).NotTo(HaveOccurred())
			Expect(vmResource.Changed().Status.USBDevices).To(HaveLen(1))
			Expect(vmResource.Changed().Status.USBDevices[0].Attached).To(Equal(expectAttached))
			Expect(mockVM.addResourceClaimCalls).To(HaveLen(1))
		},
		Entry("non AlreadyExists error keeps detached", apierrors.NewInternalError(errors.New("boom")), false),
		Entry("AlreadyExists keeps detached until KVVMI reflects claim", apierrors.NewAlreadyExists(schema.GroupResource{Resource: "resourceclaims"}, "usb-device-1"), false),
	)

	DescribeTable("clears stale attachment fields in non-attached branches",
		func(markDeleting bool, addErr error) {
			now := metav1.NewTime(time.Now())
			vm := &v1alpha2.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{Name: "test-vm", Namespace: "default", UID: types.UID("vm-uid")},
				Spec:       v1alpha2.VirtualMachineSpec{USBDevices: []v1alpha2.USBDeviceSpecRef{{Name: "usb-device-1"}}},
				Status: v1alpha2.VirtualMachineStatus{
					Phase: v1alpha2.MachineRunning,
					USBDevices: []v1alpha2.USBDeviceStatusRef{{
						Name:       "usb-device-1",
						Attached:   true,
						Ready:      true,
						Address:    &v1alpha2.USBAddress{Bus: 1, Port: 2},
						Hotplugged: true,
					}},
				},
			}

			usbMeta := metav1.ObjectMeta{Name: "usb-device-1", Namespace: "default"}
			if markDeleting {
				usbMeta.DeletionTimestamp = &now
				usbMeta.Finalizers = []string{"test-finalizer"}
			}

			usbDevice := &v1alpha2.USBDevice{
				ObjectMeta: usbMeta,
				Status: v1alpha2.USBDeviceStatus{
					Attributes: v1alpha2.NodeUSBDeviceAttributes{Name: "usb-device-1", VendorID: "1234", ProductID: "5678"},
					NodeName:   "node-1",
					Conditions: []metav1.Condition{{Type: "Ready", Status: metav1.ConditionTrue}},
				},
			}

			objs := []client.Object{usbDevice}
			if !markDeleting {
				objs = append(objs, &resourcev1.ResourceClaimTemplate{ObjectMeta: metav1.ObjectMeta{Name: "usb-device-1-template", Namespace: "default"}})
			}

			fakeClient, vmResource, vmState = setupEnvironment(vm, objs...)
			handler = NewUSBDeviceAttachHandler(fakeClient, mockVirtCl)

			if addErr != nil {
				mockVM := mockVirtCl.VirtualMachines("default").(*mockVirtualMachines)
				mockVM.addResourceClaimErr = addErr
			}

			_, err := handler.Handle(ctx, vmState)
			Expect(err).NotTo(HaveOccurred())
			Expect(vmResource.Changed().Status.USBDevices).To(HaveLen(1))
			status := vmResource.Changed().Status.USBDevices[0]
			Expect(status.Attached).To(BeFalse())
			Expect(status.Address).To(BeNil())
			Expect(status.Hotplugged).To(BeFalse())
		},
		Entry("device deleting", true, nil),
		Entry("attach failed", false, apierrors.NewInternalError(errors.New("boom"))),
	)
})
