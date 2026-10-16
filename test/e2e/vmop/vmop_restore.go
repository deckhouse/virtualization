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

package vmop

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	storagev1 "k8s.io/api/storage/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/client-go/util/retry"
	"k8s.io/utils/ptr"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"

	vdbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vd"
	vibuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vi"
	vmbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vm"
	vmbdabuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vmbda"
	vmopbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vmop"
	vmsnapshotbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vmsnapshot"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
	"github.com/deckhouse/virtualization/test/e2e/eventually"
	"github.com/deckhouse/virtualization/test/e2e/internal/config"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	"github.com/deckhouse/virtualization/test/e2e/internal/label"
	"github.com/deckhouse/virtualization/test/e2e/internal/object"
	"github.com/deckhouse/virtualization/test/e2e/internal/observer"
	vmobs "github.com/deckhouse/virtualization/test/e2e/internal/observer/vm"
	vmbdaobs "github.com/deckhouse/virtualization/test/e2e/internal/observer/vmbda"
	vmopobs "github.com/deckhouse/virtualization/test/e2e/internal/observer/vmop"
	vmsnapshotobs "github.com/deckhouse/virtualization/test/e2e/internal/observer/vmsnapshot"
	"github.com/deckhouse/virtualization/test/e2e/internal/precheck"
	"github.com/deckhouse/virtualization/test/e2e/internal/util"
)

const (
	vmAnnotationName          = "vmAnnotationName"
	vmAnnotationOriginalValue = "vmAnnotationOriginalValue"
	vmAnnotationChangedValue  = "vmAnnotationChangedValue"
	vmLabelName               = "vmLabelName"
	vmLabelOriginalValue      = "vmLabelOriginalValue"
	vmLabelChangedValue       = "vmLabelChangedValue"
	resourceAnnotationName    = "resourceAnnotation"
	resourceAnnotationValue   = "resourceAnnotationValue"
	resourceLabelName         = "resourceLabelName"
	resourceLabelValue        = "resourceLabelValue"
	originalValueOnDisk       = "originalValueOnDisk"
	changedValueOnDisk        = "changedValueOnDisk"
	originalCPUCores          = 1
	originalMemorySize        = "64Mi"
	changedCPUCores           = 2
	changedMemorySize         = "128Mi"
	mountPoint                = "/mnt"
	fileDataPath              = "/mnt/value"
	mountPointNoFstab         = "/mnt-no-fstab"
	fileDataPathNoFstab       = "/mnt-no-fstab/value"
	// WithIPPoolNetworkVLANID is the ClusterNetwork with an IPAM pool used for
	// IPAM restore tests (auto allocation via DHCP).
	WithIPPoolNetworkVLANID = 4006

	// vmSnapshotTimeout bounds the wait for a VirtualMachineSnapshot to reach a terminal phase.
	//
	// It has to outlast every budget the snapshot controller spends on the way there. Expiring first
	// buys nothing and costs the diagnosis: the controller fails a capture with a reason naming what
	// it gave up on, while this timeout reports only that the snapshot never became Ready.
	//
	// The budgets are consecutive, each counted from its own start, and all live in
	// unified-snapshotter/vmsnapshot/controller.go:
	//
	//	planningDeadline         2m   resolving the machine and its disks
	//	freezeConfirmDeadline    5m   the guest confirming the filesystem freeze
	//	childrenSettleDeadline  10m   every child VirtualDiskSnapshot going terminal
	//	unfreezeConfirmDeadline  5m   the guest confirming the thaw
	//	                        ---
	//	                        22m
	//
	// Only the last one still ends in Ready once exceeded — it abandons the unfreeze rather than
	// failing the capture — so 22m is the longest a healthy snapshot can take. The remaining three
	// minutes cover the 2s requeue interval, API latency and this suite's own polling.
	vmSnapshotTimeout = 25 * time.Minute

	// vmCoreFraction is a bigger coreFraction to ensure guest agent is responsive.
	// The guest agent walks every mounted filesystem to serve a freeze request,
	// so the time it takes to confirm one scales with the coreFraction.
	//
	// 10% is used instead of object.CustomImageVMCoreFraction (5%), at which
	// confirmations took several minutes (3m36s, 4m28s) instead of seconds and
	// overran the controller's freeze deadline.
	vmCoreFraction = "10%"
)

