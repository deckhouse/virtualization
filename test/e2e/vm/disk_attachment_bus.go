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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/utils/ptr"
	virtv1 "kubevirt.io/api/core/v1"

	vdbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vd"
	vmbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vm"
	vmbdabuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vmbda"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	"github.com/deckhouse/virtualization/test/e2e/internal/object"
	"github.com/deckhouse/virtualization/test/e2e/internal/precheck"
	"github.com/deckhouse/virtualization/test/e2e/internal/util"
)

// Regression guard: a disk hot-plugged via VMBDA is always attached on the scsi
// bus (AddVolume forces it), independent of the VM's paravirtualization mode. A
// VM with enableParavirtualization=false uses the sata preset for its own disks,
// but the hot-plugged disk must not be moved to sata — sata is invalid for a
// hot-plugged device and the attachment would break.
var _ = Describe("DiskAttachmentBus", Label(precheck.NoPrecheck), func() {
	var (
		f       *framework.Framework
		vdRoot  *v1alpha2.VirtualDisk
		vdBlank *v1alpha2.VirtualDisk
		vm      *v1alpha2.VirtualMachine
		vmbda   *v1alpha2.VirtualMachineBlockDeviceAttachment

		ctx context.Context
	)

	BeforeEach(func() {
		f = framework.NewFramework("disk-attachment-bus")
		ctx = context.Background()
		DeferCleanup(f.After)
		f.Before()
	})

	It("attaches a VMBDA disk on the scsi bus when enableParavirtualization=false", func() {
		By("Create a VM with paravirtualization disabled and a blank disk to attach", func() {
			vdRoot = object.NewVDFromCVI("vd-root", f.Namespace().Name, object.PrecreatedCVIAlpineBIOS,
				vdbuilder.WithSize(ptr.To(resource.MustParse("400Mi"))),
			)

			vdBlank = vdbuilder.New(
				vdbuilder.WithName("vd-blank"),
				vdbuilder.WithNamespace(f.Namespace().Name),
				vdbuilder.WithPersistentVolumeClaim(nil, ptr.To(resource.MustParse("100Mi"))),
			)

			vm = object.NewMinimalVM("", f.Namespace().Name,
				vmbuilder.WithName("vm"),
				vmbuilder.WithCPU(1, ptr.To("100%")),
				vmbuilder.WithEnableParavirtualization(ptr.To(false)),
				vmbuilder.WithBlockDeviceRefs(
					v1alpha2.BlockDeviceSpecRef{
						Kind: v1alpha2.VirtualDiskKind,
						Name: vdRoot.Name,
					},
				),
			)

			vmbda = vmbdabuilder.New(
				vmbdabuilder.WithName("vmbda"),
				vmbdabuilder.WithNamespace(f.Namespace().Name),
				vmbdabuilder.WithVirtualMachineName(vm.Name),
				vmbdabuilder.WithBlockDeviceRef(v1alpha2.VMBDAObjectRefKindVirtualDisk, vdBlank.Name),
			)

			err := f.CreateWithDeferredDeletion(ctx, vdRoot, vdBlank, vm)
			Expect(err).NotTo(HaveOccurred())

			util.UntilObjectPhase(ctx, string(v1alpha2.MachineRunning), framework.LongTimeout, vm)
		})

		By("Attach the disk via VMBDA", func() {
			err := f.CreateWithDeferredDeletion(ctx, vmbda)
			Expect(err).NotTo(HaveOccurred())

			util.UntilObjectPhase(ctx, string(v1alpha2.BlockDeviceAttachmentPhaseAttached), framework.LongTimeout, vmbda)
		})

		By("Verify the attached disk is on the scsi bus", func() {
			bus, ok := util.GetBlockDeviceBus(ctx, vm, v1alpha2.DiskDevice, vdBlank.Name)
			Expect(ok).To(BeTrue(), "attached VMBDA disk not found on the VMI")
			Expect(bus).To(Equal(virtv1.DiskBusSCSI), "hot-plugged disk must stay on the scsi bus")
		})
	})
})
