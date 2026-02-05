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
	"log/slog"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	resourcev1beta1 "k8s.io/api/resource/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apiruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	virtv1 "kubevirt.io/api/core/v1"

	"github.com/deckhouse/virtualization-controller/pkg/controller/reconciler"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vm/internal/state"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	virtualizationv1alpha2 "github.com/deckhouse/virtualization/api/client/generated/clientset/versioned/typed/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	subv1alpha2 "github.com/deckhouse/virtualization/api/subresources/v1alpha2"
)

// mockVirtClient implements kubeclient.Client interface for testing
type mockVirtClient struct {
	vmClients map[string]*mockVirtualMachines
}

func newMockVirtClient() *mockVirtClient {
	return &mockVirtClient{
		vmClients: make(map[string]*mockVirtualMachines),
	}
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

// mockVirtualMachines implements VirtualMachineInterface for testing
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

func runUSBDeviceHandlers(ctx context.Context, detach *USBDeviceDetachHandler, attach *USBDeviceAttachHandler, vmState state.VirtualMachineState) (reconcile.Result, error) {
	result, err := detach.Handle(ctx, vmState)
	if err != nil {
		return result, err
	}
	return attach.Handle(ctx, vmState)
}

var _ = Describe("USB device handlers", func() {
	var ctx context.Context
	var fakeClient client.WithWatch
	var mockVirtCl *mockVirtClient
	var detachHandler *USBDeviceDetachHandler
	var attachHandler *USBDeviceAttachHandler
	var vmState state.VirtualMachineState
	var vmResource *reconciler.Resource[*v1alpha2.VirtualMachine, v1alpha2.VirtualMachineStatus]

	BeforeEach(func() {
		ctx = logger.ToContext(context.TODO(), slog.Default())
		mockVirtCl = newMockVirtClient()
	})

	Context("when handling USB devices", func() {
		It("should use ResourceClaimTemplate from USBDevice when present", func() {
			vm := &v1alpha2.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-vm",
					Namespace: "default",
					UID:       types.UID("vm-uid"),
				},
				Spec: v1alpha2.VirtualMachineSpec{
					USBDevices: []v1alpha2.USBDeviceSpecRef{
						{Name: "usb-device-1"},
					},
				},
				Status: v1alpha2.VirtualMachineStatus{
					Phase: v1alpha2.MachineStopped,
				},
			}

			usbDevice := &v1alpha2.USBDevice{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "usb-device-1",
					Namespace: "default",
				},
				Status: v1alpha2.USBDeviceStatus{
					Attributes: v1alpha2.NodeUSBDeviceAttributes{
						Name:      "usb-device-1",
						VendorID:  "1234",
						ProductID: "5678",
					},
					NodeName: "node-1",
				},
			}

			// ResourceClaimTemplate is created by USBDevice controller
			resourceClaimTemplate := &resourcev1beta1.ResourceClaimTemplate{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "usb-device-1-template",
					Namespace: "default",
				},
			}

			scheme := apiruntime.NewScheme()
			Expect(v1alpha2.AddToScheme(scheme)).To(Succeed())
			Expect(resourcev1beta1.AddToScheme(scheme)).To(Succeed())

			fakeClient = fake.NewClientBuilder().WithScheme(scheme).WithObjects(vm, usbDevice, resourceClaimTemplate).Build()

			vmResource = reconciler.NewResource(
				types.NamespacedName{Name: vm.Name, Namespace: vm.Namespace},
				fakeClient,
				func() *v1alpha2.VirtualMachine { return &v1alpha2.VirtualMachine{} },
				func(obj *v1alpha2.VirtualMachine) v1alpha2.VirtualMachineStatus { return obj.Status },
			)
			Expect(vmResource.Fetch(ctx)).To(Succeed())

			vmState = state.New(fakeClient, vmResource)
			detachHandler = NewUSBDeviceDetachHandler(fakeClient, mockVirtCl)
			attachHandler = NewUSBDeviceAttachHandler(fakeClient, mockVirtCl)

			result, err := runUSBDeviceHandlers(ctx, detachHandler, attachHandler, vmState)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(reconcile.Result{}))

			// Template is managed by USBDevice controller, VM only uses it
			template := &resourcev1beta1.ResourceClaimTemplate{}
			err = fakeClient.Get(ctx, types.NamespacedName{Name: "usb-device-1-template", Namespace: "default"}, template)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should attach USB device when ready", func() {
			vm := &v1alpha2.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-vm",
					Namespace: "default",
					UID:       types.UID("vm-uid"),
				},
				Spec: v1alpha2.VirtualMachineSpec{
					USBDevices: []v1alpha2.USBDeviceSpecRef{
						{Name: "usb-device-1"},
					},
				},
				Status: v1alpha2.VirtualMachineStatus{
					Phase: v1alpha2.MachineRunning,
				},
			}

			usbDevice := &v1alpha2.USBDevice{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "usb-device-1",
					Namespace: "default",
				},
				Status: v1alpha2.USBDeviceStatus{
					Attributes: v1alpha2.NodeUSBDeviceAttributes{
						Name:      "usb-device-1",
						VendorID:  "1234",
						ProductID: "5678",
					},
					NodeName: "node-1",
					Conditions: []metav1.Condition{
						{
							Type:   "Ready",
							Status: metav1.ConditionTrue,
						},
					},
				},
			}

			resourceClaimTemplate := &resourcev1beta1.ResourceClaimTemplate{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "usb-device-1-template",
					Namespace: "default",
				},
			}

			scheme := apiruntime.NewScheme()
			Expect(v1alpha2.AddToScheme(scheme)).To(Succeed())
			Expect(resourcev1beta1.AddToScheme(scheme)).To(Succeed())

			fakeClient = fake.NewClientBuilder().WithScheme(scheme).WithObjects(vm, usbDevice, resourceClaimTemplate).Build()

			vmResource = reconciler.NewResource(
				types.NamespacedName{Name: vm.Name, Namespace: vm.Namespace},
				fakeClient,
				func() *v1alpha2.VirtualMachine { return &v1alpha2.VirtualMachine{} },
				func(obj *v1alpha2.VirtualMachine) v1alpha2.VirtualMachineStatus { return obj.Status },
			)
			Expect(vmResource.Fetch(ctx)).To(Succeed())

			vmState = state.New(fakeClient, vmResource)
			detachHandler = NewUSBDeviceDetachHandler(fakeClient, mockVirtCl)
			attachHandler = NewUSBDeviceAttachHandler(fakeClient, mockVirtCl)

			result, err := runUSBDeviceHandlers(ctx, detachHandler, attachHandler, vmState)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(reconcile.Result{}))

			// Verify AddResourceClaim was called
			mockVM := mockVirtCl.vmClients["default"]
			Expect(mockVM.addResourceClaimCalls).To(HaveLen(1))
			Expect(mockVM.addResourceClaimCalls[0].Name).To(Equal("usb-device-1"))

			// Verify status was updated
			Expect(vmResource.Changed().Status.USBDevices).To(HaveLen(1))
			Expect(vmResource.Changed().Status.USBDevices[0].Name).To(Equal("usb-device-1"))
			Expect(vmResource.Changed().Status.USBDevices[0].Attached).To(BeTrue())
			Expect(vmResource.Changed().Status.USBDevices[0].Hotplugged).To(BeTrue())
			// Hotplugged devices don't get a fixed address - it's assigned dynamically by hypervisor
			Expect(vmResource.Changed().Status.USBDevices[0].Address).To(BeNil())
		})

		It("should not attach USB device when not ready", func() {
			vm := &v1alpha2.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-vm",
					Namespace: "default",
					UID:       types.UID("vm-uid"),
				},
				Spec: v1alpha2.VirtualMachineSpec{
					USBDevices: []v1alpha2.USBDeviceSpecRef{
						{Name: "usb-device-1"},
					},
				},
				Status: v1alpha2.VirtualMachineStatus{
					Phase: v1alpha2.MachineStopped,
				},
			}

			usbDevice := &v1alpha2.USBDevice{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "usb-device-1",
					Namespace: "default",
				},
				Status: v1alpha2.USBDeviceStatus{
					Attributes: v1alpha2.NodeUSBDeviceAttributes{
						VendorID:  "", // Missing vendor ID
						ProductID: "5678",
					},
					NodeName: "node-1",
				},
			}

			scheme := apiruntime.NewScheme()
			Expect(v1alpha2.AddToScheme(scheme)).To(Succeed())
			Expect(resourcev1beta1.AddToScheme(scheme)).To(Succeed())

			fakeClient = fake.NewClientBuilder().WithScheme(scheme).WithObjects(vm, usbDevice).Build()

			vmResource = reconciler.NewResource(
				types.NamespacedName{Name: vm.Name, Namespace: vm.Namespace},
				fakeClient,
				func() *v1alpha2.VirtualMachine { return &v1alpha2.VirtualMachine{} },
				func(obj *v1alpha2.VirtualMachine) v1alpha2.VirtualMachineStatus { return obj.Status },
			)
			Expect(vmResource.Fetch(ctx)).To(Succeed())

			vmState = state.New(fakeClient, vmResource)
			detachHandler = NewUSBDeviceDetachHandler(fakeClient, mockVirtCl)
			attachHandler = NewUSBDeviceAttachHandler(fakeClient, mockVirtCl)

			result, err := runUSBDeviceHandlers(ctx, detachHandler, attachHandler, vmState)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(reconcile.Result{}))

			// Verify device was not attached
			Expect(vmResource.Changed().Status.USBDevices).To(HaveLen(1))
			Expect(vmResource.Changed().Status.USBDevices[0].Attached).To(BeFalse())
		})

		It("should handle missing USB device gracefully", func() {
			vm := &v1alpha2.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-vm",
					Namespace: "default",
					UID:       types.UID("vm-uid"),
				},
				Spec: v1alpha2.VirtualMachineSpec{
					USBDevices: []v1alpha2.USBDeviceSpecRef{
						{Name: "non-existent-device"},
					},
				},
				Status: v1alpha2.VirtualMachineStatus{
					Phase: v1alpha2.MachineStopped,
				},
			}

			scheme := apiruntime.NewScheme()
			Expect(v1alpha2.AddToScheme(scheme)).To(Succeed())

			fakeClient = fake.NewClientBuilder().WithScheme(scheme).WithObjects(vm).Build()

			vmResource = reconciler.NewResource(
				types.NamespacedName{Name: vm.Name, Namespace: vm.Namespace},
				fakeClient,
				func() *v1alpha2.VirtualMachine { return &v1alpha2.VirtualMachine{} },
				func(obj *v1alpha2.VirtualMachine) v1alpha2.VirtualMachineStatus { return obj.Status },
			)
			Expect(vmResource.Fetch(ctx)).To(Succeed())

			vmState = state.New(fakeClient, vmResource)
			detachHandler = NewUSBDeviceDetachHandler(fakeClient, mockVirtCl)
			attachHandler = NewUSBDeviceAttachHandler(fakeClient, mockVirtCl)

			result, err := runUSBDeviceHandlers(ctx, detachHandler, attachHandler, vmState)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(reconcile.Result{}))

			// Verify device is tracked in status but not attached
			Expect(vmResource.Changed().Status.USBDevices).To(HaveLen(1))
			Expect(vmResource.Changed().Status.USBDevices[0].Name).To(Equal("non-existent-device"))
			Expect(vmResource.Changed().Status.USBDevices[0].Attached).To(BeFalse())
		})

		It("should detach USB device when removed from spec", func() {
			vm := &v1alpha2.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-vm",
					Namespace: "default",
					UID:       types.UID("vm-uid"),
				},
				Spec: v1alpha2.VirtualMachineSpec{
					USBDevices: []v1alpha2.USBDeviceSpecRef{}, // Empty - device removed
				},
				Status: v1alpha2.VirtualMachineStatus{
					Phase: v1alpha2.MachineRunning,
					USBDevices: []v1alpha2.USBDeviceStatusRef{
						{
							Name:     "usb-device-1",
							Attached: true,
							Address: &v1alpha2.USBAddress{
								Bus:  0,
								Port: 1,
							},
						},
					},
				},
			}

			scheme := apiruntime.NewScheme()
			Expect(v1alpha2.AddToScheme(scheme)).To(Succeed())

			fakeClient = fake.NewClientBuilder().WithScheme(scheme).WithObjects(vm).Build()

			vmResource = reconciler.NewResource(
				types.NamespacedName{Name: vm.Name, Namespace: vm.Namespace},
				fakeClient,
				func() *v1alpha2.VirtualMachine { return &v1alpha2.VirtualMachine{} },
				func(obj *v1alpha2.VirtualMachine) v1alpha2.VirtualMachineStatus { return obj.Status },
			)
			Expect(vmResource.Fetch(ctx)).To(Succeed())

			vmState = state.New(fakeClient, vmResource)
			detachHandler = NewUSBDeviceDetachHandler(fakeClient, mockVirtCl)
			attachHandler = NewUSBDeviceAttachHandler(fakeClient, mockVirtCl)

			result, err := runUSBDeviceHandlers(ctx, detachHandler, attachHandler, vmState)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(reconcile.Result{}))

			// Verify RemoveResourceClaim was called
			mockVM := mockVirtCl.vmClients["default"]
			Expect(mockVM.removeResourceClaimCalls).To(HaveLen(1))
			Expect(mockVM.removeResourceClaimCalls[0].Name).To(Equal("usb-device-1"))

			// Verify device was removed from status
			Expect(vmResource.Changed().Status.USBDevices).To(BeEmpty())
			// ResourceClaimTemplate is owned by USBDevice and is not deleted by VM controller
		})

		It("should set address from KVVMI when device has usbAddress in status", func() {
			vm := &v1alpha2.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-vm",
					Namespace: "default",
					UID:       types.UID("vm-uid"),
				},
				Spec: v1alpha2.VirtualMachineSpec{
					USBDevices: []v1alpha2.USBDeviceSpecRef{
						{Name: "usb-device-1"},
					},
				},
				Status: v1alpha2.VirtualMachineStatus{
					Phase: v1alpha2.MachineRunning,
					USBDevices: []v1alpha2.USBDeviceStatusRef{
						{
							Name:     "usb-device-1",
							Attached: true,
						},
					},
				},
			}

			usbDevice := &v1alpha2.USBDevice{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "usb-device-1",
					Namespace: "default",
				},
				Status: v1alpha2.USBDeviceStatus{
					Attributes: v1alpha2.NodeUSBDeviceAttributes{
						Name:      "usb-device-1",
						VendorID:  "1234",
						ProductID: "5678",
					},
					NodeName: "node-1",
					Conditions: []metav1.Condition{
						{
							Type:   "Ready",
							Status: metav1.ConditionTrue,
						},
					},
				},
			}

			resourceClaimTemplate := &resourcev1beta1.ResourceClaimTemplate{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "usb-device-1-template",
					Namespace: "default",
				},
			}

			// KVVMI reports bus/device for the USB device — we only use address from here
			kvvmi := &virtv1.VirtualMachineInstance{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-vm",
					Namespace: "default",
				},
				Status: virtv1.VirtualMachineInstanceStatus{
					DeviceStatus: &virtv1.DeviceStatus{
						HostDeviceStatuses: []virtv1.DeviceStatusInfo{
							{
								Name: "usb-device-1",
								DeviceResourceClaimStatus: &virtv1.DeviceResourceClaimStatus{
									Attributes: &virtv1.DeviceAttribute{
										USBAddress: &virtv1.USBAddress{
											Bus:           0,
											DeviceNumber:  2,
										},
									},
								},
							},
						},
					},
				},
			}

			scheme := apiruntime.NewScheme()
			Expect(v1alpha2.AddToScheme(scheme)).To(Succeed())
			Expect(resourcev1beta1.AddToScheme(scheme)).To(Succeed())
			Expect(virtv1.AddToScheme(scheme)).To(Succeed())

			fakeClient = fake.NewClientBuilder().WithScheme(scheme).WithObjects(vm, usbDevice, resourceClaimTemplate, kvvmi).Build()

			vmResource = reconciler.NewResource(
				types.NamespacedName{Name: vm.Name, Namespace: vm.Namespace},
				fakeClient,
				func() *v1alpha2.VirtualMachine { return &v1alpha2.VirtualMachine{} },
				func(obj *v1alpha2.VirtualMachine) v1alpha2.VirtualMachineStatus { return obj.Status },
			)
			Expect(vmResource.Fetch(ctx)).To(Succeed())

			vmState = state.New(fakeClient, vmResource)
			detachHandler = NewUSBDeviceDetachHandler(fakeClient, mockVirtCl)
			attachHandler = NewUSBDeviceAttachHandler(fakeClient, mockVirtCl)

			result, err := runUSBDeviceHandlers(ctx, detachHandler, attachHandler, vmState)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(reconcile.Result{}))

			// Address is taken only from KVVMI, never assigned by us
			Expect(vmResource.Changed().Status.USBDevices).To(HaveLen(1))
			Expect(vmResource.Changed().Status.USBDevices[0].Address).To(Equal(&v1alpha2.USBAddress{Bus: 0, Port: 2}))
		})
	})

	Context("USBDeviceDetachHandler only", func() {
		It("should call RemoveResourceClaim when device is removed from spec", func() {
			vm := &v1alpha2.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-vm",
					Namespace: "default",
					UID:       types.UID("vm-uid"),
				},
				Spec: v1alpha2.VirtualMachineSpec{
					USBDevices: []v1alpha2.USBDeviceSpecRef{},
				},
				Status: v1alpha2.VirtualMachineStatus{
					Phase: v1alpha2.MachineRunning,
					USBDevices: []v1alpha2.USBDeviceStatusRef{
						{Name: "usb-device-1", Attached: true},
					},
				},
			}

			scheme := apiruntime.NewScheme()
			Expect(v1alpha2.AddToScheme(scheme)).To(Succeed())
			fakeClient = fake.NewClientBuilder().WithScheme(scheme).WithObjects(vm).Build()

			vmResource = reconciler.NewResource(
				types.NamespacedName{Name: vm.Name, Namespace: vm.Namespace},
				fakeClient,
				func() *v1alpha2.VirtualMachine { return &v1alpha2.VirtualMachine{} },
				func(obj *v1alpha2.VirtualMachine) v1alpha2.VirtualMachineStatus { return obj.Status },
			)
			Expect(vmResource.Fetch(ctx)).To(Succeed())
			vmState = state.New(fakeClient, vmResource)
			detachHandler = NewUSBDeviceDetachHandler(fakeClient, mockVirtCl)

			_, err := detachHandler.Handle(ctx, vmState)
			Expect(err).NotTo(HaveOccurred())

			mockVM := mockVirtCl.vmClients["default"]
			Expect(mockVM.removeResourceClaimCalls).To(HaveLen(1))
			Expect(mockVM.removeResourceClaimCalls[0].Name).To(Equal("usb-device-1"))
		})

		It("should do nothing when VM has no USB devices in status", func() {
			vm := &v1alpha2.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-vm",
					Namespace: "default",
					UID:       types.UID("vm-uid"),
				},
				Spec: v1alpha2.VirtualMachineSpec{
					USBDevices: []v1alpha2.USBDeviceSpecRef{{Name: "usb-device-1"}},
				},
				Status: v1alpha2.VirtualMachineStatus{
					Phase:      v1alpha2.MachineRunning,
					USBDevices: []v1alpha2.USBDeviceStatusRef{},
				},
			}

			fakeClient, vmResource, vmState = setupEnvironment(vm)
			detachHandler = NewUSBDeviceDetachHandler(fakeClient, mockVirtCl)

			_, err := detachHandler.Handle(ctx, vmState)
			Expect(err).NotTo(HaveOccurred())

			_ = mockVirtCl.VirtualMachines("default") // ensure entry exists
			mockVM := mockVirtCl.vmClients["default"]
			Expect(mockVM.removeResourceClaimCalls).To(BeEmpty())
		})
	})

	Context("USBDeviceAttachHandler only", func() {
		It("should not attach and set Attached false when ResourceClaimTemplate is missing", func() {
			vm := &v1alpha2.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-vm",
					Namespace: "default",
					UID:       types.UID("vm-uid"),
				},
				Spec: v1alpha2.VirtualMachineSpec{
					USBDevices: []v1alpha2.USBDeviceSpecRef{{Name: "usb-device-1"}},
				},
				Status: v1alpha2.VirtualMachineStatus{
					Phase: v1alpha2.MachineRunning,
				},
			}
			usbDevice := &v1alpha2.USBDevice{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "usb-device-1",
					Namespace: "default",
				},
				Status: v1alpha2.USBDeviceStatus{
					Attributes: v1alpha2.NodeUSBDeviceAttributes{
						Name:      "usb-device-1",
						VendorID:  "1234",
						ProductID: "5678",
					},
					NodeName: "node-1",
					Conditions: []metav1.Condition{
						{Type: "Ready", Status: metav1.ConditionTrue},
					},
				},
			}
			// No ResourceClaimTemplate in cluster

			fakeClient, vmResource, vmState = setupEnvironment(vm, usbDevice)
			attachHandler = NewUSBDeviceAttachHandler(fakeClient, mockVirtCl)

			_, err := attachHandler.Handle(ctx, vmState)
			Expect(err).NotTo(HaveOccurred())

			changed := vmResource.Changed()
			Expect(changed).ToNot(BeNil())
			Expect(changed.Status.USBDevices).To(HaveLen(1))
			Expect(changed.Status.USBDevices[0].Attached).To(BeFalse())
			_ = mockVirtCl.VirtualMachines("default") // ensure entry exists
			mockVM := mockVirtCl.vmClients["default"]
			Expect(mockVM.addResourceClaimCalls).To(BeEmpty())
		})
	})
})
