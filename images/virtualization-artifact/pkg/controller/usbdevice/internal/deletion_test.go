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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apiruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/indexer"
	"github.com/deckhouse/virtualization-controller/pkg/controller/reconciler"
	"github.com/deckhouse/virtualization-controller/pkg/controller/usbdevice/internal/state"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	fakeversioned "github.com/deckhouse/virtualization/api/client/generated/clientset/versioned/fake"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/usbdevicecondition"
)

var _ = Describe("DeletionHandler", func() {
	var ctx context.Context
	var fakeClient client.WithWatch
	var handler *DeletionHandler
	var usbDeviceState state.USBDeviceState
	var usbDeviceResource *reconciler.Resource[*v1alpha2.USBDevice, v1alpha2.USBDeviceStatus]

	BeforeEach(func() {
		ctx = logger.ToContext(context.TODO(), slog.Default())
	})

	Context("when USBDevice is not being deleted", func() {
		It("should add finalizer", func() {
			usbDevice := &v1alpha2.USBDevice{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "usb-device-1",
					Namespace: "default",
				},
			}

			scheme := apiruntime.NewScheme()
			Expect(v1alpha2.AddToScheme(scheme)).To(Succeed())

			vmObj, vmField, vmExtractValue := indexer.IndexVMByUSBDevice()
			nodeUSBDeviceObj, nodeUSBDeviceField, nodeUSBDeviceExtractValue := indexer.IndexNodeUSBDeviceByName()
			fakeClient = fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(usbDevice).
				WithIndex(vmObj, vmField, vmExtractValue).
				WithIndex(nodeUSBDeviceObj, nodeUSBDeviceField, nodeUSBDeviceExtractValue).
				Build()

			usbDeviceResource = reconciler.NewResource(
				types.NamespacedName{Name: usbDevice.Name, Namespace: usbDevice.Namespace},
				fakeClient,
				func() *v1alpha2.USBDevice { return &v1alpha2.USBDevice{} },
				func(obj *v1alpha2.USBDevice) v1alpha2.USBDeviceStatus { return obj.Status },
			)
			Expect(usbDeviceResource.Fetch(ctx)).To(Succeed())

			usbDeviceState = state.New(fakeClient, usbDeviceResource)
			recorder := &eventrecord.EventRecorderLoggerMock{}
			fakeVirtClient := fakeversioned.NewSimpleClientset()
			handler = NewDeletionHandler(fakeClient, fakeVirtClient, recorder)

			result, err := handler.Handle(ctx, usbDeviceState)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(reconcile.Result{}))

			// Verify finalizer was added
			Expect(usbDeviceResource.Changed().GetFinalizers()).To(ContainElement(v1alpha2.FinalizerUSBDeviceCleanup))
		})
	})

	Context("when USBDevice is being deleted", func() {
		It("should remove finalizer when device is not attached", func() {
			now := metav1.Now()
			usbDevice := &v1alpha2.USBDevice{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "usb-device-1",
					Namespace:         "default",
					Finalizers:        []string{v1alpha2.FinalizerUSBDeviceCleanup},
					DeletionTimestamp: &now,
				},
				Status: v1alpha2.USBDeviceStatus{
					Conditions: []metav1.Condition{
						{
							Type:   string(usbdevicecondition.AttachedType),
							Status: metav1.ConditionFalse,
							Reason: string(usbdevicecondition.Available),
						},
					},
				},
			}

			scheme := apiruntime.NewScheme()
			Expect(v1alpha2.AddToScheme(scheme)).To(Succeed())

			vmObj, vmField, vmExtractValue := indexer.IndexVMByUSBDevice()
			nodeUSBDeviceObj, nodeUSBDeviceField, nodeUSBDeviceExtractValue := indexer.IndexNodeUSBDeviceByName()
			fakeClient = fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(usbDevice).
				WithIndex(vmObj, vmField, vmExtractValue).
				WithIndex(nodeUSBDeviceObj, nodeUSBDeviceField, nodeUSBDeviceExtractValue).
				Build()

			usbDeviceResource = reconciler.NewResource(
				types.NamespacedName{Name: usbDevice.Name, Namespace: usbDevice.Namespace},
				fakeClient,
				func() *v1alpha2.USBDevice { return &v1alpha2.USBDevice{} },
				func(obj *v1alpha2.USBDevice) v1alpha2.USBDeviceStatus { return obj.Status },
			)
			Expect(usbDeviceResource.Fetch(ctx)).To(Succeed())

			usbDeviceState = state.New(fakeClient, usbDeviceResource)
			recorder := &eventrecord.EventRecorderLoggerMock{}
			fakeVirtClient := fakeversioned.NewSimpleClientset()
			handler = NewDeletionHandler(fakeClient, fakeVirtClient, recorder)

			result, err := handler.Handle(ctx, usbDeviceState)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(reconcile.Result{}))

			// Verify finalizer was removed
			Expect(usbDeviceResource.Changed().GetFinalizers()).NotTo(ContainElement(v1alpha2.FinalizerUSBDeviceCleanup))
		})

		It("should requeue when device is attached", func() {
			now := metav1.Now()
			usbDevice := &v1alpha2.USBDevice{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "usb-device-1",
					Namespace:         "default",
					Finalizers:        []string{v1alpha2.FinalizerUSBDeviceCleanup},
					DeletionTimestamp: &now,
				},
				Status: v1alpha2.USBDeviceStatus{
					Conditions: []metav1.Condition{
						{
							Type:   string(usbdevicecondition.AttachedType),
							Status: metav1.ConditionTrue,
							Reason: string(usbdevicecondition.AttachedToVirtualMachine),
						},
					},
				},
			}

			// Create a VM that uses this USB device
			vm := &v1alpha2.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-vm",
					Namespace: "default",
				},
				Spec: v1alpha2.VirtualMachineSpec{
					USBDevices: []v1alpha2.USBDeviceSpecRef{
						{
							Name: "usb-device-1",
						},
					},
				},
				Status: v1alpha2.VirtualMachineStatus{
					USBDevices: []v1alpha2.USBDeviceStatusRef{
						{
							Name:     "usb-device-1",
							Attached: true,
						},
					},
				},
			}

			scheme := apiruntime.NewScheme()
			Expect(v1alpha2.AddToScheme(scheme)).To(Succeed())

			vmObj, vmField, vmExtractValue := indexer.IndexVMByUSBDevice()
			nodeUSBDeviceObj, nodeUSBDeviceField, nodeUSBDeviceExtractValue := indexer.IndexNodeUSBDeviceByName()
			fakeClient = fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(usbDevice, vm).
				WithIndex(vmObj, vmField, vmExtractValue).
				WithIndex(nodeUSBDeviceObj, nodeUSBDeviceField, nodeUSBDeviceExtractValue).
				Build()

			usbDeviceResource = reconciler.NewResource(
				types.NamespacedName{Name: usbDevice.Name, Namespace: usbDevice.Namespace},
				fakeClient,
				func() *v1alpha2.USBDevice { return &v1alpha2.USBDevice{} },
				func(obj *v1alpha2.USBDevice) v1alpha2.USBDeviceStatus { return obj.Status },
			)
			Expect(usbDeviceResource.Fetch(ctx)).To(Succeed())

			usbDeviceState = state.New(fakeClient, usbDeviceResource)
			recorder := &eventrecord.EventRecorderLoggerMock{
				EventfFunc: func(involvedObject client.Object, eventtype, reason, messageFmt string, args ...any) {},
			}
			fakeVirtClient := fakeversioned.NewSimpleClientset()
			handler = NewDeletionHandler(fakeClient, fakeVirtClient, recorder)

			result, err := handler.Handle(ctx, usbDeviceState)
			// Should requeue to verify device is no longer attached
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Requeue).To(BeTrue())

			// Verify finalizer was not removed yet
			Expect(usbDeviceResource.Changed().GetFinalizers()).To(ContainElement(v1alpha2.FinalizerUSBDeviceCleanup))
		})
	})
})