var _ = Describe("VirtualMachineOperationRestore", label.Slow(), Label(label.SIGCompute, precheck.PrecheckSnapshot, precheck.PrecheckSDN), func() {
	DescribeTable("restores a virtual machine from a snapshot", func(restoreMode v1alpha2.SnapshotOperationMode, restartApprovalMode v1alpha2.RestartApprovalMode, runPolicy v1alpha2.RunPolicy, removeRecoverableResources bool) {
		ctx := context.Background()
		f := framework.NewFramework(fmt.Sprintf("vmop-restore-%s", strings.ToLower(string(restoreMode))))
		DeferCleanup(f.After)
		f.Before()

		t := newRestoreTest(f)

		By("Environment preparation", func() {
			t.GenerateResources(restoreMode, restartApprovalMode, runPolicy)
			t.StartObservers(ctx)

			// Create VM and VD first and wait for VM readiness.
			// Creating VMBDA later makes test more stable.
			err := f.CreateWithDeferredDeletion(
				ctx, t.VI, t.VDRoot, t.VDBlank, t.VM, t.VDBlankWithNoFstabEntry,
			)
			Expect(err).NotTo(HaveOccurred())
			if t.VM.Spec.RunPolicy == v1alpha2.ManualPolicy {
				err := t.VMObs.WaitFor(vmobs.BeStopped(), framework.LongTimeout)
				Expect(err).NotTo(HaveOccurred())
				util.StartVirtualMachine(ctx, f, t.VM)
			}
			err = t.VMObs.WaitFor(vmobs.BeAgentReady(), framework.LongTimeout)
			Expect(err).NotTo(HaveOccurred())

			err = f.CreateWithDeferredDeletion(ctx, t.VMBDA, t.VMBDAWithNoFstabEntry)
			Expect(err).NotTo(HaveOccurred())

			t.CheckVMFullReadiness("after creation")

			// Prepare disk with fstab entry. File should be present after reboot.
			util.CreateBlockDeviceFilesystem(ctx, f, t.VM, v1alpha2.DiskDevice, t.VDBlank.Name, "ext4")
			util.MountBlockDevice(ctx, f, t.VM, v1alpha2.DiskDevice, t.VDBlank.Name, mountPoint)
			util.RegisterFstabEntry(ctx, f, t.VM, v1alpha2.DiskDevice, t.VDBlank.Name)
			util.WriteFile(f, t.VM, fileDataPath, originalValueOnDisk)

			// Prepare disk without fstab entry. Should not be mounted after reboot.
			util.CreateBlockDeviceFilesystem(ctx, f, t.VM, v1alpha2.DiskDevice, t.VDBlankWithNoFstabEntry.Name, "ext4")
			util.MountBlockDevice(ctx, f, t.VM, v1alpha2.DiskDevice, t.VDBlankWithNoFstabEntry.Name, mountPointNoFstab)
			util.WriteFile(f, t.VM, fileDataPathNoFstab, originalValueOnDisk)
			// Unmount the disk to ensure nothing affects the hash.
			util.UnmountBlockDevice(f, t.VM, mountPointNoFstab)
			t.BlockDeviceHash = util.GetBlockDeviceHash(ctx, f, t.VM, v1alpha2.DiskDevice, t.VDBlankWithNoFstabEntry.Name)

			t.CheckAdditionalNetworkInterface(ctx, t.VM)
		})

		By("Taking snapshot", func() {
			vmsObs := vmsnapshotobs.StartObserver(ctx, f, t.VMSnapshot)
			err := f.CreateWithDeferredDeletion(ctx, t.VMSnapshot)
			Expect(err).NotTo(HaveOccurred())
			waitVMSnapshotReady(t.VMObs, vmsObs, vmSnapshotTimeout)

			t.CheckVMFullReadiness("after taking snapshot")
		})
		By("Changing VM", func() {
			util.WriteFile(f, t.VM, fileDataPath, changedValueOnDisk)

			err := f.Clients.GenericClient().Get(ctx, crclient.ObjectKeyFromObject(t.VM), t.VM)
			Expect(err).NotTo(HaveOccurred())

			runningCondition, _ := conditions.GetCondition(vmcondition.TypeRunning, t.VM.Status.Conditions)
			runningLastTransitionTime := runningCondition.LastTransitionTime.Time

			// Re-read and re-apply on conflict. The controller writes to this VirtualMachine while the
			// spec edits it — it is reconciling the snapshot that was just taken — so a plain
			// read-modify-write loses the race whenever the two land in the same moment.
			err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
				if err := f.Clients.GenericClient().Get(ctx, crclient.ObjectKeyFromObject(t.VM), t.VM); err != nil {
					return err
				}
				t.VM.Annotations[vmAnnotationName] = vmAnnotationChangedValue
				t.VM.Labels[vmLabelName] = vmLabelChangedValue
				t.VM.Spec.CPU.Cores = changedCPUCores
				t.VM.Spec.Memory.Size = resource.MustParse(changedMemorySize)
				return f.Clients.GenericClient().Update(ctx, t.VM)
			})
			Expect(err).NotTo(HaveOccurred())

			// Wait for AwaitingChanges presence if restart is required.
			if util.IsRestartRequired(t.VM, framework.ShortTimeout) {
				util.RebootVirtualMachineBySSH(f, t.VM)
			}

			t.WaitVMRebooted(ctx, runningLastTransitionTime, framework.LongTimeout)

			t.CheckVMFullReadiness("after reboot")
		})
		By("Check that VM is in changed state", func() {
			Expect(util.ReadFile(f, t.VM, fileDataPath)).To(Equal(changedValueOnDisk))
			err := f.Clients.GenericClient().Get(ctx, crclient.ObjectKeyFromObject(t.VM), t.VM)
			Expect(err).NotTo(HaveOccurred())
			Expect(t.VM.Annotations[vmAnnotationName]).To(Equal(vmAnnotationChangedValue))
			Expect(t.VM.Labels[vmLabelName]).To(Equal(vmLabelChangedValue))
			Expect(t.VM.Status.Resources.CPU.Cores).To(Equal(changedCPUCores))
			Expect(t.VM.Status.Resources.Memory.Size).To(Equal(resource.MustParse(changedMemorySize)))
		})
		By("Resource preparation", func() {
			if removeRecoverableResources {
				t.RemoveRecoverableResources(ctx)
			}
		})
		By("Restore VM from snapshot", func() {
			t.RestoreVM(ctx, t.VM, t.VMOPRestore)
			t.CheckVMFullReadiness("after restore")
		})
		By("Check VM after restore", func() {
			t.CheckVMAfterRestore(ctx, t.VM, t.VDRoot, t.VDBlank, t.VDBlankWithNoFstabEntry, t.VMBDA, t.VMBDAWithNoFstabEntry, t.VMOPRestore)
		})
		By("After restoration, verify that labels and annotations are preserved on the resources", func() {
			err := f.Clients.GenericClient().Get(ctx, crclient.ObjectKeyFromObject(t.VDRoot), t.VDRoot)
			Expect(err).NotTo(HaveOccurred())
			Expect(t.VDRoot.Annotations[resourceAnnotationName]).To(Equal(resourceAnnotationValue))
			Expect(t.VDRoot.Labels[resourceLabelName]).To(Equal(resourceLabelValue))

			err = f.Clients.GenericClient().Get(ctx, crclient.ObjectKeyFromObject(t.VDBlank), t.VDBlank)
			Expect(err).NotTo(HaveOccurred())
			Expect(t.VDBlank.Annotations[resourceAnnotationName]).To(Equal(resourceAnnotationValue))
			Expect(t.VDBlank.Labels[resourceLabelName]).To(Equal(resourceLabelValue))

			err = f.Clients.GenericClient().Get(ctx, crclient.ObjectKeyFromObject(t.VMBDA), t.VMBDA)
			Expect(err).NotTo(HaveOccurred())
			Expect(t.VMBDA.Annotations[resourceAnnotationName]).To(Equal(resourceAnnotationValue))
			Expect(t.VMBDA.Labels[resourceLabelName]).To(Equal(resourceLabelValue))

			err = f.Clients.GenericClient().Get(ctx, crclient.ObjectKeyFromObject(t.VDBlankWithNoFstabEntry), t.VDBlankWithNoFstabEntry)
			Expect(err).NotTo(HaveOccurred())
			Expect(t.VDBlankWithNoFstabEntry.Annotations[resourceAnnotationName]).To(Equal(resourceAnnotationValue))
			Expect(t.VDBlankWithNoFstabEntry.Labels[resourceLabelName]).To(Equal(resourceLabelValue))

			err = f.Clients.GenericClient().Get(ctx, crclient.ObjectKeyFromObject(t.VMBDAWithNoFstabEntry), t.VMBDAWithNoFstabEntry)
			Expect(err).NotTo(HaveOccurred())
			Expect(t.VMBDAWithNoFstabEntry.Annotations[resourceAnnotationName]).To(Equal(resourceAnnotationValue))
			Expect(t.VMBDAWithNoFstabEntry.Labels[resourceLabelName]).To(Equal(resourceLabelValue))
		})
	},
		Entry(
			"DryRun restore mode; manual restart approval mode; always on unless stopped manually run policy",
			v1alpha2.SnapshotOperationModeDryRun,   // restoreMode
			v1alpha2.Manual,                        // restartApprovalMode
			v1alpha2.AlwaysOnUnlessStoppedManually, // runPolicy
			false,                                  // removeRecoverableResources
		),
		Entry(
			"BestEffort restore mode; manual restart approval mode; always on unless stopped manually run policy",
			v1alpha2.SnapshotOperationModeBestEffort, // restoreMode
			v1alpha2.Manual,                          // restartApprovalMode
			v1alpha2.AlwaysOnUnlessStoppedManually,   // runPolicy
			false,                                    // removeRecoverableResources
		),
		Entry(
			"Strict restore mode; manual restart approval mode; always on unless stopped manually run policy",
			v1alpha2.SnapshotOperationModeStrict,   // restoreMode
			v1alpha2.Manual,                        // restartApprovalMode
			v1alpha2.AlwaysOnUnlessStoppedManually, // runPolicy
			false,                                  // removeRecoverableResources
		),
		Entry(
			"BestEffort restore mode; manual restart approval mode; always on unless stopped manually run policy; with resource deletion",
			v1alpha2.SnapshotOperationModeBestEffort, // restoreMode
			v1alpha2.Manual,                          // restartApprovalMode
			v1alpha2.AlwaysOnUnlessStoppedManually,   // runPolicy
			true,                                     // removeRecoverableResources
		),
		Entry(
			"Strict restore mode; manual restart approval mode; always on unless stopped manually run policy; with resource deletion",
			v1alpha2.SnapshotOperationModeStrict,   // restoreMode
			v1alpha2.Manual,                        // restartApprovalMode
			v1alpha2.AlwaysOnUnlessStoppedManually, // runPolicy
			true,                                   // removeRecoverableResources
		),
		Entry(
			"BestEffort restore mode; automatic restart approval mode; always on unless stopped manually run policy",
			v1alpha2.SnapshotOperationModeBestEffort, // restoreMode
			v1alpha2.Automatic,                       // restartApprovalMode
			v1alpha2.AlwaysOnUnlessStoppedManually,   // runPolicy
			false,                                    // removeRecoverableResources
		),
		Entry(
			"BestEffort restore mode; automatic restart approval mode; manual run policy",
			v1alpha2.SnapshotOperationModeBestEffort, // restoreMode
			v1alpha2.Automatic,                       // restartApprovalMode
			v1alpha2.ManualPolicy,                    // runPolicy
			false,                                    // removeRecoverableResources
		),
	)
})

