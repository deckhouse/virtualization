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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/utils/ptr"
	virtv1 "kubevirt.io/api/core/v1"

	vmbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vm"
	vmopbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vmop"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	"github.com/deckhouse/virtualization/test/e2e/internal/object"
	"github.com/deckhouse/virtualization/test/e2e/internal/precheck"
	"github.com/deckhouse/virtualization/test/e2e/internal/util"
)

var _ = DescribeTable("VirtualMachineCancelMigration", Label(precheck.NoPrecheck), func(bootloaderType v1alpha2.BootloaderType) {
	const stressngCmd = "nohup stress-ng --cpu 4 --vm 4 --vm-bytes 90% --vm-keep --vm-populate --vm-method all --timeout 1h </dev/null >/dev/null 2>errlog &"

	ctx := context.Background()
	var suffix string
	switch bootloaderType {
	case v1alpha2.BIOS:
		suffix = "bios"
	case v1alpha2.EFI:
		suffix = "efi"
	case v1alpha2.EFIWithSecureBoot:
		suffix = "efi-secureboot"
	default:
		Fail("Unknown bootloader type")
	}
	f := framework.NewFramework(fmt.Sprintf("vm-migration-cancel-%s", suffix))
	DeferCleanup(f.After)
	f.Before()

	By("Environment preparation")
	vdRoot := object.NewVDFromCVI("vd-root", f.Namespace().Name, object.PrecreatedCVIUbuntu)
	vdBlank := object.NewBlankVD("vd-blank", f.Namespace().Name, nil, ptr.To(resource.MustParse("100Mi")))

	vm := object.NewMinimalVM("", f.Namespace().Name,
		vmbuilder.WithName("vm"),
		vmbuilder.WithBootloader(bootloaderType),
		vmbuilder.WithCPU(4, ptr.To("100%")),
		vmbuilder.WithMemory(resource.MustParse("4Gi")),
		vmbuilder.WithLiveMigrationPolicy(v1alpha2.PreferSafeMigrationPolicy),
		vmbuilder.WithBlockDeviceRefs(
			v1alpha2.BlockDeviceSpecRef{
				Kind: v1alpha2.VirtualDiskKind,
				Name: vdRoot.Name,
			},
			v1alpha2.BlockDeviceSpecRef{
				Kind: v1alpha2.VirtualDiskKind,
				Name: vdBlank.Name,
			},
		),
	)

	err := f.CreateWithDeferredDeletion(ctx,
		vdRoot, vdBlank, vm,
	)
	Expect(err).NotTo(HaveOccurred())

	util.UntilObjectPhase(ctx, string(v1alpha2.MachineRunning), framework.LongTimeout, vm)
	util.UntilSSHReady(f, vm, framework.MiddleTimeout)

	By("Create memory pressure inside the virtual machine")
	_, err = f.SSHCommand(vm.Name, vm.Namespace, stressngCmd)
	Expect(err).NotTo(HaveOccurred())

	By("Wait for stress-ng to increase memory pressure")
	Consistently(func() error {
		_, err := f.SSHCommand(vm.Name, vm.Namespace, "test ! -s errlog")
		if err != nil {
			return err
		}

		return nil
	}).WithTimeout(20 * time.Second).WithPolling(time.Second).ShouldNot(HaveOccurred())

	By("Create migration VMOPs")
	evictVMOP := vmopbuilder.New(
		vmopbuilder.WithName("vmop-evict"),
		vmopbuilder.WithNamespace(vm.Namespace),
		vmopbuilder.WithType(v1alpha2.VMOPTypeEvict),
		vmopbuilder.WithVirtualMachine(vm.Name),
	)

	err = f.CreateWithDeferredDeletion(ctx, evictVMOP)
	Expect(err).NotTo(HaveOccurred())

	By("Ensure the VMOP is in the InProgress phase")
	util.UntilObjectPhase(ctx, string(v1alpha2.VMOPPhaseInProgress), framework.MiddleTimeout, evictVMOP)

	By("Ensure the KVVMI has a migration state")
	untilKVVMIMigrationStateIsNotEmpty(ctx, framework.MiddleTimeout, vm)

	By("Remove the VMOP")
	err = f.GenericClient().Delete(ctx, evictVMOP)
	Expect(err).NotTo(HaveOccurred())

	By("Ensure the VMOP is removed")
	util.UntilObjectsDeleted(ctx, framework.MiddleTimeout, evictVMOP)

	By("Ensure the KubeVirt VMI has an abort status")
	untilAbortStatusExists(ctx, framework.MiddleTimeout, vm)
},
	Entry("BIOS bootloader", v1alpha2.BIOS),
	Entry("UEFI bootloader", v1alpha2.EFI),
	Entry("UEFI bootloader with secure boot", v1alpha2.EFIWithSecureBoot),
)

func untilKVVMIMigrationStateIsNotEmpty(ctx context.Context, timeout time.Duration, vm *v1alpha2.VirtualMachine) {
	GinkgoHelper()

	Eventually(func() error {
		kvvmi, err := util.GetInternalVirtualMachineInstance(ctx, vm)
		if err != nil {
			return err
		}

		if kvvmi == nil {
			return fmt.Errorf("retry because KVVMI not found for %s/%s VM", vm.Namespace, vm.Name)
		}

		if kvvmi.Status.MigrationState == nil {
			return fmt.Errorf("%s KVVMI migration state is empty", kvvmi.Name)
		}

		return nil
	}).WithTimeout(timeout).WithPolling(time.Second).ShouldNot(HaveOccurred())
}

func untilAbortStatusExists(ctx context.Context, timeout time.Duration, vm *v1alpha2.VirtualMachine) {
	GinkgoHelper()

	validAbortStatuses := []virtv1.MigrationAbortStatus{
		virtv1.MigrationAbortInProgress,
		virtv1.MigrationAbortSucceeded,
		virtv1.MigrationAbortFailed,
	}

	Eventually(func() error {
		kvvmi, err := util.GetInternalVirtualMachineInstance(ctx, vm)
		if err != nil {
			return fmt.Errorf("retry because %w", err)
		}

		if kvvmi == nil {
			return fmt.Errorf("retry because KVVMI not found for %s/%s VM", vm.Namespace, vm.Name)
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
		return nil
	}).WithTimeout(timeout).WithPolling(time.Second).ShouldNot(HaveOccurred())
}
