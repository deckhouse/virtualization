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

	"github.com/deckhouse/virtualization-controller/pkg/controller/nodeusbdevice/internal/state"
	"github.com/deckhouse/virtualization-controller/pkg/controller/reconciler"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

var _ = Describe("DeletionHandler", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = logger.ToContext(context.TODO(), slog.Default())
	})

	DescribeTable("Handle",
		func(deleting, withOwnedUSB bool, usbNamespace string, expectFinalizerPresent, expectOwnedUSBDeleted bool) {
			scheme := apiruntime.NewScheme()
			Expect(v1alpha2.AddToScheme(scheme)).To(Succeed())

			node := &v1alpha2.NodeUSBDevice{ObjectMeta: metav1.ObjectMeta{Name: "usb-device-1", UID: "node-usb-uid"}}
			if deleting {
				now := metav1.Now()
				node.DeletionTimestamp = &now
				node.Finalizers = []string{v1alpha2.FinalizerNodeUSBDeviceCleanup}
			}

			objects := []client.Object{node}
			if withOwnedUSB {
				objects = append(objects, &v1alpha2.USBDevice{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "usb-device-1",
						Namespace: usbNamespace,
						OwnerReferences: []metav1.OwnerReference{{
							APIVersion: v1alpha2.SchemeGroupVersion.String(),
							Kind:       "NodeUSBDevice",
							Name:       node.Name,
							UID:        node.UID,
							Controller: ptr.To(true),
						}},
					},
				})
			}

			cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(objects...).Build()
			res := reconciler.NewResource(
				types.NamespacedName{Name: node.Name},
				cl,
				func() *v1alpha2.NodeUSBDevice { return &v1alpha2.NodeUSBDevice{} },
				func(obj *v1alpha2.NodeUSBDevice) v1alpha2.NodeUSBDeviceStatus { return obj.Status },
			)
			Expect(res.Fetch(ctx)).To(Succeed())

			h := NewDeletionHandler(cl)
			st := state.New(cl, res)
			_, err := h.Handle(ctx, st)
			Expect(err).NotTo(HaveOccurred())

			if expectFinalizerPresent {
				Expect(res.Changed().GetFinalizers()).To(ContainElement(v1alpha2.FinalizerNodeUSBDeviceCleanup))
			} else {
				Expect(res.Changed().GetFinalizers()).NotTo(ContainElement(v1alpha2.FinalizerNodeUSBDeviceCleanup))
			}

			if withOwnedUSB {
				usb := &v1alpha2.USBDevice{}
				err = cl.Get(ctx, types.NamespacedName{Name: "usb-device-1", Namespace: usbNamespace}, usb)
				if expectOwnedUSBDeleted {
					Expect(err).To(HaveOccurred())
				} else {
					Expect(err).NotTo(HaveOccurred())
				}
			}
		},
		Entry("not deleting adds finalizer", false, false, "", true, false),
		Entry("deleting removes finalizer and owned USB", true, true, "test-namespace", false, true),
		Entry("deleting removes finalizer even without owned USB", true, false, "", false, false),
		Entry("deleting removes owned USB in different namespace", true, true, "previous-namespace", false, true),
	)
})
