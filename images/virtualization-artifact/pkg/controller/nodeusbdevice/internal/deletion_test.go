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
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/nodeusbdevice/internal/state"
	"github.com/deckhouse/virtualization-controller/pkg/controller/reconciler"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

var _ = Describe("DeletionHandler", func() {
	var ctx context.Context
	var fakeClient client.WithWatch
	var handler *DeletionHandler
	var nodeUSBDeviceState state.NodeUSBDeviceState
	var nodeUSBDeviceResource *reconciler.Resource[*v1alpha2.NodeUSBDevice, v1alpha2.NodeUSBDeviceStatus]

	BeforeEach(func() {
		ctx = logger.ToContext(context.TODO(), slog.Default())
	})

	Context("when NodeUSBDevice is not being deleted", func() {
		It("should add finalizer", func() {
			nodeUSBDevice := &v1alpha2.NodeUSBDevice{
				ObjectMeta: metav1.ObjectMeta{
					Name: "usb-device-1",
				},
			}

			scheme := apiruntime.NewScheme()
			Expect(v1alpha2.AddToScheme(scheme)).To(Succeed())

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
			handler = NewDeletionHandler(fakeClient, recorder)

			result, err := handler.Handle(ctx, nodeUSBDeviceState)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(reconcile.Result{}))

			// Verify finalizer was added
			Expect(nodeUSBDeviceResource.Changed().GetFinalizers()).To(ContainElement(v1alpha2.FinalizerNodeUSBDeviceCleanup))
		})
	})

	Context("when NodeUSBDevice is being deleted", func() {
		It("should delete USBDevice and remove finalizer", func() {
			now := metav1.Now()
			nodeUSBDevice := &v1alpha2.NodeUSBDevice{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "usb-device-1",
					UID:               "node-usb-device-uid",
					Finalizers:        []string{v1alpha2.FinalizerNodeUSBDeviceCleanup},
					DeletionTimestamp: &now,
				},
				Spec: v1alpha2.NodeUSBDeviceSpec{
					AssignedNamespace: "test-namespace",
				},
			}
			usbDevice := &v1alpha2.USBDevice{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "usb-device-1",
					Namespace: "test-namespace",
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion: v1alpha2.SchemeGroupVersion.String(),
							Kind:       "NodeUSBDevice",
							Name:       nodeUSBDevice.Name,
							UID:        nodeUSBDevice.UID,
							Controller: ptr.To(true),
						},
					},
				},
			}

			scheme := apiruntime.NewScheme()
			Expect(v1alpha2.AddToScheme(scheme)).To(Succeed())

			fakeClient = fake.NewClientBuilder().WithScheme(scheme).WithObjects(nodeUSBDevice, usbDevice).Build()

			nodeUSBDeviceResource = reconciler.NewResource(
				types.NamespacedName{Name: nodeUSBDevice.Name},
				fakeClient,
				func() *v1alpha2.NodeUSBDevice { return &v1alpha2.NodeUSBDevice{} },
				func(obj *v1alpha2.NodeUSBDevice) v1alpha2.NodeUSBDeviceStatus { return obj.Status },
			)
			Expect(nodeUSBDeviceResource.Fetch(ctx)).To(Succeed())

			nodeUSBDeviceState = state.New(fakeClient, nodeUSBDeviceResource)
			recorder := &eventrecord.EventRecorderLoggerMock{}
			handler = NewDeletionHandler(fakeClient, recorder)

			result, err := handler.Handle(ctx, nodeUSBDeviceState)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(reconcile.Result{}))

			// Verify USBDevice was deleted
			deletedUSBDevice := &v1alpha2.USBDevice{}
			err = fakeClient.Get(ctx, types.NamespacedName{Name: "usb-device-1", Namespace: "test-namespace"}, deletedUSBDevice)
			Expect(err).To(HaveOccurred())

			// Verify finalizer was removed
			Expect(nodeUSBDeviceResource.Changed().GetFinalizers()).NotTo(ContainElement(v1alpha2.FinalizerNodeUSBDeviceCleanup))
		})

		It("should remove finalizer when no USBDevice exists", func() {
			now := metav1.Now()
			nodeUSBDevice := &v1alpha2.NodeUSBDevice{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "usb-device-1",
					Finalizers:        []string{v1alpha2.FinalizerNodeUSBDeviceCleanup},
					DeletionTimestamp: &now,
				},
				Spec: v1alpha2.NodeUSBDeviceSpec{
					AssignedNamespace: "test-namespace",
				},
			}

			scheme := apiruntime.NewScheme()
			Expect(v1alpha2.AddToScheme(scheme)).To(Succeed())

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
			handler = NewDeletionHandler(fakeClient, recorder)

			result, err := handler.Handle(ctx, nodeUSBDeviceState)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(reconcile.Result{}))

			// Verify finalizer was removed
			Expect(nodeUSBDeviceResource.Changed().GetFinalizers()).NotTo(ContainElement(v1alpha2.FinalizerNodeUSBDeviceCleanup))
		})

		It("should delete USBDevice by OwnerReference even when in different namespace than AssignedNamespace", func() {
			now := metav1.Now()
			nodeUSBDevice := &v1alpha2.NodeUSBDevice{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "usb-device-1",
					UID:               "node-usb-device-uid",
					Finalizers:        []string{v1alpha2.FinalizerNodeUSBDeviceCleanup},
					DeletionTimestamp: &now,
				},
				Spec: v1alpha2.NodeUSBDeviceSpec{
					AssignedNamespace: "other-namespace", // different from where USBDevice actually is
				},
			}
			usbDevice := &v1alpha2.USBDevice{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "usb-device-1",
					Namespace: "previous-namespace", // e.g. spec was changed but AssignedHandler did not run yet
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion: v1alpha2.SchemeGroupVersion.String(),
							Kind:       "NodeUSBDevice",
							Name:       nodeUSBDevice.Name,
							UID:        nodeUSBDevice.UID,
							Controller: ptr.To(true),
						},
					},
				},
			}

			scheme := apiruntime.NewScheme()
			Expect(v1alpha2.AddToScheme(scheme)).To(Succeed())

			fakeClient = fake.NewClientBuilder().WithScheme(scheme).WithObjects(nodeUSBDevice, usbDevice).Build()

			nodeUSBDeviceResource = reconciler.NewResource(
				types.NamespacedName{Name: nodeUSBDevice.Name},
				fakeClient,
				func() *v1alpha2.NodeUSBDevice { return &v1alpha2.NodeUSBDevice{} },
				func(obj *v1alpha2.NodeUSBDevice) v1alpha2.NodeUSBDeviceStatus { return obj.Status },
			)
			Expect(nodeUSBDeviceResource.Fetch(ctx)).To(Succeed())

			nodeUSBDeviceState = state.New(fakeClient, nodeUSBDeviceResource)
			recorder := &eventrecord.EventRecorderLoggerMock{}
			handler = NewDeletionHandler(fakeClient, recorder)

			result, err := handler.Handle(ctx, nodeUSBDeviceState)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(reconcile.Result{}))

			// USBDevice in previous-namespace must be deleted (found by OwnerReference)
			deletedUSBDevice := &v1alpha2.USBDevice{}
			err = fakeClient.Get(ctx, types.NamespacedName{Name: "usb-device-1", Namespace: "previous-namespace"}, deletedUSBDevice)
			Expect(err).To(HaveOccurred())

			Expect(nodeUSBDeviceResource.Changed().GetFinalizers()).NotTo(ContainElement(v1alpha2.FinalizerNodeUSBDeviceCleanup))
		})
	})
})