type restoreModeTest struct {
	VI                      *v1alpha2.VirtualImage
	VDRoot                  *v1alpha2.VirtualDisk
	VDBlank                 *v1alpha2.VirtualDisk
	VDBlankWithNoFstabEntry *v1alpha2.VirtualDisk
	VM                      *v1alpha2.VirtualMachine
	VMBDA                   *v1alpha2.VirtualMachineBlockDeviceAttachment
	VMBDAWithNoFstabEntry   *v1alpha2.VirtualMachineBlockDeviceAttachment
	VMSnapshot              *v1alpha2.VirtualMachineSnapshot
	VMOPRestore             *v1alpha2.VirtualMachineOperation

	// Observers are armed before the resources are created (StartObservers)
	// so no transition is missed by the waits spread across the test steps.
	VMObs           vmobs.Observer
	VMBDAObs        vmbdaobs.Observer
	VMBDANoFstabObs vmbdaobs.Observer

	Framework *framework.Framework

	BlockDeviceHash string
}

func newRestoreTest(f *framework.Framework) *restoreModeTest {
	return &restoreModeTest{
		Framework: f,
	}
}

func (t *restoreModeTest) GenerateResources(restoreMode v1alpha2.SnapshotOperationMode, restartApprovalMode v1alpha2.RestartApprovalMode, runPolicy v1alpha2.RunPolicy) {
	// The VI and the CD-ROM below are data payloads only, so they are sourced
	// from the custom images.
	// NOTE: do not use the same image that used for vd-root: same PARTUUID will make boot impossible.
	t.VI = object.NewVI(
		vibuilder.WithName("vi"),
		vibuilder.WithNamespace(t.Framework.Namespace().Name),
		vibuilder.WithDataSourceObjectRef(v1alpha2.VirtualImageObjectRefKindClusterVirtualImage, object.PrecreatedCVICustomEFI),
		vibuilder.WithStorage(v1alpha2.StorageContainerRegistry),
	)

	// The custom image brings up the SDN additional interface itself
	// (S41extranics DHCPs every non-primary NIC; the cn-4006 network carries
	// an IPAM pool), so no netplan/cloud-init networking is needed.
	t.VDRoot = object.NewVDFromCVI("vd-root", t.Framework.Namespace().Name, object.PrecreatedCVICustomBIOS,
		vdbuilder.WithSize(ptr.To(resource.MustParse("64Mi"))),
		vdbuilder.WithAnnotation(resourceAnnotationName, resourceAnnotationValue),
		vdbuilder.WithLabel(resourceLabelName, resourceLabelValue),
	)

	t.VDBlank = object.NewVD(
		vdbuilder.WithName("vd-blank"),
		vdbuilder.WithNamespace(t.Framework.Namespace().Name),
		vdbuilder.WithPersistentVolumeClaim(nil, ptr.To(resource.MustParse("51Mi"))),
		vdbuilder.WithAnnotation(resourceAnnotationName, resourceAnnotationValue),
		vdbuilder.WithLabel(resourceLabelName, resourceLabelValue),
	)

	t.VDBlankWithNoFstabEntry = object.NewVD(
		vdbuilder.WithName("vd-blank-no-fstab-entry"),
		vdbuilder.WithNamespace(t.Framework.Namespace().Name),
		vdbuilder.WithPersistentVolumeClaim(nil, ptr.To(resource.MustParse("51Mi"))),
		vdbuilder.WithAnnotation(resourceAnnotationName, resourceAnnotationValue),
		vdbuilder.WithLabel(resourceLabelName, resourceLabelValue),
	)

	// The provisioning keeps spec.provisioning part of the restore coverage;
	// the custom image executes the runcmd subset via its NoCloud
	// handler (S04cloudinit).
	cloudInit := `#cloud-config
runcmd:
  - echo provisioned > /run/e2e-provisioning-marker
`
	t.VM = vmbuilder.New(
		vmbuilder.WithName("vm"),
		vmbuilder.WithNamespace(t.Framework.Namespace().Name),
		vmbuilder.WithAnnotation(vmAnnotationName, vmAnnotationOriginalValue),
		vmbuilder.WithLabel(vmLabelName, vmLabelOriginalValue),
		vmbuilder.WithCPU(originalCPUCores, ptr.To(vmCoreFraction)),
		vmbuilder.WithMemory(resource.MustParse(originalMemorySize)),
		vmbuilder.WithLiveMigrationPolicy(v1alpha2.AlwaysSafeMigrationPolicy),
		vmbuilder.WithVirtualMachineClass(object.DefaultVMClass),
		vmbuilder.WithBlockDeviceRefs(
			v1alpha2.BlockDeviceSpecRef{
				Kind: v1alpha2.DiskDevice,
				Name: t.VDRoot.Name,
			},
			v1alpha2.BlockDeviceSpecRef{
				Kind: v1alpha2.ClusterImageDevice,
				Name: object.PrecreatedCVICustomISO,
			},
			v1alpha2.BlockDeviceSpecRef{
				Kind: v1alpha2.ImageDevice,
				Name: t.VI.Name,
			},
		),
		vmbuilder.WithRestartApprovalMode(restartApprovalMode),
		vmbuilder.WithRunPolicy(runPolicy),
		vmbuilder.WithProvisioningUserData(cloudInit),
		vmbuilder.WithNetwork(v1alpha2.NetworksSpec{
			Type: v1alpha2.NetworksTypeMain,
		}),
		vmbuilder.WithNetwork(v1alpha2.NetworksSpec{
			Type: v1alpha2.NetworksTypeClusterNetwork,
			Name: util.ClusterNetworkName(WithIPPoolNetworkVLANID),
		}),
	)

	t.VMBDA = vmbdabuilder.New(
		vmbdabuilder.WithName("vmbda"),
		vmbdabuilder.WithNamespace(t.VDBlank.Namespace),
		vmbdabuilder.WithVirtualMachineName(t.VM.Name),
		vmbdabuilder.WithBlockDeviceRef(v1alpha2.VMBDAObjectRefKindVirtualDisk, t.VDBlank.Name),
		vmbdabuilder.WithAnnotation(resourceAnnotationName, resourceAnnotationValue),
		vmbdabuilder.WithLabel(resourceLabelName, resourceLabelValue),
	)

	t.VMBDAWithNoFstabEntry = vmbdabuilder.New(
		vmbdabuilder.WithName("vmbda-no-fstab-entry"),
		vmbdabuilder.WithNamespace(t.VDBlankWithNoFstabEntry.Namespace),
		vmbdabuilder.WithVirtualMachineName(t.VM.Name),
		vmbdabuilder.WithBlockDeviceRef(v1alpha2.VMBDAObjectRefKindVirtualDisk, t.VDBlankWithNoFstabEntry.Name),
		vmbdabuilder.WithAnnotation(resourceAnnotationName, resourceAnnotationValue),
		vmbdabuilder.WithLabel(resourceLabelName, resourceLabelValue),
	)

	t.VMSnapshot = vmsnapshotbuilder.New(
		vmsnapshotbuilder.WithName("vmsnapshot"),
		vmsnapshotbuilder.WithNamespace(t.Framework.Namespace().Name),
		vmsnapshotbuilder.WithVirtualMachineName(t.VM.Name),
		vmsnapshotbuilder.WithRequiredConsistency(true),
		vmsnapshotbuilder.WithKeepIPAddress(v1alpha2.KeepIPAddressAlways),
	)

	t.VMOPRestore = vmopbuilder.New(
		vmopbuilder.WithName(fmt.Sprintf("restore-%s", strings.ToLower(string(restoreMode)))),
		vmopbuilder.WithNamespace(t.Framework.Namespace().Name),
		vmopbuilder.WithType(v1alpha2.VMOPTypeRestore),
		vmopbuilder.WithVirtualMachine(t.VM.Name),
		vmopbuilder.WithVMOPRestoreMode(restoreMode),
		vmopbuilder.WithVirtualMachineSnapshotName(t.VMSnapshot.Name),
	)
}

