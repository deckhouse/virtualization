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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	vdbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vd"
	vmbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vm"
	vmbdabuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vmbda"
	vmsbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vmsnapshot"
	vmsopbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vmsop"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	"github.com/deckhouse/virtualization/test/e2e/internal/label"
	"github.com/deckhouse/virtualization/test/e2e/internal/object"
	vmobs "github.com/deckhouse/virtualization/test/e2e/internal/observer/vm"
	vmbdaobs "github.com/deckhouse/virtualization/test/e2e/internal/observer/vmbda"
	vmsnapshotobs "github.com/deckhouse/virtualization/test/e2e/internal/observer/vmsnapshot"
	vmsopobs "github.com/deckhouse/virtualization/test/e2e/internal/observer/vmsop"
	"github.com/deckhouse/virtualization/test/e2e/internal/precheck"
	"github.com/deckhouse/virtualization/test/e2e/internal/util"
)

// vdSize is the size for both the image-backed root disk and the blank disk.
// The custom image (~47Mi virtual) grows its root filesystem to the
// disk on first boot, so 64Mi is enough.
const vdSize = "64Mi"

var _ = Describe("VMSOPCreateVirtualMachine", Label(label.SIGCompute, precheck.PrecheckSnapshot), func() {
	var (
		vd         *v1alpha2.VirtualDisk
		vdBlank    *v1alpha2.VirtualDisk
		vm         *v1alpha2.VirtualMachine
		vmsnapshot *v1alpha2.VirtualMachineSnapshot
		vmsop      *v1alpha2.VirtualMachineSnapshotOperation
		vmbda      *v1alpha2.VirtualMachineBlockDeviceAttachment

		ctx context.Context
		f   *framework.Framework
	)

	// Each entry provisions its own environment (VM, VMBDA, snapshot) in its
	// own namespace, so the entries run in parallel.
	BeforeEach(func() {
		// TODO: Re-enable the suite.
		Skip("skipped as flaky: fix the instability, then remove this skip")

		ctx = context.Background()
		f = framework.NewFramework("vmsop-create-vm")
		cfg := framework.GetConfig()
		if cfg.StorageClass.DefaultStorageClass != nil && cfg.StorageClass.DefaultStorageClass.Provisioner == framework.NFS {
			Skip("Not working due to bug with VMBDA on NFS right now, skipping")
		}

		DeferCleanup(f.After)

		f.Before()

		By("create vm", func() {
			vd = object.NewVDFromCVI("vd-root", f.Namespace().Name, object.PrecreatedCVICustomBIOS, vdbuilder.WithSize(ptr.To(resource.MustParse(vdSize))))

			vm = object.NewMinimalVM("vmsop-origin-", f.Namespace().Name,
				// The custom image has no cloud-init; the guest agent is
				// baked into the image, so no provisioning is needed.
				vmbuilder.WithBlockDeviceRefs(
					v1alpha2.BlockDeviceSpecRef{
						Kind: v1alpha2.VirtualDiskKind,
						Name: vd.Name,
					},
				),
			)

			err := f.CreateWithDeferredDeletion(ctx, vd, vm)
			Expect(err).NotTo(HaveOccurred())

			vmObs := vmobs.StartObserver(ctx, f, vm)
			err = vmObs.WaitFor(vmobs.BeAgentReady(), framework.LongTimeout)
			Expect(err).NotTo(HaveOccurred())
		})

		By("create vmbda", func() {
			vdBlank = object.NewVD(
				vdbuilder.WithName("vd-blank"),
				vdbuilder.WithNamespace(f.Namespace().Name),
				vdbuilder.WithSize(ptr.To(resource.MustParse(vdSize))),
			)

			vmbda = vmbdabuilder.New(
				vmbdabuilder.WithName("vmbda"),
				vmbdabuilder.WithNamespace(f.Namespace().Name),
				vmbdabuilder.WithVirtualMachineName(vm.Name),
				vmbdabuilder.WithBlockDeviceRef(v1alpha2.VMBDAObjectRefKindVirtualDisk, vdBlank.Name),
			)
			err := f.CreateWithDeferredDeletion(ctx, vmbda, vdBlank)
			Expect(err).NotTo(HaveOccurred())

			vmbdaObs := vmbdaobs.StartObserver(ctx, f, vmbda)
			err = vmbdaObs.WaitFor(vmbdaobs.BeAttached(), framework.LongTimeout)
			Expect(err).NotTo(HaveOccurred())
		})

		By("create vmsnapshot", func() {
			vmsnapshot = vmsbuilder.New(
				vmsbuilder.WithName("vmsnapshot"),
				vmsbuilder.WithNamespace(f.Namespace().Name),
				vmsbuilder.WithVirtualMachineName(vm.Name),
				vmsbuilder.WithKeepIPAddress(v1alpha2.KeepIPAddressNever),
				vmsbuilder.WithRequiredConsistency(false),
			)

			vmsObs := vmsnapshotobs.StartObserver(ctx, f, vmsnapshot)

			err := f.CreateWithDeferredDeletion(ctx, vmsnapshot)
			Expect(err).NotTo(HaveOccurred())

			waitVMSnapshotReady(vmsObs, framework.LongTimeout)
		})
	})

	DescribeTable("VMSOP with different modes",
		func(prefix string, mode v1alpha2.SnapshotOperationMode) {
			clonedName := func(name string) string {
				return fmt.Sprintf("%s%s", prefix, name)
			}

			// The clone is created by the VMSOP asynchronously, so its observers
			// are armed before the VMSOP is created: this way no transition (nor
			// an already-settled state) is missed by the waits below.
			var clonedVMObs vmobs.Observer
			var clonedVMBDAObs vmbdaobs.Observer
			if mode != v1alpha2.SnapshotOperationModeDryRun {
				clonedVM := &v1alpha2.VirtualMachine{ObjectMeta: metav1.ObjectMeta{
					Name: clonedName(vm.Name), Namespace: f.Namespace().Name,
				}}
				clonedVMBDA := &v1alpha2.VirtualMachineBlockDeviceAttachment{ObjectMeta: metav1.ObjectMeta{
					Name: clonedName(vmbda.Name), Namespace: f.Namespace().Name,
				}}
				clonedVMObs = vmobs.StartObserver(ctx, f, clonedVM)
				clonedVMBDAObs = vmbdaobs.StartObserver(ctx, f, clonedVMBDA)
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

				vmsopObs := vmsopobs.StartObserver(ctx, f, vmsop)

				err := f.CreateWithDeferredDeletion(ctx, vmsop)
				Expect(err).NotTo(HaveOccurred())

				err = vmsopObs.WaitFor(vmsopobs.BeCompleted(), framework.LongTimeout)
				Expect(err).NotTo(HaveOccurred())
			})

			By("Check that resounsec doesn't exist for DryRun mode", func() {
				if mode != v1alpha2.SnapshotOperationModeDryRun {
					return
				}

				err := f.VirtClient().VirtualMachines(f.Namespace().Name).Delete(ctx, clonedName(vm.Name), metav1.DeleteOptions{})
				Expect(err).To(HaveOccurred())

				err = f.VirtClient().VirtualMachineBlockDeviceAttachments(f.Namespace().Name).Delete(ctx, clonedName(vmbda.Name), metav1.DeleteOptions{})
				Expect(err).To(HaveOccurred())

				err = f.VirtClient().VirtualDisks(f.Namespace().Name).Delete(ctx, clonedName(vd.Name), metav1.DeleteOptions{})
				Expect(err).To(HaveOccurred())

				err = f.VirtClient().VirtualDisks(f.Namespace().Name).Delete(ctx, clonedName(vdBlank.Name), metav1.DeleteOptions{})
				Expect(err).To(HaveOccurred())
			})

			By("Verify that the created VM is running", func() {
				if mode == v1alpha2.SnapshotOperationModeDryRun {
					return
				}

				err := clonedVMObs.WaitFor(vmobs.BeAgentReady(), framework.LongTimeout)
				Expect(err).NotTo(HaveOccurred())
				err = clonedVMBDAObs.WaitFor(vmbdaobs.BeAttached(), framework.LongTimeout)
				Expect(err).NotTo(HaveOccurred())
			})

			By("Delete created Resources", func() {
				if mode == v1alpha2.SnapshotOperationModeDryRun {
					return
				}

				err := f.VirtClient().VirtualMachines(f.Namespace().Name).Delete(ctx, clonedName(vm.Name), metav1.DeleteOptions{})
				Expect(err).NotTo(HaveOccurred())

				err = f.VirtClient().VirtualMachineBlockDeviceAttachments(f.Namespace().Name).Delete(ctx, clonedName(vmbda.Name), metav1.DeleteOptions{})
				Expect(err).NotTo(HaveOccurred())

				err = f.VirtClient().VirtualDisks(f.Namespace().Name).Delete(ctx, clonedName(vd.Name), metav1.DeleteOptions{})
				Expect(err).NotTo(HaveOccurred())

				err = f.VirtClient().VirtualDisks(f.Namespace().Name).Delete(ctx, clonedName(vdBlank.Name), metav1.DeleteOptions{})
				Expect(err).NotTo(HaveOccurred())
			})
		},
		Entry("VMSOP with BestEffort mode should complete successfully", "best-effort-", v1alpha2.SnapshotOperationModeBestEffort),
		Entry("VMSOP with Strict mode should complete successfully", "strict-", v1alpha2.SnapshotOperationModeStrict),
		Entry("VMSOP with DryRun mode should complete and do nothing", "dry-run-", v1alpha2.SnapshotOperationModeDryRun),
	)
})

// waitVMSnapshotReady waits for the VirtualMachineSnapshot to become Ready.
// A snapshot that failed because the CSI driver could not create the
// underlying VolumeSnapshot skips the spec: that is a storage-infrastructure
// problem, not a virtualization one. Any other failure fails the spec.
func waitVMSnapshotReady(obs vmsnapshotobs.Observer, timeout time.Duration) {
	GinkgoHelper()
	err := obs.WaitFor(vmsnapshotobs.BeReady(), timeout)
	if err != nil && util.IsCSIVolumeSnapshotError(err.Error()) {
		Skip(fmt.Sprintf("VirtualMachineSnapshot failed on the CSI side, skipping: %s", err))
	}
	Expect(err).NotTo(HaveOccurred())
}
