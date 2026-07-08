/*
Copyright 2024 Flant JSC

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

package validators_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/deckhouse/virtualization-controller/pkg/common/testutil"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vm/internal/validators"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

var _ = Describe("BlockDeviceSpecRefsValidator", func() {
	var validator *validators.BlockDeviceSpecRefsValidator

	BeforeEach(func() {
		fakeClient, err := testutil.NewFakeClientWithObjects()
		Expect(err).NotTo(HaveOccurred())
		validator = validators.NewBlockDeviceSpecRefsValidator(fakeClient)
	})

	DescribeTable("ValidateCreate with valid refs", func(refs []v1alpha2.BlockDeviceSpecRef) {
		vm := &v1alpha2.VirtualMachine{
			Spec: v1alpha2.VirtualMachineSpec{
				BlockDeviceRefs: refs,
			},
		}
		_, err := validator.ValidateCreate(context.Background(), vm)
		Expect(err).NotTo(HaveOccurred())
	},
		Entry("Single valid VirtualDisk", []v1alpha2.BlockDeviceSpecRef{
			{Kind: v1alpha2.DiskDevice, Name: "valid-disk"},
		}),
		Entry("Single valid VirtualImage", []v1alpha2.BlockDeviceSpecRef{
			{Kind: v1alpha2.ImageDevice, Name: "valid-image"},
		}),
		Entry("Single valid ClusterVirtualImage", []v1alpha2.BlockDeviceSpecRef{
			{Kind: v1alpha2.ClusterImageDevice, Name: "valid-cvi"},
		}),
		Entry("Multiple different kinds", []v1alpha2.BlockDeviceSpecRef{
			{Kind: v1alpha2.DiskDevice, Name: "disk1"},
			{Kind: v1alpha2.ImageDevice, Name: "image1"},
			{Kind: v1alpha2.ClusterImageDevice, Name: "cvi1"},
		}),
	)

	DescribeTable("ValidateCreate with duplicates", func(refs []v1alpha2.BlockDeviceSpecRef) {
		vm := &v1alpha2.VirtualMachine{
			Spec: v1alpha2.VirtualMachineSpec{
				BlockDeviceRefs: refs,
			},
		}
		_, err := validator.ValidateCreate(context.Background(), vm)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("duplicate reference"))
	},
		Entry("Duplicate VirtualDisk", []v1alpha2.BlockDeviceSpecRef{
			{Kind: v1alpha2.DiskDevice, Name: "disk1"},
			{Kind: v1alpha2.DiskDevice, Name: "disk1"},
		}),
		Entry("Duplicate VirtualImage", []v1alpha2.BlockDeviceSpecRef{
			{Kind: v1alpha2.ImageDevice, Name: "image1"},
			{Kind: v1alpha2.ImageDevice, Name: "image1"},
		}),
	)
})