// StartObservers arms the VM and VMBDA observers. Call it after
// GenerateResources and before the resources are created so no transition is
// missed by the waits spread across the test steps.
func (t *restoreModeTest) StartObservers(ctx context.Context) {
	GinkgoHelper()

	t.VMObs = vmobs.StartObserver(ctx, t.Framework, t.VM)
	t.VMBDAObs = vmbdaobs.StartObserver(ctx, t.Framework, t.VMBDA)
	t.VMBDANoFstabObs = vmbdaobs.StartObserver(ctx, t.Framework, t.VMBDAWithNoFstabEntry)
}

// WaitVMRebooted waits, via the VM observer, until the VM is Running again
// with a Running condition transition newer than previousRunningTime.
func (t *restoreModeTest) WaitVMRebooted(ctx context.Context, previousRunningTime time.Time, timeout time.Duration) {
	GinkgoHelper()

	err := t.VMObs.WaitFor(vmobs.BeRebootedAfter(previousRunningTime), timeout)
	if err != nil {
		// The reboot may be lost by the controller when the virt-launcher pod
		// disappears before it is reconciled; surface that as a skip, exactly
		// like util.UntilVirtualMachineRebooted did.
		util.SkipIfGuestPowerActionStuck(ctx, crclient.ObjectKeyFromObject(t.VM))
	}
	Expect(err).NotTo(HaveOccurred(), "virtual machine %s should be rebooted", t.VM.Name)
}

