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
		DeferCleanup(f.After)
		t := NewRestoreTest(f)

		By("Environment preparation", func() {
			t.PrepareEnvironment()
		})
		By("Create file on last disk", func() {
			t.CreateUniqueValueOnDisk()
		})
		By("Snapshot creation", func() {
			t.CreateSnapshot()
		})
		By("Changing VM", func() {
			t.ChangeVM()
		})
		By("Check VM in changed state", func() {
			t.CheckVMInChangedState()
		})
		if restoreMode == v1alpha2.VMOPRestoreModeStrict {
			By("Remove disks", func() {
				// Remove disks to simulate a scenario where resources from snapshot are missing.
				t.RemoveDisks()
			})
		}
		By("Create restore operation", func() {
			t.CreateRestoreOperation(restoreMode)
		})
		By("Check VM after restore", func() {
			t.CheckVMAfterRestore(restoreMode)
		})
	},
		Entry("DryRun", v1alpha2.VMOPRestoreModeDryRun),
		Entry("BestEffort", v1alpha2.VMOPRestoreModeBestEffort),
		Entry("Strict", v1alpha2.VMOPRestoreModeStrict),
	)
})

type restoreModeTest struct {
	cvi         *v1alpha2.ClusterVirtualImage
	vi          *v1alpha2.VirtualImage
	vdRoot      *v1alpha2.VirtualDisk
	vdBlank     *v1alpha2.VirtualDisk
	vm          *v1alpha2.VirtualMachine
	vmbda       *v1alpha2.VirtualMachineBlockDeviceAttachment
	vmSnapshot  *v1alpha2.VirtualMachineSnapshot
	vmopRestore *v1alpha2.VirtualMachineOperation

	generatedValue            string
	runningLastTransitionTime time.Time
	f                         *framework.Framework

	devicesWithoutVMBDA []string
}

func NewRestoreTest(f *framework.Framework) *restoreModeTest {
	return &restoreModeTest{
		f:                   f,
		devicesWithoutVMBDA: []string{},
	}
}

func (r *restoreModeTest) PrepareEnvironment() {
	GinkgoHelper()

	r.createEnvironmentResources()
	util.UntilVMAgentReady(crclient.ObjectKeyFromObject(r.vm), framework.LongTimeout)
	r.devicesWithoutVMBDA = r.listDisks()
	r.createVMBDA()
	util.UntilObjectPhase(string(v1alpha2.BlockDeviceAttachmentPhaseAttached), framework.ShortTimeout, r.vmbda)
	r.createFilesystemOnDisk()
}

func (r *restoreModeTest) CreateUniqueValueOnDisk() {
	GinkgoHelper()

	r.generatedValue = strconv.Itoa(time.Now().UTC().Second())
	r.mountVMBDADeviceIfNotMounted()
	r.createFileOnDisk(r.generatedValue)
}

func (r *restoreModeTest) CreateSnapshot() {
	GinkgoHelper()

	r.vmSnapshot = vmsnapshotbuilder.New(
		vmsnapshotbuilder.WithName("vmsnapshot"),
		vmsnapshotbuilder.WithNamespace(r.f.Namespace().Name),
		vmsnapshotbuilder.WithVirtualMachineName(r.vm.Name),
		vmsnapshotbuilder.WithRequiredConsistency(true),
		vmsnapshotbuilder.WithKeepIPAddress(v1alpha2.KeepIPAddressAlways),
	)
	err := r.f.CreateWithDeferredDeletion(context.Background(), r.vmSnapshot)
	Expect(err).NotTo(HaveOccurred())

	util.UntilObjectPhase(string(v1alpha2.VirtualMachineSnapshotPhaseReady), framework.ShortTimeout, r.vmSnapshot)
}

func (r *restoreModeTest) ChangeVM() {
	GinkgoHelper()

	r.changeVMConfiguration()
	util.UntilVMAgentReady(crclient.ObjectKeyFromObject(r.vm), framework.ShortTimeout)
	util.UntilObjectPhase(string(v1alpha2.BlockDeviceAttachmentPhaseAttached), framework.ShortTimeout, r.vmbda)

	r.rebootVM()
}

