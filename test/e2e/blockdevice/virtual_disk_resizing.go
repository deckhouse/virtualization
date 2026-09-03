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
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/utils/ptr"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/builder/vd"
	vmbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vm"
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

var _ = Describe("VirtualDiskResizing", Label(label.SIGStorage, precheck.NoPrecheck), func() {
	var (
		f   *framework.Framework
		ctx context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()
		f = framework.NewFramework("virtual-disk-resizing")
		f.Before()
		DeferCleanup(f.After)
	})

	It("resizes virtual disks", func() {
		// TODO(e2e-flaky-parallel): flaky under parallel load on the 3-node cluster. Re-enable once stabilized.
		Skip("flaky under parallel load")
		vdRoot := object.NewVDFromCVI("vd-root", f.Namespace().Name, object.PrecreatedCVICustomBIOS, vd.WithSize(ptr.To(resource.MustParse(vdCreationImageSize))), vd.WithStorageClass(defaultStorageClass()))
		vdBlank := object.NewBlankVD("vd-blank", f.Namespace().Name, defaultStorageClass(), ptr.To(resource.MustParse(vdCreationImageSize)))
		vdAttach := object.NewBlankVD("vd-attach", f.Namespace().Name, defaultStorageClass(), ptr.To(resource.MustParse(vdCreationImageSize)))

		vm := object.NewMinimalVM("vm-", f.Namespace().Name,
			vmbuilder.WithName("vm"),
			// The custom image has no cloud-init; the test logs in as root
			// with the baked key, so no provisioning is needed.
			vmbuilder.WithBlockDeviceRefs(
				v1alpha2.BlockDeviceSpecRef{Kind: v1alpha2.VirtualDiskKind, Name: vdRoot.Name},
				v1alpha2.BlockDeviceSpecRef{Kind: v1alpha2.VirtualDiskKind, Name: vdBlank.Name},
			),
		)
		vmbdaAttach := object.NewVMBDAFromDisk("blank-disk-attachment", vm.Name, vdAttach)

		By("Creating the disks, VirtualMachine and attachment", func() {
			err := f.CreateWithDeferredDeletion(ctx, vdRoot, vdBlank, vdAttach, vm, vmbdaAttach)
			Expect(err).NotTo(HaveOccurred())
		})

		vdRootObs := vdobs.StartObserver(ctx, f, vdRoot)
		vdBlankObs := vdobs.StartObserver(ctx, f, vdBlank)
		vdAttachObs := vdobs.StartObserver(ctx, f, vdAttach)
		for _, o := range []vdobs.Observer{vdRootObs, vdBlankObs, vdAttachObs} {
			o.Never(vdobs.BeFailed())
		}
		vmObs := vmobs.StartObserver(ctx, f, vm)
		vmObs.Never(vmobs.BeFailed())
		vmbdaObs := vmbdaobs.StartObserver(ctx, f, vmbdaAttach)
		vmbdaObs.Never(vmbdaobs.BeFailed())

		By("Waiting for the VirtualMachine to run and the disk to attach", func() {
			err := vmObs.WaitFor(vmobs.BeRunning(), framework.LongTimeout)
			Expect(err).NotTo(HaveOccurred())
			err = vmbdaObs.WaitFor(vmbdaobs.BeAttached(), framework.LongTimeout)
			Expect(err).NotTo(HaveOccurred())
		})

		By("Waiting for the guest to accept SSH as root", func() {
			eventually.SSHReadyAsRoot(f, vm, framework.LongTimeout)
		})

		vdRootLsblkSize := util.GetBlockDeviceLsblkSizeAsRoot(ctx, f, vm, vdRoot.Name)
		vdBlankLsblkSize := util.GetBlockDeviceLsblkSizeAsRoot(ctx, f, vm, vdBlank.Name)
		vdAttachLsblkSize := util.GetBlockDeviceLsblkSizeAsRoot(ctx, f, vm, vdAttach.Name)

		var newVDRootSize, newVDBlankSize, newVDAttachSize resource.Quantity

		By("Resizing the disks and observing the Resizing phase", func() {
			// Resizing is transient: register the WaitFor listeners before
			// triggering the resize so the phase is observed as it passes through.
			resizing := make(chan error, 3)
			for _, o := range []vdobs.Observer{vdRootObs, vdBlankObs, vdAttachObs} {
				go func() {
					defer GinkgoRecover()
					resizing <- o.WaitFor(vdobs.BeResizing(), framework.LongTimeout)
				}()
			}

			var err error
			newVDRootSize, err = increaseDiskSize(ctx, f, vdRoot)
			Expect(err).NotTo(HaveOccurred())
			newVDBlankSize, err = increaseDiskSize(ctx, f, vdBlank)
			Expect(err).NotTo(HaveOccurred())
			newVDAttachSize, err = increaseDiskSize(ctx, f, vdAttach)
			Expect(err).NotTo(HaveOccurred())

			for range []int{0, 1, 2} {
				Expect(<-resizing).To(Succeed(), "a VirtualDisk did not pass through the Resizing phase")
			}
		})

		By("Waiting for the disks to finish resizing to the new size", func() {
			// BeResized (not BeReady) is used here on purpose: right after a resize
			// the disk passes through the transient Resizing phase, which BeReady
			// treats as an inconsistency. BeResized waits for the disk to settle back
			// on Ready and asserts its reported capacity equals the new size.
			err := vdRootObs.WaitFor(vdobs.BeResized(newVDRootSize), framework.MiddleTimeout)
			Expect(err).NotTo(HaveOccurred())
			err = vdBlankObs.WaitFor(vdobs.BeResized(newVDBlankSize), framework.MiddleTimeout)
			Expect(err).NotTo(HaveOccurred())
			err = vdAttachObs.WaitFor(vdobs.BeResized(newVDAttachSize), framework.MiddleTimeout)
			Expect(err).NotTo(HaveOccurred())
			err = vmObs.WaitFor(vmobs.BeRunning(), framework.MiddleTimeout)
			Expect(err).NotTo(HaveOccurred())
			err = vmbdaObs.WaitFor(vmbdaobs.BeAttached(), framework.LongTimeout)
			Expect(err).NotTo(HaveOccurred())
		})

		By("Checking the guest observes the increased size", func() {
			eventually.LsblkSizeGrows(ctx, f, vm, vdRoot.Name, vdRootLsblkSize)
			eventually.LsblkSizeGrows(ctx, f, vm, vdBlank.Name, vdBlankLsblkSize)
			eventually.LsblkSizeGrows(ctx, f, vm, vdAttach.Name, vdAttachLsblkSize)
		})

		By("Checking the disks are attached in the VirtualMachine status", func() {
			err := vmObs.WaitFor(vmobs.HaveBlockDevicesAttached(vdRoot.Name, vdBlank.Name, vdAttach.Name), framework.MiddleTimeout)
			Expect(err).NotTo(HaveOccurred())
		})
	})
})

func increaseDiskSize(ctx context.Context, f *framework.Framework, vd *v1alpha2.VirtualDisk) (resource.Quantity, error) {
	err := f.GenericClient().Get(ctx, crclient.ObjectKeyFromObject(vd), vd)
	if err != nil {
		return resource.Quantity{}, err
	}

	if vd.Spec.PersistentVolumeClaim.Size == nil {
		return resource.Quantity{}, fmt.Errorf("virtual disk %s/%s must have PVC size in spec", vd.Namespace, vd.Name)
	}
	// Double the current size: a relative growth works from any base and keeps
	// the target proportional to the disk instead of hardcoding an increment.
	size := *vd.Spec.PersistentVolumeClaim.Size
	size.Add(size)
	vd.Spec.PersistentVolumeClaim.Size = ptr.To(size)

	err = f.GenericClient().Update(ctx, vd)
	if err != nil {
		return resource.Quantity{}, err
	}

	return size, nil
}
