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

package vm

import (
	"context"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/utils/ptr"

	vdbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vd"
	vmbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vm"
	vmbdabuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vmbda"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/test/e2e/eventually"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	"github.com/deckhouse/virtualization/test/e2e/internal/label"
	"github.com/deckhouse/virtualization/test/e2e/internal/object"
	vdobs "github.com/deckhouse/virtualization/test/e2e/internal/observer/vd"
	vmobs "github.com/deckhouse/virtualization/test/e2e/internal/observer/vm"
	vmbdaobs "github.com/deckhouse/virtualization/test/e2e/internal/observer/vmbda"
	"github.com/deckhouse/virtualization/test/e2e/internal/precheck"
	"github.com/deckhouse/virtualization/test/e2e/internal/util"
)

var _ = Describe("DiskAttachment", Label(label.SIGCompute, precheck.NoPrecheck), func() {
	var (
		f       *framework.Framework
		vdRoot  *v1alpha2.VirtualDisk
		vdBlank *v1alpha2.VirtualDisk
		vm      *v1alpha2.VirtualMachine
		vmbda   *v1alpha2.VirtualMachineBlockDeviceAttachment

		ctx context.Context

		diskCountBeforeAttachment int
		diskCountBeforeDetachment int
	)

	BeforeEach(func() {
		f = framework.NewFramework("disk-attachment")
		ctx = context.Background()
		DeferCleanup(f.After)
		f.Before()
	})

	It("attaches and detaches a virtual disk to a running VM", func() {
		// TODO(e2e-flaky-parallel): flaky under parallel load on the 3-node cluster (guest disk hot-unplug not reflected in time); passes in isolation / on a replicated storage class. Re-enable once stabilized.
		Skip("flaky under parallel load: guest disk detach not reflected in time")
		var (
			vdBlankObs vdobs.Observer
			vmObs      vmobs.Observer
			vmbdaObs   vmbdaobs.Observer
		)

		By("Create test resources", func() {
			// Use names longer than the former 60-character VirtualDisk limit to
			// exercise end-to-end that disk names up to the Kubernetes name length
			// work through VM start and disk hotplug; the underlying KubeVirt volume
			// name is derived and shortened internally.
			longName := func(base string) string { return base + "-" + strings.Repeat("a", 80) }

			// Create VD from CVI for VM root disk
			vdRoot = object.NewVDFromCVI(longName("vd-root"), f.Namespace().Name, object.PrecreatedCVICustomBIOS,
				vdbuilder.WithSize(ptr.To(resource.MustParse(vdCustomImageSize))),
			)

			// Create blank VD without consumer (for attachment test)
			vdBlank = object.NewVD(
				vdbuilder.WithName(longName("vd-blank")),
				vdbuilder.WithNamespace(f.Namespace().Name),
				vdbuilder.WithPersistentVolumeClaim(nil, ptr.To(resource.MustParse(vdCustomImageSize))),
			)

			// Create VM with root disk
			vm = object.NewMinimalVM("", f.Namespace().Name,
				vmbuilder.WithBlockDeviceRefs(
					v1alpha2.BlockDeviceSpecRef{
						Kind: v1alpha2.VirtualDiskKind,
						Name: vdRoot.Name,
					},
				),
				vmbuilder.WithName("vm"),
				// The custom image has no cloud-init; the guest agent is
				// baked into the image, so no provisioning is needed.
			)

			// Create VMBDA for attachment (to be created later)
			vmbda = vmbdabuilder.New(
				vmbdabuilder.WithName("vmbda"),
				vmbdabuilder.WithNamespace(f.Namespace().Name),
				vmbdabuilder.WithVirtualMachineName(vm.Name),
				vmbdabuilder.WithBlockDeviceRef(v1alpha2.VMBDAObjectRefKindVirtualDisk, vdBlank.Name),
			)

			vdRootObs := vdobs.StartObserver(ctx, f, vdRoot)
			vdRootObs.Never(vdobs.BeFailed())
			vdBlankObs = vdobs.StartObserver(ctx, f, vdBlank)
			vdBlankObs.Never(vdobs.BeFailed())
			vmObs = vmobs.StartObserver(ctx, f, vm)
			vmObs.Never(vmobs.BeFailed())
			vmbdaObs = vmbdaobs.StartObserver(ctx, f, vmbda)
			vmbdaObs.Never(vmbdaobs.BeFailed())

			// Create VD and VM first
			err := f.CreateWithDeferredDeletion(ctx, vdRoot, vdBlank, vm)
			Expect(err).NotTo(HaveOccurred())

			By("Wait for resources to be ready", func() {
				// A blank disk with no consumer becomes Ready on an Immediate
				// StorageClass but stays WaitForFirstConsumer on a WFFC one.
				if util.GetExpectedDiskPhaseByVolumeBindingMode() == string(v1alpha2.DiskWaitForFirstConsumer) {
					err := vdBlankObs.WaitFor(vdobs.BeWaitForFirstConsumer(), framework.LongTimeout)
					Expect(err).NotTo(HaveOccurred())
				} else {
					err := vdBlankObs.WaitFor(vdobs.BeReady(), framework.LongTimeout)
					Expect(err).NotTo(HaveOccurred())
				}
				err := vmObs.WaitFor(vmobs.BeRunning(), framework.LongTimeout)
				Expect(err).NotTo(HaveOccurred())
				eventually.SSHReadyAsRoot(f, vm, framework.LongTimeout)
				// lsblk is baked into the custom image (util-linux), so no
				// wait for cloud-init to install it is needed.
			})
		})

		By("Get disk count before attachment", func() {
			var err error
			diskCountBeforeAttachment, err = util.GetDiskCountAsRoot(f, vm.Name, vm.Namespace)
			Expect(err).NotTo(HaveOccurred())
		})

		By("Create VMBDA to attach the disk", func() {
			err := f.CreateWithDeferredDeletion(ctx, vmbda)
			Expect(err).NotTo(HaveOccurred())

			err = vmbdaObs.WaitFor(vmbdaobs.BeAttached(), framework.LongTimeout)
			Expect(err).NotTo(HaveOccurred())
		})

		By("Verify disk count increased by 1", func() {
			eventually.UntilDiskCountAsRoot(f, vm.Name, vm.Namespace,
				Equal(diskCountBeforeAttachment+1), framework.LongTimeout,
				eventually.WithExplanation("disk count after attachment should be before + 1"))
		})

		By("Get disk count before detachment", func() {
			var err error
			diskCountBeforeDetachment, err = util.GetDiskCountAsRoot(f, vm.Name, vm.Namespace)
			Expect(err).NotTo(HaveOccurred())
		})

		By("Detach virtual disk", func() {
			err := f.Delete(ctx, vmbda)
			Expect(err).NotTo(HaveOccurred())
		})

		By("Verify disk count decreased by 1", func() {
			eventually.UntilDiskCountAsRoot(f, vm.Name, vm.Namespace,
				Equal(diskCountBeforeDetachment-1), framework.LongTimeout,
				eventually.WithExplanation("disk count after detachment should be before - 1"))
		})
	})
})
