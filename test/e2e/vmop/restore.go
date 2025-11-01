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
	"strconv"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"

	cvibuilder "github.com/deckhouse/virtualization-controller/pkg/builder/cvi"
	vdbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vd"
	vibuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vi"
	vmbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vm"
	vmopbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vmop"
	vmsnapshotbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vmsnapshot"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	"github.com/deckhouse/virtualization/test/e2e/internal/object"
	"github.com/deckhouse/virtualization/test/e2e/internal/util"
)

const (
	viURL              = "https://89d64382-20df-4581-8cc7-80df331f67fa.selstorage.ru/test/test.qcow2"
	cviURL             = "https://89d64382-20df-4581-8cc7-80df331f67fa.selstorage.ru/test/test.iso"
	defaultValue       = "value"
	changedValue       = "changed"
	testAnnotationName = "test-annotation"
	testLabelName      = "test-label"
	defaultCPUCores    = 1
	defaultMemorySize  = "256Mi"
	changedCPUCores    = 2
	changedMemorySize  = "512Mi"
)

var _ = Describe("VirtualMachineOperationRestore", func() {
	DescribeTable("restores a virtual machine from a snapshot", func(restoreMode v1alpha2.VMOPRestoreMode) {
		f := framework.NewFramework(fmt.Sprintf("vmop-restore-%s", strings.ToLower(string(restoreMode))))
		f.Before()
		helper := NewRestoreModeTest(f)
		DeferCleanup(f.After)

		By("Environment preparation", func() {
			helper.CreateEnvironmentResources()

			util.UntilVMAgentReady(crclient.ObjectKeyFromObject(helper.VM), framework.LongTimeout)
			util.UntilObjectPhase(helper.VMBDA, string(v1alpha2.BlockDeviceAttachmentPhaseAttached), framework.ShortTimeout)
		})
		By("Create file on last disk", func() {
			// Create a unique value on the disk to verify it's preserved after restore
			helper.GeneratedValue = strconv.Itoa(time.Now().UTC().Second())
			_, err := f.SSHCommand(helper.VM.Name, helper.VM.Namespace, helper.ShellCreateFsAndSetValueOnDisk(helper.GeneratedValue))
			Expect(err).NotTo(HaveOccurred())
		})
		By("Snapshot creation", func() {
			helper.CreateVMSnapshot()
			util.UntilObjectPhase(helper.VMSnapshot, string(v1alpha2.VirtualMachineSnapshotPhaseReady), framework.ShortTimeout)
		})
		By("Changing VM", func() {
			helper.ChangeVMConfiguration()
			util.UntilVMAgentReady(crclient.ObjectKeyFromObject(helper.VM), framework.ShortTimeout)
			util.UntilObjectPhase(helper.VMBDA, string(v1alpha2.BlockDeviceAttachmentPhaseAttached), framework.ShortTimeout)
		})
		By("Reboot VM", func() {
			// Reboot VM to ensure configuration changes are applied and persisted
			err := f.UpdateFromCluster(context.Background(), helper.VM)
			Expect(err).NotTo(HaveOccurred())
			runningCondition, _ := conditions.GetCondition(vmcondition.TypeRunning, helper.VM.Status.Conditions)
			helper.RunningLastTransitionTime = runningCondition.LastTransitionTime.Time
			err = util.RebootVirtualMachineFromOS(f, helper.VM)
			Expect(err).NotTo(HaveOccurred())

			util.UntilVirtualMachineRebooted(crclient.ObjectKeyFromObject(helper.VM), helper.RunningLastTransitionTime, framework.LongTimeout)
			util.UntilObjectPhase(helper.VMBDA, string(v1alpha2.BlockDeviceAttachmentPhaseAttached), framework.ShortTimeout)
		})
		By("Check VM in changed state", func() {
			helper.CheckVMInChangedState()
		})
		if restoreMode == v1alpha2.VMOPRestoreModeStrict {
			By("Remove disks", func() {
				// Remove disks to simulate a scenario where resources from snapshot are missing.
				helper.RemoveDisks()
			})
		}
		By("Create restore operation", func() {
			helper.CreateRestoreOperation(restoreMode)
			util.UntilVMOPCompleted(crclient.ObjectKeyFromObject(helper.VMOPRestore), framework.LongTimeout)
		})
		By("Check VM after restore", func() {
			util.UntilVMAgentReady(crclient.ObjectKeyFromObject(helper.VM), framework.LongTimeout)
			util.UntilObjectPhase(helper.VMBDA, string(v1alpha2.BlockDeviceAttachmentPhaseAttached), framework.ShortTimeout)

			switch restoreMode {
			case v1alpha2.VMOPRestoreModeStrict, v1alpha2.VMOPRestoreModeBestEffort:
				helper.CheckVMInInitialState()
			case v1alpha2.VMOPRestoreModeDryRun:
				helper.CheckVMInChangedState()
			}
		})
	},
		Entry("DryRun", v1alpha2.VMOPRestoreModeDryRun),
		Entry("BestEffort", v1alpha2.VMOPRestoreModeBestEffort),
		Entry("Strict", v1alpha2.VMOPRestoreModeStrict),
	)
})

