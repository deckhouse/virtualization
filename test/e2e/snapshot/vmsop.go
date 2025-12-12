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

package snapshot

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"

	vdbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vd"
	vmbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vm"
	vmbdabuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vmbda"
	vmsbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vmsnapshot"
	vmsopbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vmsop"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	"github.com/deckhouse/virtualization/test/e2e/internal/object"
	"github.com/deckhouse/virtualization/test/e2e/internal/util"
)

var _ = Describe("VMSOPCreateVirtualMachine", Ordered, func() {
	var (
		vd         *v1alpha2.VirtualDisk
		vdBlank    *v1alpha2.VirtualDisk
		vm         *v1alpha2.VirtualMachine
		vmsnapshot *v1alpha2.VirtualMachineSnapshot
		vmsop      *v1alpha2.VirtualMachineSnapshotOperation
		vmbda      *v1alpha2.VirtualMachineBlockDeviceAttachment

		f = framework.NewFramework("vmsop")
	)

	BeforeAll(func() {
		DeferCleanup(f.After)

		f.Before()
	})

	AfterAll(func() {
		DeferCleanup(f.After)
	})

	It("should prepare environment", func() {
		By("create vm", func() {
			vd = vdbuilder.New(
				vdbuilder.WithName("vd-root"),
				vdbuilder.WithNamespace(f.Namespace().Name),
				vdbuilder.WithSize(ptr.To(resource.MustParse("10Gi"))),
				vdbuilder.WithDataSourceHTTP(&v1alpha2.DataSourceHTTP{
					URL: object.ImageURLAlpineBIOS,
				}),
			)

			vm = object.NewMinimalVM("vmsop-origin-", f.Namespace().Name,
				vmbuilder.WithBlockDeviceRefs(
					v1alpha2.BlockDeviceSpecRef{
						Kind: v1alpha2.VirtualDiskKind,
						Name: vd.Name,
					},
				),
			)

			err := f.CreateWithDeferredDeletion(context.Background(), vd, vm)
			Expect(err).NotTo(HaveOccurred())

			util.UntilVMAgentReady(crclient.ObjectKeyFromObject(vm), framework.LongTimeout)
		})

		By("create vmbda", func() {
			vdBlank = vdbuilder.New(
				vdbuilder.WithName("vd-blank"),
				vdbuilder.WithNamespace(f.Namespace().Name),
				vdbuilder.WithSize(ptr.To(resource.MustParse("100Mi"))),
			)

			vmbda = vmbdabuilder.New(
				vmbdabuilder.WithName("vmbda"),
				vmbdabuilder.WithNamespace(f.Namespace().Name),
				vmbdabuilder.WithVirtualMachineName(vm.Name),
				vmbdabuilder.WithBlockDeviceRef(v1alpha2.VMBDAObjectRefKindVirtualDisk, vdBlank.Name),
			)
			err := f.CreateWithDeferredDeletion(context.Background(), vmbda, vdBlank)
			Expect(err).NotTo(HaveOccurred())

			util.UntilObjectPhase(string(v1alpha2.BlockDeviceAttachmentPhaseAttached), framework.LongTimeout, vmbda)
		})

		By("create vmsnapshot", func() {
			vmsnapshot = vmsbuilder.New(
				vmsbuilder.WithName("vmsnapshot"),
				vmsbuilder.WithNamespace(f.Namespace().Name),
				vmsbuilder.WithVirtualMachineName(vm.Name),
				vmsbuilder.WithKeepIPAddress(v1alpha2.KeepIPAddressNever),
				vmsbuilder.WithRequiredConsistency(false),
			)

			err := f.CreateWithDeferredDeletion(context.Background(), vmsnapshot)
			Expect(err).NotTo(HaveOccurred())

			util.UntilObjectPhase(string(v1alpha2.VirtualMachineSnapshotPhaseReady), framework.LongTimeout, vmsnapshot)
		})
	})

	DescribeTable("VMSOP with different modes",
		func(prefix string, mode v1alpha2.SnapshotOperationMode) {
			clonedName := func(name string) string {
				return fmt.Sprintf("%s%s", prefix, name)
			}

			By("Create and wait for VMSOP", func() {
				vmsop = vmsopbuilder.New(
					vmsopbuilder.WithName(prefix+"vmsop"),
					vmsopbuilder.WithNamespace(f.Namespace().Name),
					vmsopbuilder.WithVirtualMachineSnapshotName(vmsnapshot.Name),
					vmsopbuilder.WithCreateVirtualMachine(&v1alpha2.VMSOPCreateVirtualMachineSpec{
						Mode: mode,
						Customization: &v1alpha2.VMSOPCreateVirtualMachineCustomization{
							NamePrefix: prefix,
						},
					}),
				)

				err := f.CreateWithDeferredDeletion(context.Background(), vmsop)
				Expect(err).NotTo(HaveOccurred())

				util.UntilObjectPhase(string(v1alpha2.VMSOPPhaseCompleted), framework.LongTimeout, vmsop)
			})

			By("Check that resounsec doesn't exist for DryRun mode", func() {
				if mode != v1alpha2.SnapshotOperationModeDryRun {
					return
				}

				err := f.VirtClient().VirtualMachines(f.Namespace().Name).Delete(context.Background(), clonedName(vm.Name), metav1.DeleteOptions{})
				Expect(err).To(HaveOccurred())

				err = f.VirtClient().VirtualMachineBlockDeviceAttachments(f.Namespace().Name).Delete(context.Background(), clonedName(vmbda.Name), metav1.DeleteOptions{})
				Expect(err).To(HaveOccurred())

				err = f.VirtClient().VirtualDisks(f.Namespace().Name).Delete(context.Background(), clonedName(vd.Name), metav1.DeleteOptions{})
				Expect(err).To(HaveOccurred())

				err = f.VirtClient().VirtualDisks(f.Namespace().Name).Delete(context.Background(), clonedName(vdBlank.Name), metav1.DeleteOptions{})
				Expect(err).To(HaveOccurred())
			})

			By("Verify that the created VM is running", func() {
				if mode == v1alpha2.SnapshotOperationModeDryRun {
					return
				}

				createdVM, err := f.VirtClient().VirtualMachines(f.Namespace().Name).Get(context.Background(), clonedName(vm.Name), metav1.GetOptions{})
				Expect(err).NotTo(HaveOccurred())

				createdVMBDA, err := f.VirtClient().VirtualMachineBlockDeviceAttachments(f.Namespace().Name).Get(context.Background(), clonedName(vmbda.Name), metav1.GetOptions{})
				Expect(err).NotTo(HaveOccurred())

				util.UntilVMAgentReady(crclient.ObjectKeyFromObject(createdVM), framework.LongTimeout)
				util.UntilObjectPhase(string(v1alpha2.BlockDeviceAttachmentPhaseAttached), framework.LongTimeout, createdVMBDA)
			})

			By("Delete created Resources", func() {
				if mode == v1alpha2.SnapshotOperationModeDryRun {
					return
				}

				err := f.VirtClient().VirtualMachines(f.Namespace().Name).Delete(context.Background(), clonedName(vm.Name), metav1.DeleteOptions{})
				Expect(err).NotTo(HaveOccurred())

				err = f.VirtClient().VirtualMachineBlockDeviceAttachments(f.Namespace().Name).Delete(context.Background(), clonedName(vmbda.Name), metav1.DeleteOptions{})
				Expect(err).NotTo(HaveOccurred())

				err = f.VirtClient().VirtualDisks(f.Namespace().Name).Delete(context.Background(), clonedName(vd.Name), metav1.DeleteOptions{})
				Expect(err).NotTo(HaveOccurred())

				err = f.VirtClient().VirtualDisks(f.Namespace().Name).Delete(context.Background(), clonedName(vdBlank.Name), metav1.DeleteOptions{})
				Expect(err).NotTo(HaveOccurred())
			})
		},
		Entry("VMSOP with BestEffort mode should complete successfully", "best-effort-", v1alpha2.SnapshotOperationModeBestEffort),
		Entry("VMSOP with Strict mode should complete successfully", "strict-", v1alpha2.SnapshotOperationModeStrict),
		Entry("VMSOP with DryRun mode should complete and do nothing", "dry-run-", v1alpha2.SnapshotOperationModeDryRun),
	)
})
