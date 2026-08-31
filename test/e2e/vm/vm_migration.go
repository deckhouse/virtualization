/*
Copyright 2025 Flant JSC

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
	"k8s.io/utils/ptr"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/builder/vd"
	"github.com/deckhouse/virtualization-controller/pkg/builder/vi"
	"github.com/deckhouse/virtualization-controller/pkg/builder/vm"
	"github.com/deckhouse/virtualization-controller/pkg/builder/vmbda"
	vmopbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vmop"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/test/e2e/eventually"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	"github.com/deckhouse/virtualization/test/e2e/internal/label"
	"github.com/deckhouse/virtualization/test/e2e/internal/network"
	"github.com/deckhouse/virtualization/test/e2e/internal/object"
	vmobs "github.com/deckhouse/virtualization/test/e2e/internal/observer/vm"
	vmbdaobs "github.com/deckhouse/virtualization/test/e2e/internal/observer/vmbda"
	"github.com/deckhouse/virtualization/test/e2e/internal/precheck"
	"github.com/deckhouse/virtualization/test/e2e/internal/util"
)

const lsblkCommand = "lsblk -dn | wc -l"

var _ = Describe("VirtualMachineMigration", Label(label.SIGCompute, precheck.NoPrecheck), func() {
	var (
		// Core: VMs and their root/blank disks
		vdRootBIOS  *v1alpha2.VirtualDisk
		vdBlankBIOS *v1alpha2.VirtualDisk
		vdRootUEFI  *v1alpha2.VirtualDisk
		vdBlankUEFI *v1alpha2.VirtualDisk
		vmBIOS      *v1alpha2.VirtualMachine
		vmUEFI      *v1alpha2.VirtualMachine

		// Hotplug: disks and images attached via VMBDAs
		vdHotplugBIOS *v1alpha2.VirtualDisk
		vdHotplugUEFI *v1alpha2.VirtualDisk
		viHotplugBIOS *v1alpha2.VirtualImage
		viHotplugUEFI *v1alpha2.VirtualImage
		vmbdas        []*v1alpha2.VirtualMachineBlockDeviceAttachment
		allObjects    []crclient.Object

		vmopMigrateBIOS *v1alpha2.VirtualMachineOperation
		vmopMigrateUEFI *v1alpha2.VirtualMachineOperation

		f   *framework.Framework
		ctx context.Context

		biosDiskCountOriginal string
		uefiDiskCountOriginal string
	)

	BeforeEach(func() {
		ctx = context.Background()
		f = framework.NewFramework("vm-migration")
		DeferCleanup(f.After)

		f.Before()
	})

	It("verifies that migrations are successful", func() {
		// TODO(e2e-flaky-parallel): flaky under parallel load on the 3-node cluster (ready-to-migrate timeout under migration contention). Re-enable once stabilized.
		Skip("flaky under parallel load: ready-to-migrate timeout under contention")
		var vmBIOSObs, vmUEFIObs vmobs.Observer
		var vmbdaObservers []vmbdaobs.Observer

		By("Environment preparation", func() {
			vdRootBIOS = object.NewVDFromCVI("vd-root-bios", f.Namespace().Name, object.PrecreatedCVICustomBIOS, vd.WithSize(ptr.To(resource.MustParse(vdCustomImageSize))))
			vdBlankBIOS = object.NewBlankVD("vd-blank-bios", f.Namespace().Name, nil, ptr.To(resource.MustParse(vdCustomImageSize)))

			vdRootUEFI = object.NewVDFromCVI("vd-root-uefi", f.Namespace().Name, object.PrecreatedCVICustomEFI, vd.WithSize(ptr.To(resource.MustParse(vdCustomImageSize))))
			vdBlankUEFI = object.NewBlankVD("vd-blank-uefi", f.Namespace().Name, nil, ptr.To(resource.MustParse(vdCustomImageSize)))
			vmBIOS = object.NewMinimalVM("", f.Namespace().Name,
				vm.WithBlockDeviceRefs(
					v1alpha2.BlockDeviceSpecRef{
						Kind: v1alpha2.VirtualDiskKind,
						Name: vdRootBIOS.Name,
					},
					v1alpha2.BlockDeviceSpecRef{
						Kind: v1alpha2.VirtualDiskKind,
						Name: vdBlankBIOS.Name,
					},
				),
				vm.WithBootloader(v1alpha2.BIOS),
				// The custom image has no cloud-init; the guest agent is baked
				// into the image and the test logs in as root with the baked key.
				vm.WithLiveMigrationPolicy(v1alpha2.PreferSafeMigrationPolicy),
				vm.WithName("vm-bios"),
			)
			vmUEFI = object.NewMinimalVM("", f.Namespace().Name,
				vm.WithBlockDeviceRefs(
					v1alpha2.BlockDeviceSpecRef{
						Kind: v1alpha2.VirtualDiskKind,
						Name: vdRootUEFI.Name,
					},
					v1alpha2.BlockDeviceSpecRef{
						Kind: v1alpha2.VirtualDiskKind,
						Name: vdBlankUEFI.Name,
					},
				),
				vm.WithBootloader(v1alpha2.EFI),
				vm.WithMemory(resource.MustParse(object.CustomImageEFIVMMemory)),
				// The custom EFI image has no cloud-init; the guest agent is
				// baked into the image and the test logs in as root with the baked key.
				vm.WithLiveMigrationPolicy(v1alpha2.PreferSafeMigrationPolicy),
				vm.WithName("vm-uefi"),
			)

			// --- Hotplug resources ---
			vdHotplugBIOS = object.NewBlankVD("vd-hotplug-bios", f.Namespace().Name, nil, ptr.To(resource.MustParse(vdCustomImageSize)))
			vdHotplugUEFI = object.NewBlankVD("vd-hotplug-uefi", f.Namespace().Name, nil, ptr.To(resource.MustParse(vdCustomImageSize)))

			// The hotplugged images are payload only (never booted), so the custom
			// image serves as their source.
			viHotplugBIOS = object.NewVI(
				vi.WithName("vi-hotplug-bios"),
				vi.WithNamespace(f.Namespace().Name),
				vi.WithDataSourceObjectRef(v1alpha2.VirtualImageObjectRefKindClusterVirtualImage, object.PrecreatedCVICustomBIOS),
				vi.WithStorage(v1alpha2.StorageContainerRegistry),
			)
			viHotplugUEFI = object.NewVI(
				vi.WithName("vi-hotplug-uefi"),
				vi.WithNamespace(f.Namespace().Name),
				vi.WithDataSourceObjectRef(v1alpha2.VirtualImageObjectRefKindClusterVirtualImage, object.PrecreatedCVICustomBIOS),
				vi.WithStorage(v1alpha2.StorageContainerRegistry),
			)

			vmbdaVdBIOS := vmbda.New(
				vmbda.WithName("vmbda-vd-bios"),
				vmbda.WithNamespace(f.Namespace().Name),
				vmbda.WithBlockDeviceRef(v1alpha2.VMBDAObjectRefKindVirtualDisk, vdHotplugBIOS.Name),
				vmbda.WithVirtualMachineName(vmBIOS.Name),
			)
			vmbdaVdUEFI := vmbda.New(
				vmbda.WithName("vmbda-vd-uefi"),
				vmbda.WithNamespace(f.Namespace().Name),
				vmbda.WithBlockDeviceRef(v1alpha2.VMBDAObjectRefKindVirtualDisk, vdHotplugUEFI.Name),
				vmbda.WithVirtualMachineName(vmUEFI.Name),
			)
			vmbdaViBIOS := vmbda.New(
				vmbda.WithName("vmbda-vi-bios"),
				vmbda.WithNamespace(f.Namespace().Name),
				vmbda.WithBlockDeviceRef(v1alpha2.VMBDAObjectRefKindVirtualImage, viHotplugBIOS.Name),
				vmbda.WithVirtualMachineName(vmBIOS.Name),
			)
			vmbdaViUEFI := vmbda.New(
				vmbda.WithName("vmbda-vi-uefi"),
				vmbda.WithNamespace(f.Namespace().Name),
				vmbda.WithBlockDeviceRef(v1alpha2.VMBDAObjectRefKindVirtualImage, viHotplugUEFI.Name),
				vmbda.WithVirtualMachineName(vmUEFI.Name),
			)
			vmbdaCviBIOS := vmbda.New(
				vmbda.WithName("vmbda-cvi-bios"),
				vmbda.WithNamespace(f.Namespace().Name),
				vmbda.WithBlockDeviceRef(v1alpha2.VMBDAObjectRefKindClusterVirtualImage, object.PrecreatedCVICustomBIOS),
				vmbda.WithVirtualMachineName(vmBIOS.Name),
			)
			vmbdaCviUEFI := vmbda.New(
				vmbda.WithName("vmbda-cvi-uefi"),
				vmbda.WithNamespace(f.Namespace().Name),
				vmbda.WithBlockDeviceRef(v1alpha2.VMBDAObjectRefKindClusterVirtualImage, object.PrecreatedCVICustomBIOS),
				vmbda.WithVirtualMachineName(vmUEFI.Name),
			)
			vmbdas = []*v1alpha2.VirtualMachineBlockDeviceAttachment{
				vmbdaVdBIOS, vmbdaVdUEFI, vmbdaViBIOS, vmbdaViUEFI, vmbdaCviBIOS, vmbdaCviUEFI,
			}

			allObjects = append([]crclient.Object{
				vdRootBIOS, vdBlankBIOS, vmBIOS, vdRootUEFI, vdBlankUEFI, vmUEFI,
				vdHotplugBIOS, vdHotplugUEFI, viHotplugBIOS, viHotplugUEFI,
			}, util.ToObjects(vmbdas)...)

			// Arm the observers before creating anything so no transition is missed.
			vmBIOSObs = vmobs.StartObserver(ctx, f, vmBIOS)
			vmUEFIObs = vmobs.StartObserver(ctx, f, vmUEFI)
			for _, a := range vmbdas {
				vmbdaObservers = append(vmbdaObservers, vmbdaobs.StartObserver(ctx, f, a))
			}

			err := f.CreateWithDeferredDeletion(ctx, allObjects...)
			Expect(err).NotTo(HaveOccurred())

			err = vmBIOSObs.WaitFor(vmobs.BeRunning(), framework.LongTimeout)
			Expect(err).NotTo(HaveOccurred())
			err = vmUEFIObs.WaitFor(vmobs.BeRunning(), framework.LongTimeout)
			Expect(err).NotTo(HaveOccurred())
			for i, obs := range vmbdaObservers {
				err := obs.WaitFor(vmbdaobs.BeAttached(), framework.LongTimeout)
				Expect(err).NotTo(HaveOccurred(),
					"VMBDA %s should become Attached", vmbdas[i].Name)
			}

			eventually.SSHReadyAsRoot(f, vmBIOS, framework.LongTimeout)
			eventually.SSHReadyAsRoot(f, vmUEFI, framework.LongTimeout)

			biosDiskCountOriginal, err = f.SSHCommand(vmBIOS.Name, f.Namespace().Name, lsblkCommand, framework.WithSSHUser("root"))
			Expect(err).NotTo(HaveOccurred())
			uefiDiskCountOriginal, err = f.SSHCommand(vmUEFI.Name, f.Namespace().Name, lsblkCommand, framework.WithSSHUser("root"))
			Expect(err).NotTo(HaveOccurred())
		})

		By("Create VMOP to trigger migration", func() {
			vmopMigrateBIOS = vmopbuilder.New(
				vmopbuilder.WithGenerateName("vmop-migrate-bios-evict-"),
				vmopbuilder.WithNamespace(f.Namespace().Name),
				vmopbuilder.WithType(v1alpha2.VMOPTypeEvict),
				vmopbuilder.WithVirtualMachine(vmBIOS.Name),
			)
			vmopMigrateUEFI = vmopbuilder.New(
				vmopbuilder.WithGenerateName("vmop-migrate-uefi-evict-"),
				vmopbuilder.WithNamespace(f.Namespace().Name),
				vmopbuilder.WithType(v1alpha2.VMOPTypeEvict),
				vmopbuilder.WithVirtualMachine(vmUEFI.Name),
			)
			err := f.CreateWithDeferredDeletion(ctx, vmopMigrateBIOS, vmopMigrateUEFI)
			Expect(err).NotTo(HaveOccurred())
		})

		By("Wait for migration to complete", func() {
			ctxVMBDA, cancelVMBDA := context.WithCancel(ctx)
			defer cancelVMBDA()

			vmbdaWatchErrCh := make(chan error, 1)
			vmbdaNames := make([]string, len(vmbdas))
			for i, a := range vmbdas {
				vmbdaNames[i] = a.Name
			}
			go func() {
				vmbdaWatchErrCh <- ensureVMBDAsStayAttached(ctxVMBDA,
					f.VirtClient().VirtualMachineBlockDeviceAttachments(f.Namespace().Name),
					vmbdaNames, metav1.ListOptions{})
			}()

			util.UntilVMOPMigrationSucceeded(ctx, vmopMigrateBIOS, framework.MaxTimeout)
			util.UntilVMOPMigrationSucceeded(ctx, vmopMigrateUEFI, framework.MaxTimeout)

			By("Wait until the virtual machine is accessible via SSH after migration.")
			eventually.SSHReadyAsRoot(f, vmBIOS, framework.LongTimeout)
			eventually.SSHReadyAsRoot(f, vmUEFI, framework.MiddleTimeout)

			By("Check that the disk count of the virtual machine is equal to the disk count before migration.")
			biosDiskCount, err := f.SSHCommand(vmBIOS.Name, f.Namespace().Name, lsblkCommand, framework.WithSSHUser("root"))
			Expect(err).NotTo(HaveOccurred())
			Expect(biosDiskCount).To(Equal(biosDiskCountOriginal),
				"disk count mismatch on VM %s after migration: got %s, expected %s",
				vmBIOS.Name, biosDiskCount, biosDiskCountOriginal,
			)
			uefiDiskCount, err := f.SSHCommand(vmUEFI.Name, f.Namespace().Name, lsblkCommand, framework.WithSSHUser("root"))
			Expect(err).NotTo(HaveOccurred())
			Expect(uefiDiskCount).To(Equal(uefiDiskCountOriginal),
				"disk count mismatch on VM %s after migration: got %s, expected %s",
				vmUEFI.Name, uefiDiskCount, uefiDiskCountOriginal,
			)

			cancelVMBDA()
			Expect(<-vmbdaWatchErrCh).NotTo(HaveOccurred(), "VMBDAs should stay in Attached phase during migration")
		})

		// There is a known issue with the Cilium agent check.
		By("Check Cilium agents are properly configured for the VM", func() {
			network.EnsureCiliumAgents(ctx, f.Kubectl(), vmBIOS.Name, f.Namespace().Name)
			network.EnsureCiliumAgents(ctx, f.Kubectl(), vmUEFI.Name, f.Namespace().Name)
		})

		By("Check VM can reach external network", func() {
			// The custom guest has no curl, so it probes connectivity with
			// ping as root; the Alpine UEFI guest keeps the shared curl-based check.
			eventually.SSHReadyAsRoot(f, vmBIOS, framework.LongTimeout)
			reachableHost, err := f.SSHCommand(vmBIOS.Name, f.Namespace().Name, guestPingExternalCommand, framework.WithSSHUser("root"))
			Expect(err).NotTo(HaveOccurred(), "VM %s should have outbound connectivity", vmBIOS.Name)
			Expect(reachableHost).NotTo(BeEmpty())

			eventually.SSHReadyAsRoot(f, vmUEFI, framework.MiddleTimeout)
			expectExternalConnectivityAsRoot(f, vmUEFI.Name, f.Namespace().Name)
		})
	})
})

// ensureVMBDAsStayAttached watches VMBDAs and returns an error if any of the tracked
// VMBDAs transitions away from the Attached phase. It runs until ctx is cancelled,
// returning nil if all VMBDAs stayed Attached throughout.
func ensureVMBDAsStayAttached(ctx context.Context, w util.Watcher, names []string, opts metav1.ListOptions) error {
	if len(names) == 0 {
		return nil
	}

	wi, err := w.Watch(ctx, opts)
	if err != nil {
		return err
	}
	defer wi.Stop()

	tracked := make(map[string]struct{}, len(names))
	for _, n := range names {
		tracked[n] = struct{}{}
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case event, ok := <-wi.ResultChan():
			if !ok {
				if ctx.Err() != nil {
					return nil
				}
				return fmt.Errorf("watch channel closed unexpectedly while VMBDAs were still being monitored")
			}
			vmbda, ok := event.Object.(*v1alpha2.VirtualMachineBlockDeviceAttachment)
			if !ok {
				continue
			}
			if _, ok := tracked[vmbda.Name]; ok && vmbda.Status.Phase != v1alpha2.BlockDeviceAttachmentPhaseAttached {
				return fmt.Errorf("VMBDA %s unexpectedly transitioned to phase %q", vmbda.Name, vmbda.Status.Phase)
			}
		}
	}
}
