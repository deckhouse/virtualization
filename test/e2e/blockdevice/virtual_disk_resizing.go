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
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/utils/ptr"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/builder/vd"
	vmbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vm"
	"github.com/deckhouse/virtualization-controller/pkg/common"
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

var _ = label.SIGDescribe(label.SIGStorage, "VirtualDiskResizing", Label(precheck.NoPrecheck), func() {
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
		vdRoot := object.NewVDFromCVI("vd-root", f.Namespace().Name, object.PrecreatedCVICustomBIOS, vd.WithSize(ptr.To(resource.MustParse("2Gi"))), vd.WithStorageClass(defaultStorageClass()))
		vdBlank := object.NewBlankVD("vd-blank", f.Namespace().Name, defaultStorageClass(), ptr.To(resource.MustParse("100Mi")))
		vdAttach := object.NewBlankVD("vd-attach", f.Namespace().Name, defaultStorageClass(), ptr.To(resource.MustParse("100Mi")))

		vm := object.NewMinimalVM("vm-", f.Namespace().Name,
			vmbuilder.WithName("vm"),
			// The custom e2e-br image has no cloud-init; the test logs in as root
			// with the baked key, so no provisioning is needed.
			vmbuilder.WithProvisioning(nil),
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
			Expect(vmObs.WaitFor(vmobs.BeRunning(), framework.LongTimeout)).To(Succeed())
			Expect(vmbdaObs.WaitFor(vmbdaobs.BeAttached(), framework.MiddleTimeout)).To(Succeed())
		})

		By("Waiting for the guest to accept SSH as root", func() {
			waitGuestSSHReadyAsRoot(f, vm)
		})

		vdRootLsblkSize := getBlockDeviceLsblkSizeAsRoot(ctx, f, vm, vdRoot.Name)
		vdBlankLsblkSize := getBlockDeviceLsblkSizeAsRoot(ctx, f, vm, vdBlank.Name)
		vdAttachLsblkSize := getBlockDeviceLsblkSizeAsRoot(ctx, f, vm, vdAttach.Name)

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
			Expect(vdRootObs.WaitFor(vdobs.BeResized(newVDRootSize), framework.MiddleTimeout)).To(Succeed())
			Expect(vdBlankObs.WaitFor(vdobs.BeResized(newVDBlankSize), framework.MiddleTimeout)).To(Succeed())
			Expect(vdAttachObs.WaitFor(vdobs.BeResized(newVDAttachSize), framework.MiddleTimeout)).To(Succeed())
			Expect(vmObs.WaitFor(vmobs.BeRunning(), framework.ShortTimeout)).To(Succeed())
			Expect(vmbdaObs.WaitFor(vmbdaobs.BeAttached(), framework.ShortTimeout)).To(Succeed())
		})

		By("Checking the guest observes the increased size", func() {
			// EXCEPTION: this is a guest-side wait, not a Kubernetes resource, so
			// there is nothing to observe via an Observer. The new size becomes
			// visible in the guest asynchronously (CSI expansion + qemu block-device
			// refresh finish after the VirtualDisk reports Ready), so Eventually is
			// used deliberately here. This is the only sanctioned Eventually in the
			// blockdevice suite.
			untilLsblkSizeGrows := func(vdName string, oldSize resource.Quantity) {
				GinkgoHelper()
				Eventually(func() int {
					size := getBlockDeviceLsblkSizeAsRoot(ctx, f, vm, vdName)
					return size.Cmp(oldSize)
				}).WithTimeout(framework.MiddleTimeout).WithPolling(5*time.Second).Should(Equal(common.CmpGreater),
					"the guest should observe the increased size of the %q disk", vdName)
			}

			untilLsblkSizeGrows(vdRoot.Name, vdRootLsblkSize)
			untilLsblkSizeGrows(vdBlank.Name, vdBlankLsblkSize)
			untilLsblkSizeGrows(vdAttach.Name, vdAttachLsblkSize)
		})

		By("Checking the disks are attached in the VirtualMachine status", func() {
			Expect(vmObs.WaitFor(func(m *v1alpha2.VirtualMachine) (bool, error) {
				for _, d := range []*v1alpha2.VirtualDisk{vdRoot, vdBlank, vdAttach} {
					if !util.IsVDAttached(m, d) {
						return false, nil
					}
				}
				return true, nil
			}, framework.ShortTimeout)).To(Succeed())
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
	size := *vd.Spec.PersistentVolumeClaim.Size
	size.Add(resource.MustParse("100Mi"))
	vd.Spec.PersistentVolumeClaim.Size = ptr.To(size)

	err = f.GenericClient().Update(ctx, vd)
	if err != nil {
		return resource.Quantity{}, err
	}

	return size, nil
}

// waitGuestSSHReadyAsRoot polls the guest over SSH as root until it responds.
// This is a guest-side readiness probe (there is no Kubernetes resource to
// observe), so Eventually is used deliberately.
func waitGuestSSHReadyAsRoot(f *framework.Framework, vm *v1alpha2.VirtualMachine) {
	GinkgoHelper()
	Eventually(func(g Gomega) {
		out, err := f.SSHCommand(vm.Name, vm.Namespace, "echo ok",
			framework.WithSSHUser("root"), framework.WithSSHTimeout(5*time.Second))
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(out).To(ContainSubstring("ok"))
	}).WithTimeout(framework.LongTimeout).WithPolling(time.Second).Should(Succeed())
}

// getBlockDeviceLsblkSizeAsRoot returns the lsblk-reported size (in bytes) of
// the VirtualDisk bdName, logging in as root without sudo.
//
// The custom e2e-br image has no cloud user and no sudo, and runs no udev, so
// lsblk cannot populate the SERIAL column. The device is instead resolved by
// serial through guestDeviceBySerial (which reads the SCSI VPD from sysfs), and
// its size is read with "lsblk -b" (fed from sysfs, so it needs no udev either).
func getBlockDeviceLsblkSizeAsRoot(ctx context.Context, f *framework.Framework, vm *v1alpha2.VirtualMachine, bdName string) resource.Quantity {
	GinkgoHelper()

	dev := guestDeviceBySerial(ctx, f, vm, v1alpha2.VirtualDiskKind, bdName)

	out, err := f.SSHCommand(vm.Name, vm.Namespace, "lsblk --nodeps -bno SIZE "+dev, framework.WithSSHUser("root"))
	Expect(err).NotTo(HaveOccurred())

	return resource.MustParse(strings.TrimSpace(out))
}
