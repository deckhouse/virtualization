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

package blockdevice

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"

	vdbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vd"
	vdsnapshotbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vdsnapshot"
	vmbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vm"
	vmopbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vmop"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	"github.com/deckhouse/virtualization/test/e2e/internal/label"
	"github.com/deckhouse/virtualization/test/e2e/internal/object"
	"github.com/deckhouse/virtualization/test/e2e/internal/util"
)

const (
	mountPointData = "/mnt"
	fileDataPath   = "/mnt/testfile"
	testFileValue  = "test-file-value"
)

var _ = Describe("DataExports", label.Slow(), func() {
	f := framework.NewFramework("data-exports")

	BeforeEach(func() {
		f.Before()
		DeferCleanup(f.After)
	})

	It("restores disk data from a VirtualDiskSnapshot", func() {
		var (
			vdRoot     *v1alpha2.VirtualDisk
			vdBlank    *v1alpha2.VirtualDisk
			vm         *v1alpha2.VirtualMachine
			vdSnapshot *v1alpha2.VirtualDiskSnapshot
			vdRestored *v1alpha2.VirtualDisk
		)

		By("Creating VirtualDisks", func() {
			vdRoot = vdbuilder.New(
				vdbuilder.WithName("vd-root"),
				vdbuilder.WithNamespace(f.Namespace().Name),
				vdbuilder.WithDataSourceHTTP(&v1alpha2.DataSourceHTTP{
					URL: object.ImageURLUbuntu,
				}),
			)

			vdBlank = vdbuilder.New(
				vdbuilder.WithName("vd-blank"),
				vdbuilder.WithNamespace(f.Namespace().Name),
				vdbuilder.WithPersistentVolumeClaim(nil, ptr.To(resource.MustParse("51Mi"))),
			)

			err := f.CreateWithDeferredDeletion(context.Background(), vdRoot, vdBlank)
			Expect(err).NotTo(HaveOccurred())
		})

		By("Creating VirtualMachine with two disks", func() {
			vm = vmbuilder.New(
				vmbuilder.WithName("vm"),
				vmbuilder.WithNamespace(f.Namespace().Name),
				vmbuilder.WithCPU(1, ptr.To("5%")),
				vmbuilder.WithMemory(resource.MustParse("256Mi")),
				vmbuilder.WithLiveMigrationPolicy(v1alpha2.AlwaysSafeMigrationPolicy),
				vmbuilder.WithVirtualMachineClass(object.DefaultVMClass),
				vmbuilder.WithBlockDeviceRefs(
					v1alpha2.BlockDeviceSpecRef{
						Kind: v1alpha2.DiskDevice,
						Name: vdRoot.Name,
					},
					v1alpha2.BlockDeviceSpecRef{
						Kind: v1alpha2.DiskDevice,
						Name: vdBlank.Name,
					},
				),
				vmbuilder.WithRunPolicy(v1alpha2.AlwaysOnUnlessStoppedManually),
				vmbuilder.WithProvisioningUserData(object.DefaultCloudInit),
			)

			err := f.CreateWithDeferredDeletion(context.Background(), vm)
			Expect(err).NotTo(HaveOccurred())
		})

		By("Waiting for VM to be ready", func() {
			util.UntilVMAgentReady(crclient.ObjectKeyFromObject(vm), framework.LongTimeout)
		})

		By("Writing data to the blank disk", func() {
			util.CreateBlockDeviceFilesystem(f, vm, v1alpha2.DiskDevice, vdBlank.Name, "ext4")
			util.MountBlockDevice(f, vm, v1alpha2.DiskDevice, vdBlank.Name, mountPointData)
			util.WriteFile(f, vm, fileDataPath, testFileValue)
			util.UnmountBlockDevice(f, vm, mountPointData)
		})

		By("Stopping the VM", func() {
			vmopStop := vmopbuilder.New(
				vmopbuilder.WithGenerateName(fmt.Sprintf("%s-stop-", util.VmopE2ePrefix)),
				vmopbuilder.WithNamespace(vm.Namespace),
				vmopbuilder.WithType(v1alpha2.VMOPTypeStop),
				vmopbuilder.WithVirtualMachine(vm.Name),
			)
			err := f.CreateWithDeferredDeletion(context.Background(), vmopStop)
			Expect(err).NotTo(HaveOccurred())

			util.UntilObjectPhase(string(v1alpha2.VMOPPhaseCompleted), framework.LongTimeout, vmopStop)
			util.UntilObjectPhase(string(v1alpha2.MachineStopped), framework.ShortTimeout, vm)
		})

		By("Creating VirtualDiskSnapshot from the blank disk", func() {
			vdSnapshot = vdsnapshotbuilder.New(
				vdsnapshotbuilder.WithName("vdsnapshot-blank"),
				vdsnapshotbuilder.WithNamespace(f.Namespace().Name),
				vdsnapshotbuilder.WithVirtualDiskName(vdBlank.Name),
				vdsnapshotbuilder.WithRequiredConsistency(true),
			)

			err := f.CreateWithDeferredDeletion(context.Background(), vdSnapshot)
			Expect(err).NotTo(HaveOccurred())
			util.UntilObjectPhase(string(v1alpha2.VirtualDiskSnapshotPhaseReady), framework.ShortTimeout, vdSnapshot)
		})

		By("Deleting the blank disk", func() {
			err := f.Delete(context.Background(), vdBlank)
			Expect(err).NotTo(HaveOccurred())

			Eventually(func(g Gomega) {
				var vdLocal v1alpha2.VirtualDisk
				err := f.Clients.GenericClient().Get(context.Background(), types.NamespacedName{
					Namespace: vdBlank.Namespace,
					Name:      vdBlank.Name,
				}, &vdLocal)
				g.Expect(crclient.IgnoreNotFound(err)).NotTo(HaveOccurred())
				g.Expect(err).To(HaveOccurred(), "VirtualDisk should be deleted")
			}, framework.MiddleTimeout, time.Second).Should(Succeed())
		})

		By("Creating a new disk from the VirtualDiskSnapshot", func() {
			vdRestored = vdbuilder.New(
				vdbuilder.WithName("vd-restored"),
				vdbuilder.WithNamespace(f.Namespace().Name),
				vdbuilder.WithDataSourceObjectRef(v1alpha2.VirtualDiskObjectRefKindVirtualDiskSnapshot, vdSnapshot.Name),
			)

			err := f.CreateWithDeferredDeletion(context.Background(), vdRestored)
			Expect(err).NotTo(HaveOccurred())
			util.UntilObjectPhase(string(v1alpha2.DiskWaitForFirstConsumer), framework.LongTimeout, vdRestored)
		})

		By("Updating VM to use the restored disk", func() {
			err := f.Clients.GenericClient().Get(context.Background(), crclient.ObjectKeyFromObject(vm), vm)
			Expect(err).NotTo(HaveOccurred())

			vm.Spec.BlockDeviceRefs = []v1alpha2.BlockDeviceSpecRef{
				{
					Kind: v1alpha2.DiskDevice,
					Name: vdRoot.Name,
				},
				{
					Kind: v1alpha2.DiskDevice,
					Name: vdRestored.Name,
				},
			}

			err = f.Clients.GenericClient().Update(context.Background(), vm)
			Expect(err).NotTo(HaveOccurred())
		})

		By("Starting the VM", func() {
			util.StartVirtualMachine(f, vm)
			util.UntilVMAgentReady(crclient.ObjectKeyFromObject(vm), framework.LongTimeout)
		})

		By("Verifying that the data is restored", func() {
			util.MountBlockDevice(f, vm, v1alpha2.DiskDevice, vdRestored.Name, mountPointData)
			restoredValue := util.ReadFile(f, vm, fileDataPath)
			Expect(restoredValue).To(Equal(testFileValue), "The data should be restored from the snapshot")
		})
	})
})
