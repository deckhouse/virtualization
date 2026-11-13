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
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/utils/ptr"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"

	vdbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vd"
	vdsnapshotbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vdsnapshot"
	vmbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vm"
	coordinationv1alpha1 "github.com/deckhouse/virtualization/api/coordination/v1alpha1"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdscondition"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	"github.com/deckhouse/virtualization/test/e2e/internal/label"
	"github.com/deckhouse/virtualization/test/e2e/internal/object"
	vdobs "github.com/deckhouse/virtualization/test/e2e/internal/observer/vd"
	vdsnapshotobs "github.com/deckhouse/virtualization/test/e2e/internal/observer/vdsnapshot"
	vmobs "github.com/deckhouse/virtualization/test/e2e/internal/observer/vm"
	vmbdaobs "github.com/deckhouse/virtualization/test/e2e/internal/observer/vmbda"
	"github.com/deckhouse/virtualization/test/e2e/internal/precheck"
	"github.com/deckhouse/virtualization/test/e2e/internal/util"
)

var _ = Describe("VirtualDiskSnapshots", Label(label.SIGStorage, precheck.PrecheckDefaultStorageClass, precheck.PrecheckSnapshot), func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	It("validates snapshot lifecycle for running VM's disk", func() {
		f := framework.NewFramework("virtual-disk-snapshots-running-vm-disk")
		f.Before()
		DeferCleanup(f.After)

		By("Environment preparation")
		// Long disk name (>60 chars, the former limit) to exercise snapshotting a
		// disk whose name uses the full Kubernetes name length.
		vd := object.NewVDFromCVI("vd-"+strings.Repeat("a", 80), f.Namespace().Name, object.PrecreatedCVICustomBIOS, vdbuilder.WithSize(ptr.To(resource.MustParse(vdCreationImageSize))), vdbuilder.WithStorageClass(defaultStorageClass()))
		vm := object.NewMinimalVM("vm-", f.Namespace().Name,
			vmbuilder.WithName("vm"),
			vmbuilder.WithBlockDeviceRefs(v1alpha2.BlockDeviceSpecRef{
				Kind: v1alpha2.VirtualDiskKind,
				Name: vd.Name,
			}),
		)

		err := f.CreateWithDeferredDeletion(ctx, vd, vm)
		Expect(err).NotTo(HaveOccurred())

		vmObs := vmobs.StartObserver(ctx, f, vm)
		vmObs.Never(vmobs.BeFailed())
		err = vmObs.WaitFor(vmobs.BeAgentReady(), framework.LongTimeout)
		Expect(err).NotTo(HaveOccurred())

		By("Creating snapshot")

		vdSnapshot := generateVDSnapshot("vdsnapshot", vd)
		Expect(f.CreateWithDeferredDeletion(ctx, vdSnapshot)).To(Succeed())

		By("Waiting for ready snapshot phase")
		waitVDSnapshotsReady(ctx, f, framework.LongTimeout, vdSnapshot)

		By("Checking VirtualDiskSnapshot consistency")
		checkVdSnapshotConsistentlyAndReadyToUse(ctx, f, vdSnapshot)
		checkFilesystemFrozeForSnapshot(ctx, f, vdSnapshot)

		By("Ensuring the virtual machine filesystem is unfrozen")
		checkVMUnfrozen(vmObs)

		By("Ensuring the disk is attached to the VM")
		expectDisksAttached(vmObs, vd)
	})

	It("validates snapshots for a disk with no consumer", func() {
		f := framework.NewFramework("virtual-disk-snapshots-no-consumer")
		f.Before()
		DeferCleanup(f.After)

		By("Environment preparation")

		vd := object.NewVDFromCVI(
			"vd-no-consumer",
			f.Namespace().Name,
			object.PrecreatedCVICustomBIOS,
			vdbuilder.WithSize(ptr.To(resource.MustParse(vdCreationImageSize))),
			vdbuilder.WithStorageClass(defaultStorageClass()),
		)

		// With a WaitForFirstConsumer storage class the disk stays in the
		// WaitForFirstConsumer phase until a VM consumes it, so run a throwaway
		// VM to get the disk provisioned, then delete it to snapshot the disk
		// without a consumer.
		vm := object.NewMinimalVM("vm-", f.Namespace().Name,
			vmbuilder.WithName("vm-first-consumer"),
			vmbuilder.WithBlockDeviceRefs(v1alpha2.BlockDeviceSpecRef{
				Kind: v1alpha2.VirtualDiskKind,
				Name: vd.Name,
			}),
		)

		err := f.CreateWithDeferredDeletion(ctx, vd, vm)
		Expect(err).NotTo(HaveOccurred())

		vdObs := vdobs.StartObserver(ctx, f, vd)
		vdObs.Never(vdobs.BeFailed())
		err = vdObs.WaitFor(vdobs.BeReady(), framework.LongTimeout)
		Expect(err).NotTo(HaveOccurred())

		By("Deleting the VM so the disk has no consumer")
		Expect(f.Delete(ctx, vm)).To(Succeed())
		// The snapshot below must be taken while the disk has no consumer. Wait for
		// that actual precondition — the disk detaching — rather than for the VM
		// object to disappear: it is what the test needs and it does not gate on VM
		// teardown latency.
		err = vdObs.WaitFor(vdobs.BeDetached(), framework.LongTimeout)
		Expect(err).NotTo(HaveOccurred())

		By("Creating snapshot")

		vdSnapshot := generateVDSnapshot("vdsnapshot", vd)
		Expect(f.CreateWithDeferredDeletion(ctx, vdSnapshot)).To(Succeed())

		By("Waiting for ready snapshot phase")
		waitVDSnapshotsReady(ctx, f, framework.LongTimeout, vdSnapshot)

		By("Checking VirtualDiskSnapshot consistency")
		checkVdSnapshotConsistentlyAndReadyToUse(ctx, f, vdSnapshot)
	})

	It("validates snapshots for a hotplug scenario", func() {
		f := framework.NewFramework("virtual-disk-snapshots-hotplug")
		f.Before()
		DeferCleanup(f.After)

		By("Environment preparation")

		vdRoot := object.NewVDFromCVI("vd-root", f.Namespace().Name, object.PrecreatedCVICustomBIOS, vdbuilder.WithSize(ptr.To(resource.MustParse(vdCreationImageSize))), vdbuilder.WithStorageClass(defaultStorageClass()))
		vdAttach := object.NewBlankVD("vd-attach", f.Namespace().Name, defaultStorageClass(), ptr.To(resource.MustParse(vdCreationImageSize)))

		vm := object.NewMinimalVM("vm-", f.Namespace().Name,
			vmbuilder.WithName("vm-hotplug"),
			vmbuilder.WithBlockDeviceRefs(v1alpha2.BlockDeviceSpecRef{
				Kind: v1alpha2.VirtualDiskKind,
				Name: vdRoot.Name,
			}),
		)
		vmbda := object.NewVMBDAFromDisk("vd-attach-attachment", vm.Name, vdAttach)

		err := f.CreateWithDeferredDeletion(ctx, vdRoot, vdAttach, vm, vmbda)
		Expect(err).NotTo(HaveOccurred())

		vmObs := vmobs.StartObserver(ctx, f, vm)
		vmObs.Never(vmobs.BeFailed())

		vmbdaObs := vmbdaobs.StartObserver(ctx, f, vmbda)
		vmbdaObs.Never(vmbdaobs.BeFailed())

		err = vmObs.WaitFor(vmobs.BeAgentReady(), framework.LongTimeout)
		Expect(err).NotTo(HaveOccurred())
		err = vmbdaObs.WaitFor(vmbdaobs.BeAttached(), framework.LongTimeout)
		Expect(err).NotTo(HaveOccurred())

		// One snapshot at a time: a virtual machine is held by one snapshot, and a second one taken while the
		// first is in progress is refused (see "validates concurrent snapshots").
		for _, vd := range []*v1alpha2.VirtualDisk{vdRoot, vdAttach} {
			By(fmt.Sprintf("Taking a snapshot of %s", vd.Name))
			vdSnapshot := generateVDSnapshot("vdsnapshot-"+vd.Name, vd)
			Expect(f.CreateWithDeferredDeletion(ctx, vdSnapshot)).To(Succeed())

			waitVDSnapshotsReady(ctx, f, framework.LongTimeout, vdSnapshot)
			checkVdSnapshotConsistentlyAndReadyToUse(ctx, f, vdSnapshot)
			checkFilesystemFrozeForSnapshot(ctx, f, vdSnapshot)
			checkVMUnfrozen(vmObs)
		}

		By("Ensuring disks are attached to the VM")
		expectDisksAttached(vmObs, vdRoot, vdAttach)
	})

	It("validates concurrent snapshots", func() {
		f := framework.NewFramework("virtual-disk-snapshots-concurrent")
		f.Before()
		DeferCleanup(f.After)

		By("Environment preparation")

		vdRoot := object.NewVDFromCVI("vd-root", f.Namespace().Name, object.PrecreatedCVICustomBIOS, vdbuilder.WithSize(ptr.To(resource.MustParse(vdCreationImageSize))), vdbuilder.WithStorageClass(defaultStorageClass()))
		vdAttach := object.NewBlankVD("vd-attach", f.Namespace().Name, defaultStorageClass(), ptr.To(resource.MustParse(vdCreationImageSize)))

		vm := object.NewMinimalVM("vm-", f.Namespace().Name,
			vmbuilder.WithName("vm-concurrent"),
			vmbuilder.WithBlockDeviceRefs(v1alpha2.BlockDeviceSpecRef{
				Kind: v1alpha2.VirtualDiskKind,
				Name: vdRoot.Name,
			}),
		)
		vmbda := object.NewVMBDAFromDisk("vd-attach-attachment", vm.Name, vdAttach)

		err := f.CreateWithDeferredDeletion(ctx, vdRoot, vdAttach, vm, vmbda)
		Expect(err).NotTo(HaveOccurred())

		vmObs := vmobs.StartObserver(ctx, f, vm)
		vmObs.Never(vmobs.BeFailed())

		vmbdaObs := vmbdaobs.StartObserver(ctx, f, vmbda)
		vmbdaObs.Never(vmbdaobs.BeFailed())

		err = vmObs.WaitFor(vmobs.BeAgentReady(), framework.LongTimeout)
		Expect(err).NotTo(HaveOccurred())
		err = vmbdaObs.WaitFor(vmbdaobs.BeAttached(), framework.LongTimeout)
		Expect(err).NotTo(HaveOccurred())

		By("Creating snapshots")

		vdSnapshots := concurentlyVDSnapshotsCreation(ctx, f, []*v1alpha2.VirtualDisk{vdRoot, vdAttach}, 5)

		// The outcome is checked by its properties, not by its order: whether a snapshot finds the virtual
		// machine held depends on how fast the storage captures the others. Every snapshot finishes (no
		// deadlock), a refused one is refused because another snapshot holds the machine, and every one taken
		// is consistent.
		By("Waiting for every snapshot to finish")
		// 10 concurrent snapshots are processed nearly sequentially by the CSI
		// driver (LINSTOR lock contention), so the tail does not fit in MiddleTimeout.
		var ready int

		for _, vdSnapshot := range vdSnapshots {
			obs := vdsnapshotobs.StartObserver(ctx, f, vdSnapshot)
			Expect(obs.WaitFor(vdsnapshotobs.BeTerminal(), framework.LongTimeout)).To(Succeed(),
				"VirtualDiskSnapshot %s/%s should finish", vdSnapshot.Namespace, vdSnapshot.Name)

			actual := &v1alpha2.VirtualDiskSnapshot{}
			Expect(f.GenericClient().Get(ctx, crclient.ObjectKeyFromObject(vdSnapshot), actual)).To(Succeed())

			if actual.Status.Phase == v1alpha2.VirtualDiskSnapshotPhaseFailed {
				Expect(vdsnapshotobs.Reason(actual)).To(Equal(vdscondition.VirtualMachineBusy.String()),
					"VirtualDiskSnapshot %s/%s may only be refused because the virtual machine is held", actual.Namespace, actual.Name)

				continue
			}

			ready++

			checkVdSnapshotConsistentlyAndReadyToUse(ctx, f, vdSnapshot)
			checkFilesystemFrozeForSnapshot(ctx, f, vdSnapshot)
		}

		Expect(ready).To(BeNumerically(">", 0), "at least one snapshot should be taken")

		By("Ensuring the virtual machine filesystem is unfrozen")
		checkVMUnfrozen(vmObs)

		By("Ensuring disks are attached to the VM")
		expectDisksAttached(vmObs, vdRoot, vdAttach)
	})
})

