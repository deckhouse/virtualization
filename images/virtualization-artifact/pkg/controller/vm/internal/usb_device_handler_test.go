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
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

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

var _ = Describe("USBDeviceHandler", func() {
	var ctx context.Context
	var fakeClient client.WithWatch
	var mockVirtCl *mockVirtClient
	var handler *USBDeviceHandler
	var vmState state.VirtualMachineState
	var vmResource *reconciler.Resource[*v1alpha2.VirtualMachine, v1alpha2.VirtualMachineStatus]

	BeforeEach(func() {
		ctx = logger.ToContext(context.TODO(), slog.Default())
		mockVirtCl = newMockVirtClient()
	})

	Context("when handling USB devices", func() {
		It("should create ResourceClaimTemplate for new USB device", func() {
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
						VendorID:  "1234",
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
			handler = NewUSBDeviceHandler(fakeClient, mockVirtCl)

			result, err := handler.Handle(ctx, vmState)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(reconcile.Result{}))

			// Verify ResourceClaimTemplate was created
			template := &resourcev1beta1.ResourceClaimTemplate{}
			templateName := "test-vm-usb-usb-device-1-template"
			err = fakeClient.Get(ctx, types.NamespacedName{Name: templateName, Namespace: "default"}, template)
			Expect(err).NotTo(HaveOccurred())
			Expect(template.OwnerReferences).To(HaveLen(1))
			Expect(template.OwnerReferences[0].Name).To(Equal("test-vm"))
			Expect(template.OwnerReferences[0].Controller).To(Equal(ptr.To(true)))
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
			handler = NewUSBDeviceHandler(fakeClient, mockVirtCl)

			result, err := handler.Handle(ctx, vmState)
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
			handler = NewUSBDeviceHandler(fakeClient, mockVirtCl)

			result, err := handler.Handle(ctx, vmState)
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
			handler = NewUSBDeviceHandler(fakeClient, mockVirtCl)

			result, err := handler.Handle(ctx, vmState)
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
			handler = NewUSBDeviceHandler(fakeClient, mockVirtCl)

			result, err := handler.Handle(ctx, vmState)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(reconcile.Result{}))

			// Verify RemoveResourceClaim was called
			mockVM := mockVirtCl.vmClients["default"]
			Expect(mockVM.removeResourceClaimCalls).To(HaveLen(1))
			Expect(mockVM.removeResourceClaimCalls[0].Name).To(Equal("usb-device-1"))

			// Verify device was removed from status
			Expect(vmResource.Changed().Status.USBDevices).To(BeEmpty())
		})

		It("should keep existing address when device already attached", func() {
			existingAddress := &v1alpha2.USBAddress{
				Bus:  0,
				Port: 2,
			}

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
							Address:  existingAddress,
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
			handler = NewUSBDeviceHandler(fakeClient, mockVirtCl)

			result, err := handler.Handle(ctx, vmState)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(reconcile.Result{}))

			// Verify existing address was preserved
			Expect(vmResource.Changed().Status.USBDevices).To(HaveLen(1))
			Expect(vmResource.Changed().Status.USBDevices[0].Address).To(Equal(existingAddress))
		})
	})
})
