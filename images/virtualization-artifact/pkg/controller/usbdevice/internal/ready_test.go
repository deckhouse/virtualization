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
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/nodeusbdevicecondition"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/usbdevicecondition"
)

var _ = Describe("SyncReadyHandler - Ready condition", func() {
	var ctx context.Context
	var fakeClient client.WithWatch
	var handler *SyncReadyHandler
	var usbDeviceState state.USBDeviceState
	var usbDeviceResource *reconciler.Resource[*v1alpha2.USBDevice, v1alpha2.USBDeviceStatus]

	BeforeEach(func() {
		ctx = logger.ToContext(context.TODO(), slog.Default())
	})

	Context("when NodeUSBDevice is found", func() {
		It("should translate Ready condition from NodeUSBDevice when Ready", func() {
			usbDevice := &v1alpha2.USBDevice{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "usb-device-1",
					Namespace: "default",
				},
			}

			nodeUSBDevice := &v1alpha2.NodeUSBDevice{
				ObjectMeta: metav1.ObjectMeta{
					Name: "usb-device-1",
				},
				Status: v1alpha2.NodeUSBDeviceStatus{
					Conditions: []metav1.Condition{
						{
							Type:               string(nodeusbdevicecondition.ReadyType),
							Status:             metav1.ConditionTrue,
							Reason:             string(nodeusbdevicecondition.Ready),
							Message:            "Device is ready",
							LastTransitionTime: metav1.Now(),
						},
					},
				},
			}

			scheme := apiruntime.NewScheme()
			Expect(v1alpha2.AddToScheme(scheme)).To(Succeed())

			obj, field, extractValue := indexer.IndexNodeUSBDeviceByName()
			fakeClient = fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(usbDevice, nodeUSBDevice).
				WithIndex(obj, field, extractValue).
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
			handler = NewSyncReadyHandler(recorder)

			result, err := handler.Handle(ctx, usbDeviceState)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(reconcile.Result{}))

			// Verify Ready condition was set
			conditions := usbDeviceResource.Changed().Status.Conditions
			Expect(conditions).To(HaveLen(1))
			Expect(conditions[0].Type).To(Equal(string(usbdevicecondition.ReadyType)))
			Expect(conditions[0].Status).To(Equal(metav1.ConditionTrue))
			Expect(conditions[0].Reason).To(Equal(string(usbdevicecondition.Ready)))
		})

		It("should translate NotReady condition from NodeUSBDevice", func() {
			usbDevice := &v1alpha2.USBDevice{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "usb-device-1",
					Namespace: "default",
				},
			}

			nodeUSBDevice := &v1alpha2.NodeUSBDevice{
				ObjectMeta: metav1.ObjectMeta{
					Name: "usb-device-1",
				},
				Status: v1alpha2.NodeUSBDeviceStatus{
					Conditions: []metav1.Condition{
						{
							Type:               string(nodeusbdevicecondition.ReadyType),
							Status:             metav1.ConditionFalse,
							Reason:             string(nodeusbdevicecondition.NotReady),
							Message:            "Device is not ready",
							LastTransitionTime: metav1.Now(),
						},
					},
				},
			}

			scheme := apiruntime.NewScheme()
			Expect(v1alpha2.AddToScheme(scheme)).To(Succeed())

			obj, field, extractValue := indexer.IndexNodeUSBDeviceByName()
			fakeClient = fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(usbDevice, nodeUSBDevice).
				WithIndex(obj, field, extractValue).
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
			handler = NewSyncReadyHandler(recorder)

			result, err := handler.Handle(ctx, usbDeviceState)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(reconcile.Result{}))

			// Verify NotReady condition was set
			conditions := usbDeviceResource.Changed().Status.Conditions
			Expect(conditions).To(HaveLen(1))
			Expect(conditions[0].Type).To(Equal(string(usbdevicecondition.ReadyType)))
			Expect(conditions[0].Status).To(Equal(metav1.ConditionFalse))
			Expect(conditions[0].Reason).To(Equal(string(usbdevicecondition.NotReady)))
		})
	})

	Context("when NodeUSBDevice is not found", func() {
		It("should set NotFound condition", func() {
			usbDevice := &v1alpha2.USBDevice{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "usb-device-1",
					Namespace: "default",
				},
			}

			scheme := apiruntime.NewScheme()
			Expect(v1alpha2.AddToScheme(scheme)).To(Succeed())

			obj, field, extractValue := indexer.IndexNodeUSBDeviceByName()
			fakeClient = fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(usbDevice).
				WithIndex(obj, field, extractValue).
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
			handler = NewSyncReadyHandler(recorder)

			result, err := handler.Handle(ctx, usbDeviceState)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(reconcile.Result{}))

			// Verify NotFound condition was set
			conditions := usbDeviceResource.Changed().Status.Conditions
			Expect(conditions).To(HaveLen(1))
			Expect(conditions[0].Type).To(Equal(string(usbdevicecondition.ReadyType)))
			Expect(conditions[0].Status).To(Equal(metav1.ConditionFalse))
			Expect(conditions[0].Reason).To(Equal(string(usbdevicecondition.NotFound)))
		})
	})

	Context("when NodeUSBDevice has no Ready condition", func() {
		It("should set NotReady condition", func() {
			usbDevice := &v1alpha2.USBDevice{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "usb-device-1",
					Namespace: "default",
				},
			}

			nodeUSBDevice := &v1alpha2.NodeUSBDevice{
				ObjectMeta: metav1.ObjectMeta{
					Name: "usb-device-1",
				},
				Status: v1alpha2.NodeUSBDeviceStatus{
					Conditions: []metav1.Condition{}, // No Ready condition
				},
			}

			scheme := apiruntime.NewScheme()
			Expect(v1alpha2.AddToScheme(scheme)).To(Succeed())

			obj, field, extractValue := indexer.IndexNodeUSBDeviceByName()
			fakeClient = fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(usbDevice, nodeUSBDevice).
				WithIndex(obj, field, extractValue).
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
			handler = NewSyncReadyHandler(recorder)

			result, err := handler.Handle(ctx, usbDeviceState)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(reconcile.Result{}))

			// Verify NotReady condition was set
			conditions := usbDeviceResource.Changed().Status.Conditions
			Expect(conditions).To(HaveLen(1))
			Expect(conditions[0].Type).To(Equal(string(usbdevicecondition.ReadyType)))
			Expect(conditions[0].Status).To(Equal(metav1.ConditionFalse))
			Expect(conditions[0].Reason).To(Equal(string(usbdevicecondition.NotReady)))
		})
	})
})