// waitVMSnapshotReady waits for the VirtualMachineSnapshot to become Ready.
// A snapshot that failed because the CSI driver could not create the
// underlying VolumeSnapshot skips the spec: that is a storage-infrastructure
// problem, not a virtualization one. Any other failure fails the spec.
func waitVMSnapshotReady(vmObs vmobs.Observer, vmsObs vmsnapshotobs.Observer, timeout time.Duration) {
	GinkgoHelper()
	err := vmsObs.WaitFor(vmsnapshotobs.BeReady(), timeout)
	if err != nil && util.IsCSIVolumeSnapshotError(err.Error()) {
		Skip(fmt.Sprintf("VirtualMachineSnapshot failed on the CSI side, skipping: %s", err))
	}
	Expect(err).NotTo(HaveOccurred(), "should take snapshot successfully")

	err = vmObs.WaitFor(vmobs.BeUnmaintained(), timeout)
	Expect(err).NotTo(HaveOccurred())
}

func (t *restoreModeTest) RemoveRecoverableResources(ctx context.Context) {
	GinkgoHelper()

	util.StopVirtualMachineFromOS(t.Framework, t.VM)
	err := t.VMObs.WaitFor(vmobs.BeStopped(), framework.ShortTimeout)
	Expect(err).NotTo(HaveOccurred())

	err = t.Framework.Delete(ctx, t.VDRoot, t.VDBlank, t.VMBDA, t.VDBlankWithNoFstabEntry, t.VMBDAWithNoFstabEntry)
	Expect(err).NotTo(HaveOccurred())

	// Wait for the resources to be fully gone before proceeding: the restore
	// must recreate them from the snapshot, not find leftovers.
	for _, vd := range []*v1alpha2.VirtualDisk{t.VDRoot, t.VDBlank, t.VDBlankWithNoFstabEntry} {
		err := observer.WaitForDeleted(ctx,
			t.Framework.VirtClient().VirtualDisks(vd.Namespace), vd.Name, vd.Namespace,
			framework.LongTimeout, t.isVDDeleted(vd),
		)
		Expect(err).NotTo(HaveOccurred())
	}
	for _, vmbda := range []*v1alpha2.VirtualMachineBlockDeviceAttachment{t.VMBDA, t.VMBDAWithNoFstabEntry} {
		err := observer.WaitForDeleted(ctx,
			t.Framework.VirtClient().VirtualMachineBlockDeviceAttachments(vmbda.Namespace), vmbda.Name, vmbda.Namespace,
			framework.LongTimeout, t.isVMBDADeleted(vmbda),
		)
		Expect(err).NotTo(HaveOccurred())
	}
}

