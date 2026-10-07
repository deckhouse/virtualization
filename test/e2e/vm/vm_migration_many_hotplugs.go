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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"

	vdbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vd"
	vmbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vm"
	vmbdabuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vmbda"
	vmopbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vmop"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/test/e2e/eventually"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	"github.com/deckhouse/virtualization/test/e2e/internal/label"
	"github.com/deckhouse/virtualization/test/e2e/internal/object"
	vmobs "github.com/deckhouse/virtualization/test/e2e/internal/observer/vm"
	vmbdaobs "github.com/deckhouse/virtualization/test/e2e/internal/observer/vmbda"
	"github.com/deckhouse/virtualization/test/e2e/internal/precheck"
	"github.com/deckhouse/virtualization/test/e2e/internal/util"
)

const (
	// manyHotplugsDiskCount is the number of disks hotplugged to the VM through
	// VMBDAs: together with the root disk it fills the spec.blockDeviceRefs
	// limit of 16 block devices, the largest set a VM can carry.
	manyHotplugsDiskCount = 15

	// manyHotplugsMigrationCount is how many migrations run back to back. The
	// second one is the point of the spec: after the first migration every
	// hotplugged volume has been re-attached on the target, and that state
	// must be migratable again (the volumes of the internal VM and VMI must
	// still converge, otherwise the migration readiness check blocks it).
	manyHotplugsMigrationCount = 2
)

