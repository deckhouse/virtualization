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

	"github.com/deckhouse/virtualization-controller/pkg/controller/reconciler"
	"github.com/deckhouse/virtualization-controller/pkg/controller/usbdevice/internal/state"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

var _ = Describe("ResourceClaimTemplateHandler", func() {
	var ctx context.Context
	var fakeClient client.WithWatch
	var scheme *apiruntime.Scheme
	var handler *ResourceClaimTemplateHandler
	var usbDeviceState state.USBDeviceState
	var usbDeviceResource *reconciler.Resource[*v1alpha2.USBDevice, v1alpha2.USBDeviceStatus]

	BeforeEach(func() {
		ctx = logger.ToContext(context.TODO(), slog.Default())
		scheme = apiruntime.NewScheme()
		Expect(v1alpha2.AddToScheme(scheme)).To(Succeed())
		Expect(resourcev1beta1.AddToScheme(scheme)).To(Succeed())
	})

	It("should create ResourceClaimTemplate when USBDevice has attributes name", func() {
		usbDevice := &v1alpha2.USBDevice{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "usb-device-1",
				Namespace: "default",
				UID:       "usb-uid-1",
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

		fakeClient = fake.NewClientBuilder().WithScheme(scheme).WithObjects(usbDevice).Build()
		usbDeviceResource = reconciler.NewResource(
			types.NamespacedName{Name: usbDevice.Name, Namespace: usbDevice.Namespace},
			fakeClient,
			func() *v1alpha2.USBDevice { return &v1alpha2.USBDevice{} },
			func(obj *v1alpha2.USBDevice) v1alpha2.USBDeviceStatus { return obj.Status },
		)
		Expect(usbDeviceResource.Fetch(ctx)).To(Succeed())
		usbDeviceState = state.New(fakeClient, usbDeviceResource)
		handler = NewResourceClaimTemplateHandler(fakeClient, scheme)

		result, err := handler.Handle(ctx, usbDeviceState)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(Equal(reconcile.Result{}))

		template := &resourcev1beta1.ResourceClaimTemplate{}
		err = fakeClient.Get(ctx, types.NamespacedName{Name: "usb-device-1-template", Namespace: "default"}, template)
		Expect(err).NotTo(HaveOccurred())
		Expect(template.OwnerReferences).To(HaveLen(1))
		Expect(template.OwnerReferences[0].Kind).To(Equal(v1alpha2.USBDeviceKind))
		Expect(template.OwnerReferences[0].Name).To(Equal("usb-device-1"))
		Expect(template.Spec.Spec.Devices.Requests).To(HaveLen(1))
		Expect(template.Spec.Spec.Devices.Requests[0].Name).To(Equal("req-usb-device-1"))
		Expect(template.Spec.Spec.Devices.Requests[0].DeviceClassName).To(Equal("usb-devices.virtualization.deckhouse.io"))
	})

	It("should skip when USBDevice has no attributes name", func() {
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

		fakeClient = fake.NewClientBuilder().WithScheme(scheme).WithObjects(usbDevice).Build()
		usbDeviceResource = reconciler.NewResource(
			types.NamespacedName{Name: usbDevice.Name, Namespace: usbDevice.Namespace},
			fakeClient,
			func() *v1alpha2.USBDevice { return &v1alpha2.USBDevice{} },
			func(obj *v1alpha2.USBDevice) v1alpha2.USBDeviceStatus { return obj.Status },
		)
		Expect(usbDeviceResource.Fetch(ctx)).To(Succeed())
		usbDeviceState = state.New(fakeClient, usbDeviceResource)
		handler = NewResourceClaimTemplateHandler(fakeClient, scheme)

		result, err := handler.Handle(ctx, usbDeviceState)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(Equal(reconcile.Result{}))

		template := &resourcev1beta1.ResourceClaimTemplate{}
		err = fakeClient.Get(ctx, types.NamespacedName{Name: "usb-device-1-template", Namespace: "default"}, template)
		Expect(err).To(HaveOccurred())
	})

	It("should not create duplicate when ResourceClaimTemplate already exists", func() {
		usbDevice := &v1alpha2.USBDevice{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "usb-device-1",
				Namespace: "default",
				UID:       "usb-uid-1",
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

		existingTemplate := &resourcev1beta1.ResourceClaimTemplate{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "usb-device-1-template",
				Namespace: "default",
			},
		}

		fakeClient = fake.NewClientBuilder().WithScheme(scheme).WithObjects(usbDevice, existingTemplate).Build()
		usbDeviceResource = reconciler.NewResource(
			types.NamespacedName{Name: usbDevice.Name, Namespace: usbDevice.Namespace},
			fakeClient,
			func() *v1alpha2.USBDevice { return &v1alpha2.USBDevice{} },
			func(obj *v1alpha2.USBDevice) v1alpha2.USBDeviceStatus { return obj.Status },
		)
		Expect(usbDeviceResource.Fetch(ctx)).To(Succeed())
		usbDeviceState = state.New(fakeClient, usbDeviceResource)
		handler = NewResourceClaimTemplateHandler(fakeClient, scheme)

		result, err := handler.Handle(ctx, usbDeviceState)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(Equal(reconcile.Result{}))

		// Template still exists and was not recreated (immutable - no update)
		templateList := &resourcev1beta1.ResourceClaimTemplateList{}
		Expect(fakeClient.List(ctx, templateList, client.InNamespace("default"))).To(Succeed())
		Expect(templateList.Items).To(HaveLen(1))
		Expect(templateList.Items[0].Name).To(Equal("usb-device-1-template"))
	})

	It("should return early when USBDevice is empty", func() {
		fakeClient = fake.NewClientBuilder().WithScheme(scheme).Build()
		usbDeviceResource = reconciler.NewResource(
			types.NamespacedName{Name: "missing", Namespace: "default"},
			fakeClient,
			func() *v1alpha2.USBDevice { return &v1alpha2.USBDevice{} },
			func(obj *v1alpha2.USBDevice) v1alpha2.USBDeviceStatus { return obj.Status },
		)
		Expect(usbDeviceResource.Fetch(ctx)).To(Succeed())
		usbDeviceState = state.New(fakeClient, usbDeviceResource)
		handler = NewResourceClaimTemplateHandler(fakeClient, scheme)

		result, err := handler.Handle(ctx, usbDeviceState)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(Equal(reconcile.Result{}))
	})
})
