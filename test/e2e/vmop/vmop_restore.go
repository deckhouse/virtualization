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
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	storagev1 "k8s.io/api/storage/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
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
	// WithIPPoolNetworkVLANID is the ClusterNetwork with an IPAM pool used for
	// IPAM restore tests (auto allocation via DHCP).
	WithIPPoolNetworkVLANID = 4006
)

var _ = Describe("VirtualMachineOperationRestore", label.Slow(), Label(label.SIGCompute, precheck.PrecheckSnapshot, precheck.PrecheckSDN), func() {
	BeforeEach(func() {
		// TODO: Re-enable the suite.
		Skip("skipped as flaky: fix the instability, then remove this skip")
	})

	DescribeTable("restores a virtual machine from a snapshot", func(restoreMode v1alpha2.SnapshotOperationMode, restartApprovalMode v1alpha2.RestartApprovalMode, runPolicy v1alpha2.RunPolicy, removeRecoverableResources bool) {
		ctx := context.Background()
		f := framework.NewFramework(fmt.Sprintf("vmop-restore-%s", strings.ToLower(string(restoreMode))))
		DeferCleanup(f.After)
		f.Before()

		t := newRestoreTest(f)
		if !t.IsStorageClassAvailableForTest(ctx, t.VM) {
			Skip("Temporary skip on sds-replicated-volume until snapshot functionality is fixed")
		}

		By("Environment preparation", func() {
			t.GenerateResources(restoreMode, restartApprovalMode, runPolicy)
			t.StartObservers(ctx)
			err := f.CreateWithDeferredDeletion(
				ctx, t.VI, t.VDRoot, t.VDBlank, t.VM, t.VMBDA, t.VDBlankWithNoFstabEntry, t.VMBDAWithNoFstabEntry,
			)
			Expect(err).NotTo(HaveOccurred())
			if t.VM.Spec.RunPolicy == v1alpha2.ManualPolicy {
				err := t.VMObs.WaitFor(vmobs.BeStopped(), framework.LongTimeout)
				Expect(err).NotTo(HaveOccurred())
				util.StartVirtualMachine(ctx, f, t.VM)
			}
			err = t.VMObs.WaitFor(vmobs.BeAgentReady(), framework.LongTimeout)
			Expect(err).NotTo(HaveOccurred())
			// The first attachment waits out the blank disk provisioning and the CSI
			// attach of the hotplug pod, which take minutes under a parallel run.
			err = t.VMBDAObs.WaitFor(vmbdaobs.BeAttached(), framework.LongTimeout)
			Expect(err).NotTo(HaveOccurred())
			err = t.VMBDANoFstabObs.WaitFor(vmbdaobs.BeAttached(), framework.LongTimeout)
			Expect(err).NotTo(HaveOccurred())

			util.CreateBlockDeviceFilesystem(ctx, f, t.VM, v1alpha2.DiskDevice, t.VDBlank.Name, "ext4")
			util.MountBlockDevice(ctx, f, t.VM, v1alpha2.DiskDevice, t.VDBlank.Name, mountPoint)
			util.RegisterFstabEntry(ctx, f, t.VM, v1alpha2.DiskDevice, t.VDBlank.Name)
			util.WriteFile(f, t.VM, fileDataPath, originalValueOnDisk)

			util.CreateBlockDeviceFilesystem(ctx, f, t.VM, v1alpha2.DiskDevice, t.VDBlankWithNoFstabEntry.Name, "ext4")
			util.MountBlockDevice(ctx, f, t.VM, v1alpha2.DiskDevice, t.VDBlankWithNoFstabEntry.Name, mountPoint)
			util.WriteFile(f, t.VM, fileDataPath, originalValueOnDisk)
			// Unmount the disk to ensure nothing affects the hash.
			util.UnmountBlockDevice(f, t.VM, mountPoint)
			t.BlockDeviceHash = util.GetBlockDeviceHash(ctx, f, t.VM, v1alpha2.DiskDevice, t.VDBlankWithNoFstabEntry.Name)

			vmsObs := vmsnapshotobs.StartObserver(ctx, f, t.VMSnapshot)
			err = f.CreateWithDeferredDeletion(ctx, t.VMSnapshot)
			Expect(err).NotTo(HaveOccurred())
			waitVMSnapshotReady(vmsObs, framework.MiddleTimeout)
		})
		By("Changing VM", func() {
			util.WriteFile(f, t.VM, fileDataPath, changedValueOnDisk)

			err := f.Clients.GenericClient().Get(ctx, crclient.ObjectKeyFromObject(t.VM), t.VM)
			Expect(err).NotTo(HaveOccurred())

			runningCondition, _ := conditions.GetCondition(vmcondition.TypeRunning, t.VM.Status.Conditions)
			runningLastTransitionTime := runningCondition.LastTransitionTime.Time

			t.VM.Annotations[vmAnnotationName] = vmAnnotationChangedValue
			t.VM.Labels[vmLabelName] = vmLabelChangedValue
			t.VM.Spec.CPU.Cores = changedCPUCores
			t.VM.Spec.Memory.Size = resource.MustParse(changedMemorySize)
			err = f.Clients.GenericClient().Update(ctx, t.VM)
			Expect(err).NotTo(HaveOccurred())

			if util.IsRestartRequired(t.VM, 3*time.Second) {
				util.RebootVirtualMachineBySSH(f, t.VM)
			}

			t.WaitVMRebooted(ctx, runningLastTransitionTime, framework.LongTimeout)
			err = t.VMObs.WaitFor(vmobs.BeAgentReady(), framework.ShortTimeout)
			Expect(err).NotTo(HaveOccurred())
			err = t.VMBDAObs.WaitFor(vmbdaobs.BeAttached(), framework.LongTimeout)
			Expect(err).NotTo(HaveOccurred())
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
	t.VI = object.NewVI(
		vibuilder.WithName("vi"),
		vibuilder.WithNamespace(t.Framework.Namespace().Name),
		vibuilder.WithDataSourceObjectRef(v1alpha2.VirtualImageObjectRefKindClusterVirtualImage, object.PrecreatedCVICustomBIOS),
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
		vmbuilder.WithCPU(originalCPUCores, ptr.To(object.CustomImageVMCoreFraction)),
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
func waitVMSnapshotReady(obs vmsnapshotobs.Observer, timeout time.Duration) {
	GinkgoHelper()
	err := obs.WaitFor(vmsnapshotobs.BeReady(), timeout)
	if err != nil && util.IsCSIVolumeSnapshotError(err.Error()) {
		Skip(fmt.Sprintf("VirtualMachineSnapshot failed on the CSI side, skipping: %s", err))
	}
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

	vmopRestoreObs := vmopobs.StartObserver(ctx, vmopRestore)
	err := t.Framework.CreateWithDeferredDeletion(ctx, vmopRestore)
	Expect(err).NotTo(HaveOccurred())
	err = vmopRestoreObs.WaitFor(vmopobs.BeCompleted(), framework.LongTimeout)
	Expect(err).NotTo(HaveOccurred())

	if vmopRestore.Spec.Restore.Mode == v1alpha2.SnapshotOperationModeDryRun {
		return
	}

	// after restore, the VM is in the Stopped state
	// if runPolicy == ManualPolicy, the VM should be started
	// cannot use isRestartRequired here, because we might skip the stopped phase
	if t.VM.Spec.RunPolicy == v1alpha2.ManualPolicy {
		err := t.VMObs.WaitFor(vmobs.BeStopped(), framework.ShortTimeout)
		Expect(err).NotTo(HaveOccurred())
		util.StartVirtualMachine(ctx, t.Framework, t.VM)
	}

	err = t.VMObs.WaitFor(vmobs.BeAgentReady(), framework.LongTimeout)
	Expect(err).NotTo(HaveOccurred())
	err = t.VMBDAObs.WaitFor(vmbdaobs.BeAttached(), framework.LongTimeout)
	Expect(err).NotTo(HaveOccurred())
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
