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
	"errors"
	"fmt"
	"slices"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/utils/ptr"
	virtv1 "kubevirt.io/api/core/v1"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"

	vmbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vm"
	vmopbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vmop"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	"github.com/deckhouse/virtualization/test/e2e/internal/object"
	"github.com/deckhouse/virtualization/test/e2e/internal/precheck"
	"github.com/deckhouse/virtualization/test/e2e/internal/util"
)

var validAbortStatuses = []virtv1.MigrationAbortStatus{
	virtv1.MigrationAbortInProgress,
	virtv1.MigrationAbortSucceeded,
	virtv1.MigrationAbortFailed,
}

const stressCommand = "nohup stress-ng --cpu 4 --vm 4 --vm-bytes 90% --vm-keep --vm-populate --vm-method all --timeout 1h </dev/null >/dev/null 2>errlog &"

var _ = Describe("VirtualMachineCancelMigration", Label(precheck.NoPrecheck), func() {
	var (
		f *framework.Framework

		vmBIOS *v1alpha2.VirtualMachine
		vmUEFI *v1alpha2.VirtualMachine

		migrationVMOPNames []string
	)

	BeforeEach(func() {
		f = framework.NewFramework("vm-migration-cancel")
		DeferCleanup(f.After)
		f.Before()
	})

	It("Cancel migrate", func() {
		By("Environment preparation")
		vdRootBIOS := object.NewVDFromCVI("vd-root-bios", f.Namespace().Name, object.PrecreatedCVIUbuntu)
		vdBlankBIOS := object.NewBlankVD("vd-blank-bios", f.Namespace().Name, nil, ptr.To(resource.MustParse("100Mi")))

		vdRootUEFI := object.NewVDFromCVI("vd-root-uefi", f.Namespace().Name, object.PrecreatedCVIUbuntu)
		vdBlankUEFI := object.NewBlankVD("vd-blank-uefi", f.Namespace().Name, nil, ptr.To(resource.MustParse("100Mi")))

		vmBIOS = object.NewMinimalVM("", f.Namespace().Name,
			vmbuilder.WithName("vm-bios"),
			vmbuilder.WithBootloader(v1alpha2.BIOS),
			vmbuilder.WithCPU(4, ptr.To("100%")),
			vmbuilder.WithMemory(resource.MustParse("4Gi")),
			vmbuilder.WithLiveMigrationPolicy(v1alpha2.PreferSafeMigrationPolicy),
			vmbuilder.WithBlockDeviceRefs(
				v1alpha2.BlockDeviceSpecRef{
					Kind: v1alpha2.VirtualDiskKind,
					Name: vdRootBIOS.Name,
				},
				v1alpha2.BlockDeviceSpecRef{
					Kind: v1alpha2.VirtualDiskKind,
					Name: vdBlankBIOS.Name,
				},
			),
		)
		vmUEFI = object.NewMinimalVM("", f.Namespace().Name,
			vmbuilder.WithName("vm-uefi"),
			vmbuilder.WithBootloader(v1alpha2.EFI),
			vmbuilder.WithCPU(4, ptr.To("100%")),
			vmbuilder.WithMemory(resource.MustParse("4Gi")),
			vmbuilder.WithLiveMigrationPolicy(v1alpha2.PreferSafeMigrationPolicy),
			vmbuilder.WithBlockDeviceRefs(
				v1alpha2.BlockDeviceSpecRef{
					Kind: v1alpha2.VirtualDiskKind,
					Name: vdRootUEFI.Name,
				},
				v1alpha2.BlockDeviceSpecRef{
					Kind: v1alpha2.VirtualDiskKind,
					Name: vdBlankUEFI.Name,
				},
			),
		)

		err := f.CreateWithDeferredDeletion(context.Background(),
			vdRootBIOS, vdBlankBIOS, vmBIOS,
			vdRootUEFI, vdBlankUEFI, vmUEFI,
		)
		Expect(err).NotTo(HaveOccurred())

		util.UntilVMAgentReady(crclient.ObjectKeyFromObject(vmBIOS), framework.LongTimeout)
		util.UntilVMAgentReady(crclient.ObjectKeyFromObject(vmUEFI), framework.LongTimeout)
		util.UntilSSHReady(f, vmBIOS, framework.MiddleTimeout)
		util.UntilSSHReady(f, vmUEFI, framework.MiddleTimeout)

		By("Create memory pressure inside virtual machines")
		vmList := []*v1alpha2.VirtualMachine{vmBIOS, vmUEFI}
		for _, vm := range vmList {
			By(fmt.Sprintf("Exec StressNG command for virtualmachine %s/%s", vm.Namespace, vm.Name))
			_, err := f.SSHCommand(vm.Name, vm.Namespace, stressCommand)
			Expect(err).NotTo(HaveOccurred())
		}

		By("Wait until stress-ng loads the memory more heavily")
		Consistently(func() error {
			for _, vm := range vmList {
				_, err := f.SSHCommand(vm.Name, vm.Namespace, "test ! -s errlog")
				if err != nil {
					return err
				}
			}

			return nil
		}).WithTimeout(20 * time.Second).WithPolling(time.Second).ShouldNot(HaveOccurred())

		By("Create migration VMOPs")
		for _, vm := range []*v1alpha2.VirtualMachine{vmBIOS, vmUEFI} {
			vmop := vmopbuilder.New(
				vmopbuilder.WithGenerateName(fmt.Sprintf("%s-evict-", util.VmopE2ePrefix)),
				vmopbuilder.WithNamespace(vm.Namespace),
				vmopbuilder.WithType(v1alpha2.VMOPTypeEvict),
				vmopbuilder.WithVirtualMachine(vm.Name),
			)

			err := f.CreateWithDeferredDeletion(context.Background(), vmop)
			Expect(err).NotTo(HaveOccurred())
			migrationVMOPNames = append(migrationVMOPNames, vmop.Name)
		}

		By("Collect migration VMOP names, delete them and verify they are gone")
		var migrationVMOPs []*v1alpha2.VirtualMachineOperation

		Eventually(func() error {
			migrationVMOPs = nil
			vmopInProgressCount := 0

			for _, vmopName := range migrationVMOPNames {
				vmop := &v1alpha2.VirtualMachineOperation{}
				err := f.GenericClient().Get(context.Background(), crclient.ObjectKey{Namespace: f.Namespace().Name, Name: vmopName}, vmop)
				if err != nil {
					return err
				}
				migrationVMOPs = append(migrationVMOPs, vmop)

				switch vmop.Status.Phase {
				case v1alpha2.VMOPPhaseCompleted, v1alpha2.VMOPPhaseFailed:
					Fail(fmt.Sprintf("vmop %s is in %s", vmop.Name, vmop.Status.Phase))
				case v1alpha2.VMOPPhaseInProgress:
					vmopInProgressCount++
				default:
					return fmt.Errorf("retry, waiting while vmop %s will be in InProgress phase", vmop.Name)
				}
			}

			if vmopInProgressCount != len(migrationVMOPNames) {
				return errors.New("retry: not all VMOPs are in InProgress phase")
			}

			return nil
		}).WithTimeout(framework.ShortTimeout).WithPolling(time.Second).ShouldNot(HaveOccurred())

		Eventually(func() error {
			for _, vm := range []*v1alpha2.VirtualMachine{vmBIOS, vmUEFI} {
				kvvmi, err := util.GetKVVMI(context.Background(), f, vm.Name, vm.Namespace)

				if err != nil {
					return err
				}

				if kvvmi.Status.MigrationState == nil {
					return fmt.Errorf("%s kvvmi migration state is empty", kvvmi.Name)
				}

			}

			return nil
		}).WithTimeout(framework.MiddleTimeout).WithPolling(time.Second).ShouldNot(HaveOccurred())

		for _, vmop := range migrationVMOPs {
			err := f.GenericClient().Delete(context.Background(), vmop)
			Expect(err).NotTo(HaveOccurred())
		}

		Eventually(func() error {
			for _, vmop := range migrationVMOPs {
				err := f.GenericClient().Get(context.Background(), crclient.ObjectKey{Namespace: f.Namespace().Name, Name: vmop.Name}, vmop)
				if err == nil {
					return fmt.Errorf("retry because VMOP %s/%s still exists", vmop.Namespace, vmop.Name)
				}
				if !k8serrors.IsNotFound(err) {
					return fmt.Errorf("unexpected error while checking VMOP %s/%s deletion: %w", vmop.Namespace, vmop.Name, err)
				}
			}

			return nil
		}).WithTimeout(framework.ShortTimeout).WithPolling(time.Second).ShouldNot(HaveOccurred())

		By("Abort status should be exists in Kubevirt VMIs")
		Eventually(func() error {
			vmList := []*v1alpha2.VirtualMachine{vmBIOS, vmUEFI}
			for _, vm := range vmList {
				kvvmi, err := util.GetKVVMI(context.Background(), f, vm.Name, vm.Namespace)
				if err != nil {
					return fmt.Errorf("retry because %w", err)
				}

				migrationState := kvvmi.Status.MigrationState
				if migrationState == nil {
					return fmt.Errorf("retry because migration state is nil for VMI %s/%s", vm.Namespace, vm.Name)
				}
				if !migrationState.AbortRequested {
					return fmt.Errorf("retry because migration abort requested is false for VMI %s/%s", vm.Namespace, vm.Name)
				}

				if !slices.Contains(validAbortStatuses, migrationState.AbortStatus) {
					return fmt.Errorf("retry because migration abort status is %s for VMI %s/%s", migrationState.AbortStatus, vm.Namespace, vm.Name)
				}

				if migrationState.EndTimestamp.IsZero() {
					return fmt.Errorf("retry because migration is not finished yet for VMI %s/%s", vm.Namespace, vm.Name)
				}
			}
			return nil
		}).WithTimeout(framework.LongTimeout).WithPolling(time.Second).ShouldNot(HaveOccurred())
	})
})
