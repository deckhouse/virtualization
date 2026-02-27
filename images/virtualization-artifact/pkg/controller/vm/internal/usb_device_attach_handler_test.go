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
	const (
		vmName        = "test-vm"
		vmNamespace   = "default"
		vmUID         = "vm-uid"
		usbDeviceName = "usb-device-1"
		vendorID      = "1234"
		productID     = "5678"
	)

	type attachMatrixInput struct {
		vmPhase       v1alpha2.MachinePhase
		usbReady      bool
		withTemplate  bool
		attributeName string
	}

	type attachMatrixExpected struct {
		attached    bool
		addCalls    int
		requestName string
	}

	type hostDeviceInput struct {
		phase         virtv1.DevicePhase
		attachPodName string
		address       string
	}

	type hostDeviceExpected struct {
		attached bool
		address  *v1alpha2.USBAddress
	}

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

	newVM := func(phase v1alpha2.MachinePhase, statusUSBDevices ...v1alpha2.USBDeviceStatusRef) *v1alpha2.VirtualMachine {
		return &v1alpha2.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{Name: vmName, Namespace: vmNamespace, UID: types.UID(vmUID)},
			Spec:       v1alpha2.VirtualMachineSpec{USBDevices: []v1alpha2.USBDeviceSpecRef{{Name: usbDeviceName}}},
			Status: v1alpha2.VirtualMachineStatus{
				Phase:      phase,
				USBDevices: statusUSBDevices,
			},
		}
	}

	newUSBDevice := func(ready bool, attributeName string, deleting bool) *v1alpha2.USBDevice {
		usbMeta := metav1.ObjectMeta{Name: usbDeviceName, Namespace: vmNamespace}
		if deleting {
			now := metav1.NewTime(time.Now())
			usbMeta.DeletionTimestamp = &now
			usbMeta.Finalizers = []string{"test-finalizer"}
		}

		conds := []metav1.Condition{}
		vid := ""
		if ready {
			conds = append(conds, metav1.Condition{Type: "Ready", Status: metav1.ConditionTrue})
			vid = vendorID
		}

		return &v1alpha2.USBDevice{
			ObjectMeta: usbMeta,
			Status: v1alpha2.USBDeviceStatus{
				Attributes: v1alpha2.NodeUSBDeviceAttributes{Name: attributeName, VendorID: vid, ProductID: productID},
				NodeName:   "node-1",
				Conditions: conds,
			},
		}
	}

	newResourceClaimTemplate := func() *resourcev1.ResourceClaimTemplate {
		return &resourcev1.ResourceClaimTemplate{ObjectMeta: metav1.ObjectMeta{Name: usbDeviceName + "-template", Namespace: vmNamespace}}
	}

	newKVVMI := func(hostDeviceStatuses ...virtv1.DeviceStatusInfo) *virtv1.VirtualMachineInstance {
		return &virtv1.VirtualMachineInstance{
			ObjectMeta: metav1.ObjectMeta{Name: vmName, Namespace: vmNamespace},
			Status: virtv1.VirtualMachineInstanceStatus{
				DeviceStatus: &virtv1.DeviceStatus{HostDeviceStatuses: hostDeviceStatuses},
			},
		}
	}

	runHandle := func(vm *v1alpha2.VirtualMachine, objs ...client.Object) (reconcile.Result, *reconciler.Resource[*v1alpha2.VirtualMachine, v1alpha2.VirtualMachineStatus], state.VirtualMachineState, error) {
		fakeClient, vmResource, vmState = setupEnvironment(vm, objs...)
		handler = NewUSBDeviceAttachHandler(fakeClient, mockVirtCl)

		result, err := handler.Handle(ctx, vmState)
		_ = mockVirtCl.VirtualMachines(vmNamespace)

		return result, vmResource, vmState, err
	}

	expectSingleUSBStatus := func() v1alpha2.USBDeviceStatusRef {
		Expect(vmResource.Changed().Status.USBDevices).To(HaveLen(1))
		return vmResource.Changed().Status.USBDevices[0]
	}

	DescribeTable("Handle attach matrix",
		func(input attachMatrixInput, expected attachMatrixExpected) {
			vm := newVM(input.vmPhase)
			objs := []client.Object{newUSBDevice(input.usbReady, input.attributeName, false)}
			if input.withTemplate {
				objs = append(objs, newResourceClaimTemplate())
			}

			result, _, _, err := runHandle(vm, objs...)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(reconcile.Result{}))
			Expect(expectSingleUSBStatus().Attached).To(Equal(expected.attached))

			mockVM := mockVirtCl.vmClients[vmNamespace]
			Expect(mockVM.addResourceClaimCalls).To(HaveLen(expected.addCalls))
			if expected.addCalls > 0 {
				Expect(mockVM.addResourceClaimCalls[0].Name).To(Equal(usbDeviceName))
				Expect(mockVM.addResourceClaimCalls[0].RequestName).To(Equal(expected.requestName))
			}
		},
		Entry(
			"ready + template",
			attachMatrixInput{vmPhase: v1alpha2.MachineRunning, usbReady: true, withTemplate: true, attributeName: usbDeviceName},
			attachMatrixExpected{attached: false, addCalls: 1, requestName: "req-" + usbDeviceName},
		),
		Entry(
			"ready + no template",
			attachMatrixInput{vmPhase: v1alpha2.MachineRunning, usbReady: true, withTemplate: false, attributeName: usbDeviceName},
			attachMatrixExpected{attached: false, addCalls: 0},
		),
		Entry(
			"not ready + template",
			attachMatrixInput{vmPhase: v1alpha2.MachineRunning, usbReady: false, withTemplate: true, attributeName: usbDeviceName},
			attachMatrixExpected{attached: false, addCalls: 0},
		),
		Entry(
			"not ready + vm stopped",
			attachMatrixInput{vmPhase: v1alpha2.MachineStopped, usbReady: false, withTemplate: true, attributeName: ""},
			attachMatrixExpected{attached: false, addCalls: 0},
		),
		Entry(
			"request name ignores attribute name",
			attachMatrixInput{vmPhase: v1alpha2.MachineRunning, usbReady: true, withTemplate: true, attributeName: "usb-raw-name"},
			attachMatrixExpected{attached: false, addCalls: 1, requestName: "req-" + usbDeviceName},
		),
	)

	It("should handle missing USB device gracefully", func() {
		vm := newVM(v1alpha2.MachineStopped)
		vm.Spec.USBDevices = []v1alpha2.USBDeviceSpecRef{{Name: "missing-device"}}

		_, _, _, err := runHandle(vm)
		Expect(err).NotTo(HaveOccurred())
		status := expectSingleUSBStatus()
		Expect(status.Name).To(Equal("missing-device"))
		Expect(status.Attached).To(BeFalse())
	})

	DescribeTable("maps KVVMI host device phase to attached status",
		func(input hostDeviceInput, expected hostDeviceExpected) {
			vm := newVM(v1alpha2.MachineRunning)
			hostDeviceStatus := virtv1.DeviceStatusInfo{Name: usbDeviceName, Phase: input.phase, Address: input.address}
			if input.attachPodName != "" {
				hostDeviceStatus.Hotplug = &virtv1.HotplugDeviceStatus{AttachPodName: input.attachPodName}
			}

			_, _, _, err := runHandle(vm, newUSBDevice(true, usbDeviceName, false), newResourceClaimTemplate(), newKVVMI(hostDeviceStatus))
			Expect(err).NotTo(HaveOccurred())

			status := expectSingleUSBStatus()
			Expect(status.Attached).To(Equal(expected.attached))
			Expect(status.Address).To(Equal(expected.address))
		},
		Entry(
			"HostDeviceReady with attach pod name",
			hostDeviceInput{phase: virtv1.DeviceReady, attachPodName: "hp-usb-device-1"},
			hostDeviceExpected{attached: true},
		),
		Entry(
			"HostDeviceReady with address",
			hostDeviceInput{phase: virtv1.DeviceReady, address: "0:2"},
			hostDeviceExpected{attached: true, address: &v1alpha2.USBAddress{Bus: 0, Port: 2}},
		),
		Entry(
			"AttachedToPod with attach pod name",
			hostDeviceInput{phase: virtv1.DeviceAttachedToPod, attachPodName: "hp-usb-device-1"},
			hostDeviceExpected{attached: false},
		),
		Entry(
			"Pending with attach pod name",
			hostDeviceInput{phase: virtv1.DevicePending, attachPodName: "hp-usb-device-1"},
			hostDeviceExpected{attached: false},
		),
	)

	DescribeTable("should keep USB status address nil when KVVMI host device address format is invalid",
		func(invalidAddress string) {
			vm := newVM(v1alpha2.MachineRunning)
			hostDeviceStatus := virtv1.DeviceStatusInfo{Name: usbDeviceName, Phase: virtv1.DeviceReady, Address: invalidAddress}

			var result reconcile.Result
			var err error
			Expect(func() {
				result, _, _, err = runHandle(vm, newUSBDevice(true, usbDeviceName, false), newResourceClaimTemplate(), newKVVMI(hostDeviceStatus))
			}).NotTo(Panic())

			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(reconcile.Result{}))
			status := expectSingleUSBStatus()
			Expect(status.Attached).To(BeTrue())
			Expect(status.Address).To(BeNil())
		},
		Entry("malformed separator", "0-"),
		Entry("empty address", ""),
		Entry("missing bus", ":2"),
		Entry("missing port", "1:"),
	)

	DescribeTable("AddResourceClaim error handling",
		func(input struct{ addErr error }, expected struct{ attached bool }) {
			vm := newVM(v1alpha2.MachineRunning)
			mockVM := mockVirtCl.VirtualMachines(vmNamespace).(*mockVirtualMachines)
			mockVM.addResourceClaimErr = input.addErr

			_, _, _, err := runHandle(vm, newUSBDevice(true, usbDeviceName, false), newResourceClaimTemplate())
			if input.addErr != nil && !apierrors.IsAlreadyExists(input.addErr) {
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to attach USB device"))
			} else {
				Expect(err).NotTo(HaveOccurred())
				Expect(expectSingleUSBStatus().Attached).To(Equal(expected.attached))
				Expect(mockVM.addResourceClaimCalls).To(HaveLen(1))
			}
		},
		Entry(
			"non AlreadyExists error returns error",
			struct{ addErr error }{addErr: apierrors.NewInternalError(errors.New("boom"))},
			struct{ attached bool }{attached: false},
		),
		Entry(
			"AlreadyExists keeps detached until KVVMI reflects claim",
			struct{ addErr error }{addErr: apierrors.NewAlreadyExists(schema.GroupResource{Resource: "resourceclaims"}, usbDeviceName)},
			struct{ attached bool }{attached: false},
		),
	)

	DescribeTable("clears stale attachment fields in non-attached branches",
		func(input struct {
			markDeleting bool
			addErr       error
		},
		) {
			vm := newVM(v1alpha2.MachineRunning, v1alpha2.USBDeviceStatusRef{
				Name:       usbDeviceName,
				Attached:   true,
				Ready:      true,
				Address:    &v1alpha2.USBAddress{Bus: 1, Port: 2},
				Hotplugged: true,
			})

			objs := []client.Object{newUSBDevice(true, usbDeviceName, input.markDeleting)}
			if !input.markDeleting {
				objs = append(objs, newResourceClaimTemplate())
			}

			if input.addErr != nil {
				mockVM := mockVirtCl.VirtualMachines(vmNamespace).(*mockVirtualMachines)
				mockVM.addResourceClaimErr = input.addErr
			}

			_, _, _, err := runHandle(vm, objs...)
			if input.addErr != nil && !apierrors.IsAlreadyExists(input.addErr) {
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to attach USB device"))
			} else {
				Expect(err).NotTo(HaveOccurred())
				status := expectSingleUSBStatus()
				Expect(status.Attached).To(BeFalse())
				Expect(status.Address).To(BeNil())
				Expect(status.Hotplugged).To(BeFalse())
			}
		},
		Entry(
			"device deleting",
			struct {
				markDeleting bool
				addErr       error
			}{markDeleting: true},
		),
		Entry(
			"attach failed",
			struct {
				markDeleting bool
				addErr       error
			}{
				markDeleting: false,
				addErr:       apierrors.NewInternalError(errors.New("boom")),
			},
		),
	)

	DescribeTable("should detach USB device when conditions change",
		func(detachScenario struct {
			deviceReady     bool
			templatePresent bool
		},
		) {
			vm := newVM(v1alpha2.MachineRunning, v1alpha2.USBDeviceStatusRef{
				Name:       usbDeviceName,
				Attached:   true,
				Ready:      true,
				Address:    &v1alpha2.USBAddress{Bus: 1, Port: 2},
				Hotplugged: true,
			})

			objs := []client.Object{}
			if detachScenario.deviceReady {
				objs = append(objs, newUSBDevice(true, usbDeviceName, false))
			} else {
				objs = append(objs, newUSBDevice(false, usbDeviceName, false))
			}
			if detachScenario.templatePresent {
				objs = append(objs, newResourceClaimTemplate())
			}

			_, _, _, err := runHandle(vm, objs...)
			Expect(err).NotTo(HaveOccurred())
			status := expectSingleUSBStatus()
			Expect(status.Attached).To(BeFalse())
			Expect(status.Address).To(BeNil())
			Expect(status.Hotplugged).To(BeFalse())
		},
		Entry(
			"USB device readiness becomes false",
			struct {
				deviceReady     bool
				templatePresent bool
			}{deviceReady: false, templatePresent: true},
		),
		Entry(
			"template removed",
			struct {
				deviceReady     bool
				templatePresent bool
			}{deviceReady: true, templatePresent: false},
		),
	)

	It("handles KVVMI present when VM stopped", func() {
		vm := newVM(v1alpha2.MachineStopped, v1alpha2.USBDeviceStatusRef{
			Name:     usbDeviceName,
			Attached: false,
		})

		hostDeviceStatus := virtv1.DeviceStatusInfo{
			Name:    usbDeviceName,
			Phase:   virtv1.DeviceReady,
			Address: "0:1",
			Hotplug: &virtv1.HotplugDeviceStatus{AttachPodName: "hp-pod"},
		}

		_, _, _, err := runHandle(vm, newUSBDevice(true, usbDeviceName, false), newResourceClaimTemplate(), newKVVMI(hostDeviceStatus))
		Expect(err).NotTo(HaveOccurred())

		status := expectSingleUSBStatus()
		Expect(status.Attached).To(BeFalse())
	})

	It("does not re-add already claimed device", func() {
		vm := newVM(v1alpha2.MachineRunning, v1alpha2.USBDeviceStatusRef{
			Name:       usbDeviceName,
			Attached:   true,
			Ready:      true,
			Address:    &v1alpha2.USBAddress{Bus: 1, Port: 2},
			Hotplugged: true,
		})

		hostDeviceStatus := virtv1.DeviceStatusInfo{
			Name:    usbDeviceName,
			Phase:   virtv1.DeviceReady,
			Address: "1:2",
		}

		_, _, _, err := runHandle(vm, newUSBDevice(true, usbDeviceName, false), newResourceClaimTemplate(), newKVVMI(hostDeviceStatus))
		Expect(err).NotTo(HaveOccurred())

		mockVM := mockVirtCl.vmClients[vmNamespace]
		Expect(mockVM.addResourceClaimCalls).To(HaveLen(0))

		status := expectSingleUSBStatus()
		Expect(status.Attached).To(BeTrue())
		Expect(status.Ready).To(BeTrue())
	})

	It("clears USB status on VM deletion", func() {
		now := metav1.NewTime(time.Now())
		vm := &v1alpha2.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{
				Name:              vmName,
				Namespace:         vmNamespace,
				UID:               types.UID(vmUID),
				DeletionTimestamp: &now,
				Finalizers:        []string{"test-finalizer"},
			},
			Spec: v1alpha2.VirtualMachineSpec{USBDevices: []v1alpha2.USBDeviceSpecRef{{Name: usbDeviceName}}},
			Status: v1alpha2.VirtualMachineStatus{
				Phase: v1alpha2.MachineRunning,
				USBDevices: []v1alpha2.USBDeviceStatusRef{{
					Name:       usbDeviceName,
					Attached:   true,
					Ready:      true,
					Address:    &v1alpha2.USBAddress{Bus: 1, Port: 2},
					Hotplugged: true,
				}},
			},
		}

		_, _, _, err := runHandle(vm, newUSBDevice(true, usbDeviceName, false), newResourceClaimTemplate())
		Expect(err).NotTo(HaveOccurred())

		status := expectSingleUSBStatus()
		Expect(status.Attached).To(BeFalse())
		Expect(status.Address).To(BeNil())
		Expect(status.Hotplugged).To(BeFalse())
	})
})