func checkVdSnapshotConsistentlyAndReadyToUse(ctx context.Context, f *framework.Framework, vdSnapshot *v1alpha2.VirtualDiskSnapshot) {
	GinkgoHelper()

	key := crclient.ObjectKeyFromObject(vdSnapshot)
	actualVDSnapshot := &v1alpha2.VirtualDiskSnapshot{}
	err := f.GenericClient().Get(ctx, key, actualVDSnapshot)
	Expect(err).NotTo(HaveOccurred())

	Expect(actualVDSnapshot.Status.Consistent).NotTo(BeNil(), "VirtualDiskSnapshot status.consistent must be set")
	Expect(*actualVDSnapshot.Status.Consistent).To(BeTrue(), "VirtualDiskSnapshot status.consistent must be true")
	// The unified snapshotter records the captured data in status.data, the built-in mechanism records the
	// VolumeSnapshot it created. The mechanism is told by status.captureState, which only the unified
	// snapshotter sets: the annotation is an optional pin, and the cluster default leaves it unset.
	if actualVDSnapshot.Status.CaptureState != nil {
		Expect(actualVDSnapshot.Status.Data).NotTo(BeNil(), "VirtualDiskSnapshot status.data must be set")
	} else {
		Expect(actualVDSnapshot.Status.VolumeSnapshotName).NotTo(BeEmpty(), "VirtualDiskSnapshot status.volumeSnapshotName must be set")
	}
}