func (r *restoreModeTest) CreateRestoreOperation(restoreMode v1alpha2.VMOPRestoreMode) {
	r.vmopRestore = vmopbuilder.New(
		vmopbuilder.WithName("restore-strict"),
		vmopbuilder.WithNamespace(r.f.Namespace().Name),
		vmopbuilder.WithType(v1alpha2.VMOPTypeRestore),
		vmopbuilder.WithVirtualMachine(r.vm.Name),
		vmopbuilder.WithVMOPRestoreMode(restoreMode),
		vmopbuilder.WithVirtualMachineSnapshotName(r.vmSnapshot.Name),
	)
	err := r.f.CreateWithDeferredDeletion(context.Background(), r.vmopRestore)
	Expect(err).NotTo(HaveOccurred())

	util.UntilObjectPhase(string(v1alpha2.VMOPPhaseCompleted), framework.LongTimeout, r.vmopRestore)
}

func (r *restoreModeTest) CheckVMAfterRestore(restoreMode v1alpha2.VMOPRestoreMode) {
	GinkgoHelper()

	util.UntilVMAgentReady(crclient.ObjectKeyFromObject(r.vm), framework.LongTimeout)
	util.UntilObjectPhase(string(v1alpha2.BlockDeviceAttachmentPhaseAttached), framework.ShortTimeout, r.vmbda)

	switch restoreMode {
	case v1alpha2.VMOPRestoreModeStrict, v1alpha2.VMOPRestoreModeBestEffort:
		r.checkVMInInitialState()
	case v1alpha2.VMOPRestoreModeDryRun:
		r.CheckVMInChangedState()
	}
}

func (r *restoreModeTest) RemoveDisks() {
	GinkgoHelper()

	// Stop VM and remove all disks to test Strict restore mode.
	err := util.StopVirtualMachineFromOS(r.f, r.vm)
	Expect(err).NotTo(HaveOccurred())
	util.UntilVirtualMachineStopped(crclient.ObjectKeyFromObject(r.vm), framework.ShortTimeout)

	err = r.f.Delete(context.Background(), r.vdRoot)
	Expect(err).NotTo(HaveOccurred())
	err = r.f.Delete(context.Background(), r.vdBlank)
	Expect(err).NotTo(HaveOccurred())

	// Wait for disks to be fully deleted before proceeding with restore
	Eventually(func(g Gomega) {
		var vdRootLocal v1alpha2.VirtualDisk
		err = r.f.Clients.GenericClient().Get(context.Background(), types.NamespacedName{
			Namespace: r.vdRoot.Namespace,
			Name:      r.vdRoot.Name,
		}, &vdRootLocal)
		g.Expect(k8serrors.IsNotFound(err)).Should(BeTrue())

		var vdBlankLocal v1alpha2.VirtualDisk
		err = r.f.Clients.GenericClient().Get(context.Background(), types.NamespacedName{
			Namespace: r.vdBlank.Namespace,
			Name:      r.vdBlank.Name,
		}, &vdBlankLocal)
		g.Expect(k8serrors.IsNotFound(err)).Should(BeTrue())
	}, framework.LongTimeout, time.Second).Should(Succeed())
}

