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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/builder/cvi"
	"github.com/deckhouse/virtualization-controller/pkg/builder/vd"
	"github.com/deckhouse/virtualization-controller/pkg/builder/vi"
	"github.com/deckhouse/virtualization-controller/pkg/builder/vm"
	"github.com/deckhouse/virtualization-controller/pkg/builder/vmbda"
	vmopbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vmop"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	"github.com/deckhouse/virtualization/test/e2e/internal/network"
	"github.com/deckhouse/virtualization/test/e2e/internal/object"
	"github.com/deckhouse/virtualization/test/e2e/internal/util"
)

const lsblkCommand = "lsblk -dn | wc -l"

var _ = Describe("VirtualMachineMigration", func() {
	var (
		// Core: VMs and their root/blank disks
		vdRootBIOS  *v1alpha2.VirtualDisk
		vdBlankBIOS *v1alpha2.VirtualDisk
		vdRootUEFI  *v1alpha2.VirtualDisk
		vdBlankUEFI *v1alpha2.VirtualDisk
		vmBIOS      *v1alpha2.VirtualMachine
		vmUEFI      *v1alpha2.VirtualMachine

		// Hotplug: disks and images attached via VMBDAs
		vdHotplugBIOS  *v1alpha2.VirtualDisk
		vdHotplugUEFI  *v1alpha2.VirtualDisk
		viHotplugBIOS  *v1alpha2.VirtualImage
		viHotplugUEFI  *v1alpha2.VirtualImage
		cviHotplugBIOS *v1alpha2.ClusterVirtualImage
		cviHotplugUEFI *v1alpha2.ClusterVirtualImage

		vmbdas     []*v1alpha2.VirtualMachineBlockDeviceAttachment
		allObjects []crclient.Object

		vmopMigrateBIOS *v1alpha2.VirtualMachineOperation
		vmopMigrateUEFI *v1alpha2.VirtualMachineOperation

		f                     = framework.NewFramework("vm-migration")
		biosDiskCountOriginal string
		uefiDiskCountOriginal string
	)

	BeforeEach(func() {
		DeferCleanup(f.After)

		f.Before()
	})

	It("verifies that migrations are successful", func() {
		By("Environment preparation", func() {
			vdRootBIOS = vd.New(
				vd.WithName("vd-root-bios"),
				vd.WithNamespace(f.Namespace().Name),
				vd.WithSize(ptr.To(resource.MustParse("10Gi"))),
				vd.WithDataSourceHTTP(&v1alpha2.DataSourceHTTP{
					URL: object.ImageURLUbuntu,
				}),
			)
			vdBlankBIOS = vd.New(
				vd.WithName("vd-blank-bios"),
				vd.WithNamespace(f.Namespace().Name),
				vd.WithSize(ptr.To(resource.MustParse("100Mi"))),
			)

			vdRootUEFI = vd.New(
				vd.WithName("vd-root-uefi"),
				vd.WithNamespace(f.Namespace().Name),
				vd.WithSize(ptr.To(resource.MustParse("10Gi"))),
				vd.WithDataSourceHTTP(&v1alpha2.DataSourceHTTP{
					URL: object.ImageURLAlpineUEFI,
				}),
			)
			vdBlankUEFI = vd.New(
				vd.WithName("vd-blank-uefi"),
				vd.WithNamespace(f.Namespace().Name),
				vd.WithSize(ptr.To(resource.MustParse("100Mi"))),
			)
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
				vm.WithProvisioningUserData(object.DefaultCloudInit),
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
				vm.WithProvisioningUserData(object.DefaultCloudInit),
				vm.WithLiveMigrationPolicy(v1alpha2.PreferSafeMigrationPolicy),
				vm.WithName("vm-uefi"),
			)

			// --- Hotplug resources ---
			vdHotplugBIOS = vd.New(
				vd.WithName("vd-hotplug-bios"),
				vd.WithNamespace(f.Namespace().Name),
				vd.WithSize(ptr.To(resource.MustParse("100Mi"))),
			)
			vdHotplugUEFI = vd.New(
				vd.WithName("vd-hotplug-uefi"),
				vd.WithNamespace(f.Namespace().Name),
				vd.WithSize(ptr.To(resource.MustParse("100Mi"))),
			)

			viHotplugBIOS = vi.New(
				vi.WithName("vi-hotplug-bios"),
				vi.WithNamespace(f.Namespace().Name),
				vi.WithDataSourceHTTP(object.ImageTestDataQCOW, nil, nil),
				vi.WithStorage(v1alpha2.StorageContainerRegistry),
			)
			viHotplugUEFI = vi.New(
				vi.WithName("vi-hotplug-uefi"),
				vi.WithNamespace(f.Namespace().Name),
				vi.WithDataSourceHTTP(object.ImageTestDataQCOW, nil, nil),
				vi.WithStorage(v1alpha2.StorageContainerRegistry),
			)

			cviHotplugBIOS = cvi.New(
				cvi.WithName("cvi-hotplug-bios"),
				cvi.WithDataSourceHTTP(object.ImageTestDataQCOW, nil, nil),
			)
			cviHotplugUEFI = cvi.New(
				cvi.WithName("cvi-hotplug-uefi"),
				cvi.WithDataSourceHTTP(object.ImageTestDataQCOW, nil, nil),
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
				vmbda.WithBlockDeviceRef(v1alpha2.VMBDAObjectRefKindClusterVirtualImage, cviHotplugBIOS.Name),
				vmbda.WithVirtualMachineName(vmBIOS.Name),
			)
			vmbdaCviUEFI := vmbda.New(
				vmbda.WithName("vmbda-cvi-uefi"),
				vmbda.WithNamespace(f.Namespace().Name),
				vmbda.WithBlockDeviceRef(v1alpha2.VMBDAObjectRefKindClusterVirtualImage, cviHotplugUEFI.Name),
				vmbda.WithVirtualMachineName(vmUEFI.Name),
			)
			vmbdas = []*v1alpha2.VirtualMachineBlockDeviceAttachment{
				vmbdaVdBIOS, vmbdaVdUEFI, vmbdaViBIOS, vmbdaViUEFI, vmbdaCviBIOS, vmbdaCviUEFI,
			}

			allObjects = append([]crclient.Object{
				vdRootBIOS, vdBlankBIOS, vmBIOS, vdRootUEFI, vdBlankUEFI, vmUEFI,
				vdHotplugBIOS, vdHotplugUEFI, viHotplugBIOS, viHotplugUEFI,
				cviHotplugBIOS, cviHotplugUEFI,
			}, toObjects(vmbdas)...)
			err := f.CreateWithDeferredDeletion(context.Background(), allObjects...)
			Expect(err).NotTo(HaveOccurred())

			util.UntilObjectPhase(string(v1alpha2.MachineRunning), framework.LongTimeout, vmBIOS, vmUEFI)
			util.UntilObjectPhase(
				string(v1alpha2.BlockDeviceAttachmentPhaseAttached), framework.LongTimeout,
				toObjects(vmbdas)...,
			)

			util.UntilSSHReady(f, vmBIOS, framework.LongTimeout)
			util.UntilSSHReady(f, vmUEFI, framework.LongTimeout)

			biosDiskCountOriginal, err = f.SSHCommand(vmBIOS.Name, f.Namespace().Name, lsblkCommand)
			Expect(err).NotTo(HaveOccurred())
			uefiDiskCountOriginal, err = f.SSHCommand(vmUEFI.Name, f.Namespace().Name, lsblkCommand)
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
			err := f.CreateWithDeferredDeletion(context.Background(), vmopMigrateBIOS, vmopMigrateUEFI)
			Expect(err).NotTo(HaveOccurred())
		})

		By("Wait for migration to complete", func() {
			ctxVMBDA, cancelVMBDA := context.WithCancel(context.Background())
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

			Eventually(func(g Gomega) {
				err := f.GenericClient().Get(context.Background(), crclient.ObjectKeyFromObject(vmBIOS), vmBIOS)
				Expect(err).NotTo(HaveOccurred()) // Intentionally fail the test on a single error, so g.Expect is not needed
				err = f.GenericClient().Get(context.Background(), crclient.ObjectKeyFromObject(vmUEFI), vmUEFI)
				Expect(err).NotTo(HaveOccurred()) // Intentionally fail the test on a single error, so g.Expect is not needed
				// TODO: remove temporary migration skip logic when both known issues are fixed:
				// kubevirt "client socket is closed" and Volume(s)UpdateError.
				util.SkipIfKnownMigrationFailure(vmBIOS)
				util.SkipIfKnownMigrationFailure(vmUEFI)

				err = f.GenericClient().Get(context.Background(), crclient.ObjectKeyFromObject(vmopMigrateBIOS), vmopMigrateBIOS)
				Expect(err).NotTo(HaveOccurred()) // Intentionally fail the test on a single error, so g.Expect is not needed
				err = f.GenericClient().Get(context.Background(), crclient.ObjectKeyFromObject(vmopMigrateUEFI), vmopMigrateUEFI)
				Expect(err).NotTo(HaveOccurred()) // Intentionally fail the test on a single error, so g.Expect is not needed

				biosDiskCount, err := f.SSHCommand(vmBIOS.Name, f.Namespace().Name, lsblkCommand)
				Expect(err).NotTo(HaveOccurred())
				Expect(biosDiskCount).To(Equal(biosDiskCountOriginal))
				uefiDiskCount, err := f.SSHCommand(vmUEFI.Name, f.Namespace().Name, lsblkCommand)
				Expect(err).NotTo(HaveOccurred())
				Expect(uefiDiskCount).To(Equal(uefiDiskCountOriginal))

				g.Expect(vmopMigrateBIOS.Status.Phase).To(Equal(v1alpha2.VMOPPhaseCompleted))
				g.Expect(vmopMigrateUEFI.Status.Phase).To(Equal(v1alpha2.VMOPPhaseCompleted))
			}).WithPolling(time.Second).WithTimeout(framework.LongTimeout).To(Succeed())

			cancelVMBDA()
			Expect(<-vmbdaWatchErrCh).NotTo(HaveOccurred(), "VMBDAs should stay in Attached phase during migration")
		})

		// There is a known issue with the Cilium agent check.
		By("Check Cilium agents are properly configured for the VM", func() {
			err := network.CheckCiliumAgents(context.Background(), f.Kubectl(), vmBIOS.Name, f.Namespace().Name)
			Expect(err).NotTo(HaveOccurred(), "Cilium agents check should succeed for VM %s", vmBIOS.Name)
			err = network.CheckCiliumAgents(context.Background(), f.Kubectl(), vmUEFI.Name, f.Namespace().Name)
			Expect(err).NotTo(HaveOccurred(), "Cilium agents check should succeed for VM %s", vmUEFI.Name)
		})

		By("Check VM can reach external network", func() {
			util.UntilSSHReady(f, vmBIOS, framework.MiddleTimeout)
			util.UntilSSHReady(f, vmUEFI, framework.MiddleTimeout)
			network.CheckExternalConnectivity(f, vmBIOS.Name, network.ExternalHost, network.HTTPStatusOk)
			network.CheckExternalConnectivity(f, vmUEFI.Name, network.ExternalHost, network.HTTPStatusOk)
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

func toObjects[T crclient.Object](objs []T) []crclient.Object {
	out := make([]crclient.Object, len(objs))
	for i, o := range objs {
		out[i] = o
	}
	return out
}
