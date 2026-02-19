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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	resourcev1 "k8s.io/api/resource/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apiruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	resourcest "github.com/deckhouse/virtualization-controller/pkg/controller/resourceslice/internal/state"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type fakeDiscoveryState struct {
	resourceSlice *resourcev1.ResourceSlice
	slices        []resourcev1.ResourceSlice
	slicesErr     error
}

func (f *fakeDiscoveryState) ResourceSlice() *resourcev1.ResourceSlice {
	return f.resourceSlice
}

func (f *fakeDiscoveryState) ResourceSlices(_ context.Context) ([]resourcev1.ResourceSlice, error) {
	return f.slices, f.slicesErr
}

var _ = Describe("DiscoveryHandler", func() {
	DescribeTable("Handle",
		func(existingName, expectVendor string) {
			scheme := apiruntime.NewScheme()
			Expect(v1alpha2.AddToScheme(scheme)).To(Succeed())
			Expect(resourcev1.AddToScheme(scheme)).To(Succeed())

			builder := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&v1alpha2.NodeUSBDevice{})
			if existingName != "" {
				builder = builder.WithObjects(&v1alpha2.NodeUSBDevice{
					ObjectMeta: metav1.ObjectMeta{Name: existingName},
					Status:     v1alpha2.NodeUSBDeviceStatus{Attributes: v1alpha2.NodeUSBDeviceAttributes{Name: existingName}},
				})
			}

			cl := builder.Build()
			h := NewDiscoveryHandler(cl)
			st := &fakeDiscoveryState{resourceSlice: &resourcev1.ResourceSlice{
				Spec: resourcev1.ResourceSliceSpec{
					Driver: "virtualization-usb",
					Pool:   resourcev1.ResourcePool{Name: "node-a"},
					Devices: []resourcev1.Device{{
						Name: "usb-device-1",
						Attributes: map[resourcev1.QualifiedName]resourcev1.DeviceAttribute{
							"name":     {StringValue: ptrString("usb-device-1")},
							"vendorID": {StringValue: ptrString("1234")},
						},
					}},
				},
			}}

			_, err := h.Handle(context.Background(), st)
			Expect(err).NotTo(HaveOccurred())

			created := &v1alpha2.NodeUSBDevice{}
			Expect(cl.Get(context.Background(), types.NamespacedName{Name: "usb-device-1"}, created)).To(Succeed())
			if expectVendor != "" {
				Expect(created.Status.Attributes.VendorID).To(Equal(expectVendor))
			}
		},
		Entry("creates missing device", "", "1234"),
		Entry("syncs existing device attributes", "usb-device-1", "1234"),
	)

	It("should skip when current ResourceSlice is absent", func() {
		scheme := apiruntime.NewScheme()
		Expect(v1alpha2.AddToScheme(scheme)).To(Succeed())
		Expect(resourcev1.AddToScheme(scheme)).To(Succeed())

		cl := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&v1alpha2.NodeUSBDevice{}).Build()
		h := NewDiscoveryHandler(cl)
		st := &fakeDiscoveryState{}

		_, err := h.Handle(context.Background(), st)
		Expect(err).NotTo(HaveOccurred())

		created := &v1alpha2.NodeUSBDevice{}
		err = cl.Get(context.Background(), types.NamespacedName{Name: "usb-device-1"}, created)
		Expect(err).To(HaveOccurred())
	})
})

var _ resourcest.ResourceSliceState = (*fakeDiscoveryState)(nil)
