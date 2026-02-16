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

package blockdevice

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	vdbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vd"
	vibuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vi"
	vmbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vm"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	"github.com/deckhouse/virtualization/test/e2e/internal/object"
	"github.com/deckhouse/virtualization/test/e2e/internal/util"
)

var _ = Describe("VirtualDiskProvisioning", func() {
	f := framework.NewFramework("vd-provisioning")

	BeforeEach(func() {
		f.Before()
		DeferCleanup(f.After)
	})

	// Other cases are currently covered in ComplexTest
	// After splitting ComplexTest, the remaining disk provisioning checks will also be located here
	It("verifies that a VirtualDisk is provisioned successfully from a VirtualImage on a PVC", func() {
		var (
			vi *v1alpha2.VirtualImage
			vd *v1alpha2.VirtualDisk
			vm *v1alpha2.VirtualMachine
		)

		By("Creating VirtualImage", func() {
			vi = vibuilder.New(
				vibuilder.WithName("vi"),
				vibuilder.WithNamespace(f.Namespace().Name),
				vibuilder.WithDataSourceHTTP(object.ImageURLAlpineUEFIPerf, nil, nil),
				vibuilder.WithStorage(v1alpha2.StoragePersistentVolumeClaim),
			)

			err := f.CreateWithDeferredDeletion(context.Background(), vi)
			Expect(err).NotTo(HaveOccurred())
		})

		By("Waiting for VirtualImage to be ready", func() {
			util.UntilObjectPhase(string(v1alpha2.ImageReady), framework.LongTimeout, vi)
		})

		By("Creating VirtualDisk", func() {
			vd = vdbuilder.New(
				vdbuilder.WithName("vd"),
				vdbuilder.WithNamespace(f.Namespace().Name),
				vdbuilder.WithDataSourceObjectRefFromVI(vi),
			)

			err := f.CreateWithDeferredDeletion(context.Background(), vd)
			Expect(err).NotTo(HaveOccurred())
		})

		By("Creating VirtualMachine", func() {
			vm = object.NewMinimalVM("vm-", f.Namespace().Name, vmbuilder.WithBlockDeviceRefs(
				v1alpha2.BlockDeviceSpecRef{
					Kind: v1alpha2.VirtualDiskKind,
					Name: vd.Name,
				},
			))

			err := f.CreateWithDeferredDeletion(context.Background(), vm)
			Expect(err).NotTo(HaveOccurred())
		})

		By("Waiting for VirtualDisk to be ready", func() {
			util.UntilObjectPhase(string(v1alpha2.DiskReady), framework.LongTimeout, vd)
		})
	})
})
