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
	"slices"
	"sync/atomic"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	storagev1 "k8s.io/api/storage/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"
	"k8s.io/utils/ptr"
	virtv1 "kubevirt.io/api/core/v1"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"

	cvibuilder "github.com/deckhouse/virtualization-controller/pkg/builder/cvi"
	vdbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vd"
	vibuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vi"
	vmbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vm"
	vmbdabuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vmbda"
	vmopbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vmop"
	commonvd "github.com/deckhouse/virtualization-controller/pkg/common/vd"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdcondition"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmbdacondition"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
	"github.com/deckhouse/virtualization/test/e2e/eventually"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	"github.com/deckhouse/virtualization/test/e2e/internal/label"
	"github.com/deckhouse/virtualization/test/e2e/internal/object"
	vdobs "github.com/deckhouse/virtualization/test/e2e/internal/observer/vd"
	viobs "github.com/deckhouse/virtualization/test/e2e/internal/observer/vi"
	vmobs "github.com/deckhouse/virtualization/test/e2e/internal/observer/vm"
	vmbdaobs "github.com/deckhouse/virtualization/test/e2e/internal/observer/vmbda"
	"github.com/deckhouse/virtualization/test/e2e/internal/precheck"
	"github.com/deckhouse/virtualization/test/e2e/internal/util"
)

// inUseHold is how long a disk another VM mounts is checked to stay off the waiting VM.
const inUseHold = 30 * time.Second