// isVDDeleted reports whether the VirtualDisk no longer exists; it lets
// WaitForDeleted catch deletions that complete before its watch starts.
func (t *restoreModeTest) isVDDeleted(vd *v1alpha2.VirtualDisk) observer.IsDeleted {
	return func(ctx context.Context) (bool, error) {
		err := t.Framework.GenericClient().Get(ctx, crclient.ObjectKeyFromObject(vd), &v1alpha2.VirtualDisk{})
		if k8serrors.IsNotFound(err) {
			return true, nil
		}
		return false, err
	}
}

// isVMBDADeleted reports whether the VMBDA no longer exists; it lets
// WaitForDeleted catch deletions that complete before its watch starts.
func (t *restoreModeTest) isVMBDADeleted(vmbda *v1alpha2.VirtualMachineBlockDeviceAttachment) observer.IsDeleted {
	return func(ctx context.Context) (bool, error) {
		err := t.Framework.GenericClient().Get(ctx, crclient.ObjectKeyFromObject(vmbda), &v1alpha2.VirtualMachineBlockDeviceAttachment{})
		if k8serrors.IsNotFound(err) {
			return true, nil
		}
		return false, err
	}
}

func (t *restoreModeTest) CheckVMAfterRestore(
	ctx context.Context,
	vm *v1alpha2.VirtualMachine,
	vdRoot, vdBlank, vdBlankWithNoFstabEntry *v1alpha2.VirtualDisk,
	vmbda, vmbdaWithNoFstabEntry *v1alpha2.VirtualMachineBlockDeviceAttachment,
	vmopRestore *v1alpha2.VirtualMachineOperation,
) {
	GinkgoHelper()

	err := t.Framework.Clients.GenericClient().Get(ctx, crclient.ObjectKeyFromObject(vm), vm)
	Expect(err).NotTo(HaveOccurred())

	// In DryRun mode, the VM should remain unchanged and VMOPRestore should contain
	// information about resources ready for restore. In actual restore modes,
	// the VM should be restored to the snapshot state.
	switch vmopRestore.Spec.Restore.Mode {
	case v1alpha2.SnapshotOperationModeDryRun:
		err := t.Framework.Clients.GenericClient().Get(ctx, crclient.ObjectKeyFromObject(vmopRestore), vmopRestore)
		Expect(err).NotTo(HaveOccurred())

		t.CheckResourceReadyForRestore(vmopRestore, v1alpha2.VirtualMachineKind, vm.Name)
		t.CheckResourceReadyForRestore(vmopRestore, v1alpha2.VirtualDiskKind, vdRoot.Name)
		t.CheckResourceReadyForRestore(vmopRestore, v1alpha2.VirtualDiskKind, vdBlank.Name)
		t.CheckResourceReadyForRestore(vmopRestore, v1alpha2.VirtualMachineBlockDeviceAttachmentKind, vmbda.Name)
		t.CheckResourceReadyForRestore(vmopRestore, v1alpha2.VirtualDiskKind, vdBlankWithNoFstabEntry.Name)
		t.CheckResourceReadyForRestore(vmopRestore, v1alpha2.VirtualMachineBlockDeviceAttachmentKind, vmbdaWithNoFstabEntry.Name)

		Expect(util.GetBlockDeviceHash(ctx, t.Framework, vm, v1alpha2.DiskDevice, vdBlankWithNoFstabEntry.Name)).To(Equal(t.BlockDeviceHash))
		Expect(util.ReadFile(t.Framework, vm, fileDataPath)).To(Equal(changedValueOnDisk))
		Expect(vm.Annotations[vmAnnotationName]).To(Equal(vmAnnotationChangedValue))
		Expect(vm.Labels[vmLabelName]).To(Equal(vmLabelChangedValue))
		Expect(vm.Status.Resources.CPU.Cores).To(Equal(changedCPUCores))
		Expect(vm.Status.Resources.Memory.Size).To(Equal(resource.MustParse(changedMemorySize)))
	case v1alpha2.SnapshotOperationModeStrict, v1alpha2.SnapshotOperationModeBestEffort:
		Expect(util.ReadFile(t.Framework, vm, fileDataPath)).To(Equal(originalValueOnDisk))
		Expect(vm.Annotations[vmAnnotationName]).To(Equal(vmAnnotationOriginalValue))
		Expect(vm.Labels[vmLabelName]).To(Equal(vmLabelOriginalValue))
		Expect(vm.Status.Resources.CPU.Cores).To(Equal(originalCPUCores))
		Expect(vm.Status.Resources.Memory.Size).To(Equal(resource.MustParse(originalMemorySize)))
	default:
		Fail("Invalid restore mode")
	}

	t.CheckAdditionalNetworkInterface(ctx, vm)

	t.CheckIPAMAfterRestore(ctx, vm)
}

// CheckIPAMAfterRestore verifies that the auto-IPAddress for the additional network
// CheckIPAMAfterRestore verifies that the auto-IPAddress for the additional
// network was recreated by the controller after VM restore.
func (t *restoreModeTest) CheckIPAMAfterRestore(ctx context.Context, vm *v1alpha2.VirtualMachine) {
	GinkgoHelper()

	By("Verify IPAM: auto IPAddress recreated for additional network after restore", func() {
		// status.networks should have an ipAddress for cn-4006 (auto-IPAddress
		// recreated). The VM observer has been armed since before the VM was
		// created, so the update carrying the recreated address is not missed.
		err := t.VMObs.WaitFor(haveNetworkIPFor(util.ClusterNetworkName(WithIPPoolNetworkVLANID)), framework.LongTimeout)
		Expect(err).NotTo(HaveOccurred(),
			"auto IPAddress should be recreated after restore, status.networks should have an IP for %s", util.ClusterNetworkName(WithIPPoolNetworkVLANID))
	})
}

