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

package handler

import (
	"context"
	"log/slog"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apiruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/deckhouse/virtualization-controller/pkg/controller/indexer"
	"github.com/deckhouse/virtualization-controller/pkg/controller/nodeusbdevice/internal/state"
	"github.com/deckhouse/virtualization-controller/pkg/controller/reconciler"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/nodeusbdevicecondition"
)

var _ = Describe("AssignedHandler", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = logger.ToContext(context.TODO(), slog.Default())
	})

	DescribeTable("Handle",
		func(assignedNamespace string, namespaceExists, readyNotFound, seedUSB bool, expectReason string, expectUSBInAssignedNS bool) {
			scheme := apiruntime.NewScheme()
			Expect(v1alpha2.AddToScheme(scheme)).To(Succeed())
			Expect(corev1.AddToScheme(scheme)).To(Succeed())

			node := &v1alpha2.NodeUSBDevice{
				ObjectMeta: metav1.ObjectMeta{Name: "usb-device-1", UID: types.UID("node-usb-uid")},
				Spec:       v1alpha2.NodeUSBDeviceSpec{AssignedNamespace: assignedNamespace},
				Status: v1alpha2.NodeUSBDeviceStatus{
					Attributes: v1alpha2.NodeUSBDeviceAttributes{VendorID: "1234", ProductID: "5678"},
					NodeName:   "node-1",
				},
			}
			if readyNotFound {
				node.Status.Conditions = []metav1.Condition{{
					Type:   string(nodeusbdevicecondition.ReadyType),
					Status: metav1.ConditionFalse,
					Reason: string(nodeusbdevicecondition.NotFound),
				}}
			}

			objects := []client.Object{node}
			if assignedNamespace != "" && namespaceExists {
				objects = append(objects, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: assignedNamespace}})
			}
			if seedUSB {
				seedNS := assignedNamespace
				if seedNS == "" {
					seedNS = "stale-namespace"
				}
				objects = append(objects, &v1alpha2.USBDevice{ObjectMeta: metav1.ObjectMeta{Name: "usb-device-1", Namespace: seedNS}})
			}

			cl := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(objects...).
				WithStatusSubresource(&v1alpha2.USBDevice{}).
				WithIndex(&v1alpha2.USBDevice{}, indexer.IndexFieldUSBDeviceByName, func(object client.Object) []string {
					usbDevice, ok := object.(*v1alpha2.USBDevice)
					if !ok || usbDevice == nil {
						return nil
					}
					return []string{usbDevice.Name}
				}).
				Build()
			res := reconciler.NewResource(
				types.NamespacedName{Name: node.Name},
				cl,
				func() *v1alpha2.NodeUSBDevice { return &v1alpha2.NodeUSBDevice{} },
				func(obj *v1alpha2.NodeUSBDevice) v1alpha2.NodeUSBDeviceStatus { return obj.Status },
			)
			Expect(res.Fetch(ctx)).To(Succeed())

			h := NewAssignedHandler(cl)
			st := state.New(cl, res)
			_, err := h.Handle(ctx, st)
			Expect(err).NotTo(HaveOccurred())

			assigned := meta.FindStatusCondition(res.Changed().Status.Conditions, string(nodeusbdevicecondition.AssignedType))
			Expect(assigned).NotTo(BeNil())
			Expect(assigned.Reason).To(Equal(expectReason))

			if assignedNamespace != "" {
				usb := &v1alpha2.USBDevice{}
				err = cl.Get(ctx, types.NamespacedName{Name: "usb-device-1", Namespace: assignedNamespace}, usb)
				if expectUSBInAssignedNS {
					Expect(err).NotTo(HaveOccurred())
				} else {
					Expect(err).To(HaveOccurred())
				}
			}
		},
		Entry("assigned namespace exists creates/keeps USBDevice", "test-namespace", true, false, false, string(nodeusbdevicecondition.Assigned), true),
		Entry("assigned namespace missing marks available", "missing-namespace", false, false, false, string(nodeusbdevicecondition.Available), false),
		Entry("device absent on host removes USBDevice", "test-namespace", true, true, true, string(nodeusbdevicecondition.InProgress), false),
		Entry("unassigned removes stale USBDevice", "", false, false, true, string(nodeusbdevicecondition.Available), false),
	)

	It("should update existing USBDevice when attributes change", func() {
		scheme := apiruntime.NewScheme()
		Expect(v1alpha2.AddToScheme(scheme)).To(Succeed())
		Expect(corev1.AddToScheme(scheme)).To(Succeed())

		node := &v1alpha2.NodeUSBDevice{
			ObjectMeta: metav1.ObjectMeta{Name: "usb-device-1", UID: types.UID("node-usb-uid")},
			Spec:       v1alpha2.NodeUSBDeviceSpec{AssignedNamespace: "test-ns"},
			Status: v1alpha2.NodeUSBDeviceStatus{
				Attributes: v1alpha2.NodeUSBDeviceAttributes{VendorID: "5678", ProductID: "1234", Name: "updated-name"},
				NodeName:   "node-1",
			},
		}

		existingUSB := &v1alpha2.USBDevice{
			ObjectMeta: metav1.ObjectMeta{Name: "usb-device-1", Namespace: "test-ns"},
			Status:     v1alpha2.USBDeviceStatus{Attributes: v1alpha2.NodeUSBDeviceAttributes{VendorID: "1111", ProductID: "2222"}},
		}

		objects := []client.Object{node, existingUSB, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "test-ns"}}}

		cl := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(objects...).
			WithStatusSubresource(&v1alpha2.USBDevice{}).
			WithIndex(&v1alpha2.USBDevice{}, indexer.IndexFieldUSBDeviceByName, func(object client.Object) []string {
				usbDevice, ok := object.(*v1alpha2.USBDevice)
				if !ok || usbDevice == nil {
					return nil
				}
				return []string{usbDevice.Name}
			}).
			Build()

		res := reconciler.NewResource(
			types.NamespacedName{Name: node.Name},
			cl,
			func() *v1alpha2.NodeUSBDevice { return &v1alpha2.NodeUSBDevice{} },
			func(obj *v1alpha2.NodeUSBDevice) v1alpha2.NodeUSBDeviceStatus { return obj.Status },
		)
		Expect(res.Fetch(ctx)).To(Succeed())

		h := NewAssignedHandler(cl)
		st := state.New(cl, res)
		_, err := h.Handle(ctx, st)
		Expect(err).NotTo(HaveOccurred())

		updatedUSB := &v1alpha2.USBDevice{}
		err = cl.Get(ctx, types.NamespacedName{Name: "usb-device-1", Namespace: "test-ns"}, updatedUSB)
		Expect(err).NotTo(HaveOccurred())
		Expect(updatedUSB.Status.Attributes.VendorID).To(Equal("5678"))
		Expect(updatedUSB.Status.Attributes.ProductID).To(Equal("1234"))
	})

	It("should skip processing when NodeUSBDevice is being deleted", func() {
		scheme := apiruntime.NewScheme()
		Expect(v1alpha2.AddToScheme(scheme)).To(Succeed())

		now := metav1.Now()
		node := &v1alpha2.NodeUSBDevice{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "usb-device-1",
				UID:               types.UID("node-usb-uid"),
				DeletionTimestamp: &now,
				Finalizers:        []string{v1alpha2.FinalizerNodeUSBDeviceCleanup},
			},
			Spec: v1alpha2.NodeUSBDeviceSpec{AssignedNamespace: "test-ns"},
			Status: v1alpha2.NodeUSBDeviceStatus{
				Attributes: v1alpha2.NodeUSBDeviceAttributes{VendorID: "1234", ProductID: "5678"},
				NodeName:   "node-1",
			},
		}

		cl := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(node).
			Build()

		res := reconciler.NewResource(
			types.NamespacedName{Name: node.Name},
			cl,
			func() *v1alpha2.NodeUSBDevice { return &v1alpha2.NodeUSBDevice{} },
			func(obj *v1alpha2.NodeUSBDevice) v1alpha2.NodeUSBDeviceStatus { return obj.Status },
		)
		Expect(res.Fetch(ctx)).To(Succeed())

		h := NewAssignedHandler(cl)
		st := state.New(cl, res)
		_, err := h.Handle(ctx, st)
		Expect(err).NotTo(HaveOccurred())

		// No conditions should be set when deleting
		Expect(res.Changed().Status.Conditions).To(BeEmpty())
	})
})
