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
	"sync/atomic"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
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

const vdSnapshotNSPrefix = "virtual-disk-snapshots"

var _ = Describe("VirtualDiskSnapshots", Label(precheck.PrecheckImmediateStorageClass, precheck.PrecheckSnapshot), func() {
	var (
		f   *framework.Framework
		ctx context.Context
		cfg *config.Config
	)

	BeforeEach(func() {
		ctx = context.Background()
		f = framework.NewFramework(vdSnapshotNSPrefix)
		cfg = framework.GetConfig()
		if cfg.StorageClass.TemplateStorageClass != nil && cfg.StorageClass.TemplateStorageClass.Provisioner == config.NFS {
			Skip("Concurrent snapshotting is not supported on NFS on the VolumeSnapshot side, skipping")
		}
		f.Before()
		DeferCleanup(f.After)
	})

	It("validates snapshots for a plain VM scenario", func() {
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
		cancelVMWatch, vmWasFrozen, vmWatchErrCh := startVMFreezeWatch(ctx, f, vm)
		defer cancelVMWatch()

		vdSnapshot := generateVDSnapshot("vdsnapshot", vd)

		err = f.CreateWithDeferredDeletion(ctx, vdSnapshot)
		Expect(err).NotTo(HaveOccurred())
		Eventually(vmWasFrozen.Load).WithTimeout(framework.MiddleTimeout).WithPolling(time.Second).Should(BeTrue())
		cancelVMWatch()
		Expect(<-vmWatchErrCh).ShouldNot(HaveOccurred())

		By("Waiting for ready snapshot phase")
		util.UntilObjectPhase(ctx, string(v1alpha2.VirtualDiskSnapshotPhaseReady), framework.MiddleTimeout, vdSnapshot)

		By("Checking VirtualDiskSnapshot consistency and VolumeSnapshot readiness")
		checkVdSnapshotConsistentlyAndReadyToUse(ctx, f, vdSnapshot)

		By("Ensuring the virtual machine filesystem is unfrozen")
		checkVMUnfrozen(ctx, f, vm)

		By("Ensuring the disk is attached to the VM")
		checkDiskAttachedToVM(ctx, f, vm, vd)
	})

	It("validates snapshots for a disk with no consumer", func() {
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

		By("Checking VirtualDiskSnapshot consistency and VolumeSnapshot readiness")
		checkVdSnapshotConsistentlyAndReadyToUse(ctx, f, vdSnapshot)
	})

	It("validates snapshots for a hotplug scenario", func() {
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
		cancelVMWatch, vmWasFrozen, vmWatchErrCh := startVMFreezeWatch(ctx, f, vm)
		defer cancelVMWatch()

		vdSnapshotRoot := generateVDSnapshot("vdsnapshot-root", vdRoot)
		vdSnapshotAttach := generateVDSnapshot("vdsnapshot-attach", vdAttach)

		err = f.CreateWithDeferredDeletion(ctx, vdSnapshotRoot, vdSnapshotAttach)
		Expect(err).NotTo(HaveOccurred())
		Eventually(vmWasFrozen.Load).WithTimeout(framework.MiddleTimeout).WithPolling(time.Second).Should(BeTrue())
		cancelVMWatch()
		Expect(<-vmWatchErrCh).ShouldNot(HaveOccurred())

		By("Waiting for ready snapshots phase")
		util.UntilObjectPhase(ctx, string(v1alpha2.VirtualDiskSnapshotPhaseReady), framework.MiddleTimeout, vdSnapshotRoot, vdSnapshotAttach)

		By("Checking VirtualDiskSnapshots consistency and VolumeSnapshot readiness")
		checkVdSnapshotConsistentlyAndReadyToUse(ctx, f, vdSnapshotRoot)
		checkVdSnapshotConsistentlyAndReadyToUse(ctx, f, vdSnapshotAttach)

		By("Ensuring the virtual machine filesystem is unfrozen")
		checkVMUnfrozen(ctx, f, vm)

		By("Ensuring disks are attached to the VM")
		checkDiskAttachedToVM(ctx, f, vm, vdRoot)
		checkDiskAttachedToVM(ctx, f, vm, vdAttach)
	})

	It("validates concurrent snapshots", func() {
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
		cancelVMWatch, vmWasFrozen, vmWatchErrCh := startVMFreezeWatch(ctx, f, vm)
		defer cancelVMWatch()

		vdSnapshots := concurentlyVDSnapshotsCreation(ctx, f, []*v1alpha2.VirtualDisk{vdRoot, vdAttach}, 5)
		Eventually(vmWasFrozen.Load).WithTimeout(framework.MiddleTimeout).WithPolling(time.Second).Should(BeTrue())
		cancelVMWatch()
		Expect(<-vmWatchErrCh).ShouldNot(HaveOccurred())

		By("Waiting for ready snapshots phase")
		util.UntilObjectPhase(ctx, string(v1alpha2.VirtualDiskSnapshotPhaseReady), framework.MiddleTimeout, util.ToObjects(vdSnapshots)...)

		By("Checking VirtualDiskSnapshots consistency and VolumeSnapshot readiness")
		for _, vdSnapshot := range vdSnapshots {
			checkVdSnapshotConsistentlyAndReadyToUse(ctx, f, vdSnapshot)
		}

		By("Ensuring the virtual machine filesystem is unfrozen")
		checkVMUnfrozen(ctx, f, vm)

		By("Ensuring disks are attached to the VM")
		checkDiskAttachedToVM(ctx, f, vm, vdRoot)
		checkDiskAttachedToVM(ctx, f, vm, vdAttach)
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

	volumeSnapshot := &unstructured.Unstructured{}
	volumeSnapshot.SetAPIVersion("snapshot.storage.k8s.io/v1")
	volumeSnapshot.SetKind("VolumeSnapshot")
	err = f.GenericClient().Get(ctx, crclient.ObjectKey{
		Namespace: actualVDSnapshot.Namespace,
		Name:      actualVDSnapshot.Status.VolumeSnapshotName,
	}, volumeSnapshot)
	Expect(err).NotTo(HaveOccurred())

	readyToUse, found, err := unstructured.NestedBool(volumeSnapshot.Object, "status", "readyToUse")
	Expect(err).NotTo(HaveOccurred())
	Expect(found).To(BeTrue(), "VolumeSnapshot status.readyToUse must be present")
	Expect(readyToUse).To(BeTrue(), "VolumeSnapshot status.readyToUse must be true")
}

func generateVDSnapshot(name string, vd *v1alpha2.VirtualDisk, opts ...vdsnapshotbuilder.Option) *v1alpha2.VirtualDiskSnapshot {
	baseOpts := []vdsnapshotbuilder.Option{
		vdsnapshotbuilder.WithName(name),
		vdsnapshotbuilder.WithNamespace(vd.Namespace),
		vdsnapshotbuilder.WithVirtualDiskName(vd.Name),
		vdsnapshotbuilder.WithRequiredConsistency(true),
	}
	baseOpts = append(baseOpts, opts...)

	return vdsnapshotbuilder.New(baseOpts...)
}

func checkVMUnfrozen(ctx context.Context, f *framework.Framework, vm *v1alpha2.VirtualMachine) {
	GinkgoHelper()

	var currentVM v1alpha2.VirtualMachine
	err := f.GenericClient().Get(ctx, crclient.ObjectKeyFromObject(vm), &currentVM)
	Expect(err).NotTo(HaveOccurred())

	_, frozenConditionExists := conditions.GetCondition(vmcondition.TypeFilesystemFrozen, currentVM.Status.Conditions)
	Expect(frozenConditionExists).To(BeFalse(), "frozen condition must not exist")
}

func checkDiskAttachedToVM(ctx context.Context, f *framework.Framework, vm *v1alpha2.VirtualMachine, vd *v1alpha2.VirtualDisk) {
	GinkgoHelper()

	var currentVM v1alpha2.VirtualMachine
	err := f.GenericClient().Get(ctx, crclient.ObjectKeyFromObject(vm), &currentVM)
	Expect(err).NotTo(HaveOccurred())

	Expect(util.IsVDAttached(&currentVM, vd)).To(BeTrue(), "disk must be present in VM status")
}

// ensureVMWasFrozenInProgress watches VM events and returns true once the tracked
// VM reaches FilesystemFrozen=True before context cancellation.
func ensureVMWasFrozenInProgress(ctx context.Context, w util.Watcher, vm *v1alpha2.VirtualMachine) (bool, error) {
	if vm == nil || vm.Name == "" {
		return false, fmt.Errorf("tracked virtual machine is nil or has an empty name")
	}

	frozenCondition, frozenConditionExists := conditions.GetCondition(vmcondition.TypeFilesystemFrozen, vm.Status.Conditions)
	if frozenConditionExists && frozenCondition.Status == metav1.ConditionTrue {
		return true, nil
	}

	wi, err := w.Watch(ctx, metav1.ListOptions{})
	if err != nil {
		return false, err
	}
	defer wi.Stop()

	for {
		select {
		case <-ctx.Done():
			return false, nil
		case event, ok := <-wi.ResultChan():
			if !ok {
				if ctx.Err() != nil {
					return false, nil
				}
				return false, fmt.Errorf("watch channel closed unexpectedly while VM freeze condition was being monitored")
			}

			currentVM, ok := event.Object.(*v1alpha2.VirtualMachine)
			if !ok || currentVM.Name != vm.Name {
				continue
			}

			frozenCondition, frozenConditionExists := conditions.GetCondition(vmcondition.TypeFilesystemFrozen, currentVM.Status.Conditions)
			if frozenConditionExists && frozenCondition.Status == metav1.ConditionTrue {
				return true, nil
			}
		}
	}
}

func startVMFreezeWatch(ctx context.Context, f *framework.Framework, vm *v1alpha2.VirtualMachine) (context.CancelFunc, *atomic.Bool, <-chan error) {
	GinkgoHelper()

	ctxVMWatch, cancelVMWatch := context.WithCancel(ctx)
	vmWatchErrCh := make(chan error, 1)
	vmWasFrozen := &atomic.Bool{}
	go func() {
		GinkgoRecover()
		wasFrozen, err := ensureVMWasFrozenInProgress(
			ctxVMWatch,
			f.VirtClient().VirtualMachines(f.Namespace().Name),
			vm,
		)
		vmWasFrozen.Store(wasFrozen)
		vmWatchErrCh <- err
	}()

	return cancelVMWatch, vmWasFrozen, vmWatchErrCh
}

func concurentlyVDSnapshotsCreation(ctx context.Context, f *framework.Framework, vds []*v1alpha2.VirtualDisk, cnt int) []*v1alpha2.VirtualDiskSnapshot {
	GinkgoHelper()

	var vdSnapshots []*v1alpha2.VirtualDiskSnapshot

	for i := 1; i <= cnt; i++ {
		for _, vd := range vds {
			vdSnapshots = append(vdSnapshots, generateVDSnapshot(
				fmt.Sprintf("vdsnapshot-%s-%d", vd.Name, i),
				vd,
				vdsnapshotbuilder.WithLabel(framework.E2ELabel, vdSnapshotNSPrefix)),
			)
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