var _ = Describe("VirtualMachineMigrationWithManyHotplugs", Label(label.SIGCompute, precheck.NoPrecheck), func() {
	var (
		f   *framework.Framework
		ctx context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()
		f = framework.NewFramework("vm-migration-many-hotplugs")
		DeferCleanup(f.After)
		f.Before()
	})

	It("migrates a VM with 15 hotplugged disks twice in a row", func() {
		ns := f.Namespace().Name

		vdRoot := object.NewVDFromCVI("vd-root", ns, object.PrecreatedCVICustomBIOS,
			vdbuilder.WithSize(ptr.To(resource.MustParse(vdCustomImageSize))),
		)
		// The custom image has no cloud-init; the guest agent and lsblk are
		// baked into the image, so no provisioning is needed.
		vm := object.NewMinimalVM("", ns,
			vmbuilder.WithName("vm"),
			vmbuilder.WithBlockDeviceRefs(v1alpha2.BlockDeviceSpecRef{
				Kind: v1alpha2.VirtualDiskKind,
				Name: vdRoot.Name,
			}),
		)

		vds := make([]*v1alpha2.VirtualDisk, 0, manyHotplugsDiskCount)
		vmbdas := make([]*v1alpha2.VirtualMachineBlockDeviceAttachment, 0, manyHotplugsDiskCount)
		vdNames := make([]string, 0, manyHotplugsDiskCount)
		vmbdaNames := make([]string, 0, manyHotplugsDiskCount)
		for i := range manyHotplugsDiskCount {
			vd := object.NewBlankVD(fmt.Sprintf("vd-hotplug-%02d", i), ns, nil, ptr.To(resource.MustParse(vdCustomImageSize)))
			vmbda := vmbdabuilder.New(
				vmbdabuilder.WithName(fmt.Sprintf("vmbda-hotplug-%02d", i)),
				vmbdabuilder.WithNamespace(ns),
				vmbdabuilder.WithVirtualMachineName(vm.Name),
				vmbdabuilder.WithBlockDeviceRef(v1alpha2.VMBDAObjectRefKindVirtualDisk, vd.Name),
			)
			vds = append(vds, vd)
			vmbdas = append(vmbdas, vmbda)
			vdNames = append(vdNames, vd.Name)
			vmbdaNames = append(vmbdaNames, vmbda.Name)
		}

		// Arm the observers before creating anything so no transition is missed.
		vmObs := vmobs.StartObserver(ctx, f, vm)
		vmObs.Never(vmobs.BeFailed())
		vmbdaObservers := make([]vmbdaobs.Observer, 0, len(vmbdas))
		for _, vmbda := range vmbdas {
			obs := vmbdaobs.StartObserver(ctx, f, vmbda)
			obs.Never(vmbdaobs.BeFailed())
			vmbdaObservers = append(vmbdaObservers, obs)
		}

		var initialDiskCount int
		By("Creating the virtual machine with its root disk", func() {
			Expect(f.CreateWithDeferredDeletion(ctx, vdRoot, vm)).To(Succeed())

			Expect(vmObs.WaitFor(vmobs.BeRunning(), framework.LongTimeout)).To(Succeed())
			eventually.SSHReadyAsRoot(f, vm, framework.LongTimeout)

			var err error
			initialDiskCount, err = util.GetDiskCountAsRoot(f, vm.Name, vm.Namespace)
			Expect(err).NotTo(HaveOccurred())
		})

		expectedDiskCount := initialDiskCount + manyHotplugsDiskCount

		By(fmt.Sprintf("Hotplugging %d disks through VMBDAs", manyHotplugsDiskCount), func() {
			objects := append(util.ToObjects(vds), util.ToObjects(vmbdas)...)
			Expect(f.CreateWithDeferredDeletion(ctx, objects...)).To(Succeed())

			// A hotplug must apply live: enforce it as an invariant on every
			// VM update from here through the end of the spec.
			vmObs.Never(requireNoRestart())

			for i, obs := range vmbdaObservers {
				Expect(obs.WaitFor(vmbdaobs.BeAttached(), framework.LongTimeout)).To(Succeed(),
					"VMBDA %s should become Attached", vmbdas[i].Name)
			}
			Expect(vmObs.WaitFor(vmobs.HaveBlockDevicesAttached(vdNames...), framework.LongTimeout)).To(Succeed())

			eventually.UntilDiskCountAsRoot(f, vm.Name, vm.Namespace,
				Equal(expectedDiskCount), framework.LongTimeout,
				eventually.WithPolling(hotplugPolling),
				eventually.WithExplanation("expected %d block devices in guest after hotplug", expectedDiskCount))

			expectVolumesConverged(vm)
		})

		for attempt := 1; attempt <= manyHotplugsMigrationCount; attempt++ {
			By(fmt.Sprintf("Migration %d of %d", attempt, manyHotplugsMigrationCount), func() {
				sourceNode, err := util.GetVMNode(ctx, f, vm)
				Expect(err).NotTo(HaveOccurred())

				// The hotplugged disks must stay attached for the whole
				// migration: a VMBDA leaving Attached fails the spec.
				watchCtx, cancelWatch := context.WithCancel(ctx)
				defer cancelWatch()
				vmbdaWatchErrCh := make(chan error, 1)
				go func() {
					vmbdaWatchErrCh <- ensureVMBDAsStayAttached(watchCtx,
						f.VirtClient().VirtualMachineBlockDeviceAttachments(ns),
						vmbdaNames, metav1.ListOptions{})
				}()

				vmop := util.MigrateVirtualMachine(f, vm,
					vmopbuilder.WithName(fmt.Sprintf("many-hotplugs-migration-%d", attempt)))
				util.UntilVMOPMigrationSucceeded(ctx, vmop, framework.MaxTimeout)

				cancelWatch()
				Expect(<-vmbdaWatchErrCh).NotTo(HaveOccurred(), "VMBDAs should stay in Attached phase during migration")

				By("Checking the virtual machine landed on another node")
				eventually.UntilAssertion(func(g Gomega) {
					cur := &v1alpha2.VirtualMachine{}
					g.Expect(f.Clients.GenericClient().Get(ctx, crclient.ObjectKeyFromObject(vm), cur)).To(Succeed())
					g.Expect(cur.Status.Node).NotTo(BeEmpty())
					g.Expect(cur.Status.Node).NotTo(Equal(sourceNode), "the VM is still on its source node after migration %d", attempt)
				}, framework.MiddleTimeout, eventually.WithPolling(hotplugPolling))

				By("Checking every hotplugged disk is still attached and visible in the guest")
				Expect(vmObs.WaitFor(vmobs.HaveBlockDevicesAttached(vdNames...), framework.LongTimeout)).To(Succeed())
				eventually.SSHReadyAsRoot(f, vm, framework.LongTimeout)
				eventually.UntilDiskCountAsRoot(f, vm.Name, vm.Namespace,
					Equal(expectedDiskCount), framework.LongTimeout,
					eventually.WithPolling(hotplugPolling),
					eventually.WithExplanation("expected %d block devices in guest after migration %d", expectedDiskCount, attempt))

				// The internal VM and VMI are compared volume for volume when the
				// next migration readiness is decided, so they must still agree
				// after the hotplugged volumes were re-attached on the target.
				expectVolumesConverged(vm)
			})
		}
	})
})
