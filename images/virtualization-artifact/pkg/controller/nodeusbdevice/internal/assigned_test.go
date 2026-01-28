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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apiruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/nodeusbdevice/internal/state"
	"github.com/deckhouse/virtualization-controller/pkg/controller/reconciler"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/nodeusbdevicecondition"
)

var _ = Describe("AssignedHandler", func() {
	var ctx context.Context
	var fakeClient client.WithWatch
	var handler *AssignedHandler
	var nodeUSBDeviceState state.NodeUSBDeviceState
	var nodeUSBDeviceResource *reconciler.Resource[*v1alpha2.NodeUSBDevice, v1alpha2.NodeUSBDeviceStatus]

	BeforeEach(func() {
		ctx = logger.ToContext(context.TODO(), slog.Default())
	})

	Context("when namespace is assigned", func() {
		It("should create USBDevice in assigned namespace", func() {
			namespace := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-namespace",
				},
			}

			nodeUSBDevice := &v1alpha2.NodeUSBDevice{
				ObjectMeta: metav1.ObjectMeta{
					Name: "usb-device-1",
					UID:  types.UID("node-usb-device-uid-1"),
				},
				Spec: v1alpha2.NodeUSBDeviceSpec{
					AssignedNamespace: "test-namespace",
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
			Expect(corev1.AddToScheme(scheme)).To(Succeed())

			fakeClient = fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(nodeUSBDevice, namespace).
				WithStatusSubresource(&v1alpha2.USBDevice{}).
				Build()

			nodeUSBDeviceResource = reconciler.NewResource(
				types.NamespacedName{Name: nodeUSBDevice.Name},
				fakeClient,
				func() *v1alpha2.NodeUSBDevice { return &v1alpha2.NodeUSBDevice{} },
				func(obj *v1alpha2.NodeUSBDevice) v1alpha2.NodeUSBDeviceStatus { return obj.Status },
			)
			Expect(nodeUSBDeviceResource.Fetch(ctx)).To(Succeed())

			nodeUSBDeviceState = state.New(fakeClient, nodeUSBDeviceResource)
			recorder := &eventrecord.EventRecorderLoggerMock{}
			handler = NewAssignedHandler(fakeClient, recorder)

			result, err := handler.Handle(ctx, nodeUSBDeviceState)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(reconcile.Result{}))

			// Verify USBDevice was created
			usbDevice := &v1alpha2.USBDevice{}
			err = fakeClient.Get(ctx, types.NamespacedName{Name: "usb-device-1", Namespace: "test-namespace"}, usbDevice)
			Expect(err).NotTo(HaveOccurred())
			Expect(usbDevice.Status.Attributes.VendorID).To(Equal("1234"))
			Expect(usbDevice.Status.NodeName).To(Equal("node-1"))

			// Verify Assigned condition was set
			conditions := nodeUSBDeviceResource.Changed().Status.Conditions
			Expect(conditions).To(HaveLen(1))
			Expect(conditions[0].Type).To(Equal(string(nodeusbdevicecondition.AssignedType)))
			Expect(conditions[0].Status).To(Equal(metav1.ConditionTrue))
			Expect(conditions[0].Reason).To(Equal(string(nodeusbdevicecondition.Assigned)))
		})

		It("should update USBDevice when it already exists", func() {
			namespace := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-namespace",
				},
			}

			existingUSBDevice := &v1alpha2.USBDevice{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "usb-device-1",
					Namespace: "test-namespace",
				},
				Status: v1alpha2.USBDeviceStatus{
					Attributes: v1alpha2.NodeUSBDeviceAttributes{
						VendorID:  "0000",
						ProductID: "0000",
					},
				},
			}

			nodeUSBDevice := &v1alpha2.NodeUSBDevice{
				ObjectMeta: metav1.ObjectMeta{
					Name: "usb-device-1",
				},
				Spec: v1alpha2.NodeUSBDeviceSpec{
					AssignedNamespace: "test-namespace",
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
			Expect(corev1.AddToScheme(scheme)).To(Succeed())

			fakeClient = fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(nodeUSBDevice, namespace, existingUSBDevice).
				WithStatusSubresource(&v1alpha2.USBDevice{}).
				Build()

			nodeUSBDeviceResource = reconciler.NewResource(
				types.NamespacedName{Name: nodeUSBDevice.Name},
				fakeClient,
				func() *v1alpha2.NodeUSBDevice { return &v1alpha2.NodeUSBDevice{} },
				func(obj *v1alpha2.NodeUSBDevice) v1alpha2.NodeUSBDeviceStatus { return obj.Status },
			)
			Expect(nodeUSBDeviceResource.Fetch(ctx)).To(Succeed())

			nodeUSBDeviceState = state.New(fakeClient, nodeUSBDeviceResource)
			recorder := &eventrecord.EventRecorderLoggerMock{}
			handler = NewAssignedHandler(fakeClient, recorder)

			result, err := handler.Handle(ctx, nodeUSBDeviceState)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(reconcile.Result{}))

			// Verify USBDevice was updated
			usbDevice := &v1alpha2.USBDevice{}
			err = fakeClient.Get(ctx, types.NamespacedName{Name: "usb-device-1", Namespace: "test-namespace"}, usbDevice)
			Expect(err).NotTo(HaveOccurred())
			Expect(usbDevice.Status.Attributes.VendorID).To(Equal("1234"))
			Expect(usbDevice.Status.Attributes.ProductID).To(Equal("5678"))
			Expect(usbDevice.Status.NodeName).To(Equal("node-1"))
		})
	})

	Context("when namespace is not assigned", func() {
		It("should delete USBDevice and set Available condition", func() {
			existingUSBDevice := &v1alpha2.USBDevice{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "usb-device-1",
					Namespace: "test-namespace",
				},
			}

			nodeUSBDevice := &v1alpha2.NodeUSBDevice{
				ObjectMeta: metav1.ObjectMeta{
					Name: "usb-device-1",
				},
				Spec: v1alpha2.NodeUSBDeviceSpec{
					AssignedNamespace: "", // No namespace assigned
				},
			}

			scheme := apiruntime.NewScheme()
			Expect(v1alpha2.AddToScheme(scheme)).To(Succeed())

			fakeClient = fake.NewClientBuilder().WithScheme(scheme).WithObjects(nodeUSBDevice, existingUSBDevice).Build()

			nodeUSBDeviceResource = reconciler.NewResource(
				types.NamespacedName{Name: nodeUSBDevice.Name},
				fakeClient,
				func() *v1alpha2.NodeUSBDevice { return &v1alpha2.NodeUSBDevice{} },
				func(obj *v1alpha2.NodeUSBDevice) v1alpha2.NodeUSBDeviceStatus { return obj.Status },
			)
			Expect(nodeUSBDeviceResource.Fetch(ctx)).To(Succeed())

			nodeUSBDeviceState = state.New(fakeClient, nodeUSBDeviceResource)
			recorder := &eventrecord.EventRecorderLoggerMock{}
			handler = NewAssignedHandler(fakeClient, recorder)

			result, err := handler.Handle(ctx, nodeUSBDeviceState)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(reconcile.Result{}))

			// Verify USBDevice was deleted
			usbDevice := &v1alpha2.USBDevice{}
			err = fakeClient.Get(ctx, types.NamespacedName{Name: "usb-device-1", Namespace: "test-namespace"}, usbDevice)
			Expect(err).To(HaveOccurred())

			// Verify Available condition was set
			conditions := nodeUSBDeviceResource.Changed().Status.Conditions
			Expect(conditions).To(HaveLen(1))
			Expect(conditions[0].Type).To(Equal(string(nodeusbdevicecondition.AssignedType)))
			Expect(conditions[0].Status).To(Equal(metav1.ConditionFalse))
			Expect(conditions[0].Reason).To(Equal(string(nodeusbdevicecondition.Available)))
		})
	})

	Context("when assigned namespace does not exist", func() {
		It("should set Available condition", func() {
			nodeUSBDevice := &v1alpha2.NodeUSBDevice{
				ObjectMeta: metav1.ObjectMeta{
					Name: "usb-device-1",
				},
				Spec: v1alpha2.NodeUSBDeviceSpec{
					AssignedNamespace: "non-existent-namespace",
				},
			}

			scheme := apiruntime.NewScheme()
			Expect(v1alpha2.AddToScheme(scheme)).To(Succeed())
			Expect(corev1.AddToScheme(scheme)).To(Succeed())

			fakeClient = fake.NewClientBuilder().WithScheme(scheme).WithObjects(nodeUSBDevice).Build()

			nodeUSBDeviceResource = reconciler.NewResource(
				types.NamespacedName{Name: nodeUSBDevice.Name},
				fakeClient,
				func() *v1alpha2.NodeUSBDevice { return &v1alpha2.NodeUSBDevice{} },
				func(obj *v1alpha2.NodeUSBDevice) v1alpha2.NodeUSBDeviceStatus { return obj.Status },
			)
			Expect(nodeUSBDeviceResource.Fetch(ctx)).To(Succeed())

			nodeUSBDeviceState = state.New(fakeClient, nodeUSBDeviceResource)
			recorder := &eventrecord.EventRecorderLoggerMock{}
			handler = NewAssignedHandler(fakeClient, recorder)

			result, err := handler.Handle(ctx, nodeUSBDeviceState)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(reconcile.Result{}))

			// Verify Available condition was set
			conditions := nodeUSBDeviceResource.Changed().Status.Conditions
			Expect(conditions).To(HaveLen(1))
			Expect(conditions[0].Type).To(Equal(string(nodeusbdevicecondition.AssignedType)))
			Expect(conditions[0].Status).To(Equal(metav1.ConditionFalse))
			Expect(conditions[0].Reason).To(Equal(string(nodeusbdevicecondition.Available)))
		})
	})
})