type restoreModeTest struct {
	CVI         *v1alpha2.ClusterVirtualImage
	VI          *v1alpha2.VirtualImage
	VDRoot      *v1alpha2.VirtualDisk
	VDBlank     *v1alpha2.VirtualDisk
	VM          *v1alpha2.VirtualMachine
	VMBDA       *v1alpha2.VirtualMachineBlockDeviceAttachment
	VMSnapshot  *v1alpha2.VirtualMachineSnapshot
	VMOPRestore *v1alpha2.VirtualMachineOperation

	GeneratedValue            string
	RunningLastTransitionTime time.Time
	f                         *framework.Framework
}

func NewRestoreModeTest(f *framework.Framework) *restoreModeTest {
	return &restoreModeTest{
		f: f,
	}
}

func (r *restoreModeTest) CreateEnvironmentResources() {
	GinkgoHelper()

	r.CVI = cvibuilder.New(
		cvibuilder.WithGenerateName("cvi-"),
		cvibuilder.WithDataSourceHTTP(cviURL, nil, nil),
	)
	err := r.f.CreateWithDeferredDeletion(context.Background(), r.CVI)
	Expect(err).NotTo(HaveOccurred())

	r.VI = vibuilder.New(
		vibuilder.WithName("vi"),
		vibuilder.WithNamespace(r.f.Namespace().Name),
		vibuilder.WithDataSourceHTTP(viURL, nil, nil),
		vibuilder.WithStorage(v1alpha2.StorageContainerRegistry),
	)
	r.VDRoot = object.NewGeneratedHTTPVDUbuntu("", r.f.Namespace().Name, vdbuilder.WithName("vd-root"))
	r.VDBlank = object.NewBlankVD("", r.f.Namespace().Name, nil, ptr.To(resource.MustParse("51Mi")), vdbuilder.WithName("vd-blank"))
	err = r.f.CreateWithDeferredDeletion(context.Background(), r.VI, r.VDRoot, r.VDBlank)
	Expect(err).NotTo(HaveOccurred())

	r.VM = object.NewMinimalVM(
		"", r.f.Namespace().Name,
		vmbuilder.WithName("vm"),
		vmbuilder.WithAnnotation(testAnnotationName, defaultValue),
		vmbuilder.WithLabel(testLabelName, defaultValue),
		vmbuilder.WithCPU(defaultCPUCores, ptr.To("5%")),
		vmbuilder.WithMemory(resource.MustParse(defaultMemorySize)),
		vmbuilder.WithBlockDeviceRefs(
			v1alpha2.BlockDeviceSpecRef{
				Kind: v1alpha2.DiskDevice,
				Name: r.VDRoot.Name,
			},
		),
		vmbuilder.WithBlockDeviceRefs(
			v1alpha2.BlockDeviceSpecRef{
				Kind: v1alpha2.ClusterImageDevice,
				Name: r.CVI.Name,
			},
		),
		vmbuilder.WithBlockDeviceRefs(
			v1alpha2.BlockDeviceSpecRef{
				Kind: v1alpha2.ImageDevice,
				Name: r.VI.Name,
			},
		),
	)
	err = r.f.CreateWithDeferredDeletion(context.Background(), r.VM)
	Expect(err).NotTo(HaveOccurred())

	r.VMBDA = object.NewVMBDAFromDisk("vmbda", r.VM.Name, r.VDBlank)
	err = r.f.CreateWithDeferredDeletion(context.Background(), r.VMBDA)
	Expect(err).NotTo(HaveOccurred())
}

