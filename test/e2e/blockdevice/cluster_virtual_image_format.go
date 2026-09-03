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
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/utils/ptr"

	cvibuilder "github.com/deckhouse/virtualization-controller/pkg/builder/cvi"
	vdbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vd"
	vmbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vm"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	"github.com/deckhouse/virtualization/test/e2e/internal/label"
	"github.com/deckhouse/virtualization/test/e2e/internal/object"
	vmobs "github.com/deckhouse/virtualization/test/e2e/internal/observer/vm"
	"github.com/deckhouse/virtualization/test/e2e/internal/precheck"
)

// ClusterVirtualImageFormat verifies how image formats are handled when the source is an
// HTTP data source:
//   - an ISO ClusterVirtualImage boots a VirtualMachine directly (as a CD-ROM);
//   - a qcow2 ClusterVirtualImage backs a VirtualDisk, and a VirtualMachine boots from
//     that disk.
var _ = Describe("ClusterVirtualImageFormat", Label(label.SIGStorage, precheck.PrecheckDefaultStorageClass), func() {
	var (
		f   *framework.Framework
		ctx context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()
		f = framework.NewFramework("")
		f.Before()
		DeferCleanup(f.After)
		setupProject(ctx, f, "cvi-format")
	})

	It("boots a VirtualMachine from an iso ClusterVirtualImage as a CD-ROM", func() {
		cvi := newClusterVirtualImage(f, "iso",
			cvibuilder.WithDataSourceHTTP(object.ImageURLCustomISO, nil, nil),
		)

		createClusterVirtualImageAndWait(ctx, f, cvi)

		runVirtualMachineFromClusterImageUntilRunning(ctx, f, cvi)
	})

	It("provisions a VirtualDisk from a qcow2 ClusterVirtualImage and runs a VirtualMachine with a ready agent", func() {
		cvi := newClusterVirtualImage(f, "qcow2",
			cvibuilder.WithDataSourceHTTP(object.ImageURLCustomBIOS, nil, nil),
		)

		createClusterVirtualImageAndWait(ctx, f, cvi)

		vd := object.NewVDFromCVI("vd-from-cvi-qcow2", f.Namespace().Name, cvi.Name,
			vdbuilder.WithStorageClass(defaultStorageClass()))

		createVirtualDiskAndRunVM(ctx, f, vd)
	})
})

// runVirtualMachineFromClusterImageUntilRunning boots a VirtualMachine from cvi (an ISO)
// as a CD-ROM with a blank target disk and verifies it reaches Running with a bootable
// device. It does not wait for the guest agent, which is not available when booting from
// CD-ROM/ISO media.
func runVirtualMachineFromClusterImageUntilRunning(ctx context.Context, f *framework.Framework, cvi *v1alpha2.ClusterVirtualImage) {
	GinkgoHelper()

	blankVD := object.NewBlankVD("vd-blank-for-cvi-iso", f.Namespace().Name, defaultStorageClass(), ptr.To(resource.MustParse(vdCreationImageSize)))
	vm := object.NewMinimalVM("vm-from-cvi-", f.Namespace().Name,
		vmbuilder.WithBootloader(v1alpha2.BIOS),
		vmbuilder.WithCPU(1, ptr.To(object.CustomImageVMCoreFraction)),
		vmbuilder.WithMemory(resource.MustParse(object.CustomImageVMMemory)),
		vmbuilder.WithRunPolicy(v1alpha2.AlwaysOnPolicy),
		vmbuilder.WithBlockDeviceRefs(v1alpha2.BlockDeviceSpecRef{
			Kind:      v1alpha2.ClusterImageDevice,
			Name:      cvi.Name,
			BootOrder: ptr.To(uint(1)),
		}, v1alpha2.BlockDeviceSpecRef{
			Kind:      v1alpha2.DiskDevice,
			Name:      blankVD.Name,
			BootOrder: ptr.To(uint(2)),
		}),
	)

	By("Creating blank VirtualDisk and VirtualMachine from the ClusterVirtualImage", func() {
		err := f.CreateWithDeferredDeletion(ctx, blankVD, vm)
		Expect(err).NotTo(HaveOccurred())
	})

	obs := vmobs.StartObserver(ctx, f, vm)
	obs.Never(vmobs.BeFailed())
	// ImageURLCustomISO publishes the BIOS flavor of the custom ISO, so the VM
	// boots it under SeaBIOS: the firmware must find a boot device, and
	// NoBootableDevice would mean the ISO is not bootable.
	obs.Never(vmobs.HaveNoBootableDevice())

	By("Waiting for the VirtualMachine to be Running", func() {
		Expect(obs.WaitFor(vmobs.BeRunning(), framework.LongTimeout)).To(Succeed())
	})
}