// checkFilesystemFrozeForSnapshot checks that the guest filesystem was frozen while the snapshot was taken.
// It reads status.frozen of the snapshot's access claim, which the coordinator sets once the guest confirms
// the freeze and keeps for the life of the snapshot. The FilesystemFrozen condition of the virtual machine is
// not used: it follows the guest agent poll, which a freeze of a few seconds can fall between.
func checkFilesystemFrozeForSnapshot(ctx context.Context, f *framework.Framework, vdSnapshot *v1alpha2.VirtualDiskSnapshot) {
	GinkgoHelper()

	actual := &v1alpha2.VirtualDiskSnapshot{}
	Expect(f.GenericClient().Get(ctx, crclient.ObjectKeyFromObject(vdSnapshot), actual)).To(Succeed())

	// The controller names the claim of a standalone disk snapshot "vds-<snapshot UID>".
	claim := &coordinationv1alpha1.VirtualMachineAccessClaim{}
	key := crclient.ObjectKey{Namespace: actual.Namespace, Name: "vds-" + string(actual.UID)}
	Expect(f.GenericClient().Get(ctx, key, claim)).To(Succeed(), "the access claim of VirtualDiskSnapshot %s/%s must exist", actual.Namespace, actual.Name)
	Expect(claim.Status.Frozen).To(BeTrue(), "the VM filesystem should be frozen while VirtualDiskSnapshot %s/%s is taken", actual.Namespace, actual.Name)
}