func (r *restoreModeTest) CreateVMSnapshot() {
	GinkgoHelper()

	r.VMSnapshot = vmsnapshotbuilder.New(
		vmsnapshotbuilder.WithName("vmsnapshot"),
		vmsnapshotbuilder.WithNamespace(r.f.Namespace().Name),
		vmsnapshotbuilder.WithVirtualMachineName(r.VM.Name),
		vmsnapshotbuilder.WithRequiredConsistency(true),
		vmsnapshotbuilder.WithKeepIPAddress(v1alpha2.KeepIPAddressAlways),
	)
	err := r.f.CreateWithDeferredDeletion(context.Background(), r.VMSnapshot)
	Expect(err).NotTo(HaveOccurred())
}

func (r *restoreModeTest) CreateRestoreOperation(restoreMode v1alpha2.VMOPRestoreMode) {
	r.VMOPRestore = vmopbuilder.New(
		vmopbuilder.WithName("restore-strict"),
		vmopbuilder.WithNamespace(r.f.Namespace().Name),
		vmopbuilder.WithType(v1alpha2.VMOPTypeRestore),
		vmopbuilder.WithVirtualMachine(r.VM.Name),
		vmopbuilder.WithVMOPRestoreMode(restoreMode),
		vmopbuilder.WithVirtualMachineSnapshotName(r.VMSnapshot.Name),
	)
	err := r.f.CreateWithDeferredDeletion(context.Background(), r.VMOPRestore)
	Expect(err).NotTo(HaveOccurred())
}

// Change VM configuration to verify restore functionality:
// - Modify data on disk
// - Change annotations and labels
// - Update CPU and memory resources
// After restore, all these should revert to original values from snapshot
func (r *restoreModeTest) ChangeVMConfiguration() {
	GinkgoHelper()

	_, err := r.f.SSHCommand(r.VM.Name, r.VM.Namespace, r.ShellChangeValueOnDisk(changedValue))
	Expect(err).NotTo(HaveOccurred())
	err = r.f.UpdateFromCluster(context.Background(), r.VM)
	Expect(err).NotTo(HaveOccurred())
	Expect(r.VM.Annotations).ToNot(BeNil()) // otherwise a panic may potentially occur
	Expect(r.VM.Labels).ToNot(BeNil())      // otherwise a panic may potentially occur
	r.VM.Annotations[testAnnotationName] = changedValue
	r.VM.Labels[testLabelName] = changedValue
	r.VM.Spec.CPU.Cores = changedCPUCores
	r.VM.Spec.Memory.Size = resource.MustParse(changedMemorySize)
	err = r.f.Clients.GenericClient().Update(context.Background(), r.VM)
	Expect(err).NotTo(HaveOccurred())
}