func (r *restoreModeTest) createEnvironmentResources() {
	GinkgoHelper()

	r.cvi = cvibuilder.New(
		cvibuilder.WithGenerateName("cvi-"),
		cvibuilder.WithDataSourceHTTP(cviURL, nil, nil),
	)
	err := r.f.CreateWithDeferredDeletion(context.Background(), r.cvi)
	Expect(err).NotTo(HaveOccurred())

	r.vi = vibuilder.New(
		vibuilder.WithName("vi"),
		vibuilder.WithNamespace(r.f.Namespace().Name),
		vibuilder.WithDataSourceHTTP(viURL, nil, nil),
		vibuilder.WithStorage(v1alpha2.StorageContainerRegistry),
	)
	r.vdRoot = object.NewHTTPVDUbuntu("vd-root", r.f.Namespace().Name)
	r.vdBlank = object.NewBlankVD("vd-blank", r.f.Namespace().Name, nil, ptr.To(resource.MustParse("51Mi")))
	err = r.f.CreateWithDeferredDeletion(context.Background(), r.vi, r.vdRoot, r.vdBlank)
	Expect(err).NotTo(HaveOccurred())

	r.vm = object.NewMinimalVM(
		"", r.f.Namespace().Name,
		vmbuilder.WithName("vm"),
		vmbuilder.WithAnnotation(testAnnotationName, defaultValue),
		vmbuilder.WithLabel(testLabelName, defaultValue),
		vmbuilder.WithCPU(defaultCPUCores, ptr.To("5%")),
		vmbuilder.WithMemory(resource.MustParse(defaultMemorySize)),
		vmbuilder.WithBlockDeviceRefs(
			v1alpha2.BlockDeviceSpecRef{
				Kind: v1alpha2.DiskDevice,
				Name: r.vdRoot.Name,
			},
		),
		vmbuilder.WithBlockDeviceRefs(
			v1alpha2.BlockDeviceSpecRef{
				Kind: v1alpha2.ClusterImageDevice,
				Name: r.cvi.Name,
			},
		),
		vmbuilder.WithBlockDeviceRefs(
			v1alpha2.BlockDeviceSpecRef{
				Kind: v1alpha2.ImageDevice,
				Name: r.vi.Name,
			},
		),
	)
	err = r.f.CreateWithDeferredDeletion(context.Background(), r.vm)
	Expect(err).NotTo(HaveOccurred())
}

func (r *restoreModeTest) createVMBDA() {
	GinkgoHelper()

	r.vmbda = object.NewVMBDAFromDisk("vmbda", r.vm.Name, r.vdBlank)
	err := r.f.CreateWithDeferredDeletion(context.Background(), r.vmbda)
	Expect(err).NotTo(HaveOccurred())
}

// Change VM configuration to verify restore functionality:
// - Modify data on disk
// - Change annotations and labels
// - Update CPU and memory resources
// After restore, all these should revert to original values from snapshot
func (r *restoreModeTest) changeVMConfiguration() {
	GinkgoHelper()

	r.changeValueOnDisk(changedValue)
	err := r.f.UpdateFromCluster(context.Background(), r.vm)
	Expect(err).NotTo(HaveOccurred())
	Expect(r.vm.Annotations).ToNot(BeNil()) // otherwise a panic may potentially occur
	Expect(r.vm.Labels).ToNot(BeNil())      // otherwise a panic may potentially occur
	r.vm.Annotations[testAnnotationName] = changedValue
	r.vm.Labels[testLabelName] = changedValue
	r.vm.Spec.CPU.Cores = changedCPUCores
	r.vm.Spec.Memory.Size = resource.MustParse(changedMemorySize)
	err = r.f.Clients.GenericClient().Update(context.Background(), r.vm)
	Expect(err).NotTo(HaveOccurred())
}

// Verify that VM has been restored to its initial state from snapshot:
// - Disk contains the original value
// - Annotations and labels match snapshot
// - CPU and memory resources are restored to original values
func (r *restoreModeTest) checkVMInInitialState() {
	GinkgoHelper()

	r.mountVMBDADeviceIfNotMounted()
	Expect(r.getValueFromDisk()).To(Equal(r.generatedValue))
	err := r.f.UpdateFromCluster(context.Background(), r.vm)
	Expect(err).NotTo(HaveOccurred())
	Expect(r.vm.Annotations[testAnnotationName]).To(Equal(defaultValue))
	Expect(r.vm.Labels[testLabelName]).To(Equal(defaultValue))
	Expect(r.vm.Status.Resources.CPU.Cores).To(Equal(defaultCPUCores))
	Expect(r.vm.Status.Resources.Memory.Size).To(Equal(resource.MustParse(defaultMemorySize)))
}

func (r *restoreModeTest) CheckVMInChangedState() {
	GinkgoHelper()

	r.mountVMBDADeviceIfNotMounted()
	Expect(r.getValueFromDisk()).To(Equal(changedValue))
	err := r.f.UpdateFromCluster(context.Background(), r.vm)
	Expect(err).NotTo(HaveOccurred())
	Expect(r.vm.Annotations[testAnnotationName]).To(Equal(changedValue))
	Expect(r.vm.Labels[testLabelName]).To(Equal(changedValue))
	Expect(r.vm.Status.Resources.CPU.Cores).To(Equal(changedCPUCores))
	Expect(r.vm.Status.Resources.Memory.Size).To(Equal(resource.MustParse(changedMemorySize)))
}

