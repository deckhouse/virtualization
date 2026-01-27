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
	"crypto/sha256"
	"encoding/hex"
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

	"github.com/deckhouse/virtualization-controller/pkg/controller/nodeusbdevice/internal/state"
	"github.com/deckhouse/virtualization-controller/pkg/controller/reconciler"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/nodeusbdevicecondition"
)

var _ = Describe("ReadyHandler", func() {
	var ctx context.Context
	var fakeClient client.WithWatch
	var handler *ReadyHandler
	var nodeUSBDeviceState state.NodeUSBDeviceState
	var nodeUSBDeviceResource *reconciler.Resource[*v1alpha2.NodeUSBDevice, v1alpha2.NodeUSBDeviceStatus]

	BeforeEach(func() {
		ctx = logger.ToContext(context.TODO(), slog.Default())
	})

	Context("when device is found in ResourceSlice", func() {
		It("should set Ready condition", func() {
			// Create ResourceSlice with device attributes
			resourceSlice := &resourcev1beta1.ResourceSlice{
				ObjectMeta: metav1.ObjectMeta{
					Name: "slice-1",
				},
				Spec: resourcev1beta1.ResourceSliceSpec{
					Driver: "virtualization-dra",
					Pool: resourcev1beta1.ResourcePool{
						Name: "node-1",
					},
					Devices: []resourcev1beta1.Device{
						{
							Name: "usb-device-1",
							Basic: &resourcev1beta1.BasicDevice{
								Attributes: map[resourcev1beta1.QualifiedName]resourcev1beta1.DeviceAttribute{
									resourcev1beta1.QualifiedName("vendorID"): {
										StringValue: stringPtr("1234"),
									},
									resourcev1beta1.QualifiedName("productID"): {
										StringValue: stringPtr("5678"),
									},
									resourcev1beta1.QualifiedName("bus"): {
										StringValue: stringPtr("1"),
									},
									resourcev1beta1.QualifiedName("deviceNumber"): {
										StringValue: stringPtr("2"),
									},
								},
							},
						},
					},
				},
			}

			// Calculate hash from device attributes to match what the handler expects
			// Hash is calculated as: nodeName:vendorID:productID:bus:deviceNumber:serial:devicePath
			// Using the same values as in ResourceSlice (serial and devicePath are empty)
			hashInput := "node-1:1234:5678:1:2::"
			hash := calculateTestHash(hashInput)

			// Verify hash calculation matches handler logic
			// The handler will calculate hash from ResourceSlice device attributes

			nodeUSBDevice := &v1alpha2.NodeUSBDevice{
				ObjectMeta: metav1.ObjectMeta{
					Name: "usb-device-1",
				},
				Status: v1alpha2.NodeUSBDeviceStatus{
					Attributes: v1alpha2.NodeUSBDeviceAttributes{
						Hash:         hash,
						VendorID:     "1234",
						ProductID:    "5678",
						Bus:          "1",
						DeviceNumber: "2",
						NodeName:     "node-1",
					},
					NodeName: "node-1",
				},
			}

			scheme := apiruntime.NewScheme()
			Expect(v1alpha2.AddToScheme(scheme)).To(Succeed())
			Expect(resourcev1beta1.AddToScheme(scheme)).To(Succeed())

			fakeClient = fake.NewClientBuilder().WithScheme(scheme).WithObjects(nodeUSBDevice, resourceSlice).Build()

			nodeUSBDeviceResource = reconciler.NewResource(
				types.NamespacedName{Name: nodeUSBDevice.Name},
				fakeClient,
				func() *v1alpha2.NodeUSBDevice { return &v1alpha2.NodeUSBDevice{} },
				func(obj *v1alpha2.NodeUSBDevice) v1alpha2.NodeUSBDeviceStatus { return obj.Status },
			)
			Expect(nodeUSBDeviceResource.Fetch(ctx)).To(Succeed())

			nodeUSBDeviceState = state.New(fakeClient, nodeUSBDeviceResource)
			recorder := &eventrecord.EventRecorderLoggerMock{}
			handler = NewReadyHandler(recorder)

			result, err := handler.Handle(ctx, nodeUSBDeviceState)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(reconcile.Result{}))

			// Verify Ready condition was set
			conditions := nodeUSBDeviceResource.Changed().Status.Conditions
			Expect(conditions).To(HaveLen(1))
			Expect(conditions[0].Type).To(Equal(string(nodeusbdevicecondition.ReadyType)))
			Expect(conditions[0].Status).To(Equal(metav1.ConditionTrue))
			Expect(conditions[0].Reason).To(Equal(string(nodeusbdevicecondition.Ready)))
		})
	})

	Context("when device is not found in ResourceSlice", func() {
		It("should set NotFound condition", func() {
			nodeUSBDevice := &v1alpha2.NodeUSBDevice{
				ObjectMeta: metav1.ObjectMeta{
					Name: "usb-device-1",
				},
				Status: v1alpha2.NodeUSBDeviceStatus{
					Attributes: v1alpha2.NodeUSBDeviceAttributes{
						Hash:     "non-existent-hash",
						VendorID: "1234",
						NodeName: "node-1",
					},
					NodeName: "node-1",
				},
			}

			scheme := apiruntime.NewScheme()
			Expect(v1alpha2.AddToScheme(scheme)).To(Succeed())
			Expect(resourcev1beta1.AddToScheme(scheme)).To(Succeed())

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
			handler = NewReadyHandler(recorder)

			result, err := handler.Handle(ctx, nodeUSBDeviceState)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(reconcile.Result{}))

			// Verify NotFound condition was set
			conditions := nodeUSBDeviceResource.Changed().Status.Conditions
			Expect(conditions).To(HaveLen(1))
			Expect(conditions[0].Type).To(Equal(string(nodeusbdevicecondition.ReadyType)))
			Expect(conditions[0].Status).To(Equal(metav1.ConditionFalse))
			Expect(conditions[0].Reason).To(Equal(string(nodeusbdevicecondition.NotFound)))
		})
	})
})

func stringPtr(s string) *string {
	return &s
}

func calculateTestHash(input string) string {
	// This matches the hash calculation in ready.go:calculateDeviceHash
	// Hash is calculated as SHA256 and first 16 characters are used
	hash := sha256.Sum256([]byte(input))
	return hex.EncodeToString(hash[:])[:16]
}
