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
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
	"github.com/deckhouse/virtualization/test/e2e/internal/config"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	"github.com/deckhouse/virtualization/test/e2e/internal/label"
	"github.com/deckhouse/virtualization/test/e2e/internal/object"
	"github.com/deckhouse/virtualization/test/e2e/internal/observer"
	vdobs "github.com/deckhouse/virtualization/test/e2e/internal/observer/vd"
	vdsnapshotobs "github.com/deckhouse/virtualization/test/e2e/internal/observer/vdsnapshot"
	vmobs "github.com/deckhouse/virtualization/test/e2e/internal/observer/vm"
	vmbdaobs "github.com/deckhouse/virtualization/test/e2e/internal/observer/vmbda"
	"github.com/deckhouse/virtualization/test/e2e/internal/precheck"
	"github.com/deckhouse/virtualization/test/e2e/internal/util"
)

var _ = label.SIGDescribe(label.SIGStorage, "VirtualDiskSnapshots", Label(precheck.PrecheckDefaultStorageClass, precheck.PrecheckSnapshot), func() {
	var (
		ctx context.Context
		cfg *config.Config
	)

	BeforeEach(func() {
		ctx = context.Background()

		cfg = framework.GetConfig()
		if cfg.StorageClass.DefaultStorageClass != nil && cfg.StorageClass.DefaultStorageClass.Provisioner == config.NFS {
			Skip("Concurrent snapshotting is not supported on NFS on the VolumeSnapshot side, skipping")
		}
	})

	It("validates snapshot lifecycle for running VM's disk", func() {
		f := framework.NewFramework("virtual-disk-snapshots-running-vm-disk")
		f.Before()
		DeferCleanup(f.After)

		By("Environment preparation")
		// Long disk name (>60 chars, the former limit) to exercise snapshotting a
		// disk whose name uses the full Kubernetes name length.
		vd := object.NewVDFromCVI("vd-"+strings.Repeat("a", 80), f.Namespace().Name, object.PrecreatedCVICustomBIOS)
		vm := object.NewMinimalVM("vm-", f.Namespace().Name,
			vmbuilder.WithName("vm"),
			vmbuilder.WithProvisioning(nil),
			vmbuilder.WithBlockDeviceRefs(v1alpha2.BlockDeviceSpecRef{
				Kind: v1alpha2.VirtualDiskKind,
				Name: vd.Name,
			}),
		)

		err := f.CreateWithDeferredDeletion(ctx, vd, vm)
		Expect(err).NotTo(HaveOccurred())

		vmObs := vmobs.StartObserver(ctx, f, vm)
		vmObs.Never(vmobs.BeFailed())
		Expect(vmObs.WaitFor(vmobs.BeAgentReady(), framework.LongTimeout)).To(Succeed())

		By("Creating snapshot")
		vdSnapshot := generateVDSnapshot("vdsnapshot", vd)
		frozen := expectFilesystemFroze(vmObs)
		Expect(f.CreateWithDeferredDeletion(ctx, vdSnapshot)).To(Succeed())
		Expect(<-frozen).To(Succeed(), "the VM filesystem should freeze during the snapshot")

		By("Waiting for ready snapshot phase")
		waitVDSnapshotsReady(ctx, f, framework.MiddleTimeout, vdSnapshot)

		By("Checking VirtualDiskSnapshot consistency")
		checkVdSnapshotConsistentlyAndReadyToUse(ctx, f, vdSnapshot)

		By("Ensuring the virtual machine filesystem is unfrozen")
		checkVMUnfrozen(ctx, f, vm)

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
			vdbuilder.WithSize(ptr.To(resource.MustParse("400Mi"))),
			vdbuilder.WithStorageClass(ptr.To(cfg.StorageClass.DefaultStorageClass.Name)),
		)

		// With a WaitForFirstConsumer storage class the disk stays in the
		// WaitForFirstConsumer phase until a VM consumes it, so run a throwaway
		// VM to get the disk provisioned, then delete it to snapshot the disk
		// without a consumer.
		vm := object.NewMinimalVM("vm-", f.Namespace().Name,
			vmbuilder.WithName("vm-first-consumer"),
			vmbuilder.WithProvisioning(nil),
			vmbuilder.WithBlockDeviceRefs(v1alpha2.BlockDeviceSpecRef{
				Kind: v1alpha2.VirtualDiskKind,
				Name: vd.Name,
			}),
		)

		err := f.CreateWithDeferredDeletion(ctx, vd, vm)
		Expect(err).NotTo(HaveOccurred())

		vdObs := vdobs.StartObserver(ctx, f, vd)
		vdObs.Never(vdobs.BeFailed())
		Expect(vdObs.WaitFor(vdobs.BeReady(), framework.LongTimeout)).To(Succeed())

		By("Deleting the VM so the disk has no consumer")
		Expect(f.Delete(ctx, vm)).To(Succeed())
		// VM deletion stops the guest, terminates the virt-launcher pod and waits
		// for the KVVMI and pvc-protection finalizers to clear before the VM object
		// is gone, so allow the long timeout rather than the middle one.
		Expect(observer.WaitForDeleted(ctx, f.VirtClient().VirtualMachines(vm.Namespace), vm.Name, vm.Namespace, framework.LongTimeout, nil)).To(Succeed())

		By("Creating snapshot")
		vdSnapshot := generateVDSnapshot("vdsnapshot", vd)
		Expect(f.CreateWithDeferredDeletion(ctx, vdSnapshot)).To(Succeed())

		By("Waiting for ready snapshot phase")
		waitVDSnapshotsReady(ctx, f, framework.MiddleTimeout, vdSnapshot)

		By("Checking VirtualDiskSnapshot consistency")
		checkVdSnapshotConsistentlyAndReadyToUse(ctx, f, vdSnapshot)
	})

	It("validates snapshots for a hotplug scenario", func() {
		f := framework.NewFramework("virtual-disk-snapshots-hotplug")
		f.Before()
		DeferCleanup(f.After)

		By("Environment preparation")
		vdRoot := object.NewVDFromCVI("vd-root", f.Namespace().Name, object.PrecreatedCVICustomBIOS)
		vdAttach := object.NewBlankVD("vd-attach", f.Namespace().Name, nil, ptr.To(resource.MustParse("100Mi")))

		vm := object.NewMinimalVM("vm-", f.Namespace().Name,
			vmbuilder.WithName("vm-hotplug"),
			vmbuilder.WithProvisioning(nil),
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
		Expect(vmObs.WaitFor(vmobs.BeAgentReady(), framework.LongTimeout)).To(Succeed())
		Expect(vmbdaObs.WaitFor(vmbdaobs.BeAttached(), framework.MiddleTimeout)).To(Succeed())

		By("Creating snapshots")
		vdSnapshotRoot := generateVDSnapshot("vdsnapshot-root", vdRoot)
		vdSnapshotAttach := generateVDSnapshot("vdsnapshot-attach", vdAttach)
		frozen := expectFilesystemFroze(vmObs)
		Expect(f.CreateWithDeferredDeletion(ctx, vdSnapshotRoot, vdSnapshotAttach)).To(Succeed())
		Expect(<-frozen).To(Succeed(), "the VM filesystem should freeze during the snapshot")

		By("Waiting for ready snapshots phase")
		waitVDSnapshotsReady(ctx, f, framework.MiddleTimeout, vdSnapshotRoot, vdSnapshotAttach)

		By("Checking VirtualDiskSnapshots consistency")
		checkVdSnapshotConsistentlyAndReadyToUse(ctx, f, vdSnapshotRoot)
		checkVdSnapshotConsistentlyAndReadyToUse(ctx, f, vdSnapshotAttach)

		By("Ensuring the virtual machine filesystem is unfrozen")
		checkVMUnfrozen(ctx, f, vm)

		By("Ensuring disks are attached to the VM")
		expectDisksAttached(vmObs, vdRoot, vdAttach)
	})

	It("validates concurrent snapshots", func() {
		f := framework.NewFramework("virtual-disk-snapshots-concurrent")
		f.Before()
		DeferCleanup(f.After)

		By("Environment preparation")
		vdRoot := object.NewVDFromCVI("vd-root", f.Namespace().Name, object.PrecreatedCVICustomBIOS)
		vdAttach := object.NewBlankVD("vd-attach", f.Namespace().Name, nil, ptr.To(resource.MustParse("100Mi")))

		vm := object.NewMinimalVM("vm-", f.Namespace().Name,
			vmbuilder.WithName("vm-concurrent"),
			vmbuilder.WithProvisioning(nil),
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
		Expect(vmObs.WaitFor(vmobs.BeAgentReady(), framework.LongTimeout)).To(Succeed())
		Expect(vmbdaObs.WaitFor(vmbdaobs.BeAttached(), framework.MiddleTimeout)).To(Succeed())

		By("Creating snapshots")
		frozen := expectFilesystemFroze(vmObs)
		vdSnapshots := concurentlyVDSnapshotsCreation(ctx, f, []*v1alpha2.VirtualDisk{vdRoot, vdAttach}, 5)
		Expect(<-frozen).To(Succeed(), "the VM filesystem should freeze during the snapshot")

		By("Waiting for ready snapshots phase")
		// 10 concurrent snapshots are processed nearly sequentially by the CSI
		// driver (LINSTOR lock contention), so the tail does not fit in MiddleTimeout.
		waitVDSnapshotsReady(ctx, f, framework.LongTimeout, vdSnapshots...)

		By("Checking VirtualDiskSnapshots consistency")
		for _, vdSnapshot := range vdSnapshots {
			checkVdSnapshotConsistentlyAndReadyToUse(ctx, f, vdSnapshot)
		}

		By("Ensuring the virtual machine filesystem is unfrozen")
		checkVMUnfrozen(ctx, f, vm)

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
	Expect(actualVDSnapshot.Status.VolumeSnapshotName).NotTo(BeEmpty(), "VirtualDiskSnapshot status.volumeSnapshotName must be set")
}

// expectFilesystemFroze observes the transient FilesystemFrozen condition on vm.
// Call it BEFORE creating the snapshot so the freeze is not missed, then read
// the returned channel after creating the snapshot.
func expectFilesystemFroze(vmObs vmobs.Observer) <-chan error {
	ch := make(chan error, 1)
	go func() {
		defer GinkgoRecover()
		ch <- vmObs.WaitFor(vmobs.BeFilesystemFrozen(), framework.MiddleTimeout)
	}()
	return ch
}

// waitVDSnapshotsReady waits for every snapshot to reach the Ready phase via an
// Observer per snapshot.
func waitVDSnapshotsReady(ctx context.Context, f *framework.Framework, timeout time.Duration, snapshots ...*v1alpha2.VirtualDiskSnapshot) {
	GinkgoHelper()
	for _, snapshot := range snapshots {
		obs := vdsnapshotobs.StartObserver(ctx, f, snapshot)
		Expect(obs.WaitFor(vdsnapshotobs.BeReady(), timeout)).To(Succeed(),
			"VirtualDiskSnapshot %s/%s should become Ready", snapshot.Namespace, snapshot.Name)
	}
}

// expectDisksAttached waits, via the VirtualMachine Observer, until every disk
// appears attached in the VM status.
func expectDisksAttached(vmObs vmobs.Observer, vds ...*v1alpha2.VirtualDisk) {
	GinkgoHelper()
	Expect(vmObs.WaitFor(func(m *v1alpha2.VirtualMachine) (bool, error) {
		for _, d := range vds {
			if !util.IsVDAttached(m, d) {
				return false, nil
			}
		}
		return true, nil
	}, framework.ShortTimeout)).To(Succeed())
}

func generateVDSnapshot(name string, vd *v1alpha2.VirtualDisk) *v1alpha2.VirtualDiskSnapshot {
	return vdsnapshotbuilder.New(
		vdsnapshotbuilder.WithName(name),
		vdsnapshotbuilder.WithNamespace(vd.Namespace),
		vdsnapshotbuilder.WithVirtualDiskName(vd.Name),
		vdsnapshotbuilder.WithRequiredConsistency(true),
	)
}

func checkVMUnfrozen(ctx context.Context, f *framework.Framework, vm *v1alpha2.VirtualMachine) {
	GinkgoHelper()

	var currentVM v1alpha2.VirtualMachine
	err := f.GenericClient().Get(ctx, crclient.ObjectKeyFromObject(vm), &currentVM)
	Expect(err).NotTo(HaveOccurred())

	_, frozenConditionExists := conditions.GetCondition(vmcondition.TypeFilesystemFrozen, currentVM.Status.Conditions)
	Expect(frozenConditionExists).To(BeFalse(), "frozen condition must not exist")
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