func (r *restoreModeTest) rebootVM() {
	GinkgoHelper()

	err := r.f.UpdateFromCluster(context.Background(), r.vm)
	Expect(err).NotTo(HaveOccurred())
	runningCondition, _ := conditions.GetCondition(vmcondition.TypeRunning, r.vm.Status.Conditions)
	r.runningLastTransitionTime = runningCondition.LastTransitionTime.Time
	err = util.RebootVirtualMachineFromOS(r.f, r.vm)
	Expect(err).NotTo(HaveOccurred())

	util.UntilVirtualMachineRebooted(crclient.ObjectKeyFromObject(r.vm), r.runningLastTransitionTime, framework.LongTimeout)
	util.UntilObjectPhase(string(v1alpha2.BlockDeviceAttachmentPhaseAttached), framework.ShortTimeout, r.vmbda)
}

func (r *restoreModeTest) listDisks() (disks []string) {
	GinkgoHelper()

	lsblkOut, err := r.f.SSHCommand(r.vm.Name, r.vm.Namespace, "lsblk | grep disk")
	Expect(err).NotTo(HaveOccurred())
	lsblkLines := strings.Split(strings.TrimSpace(lsblkOut), "\n")

	for _, line := range lsblkLines {
		columns := strings.Split(strings.TrimSpace(line), " ")
		Expect(len(columns)).To(BeNumerically(">", 0))
		disks = append(disks, columns[0])
	}
	return
}

func (r *restoreModeTest) getVMBDADevicePath() string {
	GinkgoHelper()

	checkMap := make(map[string]struct{})

	for _, device := range r.devicesWithoutVMBDA {
		checkMap[device] = struct{}{}
	}

	disks := r.listDisks()
	for _, disk := range disks {
		if _, ok := checkMap[disk]; ok {
			continue
		}
		return fmt.Sprintf("/dev/%s", disk)
	}
	Fail("No vmbda device in VM")
	return ""
}

func (r *restoreModeTest) checkMntOccupied() bool {
	GinkgoHelper()

	cmdOut, err := r.f.SSHCommand(r.vm.Name, r.vm.Namespace, "findmnt /mnt &> /dev/null ; echo $?")
	Expect(err).NotTo(HaveOccurred())
	return strings.TrimSpace(cmdOut) == "0"
}

func (r *restoreModeTest) mountVMBDADeviceIfNotMounted() {
	GinkgoHelper()

	if r.checkMntOccupied() {
		return
	}

	devicePath := r.getVMBDADevicePath()
	_, err := r.f.SSHCommand(r.vm.Name, r.vm.Namespace, fmt.Sprintf("sudo mount %s /mnt", devicePath))
	Expect(err).NotTo(HaveOccurred())
}

func (r *restoreModeTest) createFilesystemOnDisk() {
	GinkgoHelper()

	_, err := r.f.SSHCommand(r.vm.Name, r.vm.Namespace, fmt.Sprintf("sudo mkfs.ext4 %s", r.getVMBDADevicePath()))
	Expect(err).NotTo(HaveOccurred())
}

func (r *restoreModeTest) createFileOnDisk(value string) {
	GinkgoHelper()

	_, err := r.f.SSHCommand(r.vm.Name, r.vm.Namespace, fmt.Sprintf("sudo bash -c \"echo %s > /mnt/value\"", value))
	Expect(err).NotTo(HaveOccurred())
}

func (r *restoreModeTest) changeValueOnDisk(value string) {
	GinkgoHelper()

	_, err := r.f.SSHCommand(r.vm.Name, r.vm.Namespace, fmt.Sprintf("sudo bash -c \"echo %s > /mnt/value\"", value))
	Expect(err).NotTo(HaveOccurred())
}

func (r *restoreModeTest) getValueFromDisk() string {
	GinkgoHelper()

	cmdOut, err := r.f.SSHCommand(r.vm.Name, r.vm.Namespace, "sudo cat /mnt/value")
	Expect(err).NotTo(HaveOccurred())
	return strings.TrimSpace(cmdOut)
}
