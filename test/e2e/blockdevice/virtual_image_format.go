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

	vdbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vd"
	vibuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vi"
	vmbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vm"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	"github.com/deckhouse/virtualization/test/e2e/internal/object"
	vmobs "github.com/deckhouse/virtualization/test/e2e/internal/observer/vm"
	"github.com/deckhouse/virtualization/test/e2e/internal/precheck"
)

// VirtualImageFormat verifies how image formats are handled when the source is an HTTP
// data source:
//   - an ISO VirtualImage boots a VirtualMachine directly (as a CD-ROM);
//   - a qcow2 VirtualImage backs a VirtualDisk, and a VirtualMachine boots from that disk.
//
// The qcow2 spec provisions its main VirtualDisk on the WFFC StorageClass, so the precheck
// label is declared on the Describe (the spec-label validator only reads container-hierarchy
// labels, not leaf It labels).
var _ = Describe("VirtualImageFormat", Label(precheck.PrecheckWFFCStorageClass), func() {
	var (
		f   *framework.Framework
		ctx context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()
		f = framework.NewFramework("")
		f.Before()
		DeferCleanup(f.After)
		setupProject(ctx, f, "vi-format")
	})

	It("runs a VirtualMachine directly from an iso VirtualImage", func() {
		vi := vibuilder.New(
			vibuilder.WithName("vi-iso"),
			vibuilder.WithNamespace(f.Namespace().Name),
			vibuilder.WithStorage(v1alpha2.StorageContainerRegistry),
			vibuilder.WithDataSourceHTTP(object.ImageURLUbuntuISO, nil, nil),
		)

		createVirtualImageAndWait(ctx, f, vi)

		runVirtualMachineFromImageUntilRunning(ctx, f, vi)
	})

	It("provisions a VirtualDisk from a qcow2 VirtualImage and runs a VirtualMachine with a ready agent", func() {
		vi := vibuilder.New(
			vibuilder.WithName("vi-qcow2"),
			vibuilder.WithNamespace(f.Namespace().Name),
			vibuilder.WithStorage(v1alpha2.StorageContainerRegistry),
			vibuilder.WithDataSourceHTTP(object.ImageURLAlpineBIOS, nil, nil),
		)

		createVirtualImageAndWait(ctx, f, vi)

		// The disk under test is the scenario's main resource, so it lives on the WFFC
		// storage class.
		vd := object.NewVDFromVI("vd-from-vi-qcow2", f.Namespace().Name, vi,
			vdbuilder.WithStorageClass(wffcStorageClass()),
			vdbuilder.WithSize(ptr.To(resource.MustParse("350Mi"))))

		createVirtualDiskAndRunVM(ctx, f, vd)
	})
})

// runVirtualMachineFromImageUntilRunning boots a VirtualMachine from vi and waits until
// it reaches the Running phase. It does not wait for the guest agent, which is not
// available when booting from CD-ROM/ISO media.
func runVirtualMachineFromImageUntilRunning(ctx context.Context, f *framework.Framework, vi *v1alpha2.VirtualImage) {
	GinkgoHelper()

	vm := object.NewMinimalVM("vm-from-vi-", f.Namespace().Name,
		vmbuilder.WithBlockDeviceRefs(v1alpha2.BlockDeviceSpecRef{
			Kind: v1alpha2.ImageDevice,
			Name: vi.Name,
		}),
	)

	By("Creating VirtualMachine from the VirtualImage", func() {
		err := f.CreateWithDeferredDeletion(ctx, vm)
		Expect(err).NotTo(HaveOccurred())
	})

	obs := vmobs.StartObserver(ctx, f, vm)
	obs.Never(vmobs.BeFailed())

	By("Waiting for the VirtualMachine to be Running", func() {
		err := obs.WaitFor(vmobs.BeRunning(), framework.LongTimeout)
		Expect(err).NotTo(HaveOccurred())
	})
}