// waitVDSnapshotsReady waits for every snapshot to reach the Ready phase via an
// Observer per snapshot.
func waitVDSnapshotsReady(ctx context.Context, f *framework.Framework, timeout time.Duration, snapshots ...*v1alpha2.VirtualDiskSnapshot) {
	GinkgoHelper()

	for _, snapshot := range snapshots {
		obs := vdsnapshotobs.StartObserver(ctx, f, snapshot)
		err := obs.WaitFor(vdsnapshotobs.BeReady(), timeout)
		skipIfCSISnapshotFailed(err)
		Expect(err).NotTo(HaveOccurred(),
			"VirtualDiskSnapshot %s/%s should become Ready", snapshot.Namespace, snapshot.Name)
	}
}

// skipIfCSISnapshotFailed steps aside when a snapshot failed because the CSI
// driver could not create the underlying VolumeSnapshot. On DRBD that happens
// intermittently - the driver reports the resource as being in an unknown state
// or refuses to suspend IO on the layer - and it is a storage-infrastructure
// problem rather than a virtualization one. Any other failure still fails the
// spec.
func skipIfCSISnapshotFailed(err error) {
	GinkgoHelper()

	if err != nil && util.IsCSIVolumeSnapshotError(err.Error()) {
		Skip(fmt.Sprintf("VirtualDiskSnapshot failed on the CSI side, skipping: %s", err))
	}
}