func (t *restoreModeTest) CheckResourceReadyForRestore(vmopRestore *v1alpha2.VirtualMachineOperation, kind, name string) {
	GinkgoHelper()

	resourceForRestore := t.getResourceInfoFromVMOP(vmopRestore, kind, name)
	Expect(resourceForRestore).ShouldNot(BeNil())
	Expect(resourceForRestore.Status).Should(Equal(v1alpha2.SnapshotResourceStatusCompleted))
	Expect(resourceForRestore.Message).Should(ContainSubstring("is valid for restore"))
}

func (t *restoreModeTest) getResourceInfoFromVMOP(vmopRestore *v1alpha2.VirtualMachineOperation, kind, name string) *v1alpha2.SnapshotResourceStatus {
	for _, resourceForRestore := range vmopRestore.Status.Resources {
		if resourceForRestore.Name == name && resourceForRestore.Kind == kind {
			return &resourceForRestore
		}
	}

	return nil
}

func (t *restoreModeTest) RestoreVM(ctx context.Context, vm *v1alpha2.VirtualMachine, vmopRestore *v1alpha2.VirtualMachineOperation) {
	GinkgoHelper()

	// The restore puts the VM back into the phase it held going in, so capture that
	// phase and when it was last entered. The wait afterwards has to be anchored on a
	// timestamp: the VM passes through Stopped/Running during the restore itself, and a
	// bare predicate matches that, letting the test run ahead while the VM is still
	// Pending. The anchor comes from the VM's own PhasesTransitions, so there is no
	// clock skew against this machine.
	err := t.Framework.Clients.GenericClient().Get(ctx, crclient.ObjectKeyFromObject(vm), vm)
	Expect(err).NotTo(HaveOccurred())
	vmPhaseBeforeRestore := vm.Status.Phase
	if vmPhaseBeforeRestore != v1alpha2.MachineRunning && vmPhaseBeforeRestore != v1alpha2.MachineStopped {
		Expect(errors.New("unexpected VM phase before restore. Should be Stopped or Running")).NotTo(HaveOccurred(), vm.Status.Phase, vm.Status.Stats.PhasesTransitions)
	}

	phaseTransition := object.FindPhaseLastTransition(vm, vmPhaseBeforeRestore)
	Expect(phaseTransition).ToNot(BeNil(), "Phase transition should be present in the VM status")
	phaseTransitionTime := phaseTransition.Timestamp.Time

	_, _ = fmt.Fprintf(GinkgoWriter, "Found last transition for %s phase at %s\n", vmPhaseBeforeRestore, phaseTransitionTime.Format(time.RFC3339))

	// Create VMOP Restore and wait for restore completion.
	vmopRestoreObs := vmopobs.StartObserver(ctx, vmopRestore)
	err = t.Framework.CreateWithDeferredDeletion(ctx, vmopRestore)
	Expect(err).NotTo(HaveOccurred())
	err = vmopRestoreObs.WaitFor(vmopobs.BeCompleted(), framework.LongTimeout)
	Expect(err).NotTo(HaveOccurred())

	// No need to wait for VM phase transitions for the DryRun mode.
	if vmopRestore.Spec.Restore.Mode == v1alpha2.SnapshotOperationModeDryRun {
		return
	}

	predicate := vmobs.BeStoppedAfter(phaseTransitionTime)
	if vmPhaseBeforeRestore == v1alpha2.MachineRunning {
		predicate = vmobs.BeRunningAfter(phaseTransitionTime)
	}
	// Deliberately not asserted: a VM that never leaves Stopped records no new
	// transition, so this times out while the state is perfectly fine. The phase read
	// below is what decides; the wait only keeps us from acting mid-restore.
	waitErr := t.VMObs.WaitFor(predicate, framework.LongTimeout)

	err = t.Framework.Clients.GenericClient().Get(ctx, crclient.ObjectKeyFromObject(vm), vm)
	Expect(err).NotTo(HaveOccurred())
	_, _ = fmt.Fprintf(GinkgoWriter, "VM phase after restore: %s (wait err: %v)\n", vm.Status.Phase, waitErr)

	// A VM restored into Stopped stays Stopped, so start it explicitly. Doing this
	// before the wait above is what used to fail the operation with
	// NotApplicableForVirtualMachinePhase - back then the VM was still Pending.
	if vm.Status.Phase == v1alpha2.MachineStopped {
		util.StartVirtualMachine(ctx, t.Framework, t.VM)
	}

	err = t.VMObs.WaitFor(vmobs.BeAgentReady(), framework.LongTimeout)
	Expect(err).NotTo(HaveOccurred())
	err = t.VMBDAObs.WaitFor(vmbdaobs.BeAttached(), framework.LongTimeout)
	Expect(err).NotTo(HaveOccurred())
}

func (t *restoreModeTest) CheckVMFullReadiness(extraMsg string) {
	GinkgoHelper()

	err := t.VMObs.WaitFor(vmobs.BeAgentReady(), framework.LongTimeout)
	Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("VM should be agentReady %s", extraMsg))
	err = t.VMBDAObs.WaitFor(vmbdaobs.BeAttached(), framework.LongTimeout)
	Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("VMBDA should be Attached %s", extraMsg))
	err = t.VMBDANoFstabObs.WaitFor(vmbdaobs.BeAttached(), framework.LongTimeout)
	Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("VMBDA no fstab should be Attached %s", extraMsg))

	eventually.SSHReadyAsRoot(t.Framework, t.VM, framework.LongTimeout, eventually.WithExplanation(fmt.Sprintf("SSH should be available within %s %s", framework.LongTimeout.String(), extraMsg)))

	// The machine answering is not the same as the platform being done with it: every point this is
	// called from has just stopped or started the machine, and the platform reacts by evicting it.
	// The wait belongs here rather than at each call site, so a step added later inherits it.
	t.WaitEvictionSettled(context.Background())
}

