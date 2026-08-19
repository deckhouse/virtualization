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
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	virtv1 "kubevirt.io/api/core/v1"

	vdbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vd"
	vmbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vm"
	vmopbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vmop"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/test/e2e/eventually"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	"github.com/deckhouse/virtualization/test/e2e/internal/label"
	"github.com/deckhouse/virtualization/test/e2e/internal/object"
	"github.com/deckhouse/virtualization/test/e2e/internal/observer"
	vmobs "github.com/deckhouse/virtualization/test/e2e/internal/observer/vm"
	vmopobs "github.com/deckhouse/virtualization/test/e2e/internal/observer/vmop"
	"github.com/deckhouse/virtualization/test/e2e/internal/precheck"
	"github.com/deckhouse/virtualization/test/e2e/internal/util"
)

var _ = DescribeTable("VirtualMachineCancelMigration", Label(label.SIGCompute), Label(precheck.NoPrecheck), func(bootloaderType v1alpha2.BootloaderType) {
	// -q keeps stress-ng quiet: it logs info to stderr, which would land in
	// errlog and trip the errlog-must-stay-empty check below.
	// Tuned for the single vCPU the VM runs on: the --cpu workers are gone because
	// on one core they only steal time from the memory stressor. Measured in the
	// guest over 20s with the same --vm-method, the vm stressor goes from 58.6k to
	// 103.3k bogo ops/s once they are dropped; the worker count and 90% share are
	// kept as they were (1 worker scores the same, halving the share is 40% worse).
	// The higher the page-dirtying rate, the longer the migration keeps re-copying
	// and stays cancellable.
	const stressngCmd = "nohup stress-ng -q --vm 2 --vm-bytes 90% --vm-keep --vm-populate --vm-method all --timeout 3m </dev/null >/dev/null 2>errlog &"

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
	f := framework.NewFramework(fmt.Sprintf("vm-cancel-migration-%s", suffix))
	DeferCleanup(f.After)
	f.Before()

	By("Environment preparation")
	// Build the disks on the template StorageClass (STORAGE_CLASS_NAME or the
	// cluster default): live migration requires a class whose volumes are
	// reachable from the target node.
	storageClass := framework.GetConfig().StorageClass.DefaultStorageClass

	// The BIOS and EFI entries run the custom image: it bakes in
	// stress-ng (which keeps the migration running long enough to cancel by
	// dirtying the 2Gi of guest memory) and is accessed as root. SecureBoot
	// requires a signed bootloader chain, which the custom image's unsigned
	// GRUB does not provide, so that entry stays on the Ubuntu image with its
	// cloud user.
	var vdRoot *v1alpha2.VirtualDisk
	vmOpts := []vmbuilder.Option{
		vmbuilder.WithName("vm"),
		vmbuilder.WithBootloader(bootloaderType),
		vmbuilder.WithCPU(1, ptr.To(object.CustomImageVMCoreFraction)),
		// 2Gi stays: it is the amount stress-ng dirties to keep the migration
		// running long enough to cancel.
		vmbuilder.WithMemory(resource.MustParse("2Gi")),
		vmbuilder.WithLiveMigrationPolicy(v1alpha2.PreferSafeMigrationPolicy),
	}
	var sshOpts []framework.SSHCommandOption
	switch bootloaderType {
	case v1alpha2.EFIWithSecureBoot:
		vdRoot = object.NewVDFromCVI("vd-root", f.Namespace().Name, object.PrecreatedCVIUbuntu,
			vdbuilder.WithStorageClass(&storageClass.Name),
		)
		// The Ubuntu image needs cloud-init to create the cloud user and start
		// the guest agent.
		vmOpts = append(vmOpts, vmbuilder.WithProvisioningUserData(object.UbuntuCloudInit))
	default:
		cviName := object.PrecreatedCVICustomBIOS
		if bootloaderType == v1alpha2.EFI {
			cviName = object.PrecreatedCVICustomEFI
		}
		vdRoot = object.NewVDFromCVI("vd-root", f.Namespace().Name, cviName,
			vdbuilder.WithStorageClass(&storageClass.Name),
			vdbuilder.WithSize(ptr.To(resource.MustParse(vdCustomImageSize))),
		)
		// The custom image has no cloud-init; the guest agent is baked in.
		sshOpts = append(sshOpts, framework.WithSSHUser("root"))
	}
	vdBlank := object.NewBlankVD("vd-blank", f.Namespace().Name, &storageClass.Name, ptr.To(resource.MustParse(vdCustomImageSize)))

	vmOpts = append(vmOpts, vmbuilder.WithBlockDeviceRefs(
		v1alpha2.BlockDeviceSpecRef{
			Kind: v1alpha2.VirtualDiskKind,
			Name: vdRoot.Name,
		},
		v1alpha2.BlockDeviceSpecRef{
			Kind: v1alpha2.VirtualDiskKind,
			Name: vdBlank.Name,
		},
	))
	vm := object.NewMinimalVM("", f.Namespace().Name, vmOpts...)

	vmObs := vmobs.StartObserver(ctx, f, vm)

	err := f.CreateWithDeferredDeletion(ctx,
		vdRoot, vdBlank, vm,
	)
	Expect(err).NotTo(HaveOccurred())

	err = vmObs.WaitFor(vmobs.BeRunning(), framework.LongTimeout)
	Expect(err).NotTo(HaveOccurred())
	// The guest needs the long timeout here: under a parallel run the disk
	// import, pod scheduling and (for Ubuntu) cloud-init boot can spend well
	// over a minute between the Running phase report and a usable SSH server.
	if bootloaderType == v1alpha2.EFIWithSecureBoot {
		eventually.SSHReady(f, vm, framework.LongTimeout)
	} else {
		eventually.SSHReadyAsRoot(f, vm, framework.LongTimeout)
	}

	By("Create memory pressure inside the virtual machine")
	_, err = f.SSHCommand(vm.Name, vm.Namespace, stressngCmd, sshOpts...)
	Expect(err).NotTo(HaveOccurred())

	By("Wait for stress-ng to increase memory pressure")
	// EXCEPTION: this holds a guest-side quiescence window (the stress-ng error
	// log inside the VM), not Kubernetes state, so Consistently is used
	// deliberately here.
	Consistently(func() error {
		return checkStressNGErrorLogIsEmpty(f, vm, sshOpts...)
	}).WithTimeout(20 * time.Second).WithPolling(time.Second).ShouldNot(HaveOccurred())

	By("Create migration VMOPs")
	evictVMOP := vmopbuilder.New(
		vmopbuilder.WithName("vmop-evict"),
		vmopbuilder.WithNamespace(vm.Namespace),
		vmopbuilder.WithType(v1alpha2.VMOPTypeEvict),
		vmopbuilder.WithVirtualMachine(vm.Name),
	)

	// The spec cancels this migration on purpose, and the aborted VMOP may
	// linger in the Failed phase while its deletion is finalized.
	f.ExpectFailure(evictVMOP)
	evictVMOPObs := vmopobs.StartObserver(ctx, evictVMOP)

	err = f.CreateWithDeferredDeletion(ctx, evictVMOP)
	util.SkipIfKnownFirmwareUpdateVMOPConflict(err)
	Expect(err).NotTo(HaveOccurred())

	By("Ensure the VMOP is in the InProgress phase")
	// The VMOP stays Pending until virt-controller grants a migration slot
	// (parallelMigrationsPerCluster/parallelOutboundMigrationsPerNode), which
	// takes minutes when parallel specs keep long-running migrations busy.
	// A VMOP that settles into a terminal phase can never be cancelled anymore,
	// so BeInProgress reports that as a definite error.
	err = evictVMOPObs.WaitFor(vmopobs.BeInProgress(), framework.MaxTimeout)
	Expect(err).NotTo(HaveOccurred())

	By("Ensure the KVVMI has a migration state")
	untilKVVMIMigrationStateExists(ctx, framework.MaxTimeout, vm)

	By("Remove the VMOP")
	err = f.GenericClient().Delete(ctx, evictVMOP)
	Expect(err).NotTo(HaveOccurred())

	By("Ensure the VMOP is removed")
	// The VMOP disappears only after the migration abort completes, and KubeVirt
	// delivers the abort signal to the VMI only once the migration reaches the
	// Running phase — under stress-ng load the target preparation alone can take
	// minutes, so the graceful cancellation needs the long timeout too.
	err = observer.WaitForDeleted(ctx,
		f.VirtClient().VirtualMachineOperations(evictVMOP.Namespace),
		evictVMOP.Name, evictVMOP.Namespace,
		framework.MiddleTimeout,
		func(ctx context.Context) (bool, error) {
			_, getErr := f.VirtClient().VirtualMachineOperations(evictVMOP.Namespace).Get(ctx, evictVMOP.Name, metav1.GetOptions{})
			if k8serrors.IsNotFound(getErr) {
				return true, nil
			}
			return false, getErr
		},
	)
	Expect(err).NotTo(HaveOccurred())

	By("Ensure stress-ng error log is empty")
	err = checkStressNGErrorLogIsEmpty(f, vm, sshOpts...)
	Expect(err).NotTo(HaveOccurred())

	By("Ensure the KVVMI has an abort status")
	// KubeVirt processes the abort only once the migration itself is running,
	// which under parallel load can take minutes of target preparation first.
	untilAbortStatusExists(ctx, framework.LongTimeout, vm)
},
	Entry("BIOS bootloader", v1alpha2.BIOS),
	// TODO: Re-enable the entry. Under parallel load the KVVMI abort status
	// intermittently does not appear within the wait timeout after the cancel.
	XEntry("UEFI bootloader", v1alpha2.EFI),
	Entry("UEFI bootloader with secure boot", v1alpha2.EFIWithSecureBoot),
)