// Verify that VM has been restored to its initial state from snapshot:
// - Disk contains the original value
// - Annotations and labels match snapshot
// - CPU and memory resources are restored to original values
func (r *restoreModeTest) CheckVMInInitialState() {
	GinkgoHelper()

	value, err := r.f.SSHCommand(r.VM.Name, r.VM.Namespace, r.ShellMountAndGetValueFromDisk())
	Expect(err).NotTo(HaveOccurred())
	Expect(strings.TrimSpace(value)).To(Equal(r.GeneratedValue))
	err = r.f.UpdateFromCluster(context.Background(), r.VM)
	Expect(err).NotTo(HaveOccurred())
	Expect(r.VM.Annotations[testAnnotationName]).To(Equal(defaultValue))
	Expect(r.VM.Labels[testLabelName]).To(Equal(defaultValue))
	Expect(r.VM.Status.Resources.CPU.Cores).To(Equal(defaultCPUCores))
	Expect(r.VM.Status.Resources.Memory.Size).To(Equal(resource.MustParse(defaultMemorySize)))
}

func (r *restoreModeTest) CheckVMInChangedState() {
	GinkgoHelper()

	value, err := r.f.SSHCommand(r.VM.Name, r.VM.Namespace, r.ShellMountAndGetValueFromDisk())
	Expect(err).NotTo(HaveOccurred())
	Expect(strings.TrimSpace(value)).To(Equal(changedValue))
	err = r.f.UpdateFromCluster(context.Background(), r.VM)
	Expect(err).NotTo(HaveOccurred())
	Expect(r.VM.Annotations[testAnnotationName]).To(Equal(changedValue))
	Expect(r.VM.Labels[testLabelName]).To(Equal(changedValue))
	Expect(r.VM.Status.Resources.CPU.Cores).To(Equal(changedCPUCores))
	Expect(r.VM.Status.Resources.Memory.Size).To(Equal(resource.MustParse(changedMemorySize)))
}

func (r *restoreModeTest) RemoveDisks() {
	GinkgoHelper()

	// Stop VM and remove all disks to test Strict restore mode.
	err := util.StopVirtualMachineFromOS(r.f, r.VM)
	Expect(err).NotTo(HaveOccurred())
	util.UntilVirtualMachineStopped(crclient.ObjectKeyFromObject(r.VM), framework.ShortTimeout)

	err = r.f.Delete(context.Background(), r.VDRoot)
	Expect(err).NotTo(HaveOccurred())
	err = r.f.Delete(context.Background(), r.VDBlank)
	Expect(err).NotTo(HaveOccurred())

	// Wait for disks to be fully deleted before proceeding with restore
	Eventually(func(g Gomega) {
		var vdRootLocal v1alpha2.VirtualDisk
		err = r.f.Clients.GenericClient().Get(context.Background(), types.NamespacedName{
			Namespace: r.VDRoot.Namespace,
			Name:      r.VDRoot.Name,
		}, &vdRootLocal)
		g.Expect(k8serrors.IsNotFound(err)).Should(BeTrue())

		var vdBlankLocal v1alpha2.VirtualDisk
		err = r.f.Clients.GenericClient().Get(context.Background(), types.NamespacedName{
			Namespace: r.VDBlank.Namespace,
			Name:      r.VDBlank.Name,
		}, &vdBlankLocal)
		g.Expect(k8serrors.IsNotFound(err)).Should(BeTrue())
	}, framework.LongTimeout, time.Second).Should(Succeed())
}

func (r *restoreModeTest) ShellCreateFsAndSetValueOnDisk(value string) string {
	return fmt.Sprintf("sudo umount /mnt &>/dev/null || DEV=/dev/$(sudo lsblk | grep disk | tail -n 1 | awk \"{print \\$1}\") && sudo mkfs.ext4 $DEV && sudo mount $DEV /mnt && sudo bash -c \"echo %s > /mnt/value\"", value)
}

func (r *restoreModeTest) ShellChangeValueOnDisk(value string) string {
	return fmt.Sprintf("sudo bash -c \"echo %s > /mnt/value\"", value)
}

func (r *restoreModeTest) ShellMountAndGetValueFromDisk() string {
	return "sudo umount /mnt &>/dev/null ; DEV=/dev/$(sudo lsblk | grep disk | tail -n 1 | awk \"{print \\$1}\") && sudo mount $DEV /mnt && cat /mnt/value"
}
