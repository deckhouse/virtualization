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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/utils/ptr"

	vdbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vd"
	vmbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vm"
	vmbdabuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vmbda"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	"github.com/deckhouse/virtualization/test/e2e/internal/object"
	"github.com/deckhouse/virtualization/test/e2e/internal/util"
)

var _ = Describe("DiskAttachment", func() {
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
		By("Create test resources", func() {
			// Create VD from CVI for VM root disk
			vdRoot = object.NewVDFromCVI("vd-root", f.Namespace().Name, object.PrecreatedCVIAlpineBIOS,
				vdbuilder.WithSize(ptr.To(resource.MustParse("512Mi"))),
			)

			// Create blank VD without consumer (for attachment test)
			vdBlank = vdbuilder.New(
				vdbuilder.WithName("vd-blank"),
				vdbuilder.WithNamespace(f.Namespace().Name),
				vdbuilder.WithPersistentVolumeClaim(nil, ptr.To(resource.MustParse("100Mi"))),
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
				vmbuilder.WithCPU(1, ptr.To("100%")),
			)

			// Create VMBDA for attachment (to be created later)
			vmbda = vmbdabuilder.New(
				vmbdabuilder.WithName("vmbda"),
				vmbdabuilder.WithNamespace(f.Namespace().Name),
				vmbdabuilder.WithVirtualMachineName(vm.Name),
				vmbdabuilder.WithBlockDeviceRef(v1alpha2.VMBDAObjectRefKindVirtualDisk, vdBlank.Name),
			)

			// Create VD and VM first
			err := f.CreateWithDeferredDeletion(ctx, vdRoot, vdBlank, vm)
			Expect(err).NotTo(HaveOccurred())

			// Get expected phase for VD without consumer based on VolumeBindingMode
			expectedDiskPhase := util.GetExpectedDiskPhaseByVolumeBindingMode()

			By("Wait for resources to be ready", func() {
				util.UntilObjectPhase(expectedDiskPhase, framework.LongTimeout, vdBlank)
				util.UntilObjectPhase(string(v1alpha2.MachineRunning), framework.LongTimeout, vm)
				util.UntilSSHReady(f, vm, framework.MiddleTimeout)
			})
		})

		By("Get disk count before attachment", func() {
			var err error
			diskCountBeforeAttachment, err = util.GetDiskCount(f, vm.Name, vm.Namespace)
			Expect(err).NotTo(HaveOccurred())
		})

		By("Create VMBDA to attach the disk", func() {
			err := f.CreateWithDeferredDeletion(ctx, vmbda)
			Expect(err).NotTo(HaveOccurred())

			util.UntilObjectPhase(string(v1alpha2.BlockDeviceAttachmentPhaseAttached), framework.LongTimeout, vmbda)
		})

		By("Verify disk count increased by 1", func() {
			Eventually(func(g Gomega) {
				diskCountAfterAttachment, err := util.GetDiskCount(f, vm.Name, vm.Namespace)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(diskCountAfterAttachment).To(Equal(diskCountBeforeAttachment+1),
					"disk count after attachment should be before + 1")
			}).WithTimeout(framework.MiddleTimeout).WithPolling(time.Second).Should(Succeed())
		})

		By("Get disk count before detachment", func() {
			var err error
			diskCountBeforeDetachment, err = util.GetDiskCount(f, vm.Name, vm.Namespace)
			Expect(err).NotTo(HaveOccurred())
		})

		By("Detach virtual disk", func() {
			err := f.Delete(ctx, vmbda)
			Expect(err).NotTo(HaveOccurred())
		})

		By("Verify disk count decreased by 1", func() {
			Eventually(func(g Gomega) {
				diskCountAfterDetachment, err := util.GetDiskCount(f, vm.Name, vm.Namespace)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(diskCountAfterDetachment).To(Equal(diskCountBeforeDetachment-1),
					"disk count after detachment should be before - 1")
			}).WithTimeout(framework.MiddleTimeout).WithPolling(time.Second).Should(Succeed())
		})
	})
})
