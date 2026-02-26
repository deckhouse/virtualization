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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	resourcev1 "k8s.io/api/resource/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apiruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/deckhouse/virtualization-controller/pkg/controller/indexer"
	"github.com/deckhouse/virtualization-controller/pkg/controller/nodeusbdevice/internal/state"
	"github.com/deckhouse/virtualization-controller/pkg/controller/reconciler"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/nodeusbdevicecondition"
)

var _ = Describe("ReadyHandler", func() {
	DescribeTable("Handle",
		func(nodeName, attrName string, slices []client.Object, expectedReason string, expectedStatus metav1.ConditionStatus) {
			scheme := apiruntime.NewScheme()
			Expect(v1alpha2.AddToScheme(scheme)).To(Succeed())
			Expect(resourcev1.AddToScheme(scheme)).To(Succeed())

			node := &v1alpha2.NodeUSBDevice{
				ObjectMeta: metav1.ObjectMeta{Name: "usb-device-1", Generation: 1},
				Status: v1alpha2.NodeUSBDeviceStatus{
					Attributes: v1alpha2.NodeUSBDeviceAttributes{Name: attrName},
					NodeName:   nodeName,
				},
			}

			objects := append([]client.Object{node}, slices...)
			cl := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(objects...).
				WithIndex(&resourcev1.ResourceSlice{}, indexer.IndexFieldResourceSliceByPoolName, func(object client.Object) []string {
					resourceSlice, ok := object.(*resourcev1.ResourceSlice)
					if !ok || resourceSlice == nil || resourceSlice.Spec.Pool.Name == "" {
						return nil
					}

					return []string{resourceSlice.Spec.Pool.Name}
				}).
				WithIndex(&resourcev1.ResourceSlice{}, indexer.IndexFieldResourceSliceByDriver, func(object client.Object) []string {
					resourceSlice, ok := object.(*resourcev1.ResourceSlice)
					if !ok || resourceSlice == nil || resourceSlice.Spec.Driver == "" {
						return nil
					}

					return []string{resourceSlice.Spec.Driver}
				}).
				Build()

			res := reconciler.NewResource(
				types.NamespacedName{Name: node.Name},
				cl,
				func() *v1alpha2.NodeUSBDevice { return &v1alpha2.NodeUSBDevice{} },
				func(obj *v1alpha2.NodeUSBDevice) v1alpha2.NodeUSBDeviceStatus { return obj.Status },
			)
			Expect(res.Fetch(context.Background())).To(Succeed())

			h := NewReadyHandler(cl)
			st := state.New(cl, res)
			_, err := h.Handle(context.Background(), st)
			Expect(err).NotTo(HaveOccurred())

			readyCondition := meta.FindStatusCondition(res.Changed().Status.Conditions, string(nodeusbdevicecondition.ReadyType))
			Expect(readyCondition).NotTo(BeNil())
			Expect(readyCondition.Reason).To(Equal(expectedReason))
			Expect(readyCondition.Status).To(Equal(expectedStatus))
		},
		Entry("device found", "node-a", "usb-device-1", []client.Object{&resourcev1.ResourceSlice{
			ObjectMeta: metav1.ObjectMeta{Name: "slice-1"},
			Spec: resourcev1.ResourceSliceSpec{
				Driver: "virtualization-usb",
				Pool:   resourcev1.ResourcePool{Name: "node-a"},
				Devices: []resourcev1.Device{{
					Name: "usb-device-1",
				}},
			},
		}}, string(nodeusbdevicecondition.Ready), metav1.ConditionTrue),
		Entry("device not found", "node-a", "usb-device-1", nil, string(nodeusbdevicecondition.NotFound), metav1.ConditionFalse),
		Entry("empty nodeName returns not found", "", "usb-device-1", []client.Object{&resourcev1.ResourceSlice{
			ObjectMeta: metav1.ObjectMeta{Name: "slice-1"},
			Spec: resourcev1.ResourceSliceSpec{
				Driver: "virtualization-usb",
				Pool:   resourcev1.ResourcePool{Name: "node-a"},
				Devices: []resourcev1.Device{{
					Name: "usb-device-1",
				}},
			},
		}}, string(nodeusbdevicecondition.NotFound), metav1.ConditionFalse),
		Entry("uses attribute name for matching", "node-a", "usb-attribute-name", []client.Object{&resourcev1.ResourceSlice{
			ObjectMeta: metav1.ObjectMeta{Name: "slice-1"},
			Spec: resourcev1.ResourceSliceSpec{
				Driver: "virtualization-usb",
				Pool:   resourcev1.ResourcePool{Name: "node-a"},
				Devices: []resourcev1.Device{{
					Name: "usb-attribute-name",
				}},
			},
		}}, string(nodeusbdevicecondition.Ready), metav1.ConditionTrue),
		Entry("falls back to resource name when attribute name is empty", "node-a", "", []client.Object{&resourcev1.ResourceSlice{
			ObjectMeta: metav1.ObjectMeta{Name: "slice-1"},
			Spec: resourcev1.ResourceSliceSpec{
				Driver: "virtualization-usb",
				Pool:   resourcev1.ResourcePool{Name: "node-a"},
				Devices: []resourcev1.Device{{
					Name: "usb-device-1",
				}},
			},
		}}, string(nodeusbdevicecondition.Ready), metav1.ConditionTrue),
	)
})