// expectDisksAttached waits, via the VirtualMachine Observer, until every disk
// appears attached in the VM status.
func expectDisksAttached(vmObs vmobs.Observer, vds ...*v1alpha2.VirtualDisk) {
	GinkgoHelper()

	names := make([]string, 0, len(vds))
	for _, d := range vds {
		names = append(names, d.Name)
	}

	err := vmObs.WaitFor(vmobs.HaveBlockDevicesAttached(names...), framework.ShortTimeout)
	Expect(err).NotTo(HaveOccurred())
}

func generateVDSnapshot(name string, vd *v1alpha2.VirtualDisk) *v1alpha2.VirtualDiskSnapshot {
	return vdsnapshotbuilder.New(
		vdsnapshotbuilder.WithName(name),
		vdsnapshotbuilder.WithNamespace(vd.Namespace),
		vdsnapshotbuilder.WithVirtualDiskName(vd.Name),
		vdsnapshotbuilder.WithRequiredConsistency(true),
	)
}

// checkVMUnfrozen waits for the guest to be released: the unfreeze is confirmed asynchronously after the
// snapshot is Ready, and the FilesystemFrozen condition follows it.
func checkVMUnfrozen(vmObs vmobs.Observer) {
	GinkgoHelper()

	err := vmObs.WaitFor(vmobs.BeFilesystemUnfrozen(), framework.MiddleTimeout)
	Expect(err).NotTo(HaveOccurred(), "the virtual machine filesystem must be unfrozen")
}

func concurentlyVDSnapshotsCreation(ctx context.Context, f *framework.Framework, vds []*v1alpha2.VirtualDisk, cnt int) []*v1alpha2.VirtualDiskSnapshot {
	GinkgoHelper()

	var vdSnapshots []*v1alpha2.VirtualDiskSnapshot

	for i := 1; i <= cnt; i++ {
		for _, vd := range vds {
			vdSnapshots = append(vdSnapshots, generateVDSnapshot(
				fmt.Sprintf("vdsnapshot-%s-%d", vd.Name, i),
				vd,
			))
		}
	}

	var wg sync.WaitGroup
	wg.Add(len(vds) * cnt)

	for _, vdSnapshot := range vdSnapshots {
		go func(vdSnapshot *v1alpha2.VirtualDiskSnapshot) {
			GinkgoRecover()

			defer wg.Done()

			err := f.GenericClient().Create(ctx, vdSnapshot)
			Expect(err).NotTo(HaveOccurred())
		}(vdSnapshot)
	}

	wg.Wait()

	for _, vdSnapshot := range vdSnapshots {
		f.DeferDelete(vdSnapshot)
	}

	return vdSnapshots
}
