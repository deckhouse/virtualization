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

	"github.com/deckhouse/virtualization-controller/pkg/controller/reconciler"
	"github.com/deckhouse/virtualization-controller/pkg/controller/usbdevice/internal/state"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

var _ = Describe("SyncHandler", func() {
	var ctx context.Context
	var fakeClient client.WithWatch
	var handler *SyncHandler
	var usbDeviceState state.USBDeviceState
	var usbDeviceResource *reconciler.Resource[*v1alpha2.USBDevice, v1alpha2.USBDeviceStatus]

	BeforeEach(func() {
		ctx = logger.ToContext(context.TODO(), slog.Default())
	})

	Context("when NodeUSBDevice is found", func() {
		It("should sync attributes and node name from NodeUSBDevice", func() {
			usbDevice := &v1alpha2.USBDevice{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "usb-device-1",
					Namespace: "default",
				},
				Status: v1alpha2.USBDeviceStatus{
					Attributes: v1alpha2.NodeUSBDeviceAttributes{
						VendorID:  "0000",
						ProductID: "0000",
					},
					NodeName: "",
				},
			}

			nodeUSBDevice := &v1alpha2.NodeUSBDevice{
				ObjectMeta: metav1.ObjectMeta{
					Name: "usb-device-1",
				},
				Status: v1alpha2.NodeUSBDeviceStatus{
					Attributes: v1alpha2.NodeUSBDeviceAttributes{
						VendorID:  "1234",
						ProductID: "5678",
					},
					NodeName: "node-1",
				},
			}

			scheme := apiruntime.NewScheme()
			Expect(v1alpha2.AddToScheme(scheme)).To(Succeed())

			fakeClient = fake.NewClientBuilder().WithScheme(scheme).WithObjects(usbDevice, nodeUSBDevice).Build()

			usbDeviceResource = reconciler.NewResource(
				types.NamespacedName{Name: usbDevice.Name, Namespace: usbDevice.Namespace},
				fakeClient,
				func() *v1alpha2.USBDevice { return &v1alpha2.USBDevice{} },
				func(obj *v1alpha2.USBDevice) v1alpha2.USBDeviceStatus { return obj.Status },
			)
			Expect(usbDeviceResource.Fetch(ctx)).To(Succeed())

			usbDeviceState = state.New(fakeClient, usbDeviceResource)
			recorder := &eventrecord.EventRecorderLoggerMock{}
			handler = NewSyncHandler(fakeClient, recorder)

			result, err := handler.Handle(ctx, usbDeviceState)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(reconcile.Result{}))

			// Verify attributes were synced
			changed := usbDeviceResource.Changed()
			Expect(changed.Status.Attributes.VendorID).To(Equal("1234"))
			Expect(changed.Status.Attributes.ProductID).To(Equal("5678"))
			Expect(changed.Status.NodeName).To(Equal("node-1"))
		})
	})

	Context("when NodeUSBDevice is not found", func() {
		It("should not update status", func() {
			usbDevice := &v1alpha2.USBDevice{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "usb-device-1",
					Namespace: "default",
				},
				Status: v1alpha2.USBDeviceStatus{
					Attributes: v1alpha2.NodeUSBDeviceAttributes{
						VendorID:  "0000",
						ProductID: "0000",
					},
				},
			}

			scheme := apiruntime.NewScheme()
			Expect(v1alpha2.AddToScheme(scheme)).To(Succeed())

			fakeClient = fake.NewClientBuilder().WithScheme(scheme).WithObjects(usbDevice).Build()

			usbDeviceResource = reconciler.NewResource(
				types.NamespacedName{Name: usbDevice.Name, Namespace: usbDevice.Namespace},
				fakeClient,
				func() *v1alpha2.USBDevice { return &v1alpha2.USBDevice{} },
				func(obj *v1alpha2.USBDevice) v1alpha2.USBDeviceStatus { return obj.Status },
			)
			Expect(usbDeviceResource.Fetch(ctx)).To(Succeed())

			usbDeviceState = state.New(fakeClient, usbDeviceResource)
			recorder := &eventrecord.EventRecorderLoggerMock{}
			handler = NewSyncHandler(fakeClient, recorder)

			result, err := handler.Handle(ctx, usbDeviceState)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(reconcile.Result{}))

			// Verify status was not changed
			changed := usbDeviceResource.Changed()
			Expect(changed.Status.Attributes.VendorID).To(Equal("0000"))
		})
	})
})
