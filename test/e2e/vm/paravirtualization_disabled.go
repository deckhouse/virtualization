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
	"github.com/deckhouse/virtualization/test/e2e/internal/label"
	"github.com/deckhouse/virtualization/test/e2e/internal/object"
	vdobs "github.com/deckhouse/virtualization/test/e2e/internal/observer/vd"
	vmobs "github.com/deckhouse/virtualization/test/e2e/internal/observer/vm"
	vmbdaobs "github.com/deckhouse/virtualization/test/e2e/internal/observer/vmbda"
	"github.com/deckhouse/virtualization/test/e2e/internal/precheck"
	"github.com/deckhouse/virtualization/test/e2e/internal/util"
)

// nicModelE1000 is the NIC model the non-paravirtualized device preset gives a
// VM, the counterpart of virtio in the paravirtualized one.
const nicModelE1000 = "e1000"

// Regression guard for a VM created with enableParavirtualization=false right
// away, not flipped to it after it is already running.
//
// Such a VM gets its disks inline instead of hot-plugged, and KubeVirt does not
// create the VM pod until the target PVC exists. On a WaitForFirstConsumer
// StorageClass that used to deadlock: the disk controller waited for the VM to
// be scheduled before creating the PVC, while the VM could not be scheduled
// without it (no PVC, no pod, no node, no PVC). The disk now gets its PVC as
// soon as a VM consumes it, before the VM has a node.
//
// The deadlock only reproduces on a WaitForFirstConsumer StorageClass — with
// Immediate binding the PVC is provisioned up front — so the spec is a
// regression guard only when the suite runs on WFFC storage; the device preset
// assertions below hold either way.
var _ = Describe("VirtualMachineParavirtualizationDisabled", Label(label.SIGCompute, precheck.NoPrecheck), func() {
	var (
		f       *framework.Framework
		vdRoot  *v1alpha2.VirtualDisk
		vdBlank *v1alpha2.VirtualDisk
		vm      *v1alpha2.VirtualMachine
		vmbda   *v1alpha2.VirtualMachineBlockDeviceAttachment

		vdObs    vdobs.Observer
		vmObs    vmobs.Observer
		vmbdaObs vmbdaobs.Observer

		ctx context.Context
	)

	BeforeEach(func() {
		f = framework.NewFramework("paravirtualization-disabled")
		ctx = context.Background()
		DeferCleanup(f.After)
		f.Before()
	})

	It("boots a VM created with paravirtualization disabled from an image-sourced disk", func() {
		By("Create the disks and the VM with paravirtualization disabled", func() {
			vdRoot = object.NewVDFromCVI("vd-root", f.Namespace().Name, object.PrecreatedCVICustomBIOS,
				vdbuilder.WithSize(ptr.To(resource.MustParse(vdCustomImageSize))))

			vdBlank = object.NewVD(
				vdbuilder.WithName("vd-blank"),
				vdbuilder.WithNamespace(f.Namespace().Name),
				vdbuilder.WithPersistentVolumeClaim(nil, ptr.To(resource.MustParse(vdCustomImageSize))),
			)

			vm = object.NewMinimalVM("", f.Namespace().Name,
				vmbuilder.WithName("vm"),
				// The custom image has no cloud-init; the guest agent is
				// baked in, so no provisioning is needed.
				vmbuilder.WithEnableParavirtualization(ptr.To(false)),
				vmbuilder.WithDisks(vdRoot),
			)

			vmbda = vmbdabuilder.New(
				vmbdabuilder.WithName("vmbda"),
				vmbdabuilder.WithNamespace(f.Namespace().Name),
				vmbdabuilder.WithVirtualMachineName(vm.Name),
				vmbdabuilder.WithBlockDeviceRef(v1alpha2.VMBDAObjectRefKindVirtualDisk, vdBlank.Name),
			)

			err := f.CreateWithDeferredDeletion(ctx, vdRoot, vdBlank, vm)
			Expect(err).NotTo(HaveOccurred())

			vdObs = vdobs.StartObserver(ctx, f, vdRoot)
			vdObs.Never(vdobs.BeFailed())
			vmObs = vmobs.StartObserver(ctx, f, vm)
			vmObs.Never(vmobs.BeFailed())
		})

		By("Wait for the disk to be provisioned and the VM to run", func() {
			// The disk is provisioned only once the VM consumes it, so both
			// waits are on the same chain: PVC, then pod, then node, then the
			// import. A timeout here is the deadlock coming back.
			err := vdObs.WaitFor(vdobs.BeReady(), framework.LongTimeout)
			Expect(err).NotTo(HaveOccurred())

			err = vmObs.WaitFor(vmobs.BeRunning(), framework.LongTimeout)
			Expect(err).NotTo(HaveOccurred())
		})

		By("Verify the VM runs on the non-paravirtualized device preset", func() {
			bus, ok := util.GetBlockDeviceBus(ctx, vm, v1alpha2.DiskDevice, vdRoot.Name)
			Expect(ok).To(BeTrue(), "the disk is not attached to the VMI")
			Expect(bus).To(Equal(virtv1.DiskBusSATA), "an inline disk must follow the paravirtualization preset")

			kvvmi, err := util.GetInternalVirtualMachineInstance(ctx, vm)
			Expect(err).NotTo(HaveOccurred())
			Expect(kvvmi).NotTo(BeNil(), "the VM is running but has no VMI")

			interfaces := kvvmi.Spec.Domain.Devices.Interfaces
			Expect(interfaces).NotTo(BeEmpty(), "the VMI has no network interfaces")
			for _, iface := range interfaces {
				Expect(iface.Model).To(Equal(nicModelE1000),
					"interface %q must follow the paravirtualization preset", iface.Name)
			}
		})

		By("Hot-plug a disk via VMBDA and verify it lands on the scsi bus", func() {
			// The sata preset applies to the VM's own disks only: a hot-plugged
			// disk is attached by AddVolume, which always uses scsi, and sata is
			// not a valid bus for one.
			vmbdaObs = vmbdaobs.StartObserver(ctx, f, vmbda)
			vmbdaObs.Never(vmbdaobs.BeFailed())
			err := f.CreateWithDeferredDeletion(ctx, vmbda)
			Expect(err).NotTo(HaveOccurred())

			err = vmbdaObs.WaitFor(vmbdaobs.BeAttached(), framework.LongTimeout)
			Expect(err).NotTo(HaveOccurred())

			bus, ok := util.GetBlockDeviceBus(ctx, vm, v1alpha2.DiskDevice, vdBlank.Name)
			Expect(ok).To(BeTrue(), "the hot-plugged disk is not attached to the VMI")
			Expect(bus).To(Equal(virtv1.DiskBusSCSI), "a hot-plugged disk must stay on the scsi bus")
		})
	})
})
