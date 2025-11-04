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
		h := NewRestoreModeTest(f)
		DeferCleanup(f.After)

		By("Environment preparation", func() {
			h.CreateEnvironmentResources()

			util.UntilVMAgentReady(crclient.ObjectKeyFromObject(h.VM), framework.LongTimeout)
		})
		By("Create VMBDA", func() {
			h.devicesWithoutVMBDA = h.ListDisks()
			h.CreateVMBDA()
			util.UntilObjectPhase(h.VMBDA, string(v1alpha2.BlockDeviceAttachmentPhaseAttached), framework.ShortTimeout)
		})
		By("Create file on last disk", func() {
			h.CreateFilesystemOnDisk()
			h.MountVMBDADeviceIfNotMounted()
			// Create a unique value on the disk to verify it's preserved after restore
			h.GeneratedValue = strconv.Itoa(time.Now().UTC().Second())
			h.CreateFileOnDisk(h.GeneratedValue)
		})
		By("Snapshot creation", func() {
			h.CreateVMSnapshot()
			util.UntilObjectPhase(h.VMSnapshot, string(v1alpha2.VirtualMachineSnapshotPhaseReady), framework.ShortTimeout)
		})
		By("Changing VM", func() {
			h.ChangeVMConfiguration()
			util.UntilVMAgentReady(crclient.ObjectKeyFromObject(h.VM), framework.ShortTimeout)
			util.UntilObjectPhase(h.VMBDA, string(v1alpha2.BlockDeviceAttachmentPhaseAttached), framework.ShortTimeout)
		})
		By("Reboot VM", func() {
			// Reboot VM to ensure configuration changes are applied and persisted
			err := f.UpdateFromCluster(context.Background(), h.VM)
			Expect(err).NotTo(HaveOccurred())
			runningCondition, _ := conditions.GetCondition(vmcondition.TypeRunning, h.VM.Status.Conditions)
			h.RunningLastTransitionTime = runningCondition.LastTransitionTime.Time
			err = util.RebootVirtualMachineFromOS(f, h.VM)
			Expect(err).NotTo(HaveOccurred())

			util.UntilVirtualMachineRebooted(crclient.ObjectKeyFromObject(h.VM), h.RunningLastTransitionTime, framework.LongTimeout)
			util.UntilObjectPhase(h.VMBDA, string(v1alpha2.BlockDeviceAttachmentPhaseAttached), framework.ShortTimeout)
		})
		By("Check VM in changed state", func() {
			h.CheckVMInChangedState()
		})
		if restoreMode == v1alpha2.VMOPRestoreModeStrict {
			By("Remove disks", func() {
				// Remove disks to simulate a scenario where resources from snapshot are missing.
				h.RemoveDisks()
			})
		}
		By("Create restore operation", func() {
			h.CreateRestoreOperation(restoreMode)
			util.UntilObjectPhase(h.VMOPRestore, string(v1alpha2.VMOPPhaseCompleted), framework.LongTimeout)
		})
		By("Check VM after restore", func() {
			util.UntilVMAgentReady(crclient.ObjectKeyFromObject(h.VM), framework.LongTimeout)
			util.UntilObjectPhase(h.VMBDA, string(v1alpha2.BlockDeviceAttachmentPhaseAttached), framework.ShortTimeout)

			switch restoreMode {
			case v1alpha2.VMOPRestoreModeStrict, v1alpha2.VMOPRestoreModeBestEffort:
				h.CheckVMInInitialState()
			case v1alpha2.VMOPRestoreModeDryRun:
				h.CheckVMInChangedState()
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

	devicesWithoutVMBDA []string
}

func NewRestoreModeTest(f *framework.Framework) *restoreModeTest {
	return &restoreModeTest{
		f:                   f,
		devicesWithoutVMBDA: []string{},
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
	r.VDRoot = object.NewHTTPVDUbuntu("vd-root", r.f.Namespace().Name)
	r.VDBlank = object.NewBlankVD("vd-blank", r.f.Namespace().Name, nil, ptr.To(resource.MustParse("51Mi")))
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
}

func (r *restoreModeTest) CreateVMBDA() {
	GinkgoHelper()

	r.VMBDA = object.NewVMBDAFromDisk("vmbda", r.VM.Name, r.VDBlank)
	err := r.f.CreateWithDeferredDeletion(context.Background(), r.VMBDA)
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

	r.ChangeValueOnDisk(changedValue)
	err := r.f.UpdateFromCluster(context.Background(), r.VM)
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

	r.MountVMBDADeviceIfNotMounted()
	Expect(r.GetValueFromDisk()).To(Equal(r.GeneratedValue))
	err := r.f.UpdateFromCluster(context.Background(), r.VM)
	Expect(err).NotTo(HaveOccurred())
	Expect(r.VM.Annotations[testAnnotationName]).To(Equal(defaultValue))
	Expect(r.VM.Labels[testLabelName]).To(Equal(defaultValue))
	Expect(r.VM.Status.Resources.CPU.Cores).To(Equal(defaultCPUCores))
	Expect(r.VM.Status.Resources.Memory.Size).To(Equal(resource.MustParse(defaultMemorySize)))
}

func (r *restoreModeTest) CheckVMInChangedState() {
	GinkgoHelper()

	r.MountVMBDADeviceIfNotMounted()
	Expect(r.GetValueFromDisk()).To(Equal(changedValue))
	err := r.f.UpdateFromCluster(context.Background(), r.VM)
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

func (r *restoreModeTest) ListDisks() (disks []string) {
	GinkgoHelper()

	lsblkOut, err := r.f.SSHCommand(r.VM.Name, r.VM.Namespace, "lsblk | grep disk")
	Expect(err).NotTo(HaveOccurred())
	lsblkLines := strings.Split(strings.TrimSpace(lsblkOut), "\n")

	for _, line := range lsblkLines {
		columns := strings.Split(strings.TrimSpace(line), " ")
		Expect(len(columns)).To(BeNumerically(">", 0))
		disks = append(disks, columns[0])
	}
	return
}

func (r *restoreModeTest) GetVMBDADevicePath() string {
	GinkgoHelper()

	checkMap := make(map[string]struct{})

	for _, device := range r.devicesWithoutVMBDA {
		checkMap[device] = struct{}{}
	}

	disks := r.ListDisks()
	for _, disk := range disks {
		if _, ok := checkMap[disk]; ok {
			continue
		}
		return fmt.Sprintf("/dev/%s", disk)
	}
	Fail("No vmbda device in VM")
	return ""
}

func (r *restoreModeTest) CheckMntOccupied() bool {
	GinkgoHelper()

	cmdOut, err := r.f.SSHCommand(r.VM.Name, r.VM.Namespace, "findmnt /mnt &> /dev/null ; echo $?")
	Expect(err).NotTo(HaveOccurred())
	return strings.TrimSpace(cmdOut) == "0"
}

func (r *restoreModeTest) MountVMBDADeviceIfNotMounted() {
	GinkgoHelper()

	if r.CheckMntOccupied() {
		return
	}

	devicePath := r.GetVMBDADevicePath()
	_, err := r.f.SSHCommand(r.VM.Name, r.VM.Namespace, fmt.Sprintf("sudo mount %s /mnt", devicePath))
	Expect(err).NotTo(HaveOccurred())
}

func (r *restoreModeTest) CreateFilesystemOnDisk() {
	GinkgoHelper()

	_, err := r.f.SSHCommand(r.VM.Name, r.VM.Namespace, fmt.Sprintf("sudo mkfs.ext4 %s", r.GetVMBDADevicePath()))
	Expect(err).NotTo(HaveOccurred())
}

func (r *restoreModeTest) CreateFileOnDisk(value string) {
	GinkgoHelper()

	_, err := r.f.SSHCommand(r.VM.Name, r.VM.Namespace, fmt.Sprintf("sudo bash -c \"echo %s > /mnt/value\"", value))
	Expect(err).NotTo(HaveOccurred())
}

func (r *restoreModeTest) ChangeValueOnDisk(value string) {
	GinkgoHelper()

	_, err := r.f.SSHCommand(r.VM.Name, r.VM.Namespace, fmt.Sprintf("sudo bash -c \"echo %s > /mnt/value\"", value))
	Expect(err).NotTo(HaveOccurred())
}

func (r *restoreModeTest) GetValueFromDisk() string {
	GinkgoHelper()

	cmdOut, err := r.f.SSHCommand(r.VM.Name, r.VM.Namespace, "sudo cat /mnt/value")
	Expect(err).NotTo(HaveOccurred())
	return strings.TrimSpace(cmdOut)
}