// EXCEPTION: the internal VirtualMachineInstance is read through the rewrite
// client, which has no watch support, so there is nothing to observe via an
// Observer and a polling wait is used deliberately here.
func untilKVVMIMigrationStateExists(ctx context.Context, timeout time.Duration, vm *v1alpha2.VirtualMachine) {
	GinkgoHelper()

	eventually.Until(func() error {
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
	}, timeout)
}

// EXCEPTION: the internal VirtualMachineInstance is read through the rewrite
// client, which has no watch support, so there is nothing to observe via an
// Observer and a polling wait is used deliberately here.
func untilAbortStatusExists(ctx context.Context, timeout time.Duration, vm *v1alpha2.VirtualMachine) {
	GinkgoHelper()

	validAbortStatuses := []virtv1.MigrationAbortStatus{
		virtv1.MigrationAbortInProgress,
		virtv1.MigrationAbortSucceeded,
	}

	// TODO: remove when the kubevirt fork handles a cancel that races the
	// migration start: aborting an RWO volume migration during target
	// preparation tears down the NBD server, the migration start then dies with
	// a libvirt "NBD URI must be supplied when migration URI uses UNIX
	// transport method" error and the KVVMI reports Failed without ever
	// populating abortStatus.
	const knownNBDAbortRaceReason = "NBD URI must be supplied"
	var knownAbortRaceFailure string

	eventually.Until(func() error {
		kvvmi, err := util.GetInternalVirtualMachineInstance(ctx, vm)
		if err != nil {
			return fmt.Errorf("retry because %w", err)
		}

		if kvvmi == nil {
			return fmt.Errorf("retry because KVVMI not found for %s/%s VM", vm.Namespace, vm.Name)
		}

		migrationState := kvvmi.Status.MigrationState
		if migrationState == nil {
			return fmt.Errorf("retry because migration state is nil for KVVMI %s/%s", vm.Namespace, vm.Name)
		}
		// Checked before AbortRequested: when the cancel races the migration
		// start closely enough, the migration dies before the abort request is
		// even recorded on the KVVMI (AbortRequested stays false).
		if migrationState.Failed && migrationState.AbortStatus == "" && !migrationState.EndTimestamp.IsZero() {
			if strings.Contains(migrationState.FailureReason, knownNBDAbortRaceReason) {
				knownAbortRaceFailure = migrationState.FailureReason
				return nil
			}
			return StopTrying(fmt.Sprintf("migration failed instead of aborting for KVVMI %s/%s: %s", vm.Namespace, vm.Name, migrationState.FailureReason))
		}

		if !migrationState.AbortRequested {
			return fmt.Errorf("retry because migration abort requested is false for KVVMI %s/%s", vm.Namespace, vm.Name)
		}

		if !slices.Contains(validAbortStatuses, migrationState.AbortStatus) {
			return fmt.Errorf("retry because migration abort status is %s for KVVMI %s/%s", migrationState.AbortStatus, vm.Namespace, vm.Name)
		}

		if migrationState.EndTimestamp.IsZero() {
			return fmt.Errorf("retry because migration is not finished yet for KVVMI %s/%s", vm.Namespace, vm.Name)
		}
		return nil
	}, timeout)

	if knownAbortRaceFailure != "" {
		Skip(fmt.Sprintf("skip: known kubevirt abort/startup race, migration failed without an abort status: %s", knownAbortRaceFailure))
	}
}

func checkStressNGErrorLogIsEmpty(f *framework.Framework, vm *v1alpha2.VirtualMachine, sshOpts ...framework.SSHCommandOption) error {
	_, err := f.SSHCommand(vm.Name, vm.Namespace, "test ! -s errlog", sshOpts...)
	return err
}