// CheckAdditionalNetworkInterface reads the auto-allocated IP from VM
// status.networks[].ipAddress and verifies it is present in the guest OS.
func (t *restoreModeTest) CheckAdditionalNetworkInterface(ctx context.Context, vm *v1alpha2.VirtualMachine) {
	GinkgoHelper()

	updated := &v1alpha2.VirtualMachine{}
	err := t.Framework.Clients.GenericClient().Get(ctx, crclient.ObjectKeyFromObject(vm), updated)
	Expect(err).NotTo(HaveOccurred())

	var ip string
	for _, n := range updated.Status.Networks {
		if n.Name == util.ClusterNetworkName(WithIPPoolNetworkVLANID) {
			ip = n.IPAddress
		}
	}
	Expect(ip).NotTo(BeEmpty(), "auto IP should be allocated in status.networks for %s", util.ClusterNetworkName(WithIPPoolNetworkVLANID))

	cmdOut, err := t.Framework.SSHCommand(vm.GetName(), vm.GetNamespace(), "ip -4 addr show", framework.WithSSHUser("root"))
	Expect(err).NotTo(HaveOccurred())
	Expect(cmdOut).To(ContainSubstring(fmt.Sprintf("inet %s", ip)))
}

// WaitEvictionSettled lets an eviction the platform started finish before the spec moves the machine
// again.
//
// Stopping, starting or resizing the machine makes the platform reconsider its placement and issue an
// Evict VirtualMachineOperation. On local storage that eviction migrates the volume as well, which
// takes long enough for the spec's next step to overtake it; the migration then dies with
// "VirtualMachine is stopped" and the failed operation fails the spec through the fail-fast watcher.
// Both the change step and the restore have been seen losing this race.
//
// Only local storage is waited on: elsewhere the eviction finishes within the readiness checks above,
// and an unconditional wait would add its timeout to every run of the suite.
//
// The appearance of the operation is given ShortTimeout and no more. Whether the platform always
// issues one is not asserted here — this is a race to lose, not a contract to verify, so a run that
// sees none simply carries on.
func (t *restoreModeTest) WaitEvictionSettled(ctx context.Context) {
	GinkgoHelper()

	if !t.isLocalStorage(ctx) {
		return
	}

	// Plain polling rather than Eventually: an eviction that never appears is an acceptable outcome,
	// and Gomega has no way to wait for something optional without failing when it stays absent.
	deadline := time.Now().Add(framework.ShortTimeout)
	for t.findEviction(ctx) == nil {
		if time.Now().After(deadline) {
			return
		}
		time.Sleep(time.Second)
	}

	// findEviction reports only the running one, so nil means it reached a terminal phase or was
	// cleaned up. Which of the two, and whether it succeeded, is not this wait's business: the spec
	// waits for the platform to be done, and judges the machine itself through the checks around it.
	Eventually(func() *v1alpha2.VirtualMachineOperation {
		return t.findEviction(ctx)
	}).WithTimeout(framework.LongTimeout).WithPolling(2*time.Second).Should(BeNil(),
		"the eviction of virtual machine %s never settled", t.VM.Name)
}

// findEviction returns the Evict VirtualMachineOperation that is still running for this test's
// virtual machine, or nil when there is none.
func (t *restoreModeTest) findEviction(ctx context.Context) *v1alpha2.VirtualMachineOperation {
	var vmops v1alpha2.VirtualMachineOperationList
	if err := t.Framework.GenericClient().List(ctx, &vmops, crclient.InNamespace(t.VM.Namespace)); err != nil {
		return nil
	}
	for i := range vmops.Items {
		op := &vmops.Items[i]
		if op.Spec.Type != v1alpha2.VMOPTypeEvict || op.Spec.VirtualMachine != t.VM.Name {
			continue
		}
		switch op.Status.Phase {
		case v1alpha2.VMOPPhaseCompleted, v1alpha2.VMOPPhaseFailed, v1alpha2.VMOPPhaseSuperseded:
			continue
		}
		return op
	}
	return nil
}

func (t *restoreModeTest) isLocalStorage(ctx context.Context) bool {
	var scList storagev1.StorageClassList
	if err := framework.GetClients().GenericClient().List(ctx, &scList); err != nil {
		return false
	}
	sc := config.FindDefaultStorageClass(&scList)
	return sc != nil && sc.Provisioner == framework.SDSLocalVolume
}

func (t *restoreModeTest) IsStorageClassAvailableForTest(ctx context.Context, vm *v1alpha2.VirtualMachine) bool {
	GinkgoHelper()

	var scList storagev1.StorageClassList
	err := framework.GetClients().GenericClient().List(ctx, &scList)
	Expect(err).NotTo(HaveOccurred())

	sc := config.FindDefaultStorageClass(&scList)
	Expect(sc).NotTo(BeNil())

	return sc.Provisioner != framework.SDSReplicatedVolume
}

// haveNetworkIPFor reports status.networks carries a non-empty IP address for
// the named network.
func haveNetworkIPFor(networkName string) vmobs.Predicate {
	return func(vm *v1alpha2.VirtualMachine) (bool, error) {
		for _, n := range vm.Status.Networks {
			if n.Name == networkName && n.IPAddress != "" {
				return true, nil
			}
		}
		return false, nil
	}
}
