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
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	"github.com/deckhouse/virtualization/test/e2e/internal/object"
	"github.com/deckhouse/virtualization/test/e2e/internal/precheck"
	"github.com/deckhouse/virtualization/test/e2e/internal/util"
)

var _ = Describe("VirtualDiskSnapshots", Label(precheck.PrecheckImmediateStorageClass, precheck.PrecheckSnapshot), func() {
	var (
		ctx context.Context
		cfg *config.Config
	)

	BeforeEach(func() {
		ctx = context.Background()

		cfg = framework.GetConfig()
		if cfg.StorageClass.TemplateStorageClass != nil && cfg.StorageClass.TemplateStorageClass.Provisioner == config.NFS {
			Skip("Concurrent snapshotting is not supported on NFS on the VolumeSnapshot side, skipping")
		}
	})

	It("validates snapshot lifecycle for a single VM", func() {
		f := framework.NewFramework("virtual-disk-snapshots-single-vm")
		f.Before()
		DeferCleanup(f.After)

		By("Environment preparation")
		vd := object.NewVDFromCVI("vd", f.Namespace().Name, object.PrecreatedCVIUbuntu)
		vm := object.NewMinimalVM("vm-", f.Namespace().Name,
			vmbuilder.WithName("vm"),
			vmbuilder.WithBlockDeviceRefs(v1alpha2.BlockDeviceSpecRef{
				Kind: v1alpha2.VirtualDiskKind,
				Name: vd.Name,
			}),
		)

		err := f.CreateWithDeferredDeletion(ctx, vd, vm)
		Expect(err).NotTo(HaveOccurred())

		util.UntilVMAgentReady(ctx, crclient.ObjectKeyFromObject(vm), framework.LongTimeout)

		By("Creating snapshot")
		vdSnapshot := generateVDSnapshot("vdsnapshot", vd)

		err = f.CreateWithDeferredDeletion(ctx, vdSnapshot)
		Expect(err).NotTo(HaveOccurred())
		ensureVMWasFrozen(ctx, f, vm, framework.MiddleTimeout)

		By("Waiting for ready snapshot phase")
		util.UntilObjectPhase(ctx, string(v1alpha2.VirtualDiskSnapshotPhaseReady), framework.MiddleTimeout, vdSnapshot)

		By("Checking VirtualDiskSnapshot consistency")
		checkVdSnapshotConsistentlyAndReadyToUse(ctx, f, vdSnapshot)

		By("Ensuring the virtual machine filesystem is unfrozen")
		checkVMUnfrozen(ctx, f, vm)

		By("Ensuring the disk is attached to the VM")
		util.UntilDisksAreAttachedInVMStatus(ctx, f, framework.ShortTimeout, vm, vd)
	})

	It("validates snapshots for a disk with no consumer", func() {
		f := framework.NewFramework("virtual-disk-snapshots-no-consumer")
		f.Before()
		DeferCleanup(f.After)

		By("Environment preparation")
		vd := object.NewVDFromCVI(
			"vd-no-consumer",
			f.Namespace().Name,
			object.PrecreatedCVIAlpineBIOS,
			vdbuilder.WithStorageClass(ptr.To(cfg.StorageClass.ImmediateStorageClass.Name)),
		)

		err := f.CreateWithDeferredDeletion(ctx, vd)
		Expect(err).NotTo(HaveOccurred())

		util.UntilObjectPhase(ctx, string(v1alpha2.DiskReady), framework.LongTimeout, vd)

		By("Creating snapshot")
		vdSnapshot := generateVDSnapshot("vdsnapshot", vd)

		err = f.CreateWithDeferredDeletion(ctx, vdSnapshot)
		Expect(err).NotTo(HaveOccurred())

		By("Waiting for ready snapshot phase")
		util.UntilObjectPhase(ctx, string(v1alpha2.VirtualDiskSnapshotPhaseReady), framework.MiddleTimeout, vdSnapshot)

		By("Checking VirtualDiskSnapshot consistency")
		checkVdSnapshotConsistentlyAndReadyToUse(ctx, f, vdSnapshot)
	})

	It("validates snapshots for a hotplug scenario", func() {
		f := framework.NewFramework("virtual-disk-snapshots-hotplug")
		f.Before()
		DeferCleanup(f.After)

		By("Environment preparation")
		vdRoot := object.NewVDFromCVI("vd-root", f.Namespace().Name, object.PrecreatedCVIUbuntu)
		vdAttach := object.NewBlankVD("vd-attach", f.Namespace().Name, nil, ptr.To(resource.MustParse("100Mi")))

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

		util.UntilVMAgentReady(ctx, crclient.ObjectKeyFromObject(vm), framework.LongTimeout)
		util.UntilObjectPhase(ctx, string(v1alpha2.BlockDeviceAttachmentPhaseAttached), framework.MiddleTimeout, vmbda)

		By("Creating snapshots")
		vdSnapshotRoot := generateVDSnapshot("vdsnapshot-root", vdRoot)
		vdSnapshotAttach := generateVDSnapshot("vdsnapshot-attach", vdAttach)

		err = f.CreateWithDeferredDeletion(ctx, vdSnapshotRoot, vdSnapshotAttach)
		Expect(err).NotTo(HaveOccurred())
		ensureVMWasFrozen(ctx, f, vm, framework.MiddleTimeout)

		By("Waiting for ready snapshots phase")
		util.UntilObjectPhase(ctx, string(v1alpha2.VirtualDiskSnapshotPhaseReady), framework.MiddleTimeout, vdSnapshotRoot, vdSnapshotAttach)

		By("Checking VirtualDiskSnapshots consistency")
		checkVdSnapshotConsistentlyAndReadyToUse(ctx, f, vdSnapshotRoot)
		checkVdSnapshotConsistentlyAndReadyToUse(ctx, f, vdSnapshotAttach)

		By("Ensuring the virtual machine filesystem is unfrozen")
		checkVMUnfrozen(ctx, f, vm)

		By("Ensuring disks are attached to the VM")
		util.UntilDisksAreAttachedInVMStatus(ctx, f, framework.ShortTimeout, vm, vdRoot, vdAttach)
	})

	It("validates concurrent snapshots", func() {
		f := framework.NewFramework("virtual-disk-snapshots-concurrent")
		f.Before()
		DeferCleanup(f.After)

		By("Environment preparation")
		vdRoot := object.NewVDFromCVI("vd-root", f.Namespace().Name, object.PrecreatedCVIUbuntu)
		vdAttach := object.NewBlankVD("vd-attach", f.Namespace().Name, nil, ptr.To(resource.MustParse("100Mi")))

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

		util.UntilVMAgentReady(ctx, crclient.ObjectKeyFromObject(vm), framework.LongTimeout)
		util.UntilObjectPhase(ctx, string(v1alpha2.BlockDeviceAttachmentPhaseAttached), framework.MiddleTimeout, vmbda)

		By("Creating snapshots")
		vdSnapshots := concurentlyVDSnapshotsCreation(ctx, f, []*v1alpha2.VirtualDisk{vdRoot, vdAttach}, 5)
		ensureVMWasFrozen(ctx, f, vm, framework.MiddleTimeout)

		By("Waiting for ready snapshots phase")
		util.UntilObjectPhase(ctx, string(v1alpha2.VirtualDiskSnapshotPhaseReady), framework.MiddleTimeout, util.ToObjects(vdSnapshots)...)

		By("Checking VirtualDiskSnapshots consistency")
		for _, vdSnapshot := range vdSnapshots {
			checkVdSnapshotConsistentlyAndReadyToUse(ctx, f, vdSnapshot)
		}

		By("Ensuring the virtual machine filesystem is unfrozen")
		checkVMUnfrozen(ctx, f, vm)

		By("Ensuring disks are attached to the VM")
		util.UntilDisksAreAttachedInVMStatus(ctx, f, framework.ShortTimeout, vm, vdRoot, vdAttach)
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

func ensureVMWasFrozen(ctx context.Context, f *framework.Framework, vm *v1alpha2.VirtualMachine, timeout time.Duration) {
	GinkgoHelper()

	Eventually(func() error {
		var currentVM v1alpha2.VirtualMachine
		err := f.GenericClient().Get(ctx, crclient.ObjectKeyFromObject(vm), &currentVM)
		if err != nil {
			return err
		}

		frozenCondition, ok := conditions.GetCondition(vmcondition.TypeFilesystemFrozen, currentVM.Status.Conditions)
		if !ok {
			return fmt.Errorf("filesystem frozen condition not found")
		}
		if frozenCondition.Status != metav1.ConditionTrue {
			return fmt.Errorf("filesystem frozen condition is not true")
		}

		return nil
	}).WithTimeout(timeout).WithPolling(time.Second).Should(Succeed())
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
