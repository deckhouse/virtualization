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
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	virtv1 "kubevirt.io/api/core/v1"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"

	vdbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vd"
	vmbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vm"
	vmbdabuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vmbda"
	"github.com/deckhouse/virtualization-controller/pkg/common/patch"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	"github.com/deckhouse/virtualization/test/e2e/internal/label"
	"github.com/deckhouse/virtualization/test/e2e/internal/object"
	vmobs "github.com/deckhouse/virtualization/test/e2e/internal/observer/vm"
	vmbdaobs "github.com/deckhouse/virtualization/test/e2e/internal/observer/vmbda"
	"github.com/deckhouse/virtualization/test/e2e/internal/precheck"
	"github.com/deckhouse/virtualization/test/e2e/internal/util"
)

// Regression guard: a disk hot-plugged via VMBDA is always attached on the scsi
// bus (AddVolume forces it), independent of the VM's paravirtualization mode. A
// VM with enableParavirtualization=false uses the sata preset for its own disks,
// but the hot-plugged disk must never be moved to sata — sata is invalid for a
// hot-plugged device and the attachment would break. Flipping paravirtualization
// forces the restart that rebuilds the VM, so the bus must survive both flips.
var _ = Describe("DiskAttachmentBus", Label(label.SIGCompute, precheck.NoPrecheck), func() {
	var (
		f       *framework.Framework
		vdBlank *v1alpha2.VirtualDisk
		vm      *v1alpha2.VirtualMachine
		vmbda   *v1alpha2.VirtualMachineBlockDeviceAttachment

		vmObs    vmobs.Observer
		vmbdaObs vmbdaobs.Observer

		ctx context.Context
	)

	BeforeEach(func() {
		f = framework.NewFramework("disk-attachment-bus")
		ctx = context.Background()
		DeferCleanup(f.After)
		f.Before()
	})

	// expectVMBDAScsi waits for the VMBDA disk to be (re)attached and asserts it
	// sits on the scsi bus.
	expectVMBDAScsi := func(stage string) {
		GinkgoHelper()
		err := vmbdaObs.WaitFor(vmbdaobs.BeAttached(), framework.LongTimeout)
		Expect(err).NotTo(HaveOccurred())
		bus, ok := util.GetBlockDeviceBus(ctx, vm, v1alpha2.DiskDevice, vdBlank.Name)
		Expect(ok).To(BeTrue(), fmt.Sprintf("%s: attached VMBDA disk not found on the VMI", stage))
		Expect(bus).To(Equal(virtv1.DiskBusSCSI), fmt.Sprintf("%s: hot-plugged disk must stay on the scsi bus", stage))
	}

	// flipParavirtualization patches enableParavirtualization, restarts the VM
	// (paravirtualization changes require a restart), and waits until it is back.
	flipParavirtualization := func(enable bool) {
		GinkgoHelper()
		err := f.GenericClient().Get(ctx, crclient.ObjectKeyFromObject(vm), vm)
		Expect(err).NotTo(HaveOccurred())
		runningCondition, _ := conditions.GetCondition(vmcondition.TypeRunning, vm.Status.Conditions)
		previousRunningTime := runningCondition.LastTransitionTime.Time

		patchset := patch.NewJSONPatch(patch.WithReplace("/spec/enableParavirtualization", enable))
		patchBytes, err := patchset.Bytes()
		Expect(err).NotTo(HaveOccurred())
		vm, err = f.VirtClient().VirtualMachines(vm.Namespace).Patch(ctx, vm.Name, types.JSONPatchType, patchBytes, metav1.PatchOptions{})
		Expect(err).NotTo(HaveOccurred())

		// Restart approval is Automatic here, so the controller reboots the VM
		// itself; just wait for the reboot to complete.
		key := crclient.ObjectKeyFromObject(vm)
		util.SkipIfGuestPowerActionStuck(ctx, key)
		err = vmObs.WaitFor(vmobs.BeRebootedAfter(previousRunningTime), framework.LongTimeout)
		if err != nil {
			util.SkipIfGuestPowerActionStuck(ctx, key)
		}
		Expect(err).NotTo(HaveOccurred())
		err = vmObs.WaitFor(vmobs.BeRunning(), framework.MiddleTimeout)
		Expect(err).NotTo(HaveOccurred())
	}

	It("keeps a VMBDA disk on the scsi bus across paravirtualization flips", func() {
		By("Create a VM with paravirtualization enabled and a blank disk to attach", func() {
			vdRoot := object.NewVDFromCVI("vd-root", f.Namespace().Name, object.PrecreatedCVICustomBIOS,
				vdbuilder.WithSize(ptr.To(resource.MustParse(vdCustomImageSize))))

			vdBlank = object.NewVD(
				vdbuilder.WithName("vd-blank"),
				vdbuilder.WithNamespace(f.Namespace().Name),
				vdbuilder.WithPersistentVolumeClaim(nil, ptr.To(resource.MustParse(vdCustomImageSize))),
			)

			// The VM starts with paravirtualization enabled, the default mode, and
			// the flips below cover the other one. A VM created with
			// paravirtualization disabled from the start is a scenario of its own
			// and is covered by VirtualMachineParavirtualizationDisabled.
			vm = object.NewMinimalVM("", f.Namespace().Name,
				vmbuilder.WithName("vm"),
				// The custom image has no cloud-init; the guest agent is
				// baked in, so no provisioning is needed.
				vmbuilder.WithRestartApprovalMode(v1alpha2.Automatic),
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

			vmObs = vmobs.StartObserver(ctx, f, vm)
			vmObs.Never(vmobs.BeFailed())
			err = vmObs.WaitFor(vmobs.BeRunning(), framework.LongTimeout)
			Expect(err).NotTo(HaveOccurred())
		})

		By("Attach the disk via VMBDA and verify it is on the scsi bus", func() {
			vmbdaObs = vmbdaobs.StartObserver(ctx, f, vmbda)
			err := f.CreateWithDeferredDeletion(ctx, vmbda)
			Expect(err).NotTo(HaveOccurred())

			expectVMBDAScsi("paravirtualization enabled")
		})

		By("Disable paravirtualization, restart, and verify the disk is still on scsi", func() {
			flipParavirtualization(false)
			expectVMBDAScsi("paravirtualization disabled")
		})

		By("Enable paravirtualization again, restart, and verify the disk is still on scsi", func() {
			flipParavirtualization(true)
			expectVMBDAScsi("paravirtualization enabled again")
		})
	})
})