var _ = Describe("VirtualMachineBlockDeviceInUse", Label(label.SIGCompute, precheck.NoPrecheck), func() {
	f := framework.NewFramework("vm-block-device-in-use")

	var (
		ctx       context.Context
		owner     *v1alpha2.VirtualMachine
		waiter    *v1alpha2.VirtualMachine
		ownerObs  vmobs.Observer
		waiterObs vmobs.Observer
		ownerRoot *v1alpha2.VirtualDisk
		shared    *v1alpha2.VirtualDisk
	)

	newRoot := func(name string) *v1alpha2.VirtualDisk {
		return object.NewVDFromCVI(name, f.Namespace().Name, object.PrecreatedCVICustomBIOS,
			vdbuilder.WithSize(ptr.To(resource.MustParse(vdCustomImageSize))),
		)
	}

	newVM := func(name string, disks ...*v1alpha2.VirtualDisk) *v1alpha2.VirtualMachine {
		refs := make([]v1alpha2.BlockDeviceSpecRef, 0, len(disks))
		for _, d := range disks {
			refs = append(refs, v1alpha2.BlockDeviceSpecRef{Kind: v1alpha2.DiskDevice, Name: d.Name})
		}
		return object.NewMinimalVM("", f.Namespace().Name,
			vmbuilder.WithName(name),
			vmbuilder.WithBlockDeviceRefs(refs...),
		)
	}

	// setDisks replaces the disk refs of the VM, retrying while the controllers update it.
	setDisks := func(vm *v1alpha2.VirtualMachine, names ...string) {
		GinkgoHelper()
		err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
			if err := f.Clients.GenericClient().Get(ctx, crclient.ObjectKeyFromObject(vm), vm); err != nil {
				return err
			}
			refs := make([]v1alpha2.BlockDeviceSpecRef, 0, len(names))
			for _, name := range names {
				refs = append(refs, v1alpha2.BlockDeviceSpecRef{Kind: v1alpha2.DiskDevice, Name: name})
			}
			vm.Spec.BlockDeviceRefs = refs
			return f.Clients.GenericClient().Update(ctx, vm)
		})
		Expect(err).NotTo(HaveOccurred())
	}

	mountedBy := func(g Gomega, vd *v1alpha2.VirtualDisk) string {
		cur := &v1alpha2.VirtualDisk{}
		g.Expect(f.Clients.GenericClient().Get(ctx, crclient.ObjectKeyFromObject(vd), cur)).To(Succeed())
		return commonvd.GetCurrentlyMountedVMName(cur)
	}

	// hasVolume reports whether the running instance of the VM carries the volume of the disk.
	hasVolume := func(g Gomega, vm *v1alpha2.VirtualMachine, vd *v1alpha2.VirtualDisk) bool {
		kvvmi, err := util.GetInternalVirtualMachineInstance(ctx, vm)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(kvvmi).NotTo(BeNil())
		return slices.Contains(volumeNames(kvvmi.Spec.Volumes), "vd-"+vd.Name)
	}

	// holders returns how many of the two VMs have the volume of the disk on their instances.
	holders := func(vd *v1alpha2.VirtualDisk) int {
		n := 0
		for _, vm := range []*v1alpha2.VirtualMachine{owner, waiter} {
			kvvmi, err := util.GetInternalVirtualMachineInstance(ctx, vm)
			if err != nil || kvvmi == nil {
				continue
			}
			// The volume counts from the moment it is in the instance spec, before it shows in the volume status.
			if slices.ContainsFunc(kvvmi.Status.VolumeStatus, func(vs virtv1.VolumeStatus) bool { return vs.Name == "vd-"+vd.Name }) ||
				slices.ContainsFunc(kvvmi.Spec.Volumes, func(v virtv1.Volume) bool { return v.Name == "vd-"+vd.Name }) {
				n++
			}
		}
		return n
	}

	// watchOneWriter fails the spec if the volume of the disk is ever on both instances at once:
	// the final state of a spec does not show a second writer that came and went.
	watchOneWriter := func(vd *v1alpha2.VirtualDisk) {
		stop := make(chan struct{})
		var twoWriters atomic.Bool
		go func() {
			defer GinkgoRecover()
			ticker := time.NewTicker(time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-stop:
					return
				case <-ticker.C:
					if holders(vd) > 1 {
						twoWriters.Store(true)
					}
				}
			}
		}()
		DeferCleanup(func() {
			close(stop)
			Expect(twoWriters.Load()).To(BeFalse(), "the volume of the disk %q was on two instances at once", vd.Name)
		})
	}

	newBlank := func(name string, opts ...vdbuilder.Option) *v1alpha2.VirtualDisk {
		return object.NewVD(append([]vdbuilder.Option{
			vdbuilder.WithName(name),
			vdbuilder.WithNamespace(f.Namespace().Name),
			vdbuilder.WithPersistentVolumeClaim(nil, ptr.To(resource.MustParse(vdCustomImageSize))),
		}, opts...)...)
	}

	newAttachment := func(name string, vm *v1alpha2.VirtualMachine, vd *v1alpha2.VirtualDisk) *v1alpha2.VirtualMachineBlockDeviceAttachment {
		return vmbdabuilder.New(
			vmbdabuilder.WithName(name),
			vmbdabuilder.WithNamespace(f.Namespace().Name),
			vmbdabuilder.WithVirtualMachineName(vm.Name),
			vmbdabuilder.WithBlockDeviceRef(v1alpha2.VMBDAObjectRefKindVirtualDisk, vd.Name),
		)
	}

	// expectDiskStaysWithOwner checks the waiting VM reports the disk in use and never gets it.
	expectDiskStaysWithOwner := func(vm *v1alpha2.VirtualMachine) {
		GinkgoHelper()
		eventually.UntilAssertion(func(g Gomega) {
			cur := &v1alpha2.VirtualMachine{}
			g.Expect(f.Clients.GenericClient().Get(ctx, crclient.ObjectKeyFromObject(vm), cur)).To(Succeed())
			ready, _ := conditions.GetCondition(vmcondition.TypeBlockDevicesReady, cur.Status.Conditions)
			g.Expect(ready.Status).To(Equal(metav1.ConditionFalse))
			g.Expect(ready.Message).To(ContainSubstring("in use by another VM"))
		}, framework.LongTimeout, eventually.WithPolling(hotplugPolling))

		Consistently(func(g Gomega) {
			g.Expect(hasVolume(g, vm, shared)).To(BeFalse(), "the disk another VM mounts must not reach this VM")
			g.Expect(mountedBy(g, shared)).To(Equal(owner.Name))
		}).WithTimeout(inUseHold).WithPolling(hotplugPolling).Should(Succeed())
	}

	// wffcStorageClass returns the suite storage class when it binds on the first consumer,
	// otherwise a WaitForFirstConsumer class of the same provisioner.
	wffcStorageClass := func() string {
		GinkgoHelper()
		isWFFC := func(sc *storagev1.StorageClass) bool {
			return sc.VolumeBindingMode != nil && *sc.VolumeBindingMode == storagev1.VolumeBindingWaitForFirstConsumer
		}
		suite := framework.GetConfig().StorageClass.DefaultStorageClass
		if suite != nil && isWFFC(suite) {
			return suite.Name
		}
		scs, err := f.Clients.KubeClient().StorageV1().StorageClasses().List(ctx, metav1.ListOptions{})
		Expect(err).NotTo(HaveOccurred())
		for i := range scs.Items {
			if isWFFC(&scs.Items[i]) && (suite == nil || scs.Items[i].Provisioner == suite.Provisioner) {
				return scs.Items[i].Name
			}
		}
		Skip("skip: no WaitForFirstConsumer StorageClass with the provisioner of the suite storage class")
		return ""
	}

	BeforeEach(func() {
		ctx = context.Background()
		DeferCleanup(f.After)
		f.Before()

		ownerRoot = newRoot("vd-root-owner")
		waiterRoot := newRoot("vd-root-waiter")
		shared = object.NewVD(
			vdbuilder.WithName("vd-shared"),
			vdbuilder.WithNamespace(f.Namespace().Name),
			vdbuilder.WithPersistentVolumeClaim(nil, ptr.To(resource.MustParse(vdCustomImageSize))),
		)
		owner = newVM("vm-owner", ownerRoot, shared)
		waiter = newVM("vm-waiter", waiterRoot)

		ownerObs = vmobs.StartObserver(ctx, f, owner)
		ownerObs.Never(vmobs.BeFailed())
		waiterObs = vmobs.StartObserver(ctx, f, waiter)
		waiterObs.Never(vmobs.BeFailed())

		err := f.CreateWithDeferredDeletion(ctx, ownerRoot, waiterRoot, shared, owner, waiter)
		Expect(err).NotTo(HaveOccurred())

		By("Waiting for both virtual machines to run, the owner with the shared disk")
		Expect(ownerObs.WaitFor(vmobs.BeRunning(), framework.LongTimeout)).To(Succeed())
		Expect(waiterObs.WaitFor(vmobs.BeRunning(), framework.LongTimeout)).To(Succeed())
		Expect(ownerObs.WaitFor(vmobs.HaveBlockDevicesAttached(shared.Name), framework.LongTimeout)).To(Succeed())
		eventually.UntilAssertion(func(g Gomega) {
			g.Expect(mountedBy(g, shared)).To(Equal(owner.Name))
		}, framework.MiddleTimeout, eventually.WithPolling(hotplugPolling))
		watchOneWriter(shared)
	})

	It("does not hotplug a disk another VM mounts, and hotplugs it once released", func() {
		By("Adding the shared disk to the spec of the waiting virtual machine")
		setDisks(waiter, "vd-root-waiter", shared.Name)
		expectDiskStaysWithOwner(waiter)

		By("Releasing the disk from the owner")
		setDisks(owner, ownerRoot.Name)
		Expect(ownerObs.WaitFor(vmobs.HaveBlockDeviceDetached(shared.Name), framework.LongTimeout)).To(Succeed())

		By("Waiting for the waiting virtual machine to get the disk without a restart")
		waiterObs.Never(requireNoRestart())
		Expect(waiterObs.WaitFor(vmobs.HaveBlockDevicesAttached(shared.Name), framework.LongTimeout)).To(Succeed())
		eventually.UntilAssertion(func(g Gomega) {
			g.Expect(mountedBy(g, shared)).To(Equal(waiter.Name))
		}, framework.MiddleTimeout, eventually.WithPolling(hotplugPolling))
	})

	It("hotplugs a WaitForFirstConsumer disk added to the spec of a running VM", func() {
		storageClass := wffcStorageClass()
		wffc := object.NewVD(
			vdbuilder.WithName("vd-wffc"),
			vdbuilder.WithNamespace(f.Namespace().Name),
			vdbuilder.WithPersistentVolumeClaim(&storageClass, ptr.To(resource.MustParse(vdCustomImageSize))),
		)
		wffcObs := vdobs.StartObserver(ctx, f, wffc)
		wffcObs.Never(vdobs.BeFailed())
		Expect(f.CreateWithDeferredDeletion(ctx, wffc)).To(Succeed())
		Expect(wffcObs.WaitFor(vdobs.BeWaitForFirstConsumer(), framework.LongTimeout)).To(Succeed())

		By("Adding the disk to the spec of the running virtual machine")
		waiterObs.Never(requireNoRestart())
		setDisks(waiter, "vd-root-waiter", wffc.Name)

		By("Waiting for the disk to be hotplugged and bound")
		Expect(waiterObs.WaitFor(vmobs.HaveBlockDevicesAttached(wffc.Name), framework.LongTimeout)).To(Succeed())
		Expect(wffcObs.WaitFor(vdobs.BeReady(), framework.LongTimeout)).To(Succeed())
	})

	It("does not attach a disk another VM mounts through a VMBDA, and attaches it once released", func() {
		vmbda := vmbdabuilder.New(
			vmbdabuilder.WithName("vmbda-shared"),
			vmbdabuilder.WithNamespace(f.Namespace().Name),
			vmbdabuilder.WithVirtualMachineName(waiter.Name),
			vmbdabuilder.WithBlockDeviceRef(v1alpha2.VMBDAObjectRefKindVirtualDisk, shared.Name),
		)
		vmbdaObs := vmbdaobs.StartObserver(ctx, f, vmbda)
		vmbdaObs.Never(vmbdaobs.BeFailed())

		By("Attaching the shared disk to the waiting virtual machine through a VMBDA")
		Expect(f.CreateWithDeferredDeletion(ctx, vmbda)).To(Succeed())
		eventually.UntilAssertion(func(g Gomega) {
			cur := &v1alpha2.VirtualMachineBlockDeviceAttachment{}
			g.Expect(f.Clients.GenericClient().Get(ctx, crclient.ObjectKeyFromObject(vmbda), cur)).To(Succeed())
			ready, _ := conditions.GetCondition(vmbdacondition.BlockDeviceReadyType, cur.Status.Conditions)
			g.Expect(ready.Reason).To(Equal(vmbdacondition.BlockDeviceNotReady.String()))
			g.Expect(ready.Message).To(ContainSubstring(owner.Name))
		}, framework.LongTimeout, eventually.WithPolling(hotplugPolling))
		Consistently(func(g Gomega) {
			cur := &v1alpha2.VirtualMachineBlockDeviceAttachment{}
			g.Expect(f.Clients.GenericClient().Get(ctx, crclient.ObjectKeyFromObject(vmbda), cur)).To(Succeed())
			g.Expect(cur.Status.Phase).NotTo(Equal(v1alpha2.BlockDeviceAttachmentPhaseAttached))
			g.Expect(hasVolume(g, waiter, shared)).To(BeFalse(), "the disk another VM mounts must not reach this VM")

			// Snapshots and WaitForFirstConsumer provisioning count the listed VMs: a waiting attachment adds none.
			disk := &v1alpha2.VirtualDisk{}
			g.Expect(f.Clients.GenericClient().Get(ctx, crclient.ObjectKeyFromObject(shared), disk)).To(Succeed())
			g.Expect(disk.Status.AttachedToVirtualMachines).To(HaveLen(1))
		}).WithTimeout(inUseHold).WithPolling(hotplugPolling).Should(Succeed())

		By("Releasing the disk from the owner")
		setDisks(owner, ownerRoot.Name)
		Expect(ownerObs.WaitFor(vmobs.HaveBlockDeviceDetached(shared.Name), framework.LongTimeout)).To(Succeed())

		By("Waiting for the VMBDA to attach the disk")
		Expect(vmbdaObs.WaitFor(vmbdaobs.BeAttached(), framework.LongTimeout)).To(Succeed())
		Expect(waiterObs.WaitFor(vmobs.HaveBlockDevicesAttached(shared.Name), framework.LongTimeout)).To(Succeed())
	})

	It("keeps a disk on its VM through a live migration while another VM waits for it", func() {
		By("Adding the shared disk to the spec of the waiting virtual machine")
		setDisks(waiter, "vd-root-waiter", shared.Name)
		expectDiskStaysWithOwner(waiter)
		waiterObs.Never(vmobs.HaveBlockDevicesAttached(shared.Name))

		By("Migrating the owner")
		vmop := util.MigrateVirtualMachine(f, owner)
		util.UntilVMOPMigrationSucceeded(ctx, vmop, framework.MaxTimeout)

		By("Checking the disk stayed with the owner")
		Expect(ownerObs.WaitFor(vmobs.HaveBlockDevicesAttached(shared.Name), framework.LongTimeout)).To(Succeed())
		expectDiskStaysWithOwner(waiter)
	})

	It("unplugs a disk removed from the spec without waiting for a disk that does not exist yet", func() {
		const later = "vd-later"

		ownerNotReady := func(g Gomega) {
			cur := &v1alpha2.VirtualMachine{}
			g.Expect(f.Clients.GenericClient().Get(ctx, crclient.ObjectKeyFromObject(owner), cur)).To(Succeed())
			ready, _ := conditions.GetCondition(vmcondition.TypeBlockDevicesReady, cur.Status.Conditions)
			g.Expect(ready.Status).To(Equal(metav1.ConditionFalse))
		}

		// The block devices must be not ready before the removal: in the same reconcile the
		// hotplug handler still sees the previous, ready state.
		By("Adding a disk that does not exist yet")
		ownerObs.Never(requireNoRestart())
		setDisks(owner, ownerRoot.Name, shared.Name, later)
		eventually.UntilAssertion(ownerNotReady, framework.LongTimeout, eventually.WithPolling(hotplugPolling))

		By("Removing the shared disk and waiting for it to be unplugged while the other one is missing")
		setDisks(owner, ownerRoot.Name, later)
		Expect(ownerObs.WaitFor(vmobs.HaveBlockDeviceDetached(shared.Name), framework.LongTimeout)).To(Succeed())
		eventually.UntilAssertion(func(g Gomega) {
			g.Expect(hasVolume(g, owner, shared)).To(BeFalse())
			ownerNotReady(g)
		}, framework.LongTimeout, eventually.WithPolling(hotplugPolling))

		By("Creating the missing disk and waiting for it to be hotplugged")
		vdLater := object.NewVD(
			vdbuilder.WithName(later),
			vdbuilder.WithNamespace(f.Namespace().Name),
			vdbuilder.WithPersistentVolumeClaim(nil, ptr.To(resource.MustParse(vdCustomImageSize))),
		)
		Expect(f.CreateWithDeferredDeletion(ctx, vdLater)).To(Succeed())
		Expect(ownerObs.WaitFor(vmobs.HaveBlockDevicesAttached(later), framework.LongTimeout)).To(Succeed())
	})

	It("gives a free disk to one VM when one adds it to the spec and another attaches it at once", func() {
		race := newBlank("vd-race")
		Expect(f.CreateWithDeferredDeletion(ctx, race)).To(Succeed())
		util.WaitDiskInExpectedPhase(ctx, f, race)
		watchOneWriter(race)

		By("Adding the disk to the spec of one VM and attaching it to the other at the same time")
		setDisks(owner, ownerRoot.Name, shared.Name, race.Name)
		Expect(f.CreateWithDeferredDeletion(ctx, newAttachment("vmbda-race", waiter, race))).To(Succeed())

		By("Checking the disk reaches exactly one of them and stays there")
		eventually.UntilAssertion(func(g Gomega) {
			g.Expect(holders(race)).To(Equal(1))
		}, framework.LongTimeout, eventually.WithPolling(hotplugPolling))
		Consistently(func(g Gomega) {
			g.Expect(holders(race)).To(Equal(1))
		}).WithTimeout(inUseHold).WithPolling(hotplugPolling).Should(Succeed())
	})

	It("does not plug a disk into two VMs when the attachment VM restarts while another adds the disk", func() {
		disk := newBlank("vd-restart")
		Expect(f.CreateWithDeferredDeletion(ctx, disk)).To(Succeed())
		util.WaitDiskInExpectedPhase(ctx, f, disk)
		watchOneWriter(disk)

		vmbda := newAttachment("vmbda-restart", waiter, disk)
		vmbdaObs := vmbdaobs.StartObserver(ctx, f, vmbda)
		vmbdaObs.Never(vmbdaobs.BeFailed())
		Expect(f.CreateWithDeferredDeletion(ctx, vmbda)).To(Succeed())
		Expect(vmbdaObs.WaitFor(vmbdaobs.BeAttached(), framework.LongTimeout)).To(Succeed())

		By("Restarting the attachment VM and adding the disk to the spec of the other one meanwhile")
		util.RebootVirtualMachineByVMOP(f, waiter)
		setDisks(owner, ownerRoot.Name, shared.Name, disk.Name)

		By("Checking the disk ends up with exactly one of them once the restart is over")
		Expect(waiterObs.WaitFor(vmobs.BeRunning(), framework.LongTimeout)).To(Succeed())
		eventually.UntilAssertion(func(g Gomega) {
			g.Expect(holders(disk)).To(Equal(1))
		}, framework.LongTimeout, eventually.WithPolling(hotplugPolling))
		Consistently(func(g Gomega) {
			g.Expect(holders(disk)).To(Equal(1))
		}).WithTimeout(inUseHold).WithPolling(hotplugPolling).Should(Succeed())
	})

	It("does not attach a disk while an image is being created from it, and attaches it after", func() {
		src := newBlank("vd-image-source")
		srcObs := vdobs.StartObserver(ctx, f, src)
		Expect(f.CreateWithDeferredDeletion(ctx, src)).To(Succeed())

		// A WaitForFirstConsumer disk becomes ready only on its first consumer.
		By("Binding the disk through the waiting VM and releasing it")
		setDisks(waiter, "vd-root-waiter", src.Name)
		// The VM reports the disk attached while its volume is still pending, before the claim is bound.
		Expect(srcObs.WaitFor(vdobs.BeReady(), framework.LongTimeout)).To(Succeed())
		Expect(waiterObs.WaitFor(vmobs.HaveBlockDevicesAttached(src.Name), framework.LongTimeout)).To(Succeed())
		setDisks(waiter, "vd-root-waiter")
		Expect(waiterObs.WaitFor(vmobs.HaveBlockDeviceDetached(src.Name), framework.LongTimeout)).To(Succeed())

		By("Creating an image from the disk")
		image := object.NewVI(
			vibuilder.WithName("vi-from-disk"),
			vibuilder.WithNamespace(f.Namespace().Name),
			vibuilder.WithDataSourceObjectRef(v1alpha2.VirtualImageObjectRefKindVirtualDisk, src.Name),
			vibuilder.WithStorage(v1alpha2.StorageContainerRegistry),
		)
		imageObs := viobs.StartObserver(ctx, f, image)
		imageObs.Never(viobs.BeFailed())
		Expect(f.CreateWithDeferredDeletion(ctx, image)).To(Succeed())
		eventually.UntilAssertion(func(g Gomega) {
			disk := &v1alpha2.VirtualDisk{}
			g.Expect(f.Clients.GenericClient().Get(ctx, crclient.ObjectKeyFromObject(src), disk)).To(Succeed())
			inUse, _ := conditions.GetCondition(vdcondition.InUseType, disk.Status.Conditions)
			g.Expect(inUse.Reason).To(Equal(vdcondition.UsedForImageCreation.String()))
		}, framework.MiddleTimeout, eventually.WithPolling(time.Second))

		By("Attaching the disk while the image is being created")
		vmbda := newAttachment("vmbda-image-source", waiter, src)
		vmbdaObs := vmbdaobs.StartObserver(ctx, f, vmbda)
		vmbdaObs.Never(vmbdaobs.BeFailed())
		Expect(f.CreateWithDeferredDeletion(ctx, vmbda)).To(Succeed())
		eventually.UntilAssertion(func(g Gomega) {
			cur := &v1alpha2.VirtualImage{}
			g.Expect(f.Clients.GenericClient().Get(ctx, crclient.ObjectKeyFromObject(image), cur)).To(Succeed())
			att := &v1alpha2.VirtualMachineBlockDeviceAttachment{}
			g.Expect(f.Clients.GenericClient().Get(ctx, crclient.ObjectKeyFromObject(vmbda), att)).To(Succeed())
			if cur.Status.Phase != v1alpha2.ImageReady {
				Expect(att.Status.Phase).NotTo(Equal(v1alpha2.BlockDeviceAttachmentPhaseAttached), "the disk must not be attached while an image is being created from it")
			}
			g.Expect(cur.Status.Phase).To(Equal(v1alpha2.ImageReady))
		}, framework.LongTimeout, eventually.WithPolling(time.Second))

		By("Waiting for the attachment once the image is ready")
		Expect(vmbdaObs.WaitFor(vmbdaobs.BeAttached(), framework.LongTimeout)).To(Succeed())
	})

	// DKP over DVP: the DVP CSI driver of a nested cluster plugs a persistent volume into the node through an
	// attachment named vmbda-<disk>-<node>, and moves the pod to another node by deleting that attachment and
	// creating one to the new node right away, without waiting for the volume to be detached.
	It("moves a disk between VMs the way the DVP CSI driver does", func() {
		disk := newBlank("vd-csi")
		Expect(f.CreateWithDeferredDeletion(ctx, disk)).To(Succeed())
		util.WaitDiskInExpectedPhase(ctx, f, disk)
		watchOneWriter(disk)

		attach := func(vm *v1alpha2.VirtualMachine) *v1alpha2.VirtualMachineBlockDeviceAttachment {
			GinkgoHelper()
			vmbda := newAttachment("vmbda-"+disk.Name+"-"+vm.Name, vm, disk)
			obs := vmbdaobs.StartObserver(ctx, f, vmbda)
			obs.Never(vmbdaobs.BeFailed())
			// The webhook refuses a second attachment of the disk while the previous one still exists,
			// even being deleted; the CSI driver retries the publish until it is gone.
			eventually.UntilAssertion(func(g Gomega) {
				err := f.Clients.GenericClient().Create(ctx, vmbda.DeepCopy())
				// A create that reached the server but timed out on the client comes back as AlreadyExists.
				g.Expect(err == nil || k8serrors.IsAlreadyExists(err)).To(BeTrue(), "create %s: %v", vmbda.Name, err)
			}, framework.LongTimeout, eventually.WithPolling(time.Second))
			Expect(obs.WaitFor(vmbdaobs.BeAttached(), framework.LongTimeout)).To(Succeed())
			return vmbda
		}

		By("Plugging the disk into the first VM")
		current := attach(owner)

		for i, next := range []*v1alpha2.VirtualMachine{waiter, owner, waiter} {
			By(fmt.Sprintf("Move %d: deleting the attachment and attaching the disk to %s at once", i+1, next.Name))
			previous := current
			Expect(f.Clients.GenericClient().Delete(ctx, previous)).To(Succeed())
			current = attach(next)

			eventually.UntilAssertion(func(g Gomega) {
				g.Expect(hasVolume(g, next, disk)).To(BeTrue())
				g.Expect(holders(disk)).To(Equal(1))
				err := f.Clients.GenericClient().Get(ctx, crclient.ObjectKeyFromObject(previous), &v1alpha2.VirtualMachineBlockDeviceAttachment{})
				g.Expect(k8serrors.IsNotFound(err)).To(BeTrue(), "the previous attachment must be gone")
			}, framework.LongTimeout, eventually.WithPolling(hotplugPolling))
		}
	})

	It("holds an attachment to a migrating VM until the migration ends", func() {
		disk := newBlank("vd-migrating")
		Expect(f.CreateWithDeferredDeletion(ctx, disk)).To(Succeed())
		util.WaitDiskInExpectedPhase(ctx, f, disk)
		watchOneWriter(disk)

		By("Migrating the waiting VM and attaching a disk to it meanwhile")
		vmop := util.MigrateVirtualMachine(f, waiter)
		eventually.UntilAssertion(func(g Gomega) {
			cur := &v1alpha2.VirtualMachineOperation{}
			g.Expect(f.Clients.GenericClient().Get(ctx, crclient.ObjectKeyFromObject(vmop), cur)).To(Succeed())
			g.Expect(cur.Status.Phase).To(BeElementOf(v1alpha2.VMOPPhaseInProgress, v1alpha2.VMOPPhaseCompleted))
		}, framework.MiddleTimeout, eventually.WithPolling(time.Second))
		vmbda := newAttachment("vmbda-migrating", waiter, disk)
		vmbdaObs := vmbdaobs.StartObserver(ctx, f, vmbda)
		vmbdaObs.Never(vmbdaobs.BeFailed())
		Expect(f.CreateWithDeferredDeletion(ctx, vmbda)).To(Succeed())

		// A small VM may finish migrating before the attachment is reconciled, so the invariant is
		// checked instead of the waiting reason: nothing is attached while the migration runs.
		By("Checking the disk is not attached before the migration ends")
		eventually.UntilAssertion(func(g Gomega) {
			op := &v1alpha2.VirtualMachineOperation{}
			g.Expect(f.Clients.GenericClient().Get(ctx, crclient.ObjectKeyFromObject(vmop), op)).To(Succeed())
			att := &v1alpha2.VirtualMachineBlockDeviceAttachment{}
			g.Expect(f.Clients.GenericClient().Get(ctx, crclient.ObjectKeyFromObject(vmbda), att)).To(Succeed())
			if op.Status.Phase == v1alpha2.VMOPPhaseInProgress {
				Expect(att.Status.Phase).NotTo(Equal(v1alpha2.BlockDeviceAttachmentPhaseAttached), "the disk must not be attached to a migrating VM")
			}
			g.Expect(op.Status.Phase).To(Equal(v1alpha2.VMOPPhaseCompleted))
		}, framework.MaxTimeout, eventually.WithPolling(time.Second))

		By("Waiting for the attachment once the migration is over")
		Expect(vmbdaObs.WaitFor(vmbdaobs.BeAttached(), framework.LongTimeout)).To(Succeed())
		Expect(waiterObs.WaitFor(vmobs.HaveBlockDevicesAttached(disk.Name), framework.LongTimeout)).To(Succeed())
	})

	It("starts a stopped VM without a disk whose attachment was deleted while it was off", func() {
		disk := newBlank("vd-stopped")
		Expect(f.CreateWithDeferredDeletion(ctx, disk)).To(Succeed())
		util.WaitDiskInExpectedPhase(ctx, f, disk)

		vmbda := newAttachment("vmbda-stopped", waiter, disk)
		vmbdaObs := vmbdaobs.StartObserver(ctx, f, vmbda)
		vmbdaObs.Never(vmbdaobs.BeFailed())
		Expect(f.CreateWithDeferredDeletion(ctx, vmbda)).To(Succeed())
		Expect(vmbdaObs.WaitFor(vmbdaobs.BeAttached(), framework.LongTimeout)).To(Succeed())

		By("Stopping the VM and deleting the attachment while it is off")
		Expect(f.CreateWithDeferredDeletion(ctx, vmopbuilder.New(
			vmopbuilder.WithGenerateName(fmt.Sprintf("%s-stop-", util.VmopE2ePrefix)),
			vmopbuilder.WithNamespace(waiter.Namespace),
			vmopbuilder.WithType(v1alpha2.VMOPTypeStop),
			vmopbuilder.WithVirtualMachine(waiter.Name),
			vmopbuilder.WithForce(ptr.To(true)),
		))).To(Succeed())
		Expect(waiterObs.WaitFor(vmobs.BeStopped(), framework.LongTimeout)).To(Succeed())
		Expect(f.Clients.GenericClient().Delete(ctx, vmbda)).To(Succeed())
		eventually.UntilAssertion(func(g Gomega) {
			err := f.Clients.GenericClient().Get(ctx, crclient.ObjectKeyFromObject(vmbda), &v1alpha2.VirtualMachineBlockDeviceAttachment{})
			g.Expect(k8serrors.IsNotFound(err)).To(BeTrue())
		}, framework.MiddleTimeout, eventually.WithPolling(time.Second))

		By("Starting the VM and checking it comes up without the disk")
		util.StartVirtualMachine(ctx, f, waiter)
		Expect(waiterObs.WaitFor(vmobs.BeRunning(), framework.LongTimeout)).To(Succeed())
		Consistently(func(g Gomega) {
			g.Expect(hasVolume(g, waiter, disk)).To(BeFalse(), "a disk whose attachment was deleted must not come back with the VM")
			g.Expect(mountedBy(g, disk)).To(BeEmpty())
		}).WithTimeout(inUseHold).WithPolling(hotplugPolling).Should(Succeed())
	})

	It("attaches a disk while a cluster image is created from a disk with the same name in another namespace", func() {
		disk := newBlank("vd-namesake")
		Expect(f.CreateWithDeferredDeletion(ctx, disk)).To(Succeed())
		util.WaitDiskInExpectedPhase(ctx, f, disk)

		// The source does not exist, so the image stays waiting for it for the whole spec.
		By("Creating a cluster image from a disk with the same name in another namespace")
		image := cvibuilder.New(
			cvibuilder.WithName(f.Namespace().Name+"-namesake"),
			cvibuilder.WithDataSourceObjectRef(v1alpha2.ClusterVirtualImageObjectRefKindVirtualDisk, disk.Name, f.Namespace().Name+"-other"),
		)
		Expect(f.CreateWithDeferredDeletion(ctx, image)).To(Succeed())
		eventually.UntilAssertion(func(g Gomega) {
			cur := &v1alpha2.ClusterVirtualImage{}
			g.Expect(f.Clients.GenericClient().Get(ctx, crclient.ObjectKeyFromObject(image), cur)).To(Succeed())
			g.Expect(cur.Status.Phase).To(BeElementOf(v1alpha2.ImagePending, v1alpha2.ImageProvisioning))
		}, framework.MiddleTimeout, eventually.WithPolling(time.Second))

		By("Attaching the disk to a VM")
		vmbda := newAttachment("vmbda-namesake", waiter, disk)
		vmbdaObs := vmbdaobs.StartObserver(ctx, f, vmbda)
		vmbdaObs.Never(vmbdaobs.BeFailed())
		Expect(f.CreateWithDeferredDeletion(ctx, vmbda)).To(Succeed())
		Expect(vmbdaObs.WaitFor(vmbdaobs.BeAttached(), framework.LongTimeout)).To(Succeed())

		cur := &v1alpha2.VirtualDisk{}
		Expect(f.Clients.GenericClient().Get(ctx, crclient.ObjectKeyFromObject(disk), cur)).To(Succeed())
		inUse, _ := conditions.GetCondition(vdcondition.InUseType, cur.Status.Conditions)
		Expect(inUse.Reason).NotTo(Equal(vdcondition.UsedForImageCreation.String()))
	})
})
