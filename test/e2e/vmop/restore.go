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
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"

	cvibuilder "github.com/deckhouse/virtualization-controller/pkg/builder/cvi"
	vdbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vd"
	vibuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vi"
	vmbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vm"
	vmbdabuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vmbda"
	vmopbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vmop"
	vmsnapshotbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vmsnapshot"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmopcondition"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	"github.com/deckhouse/virtualization/test/e2e/internal/object"
	"github.com/deckhouse/virtualization/test/e2e/internal/util"
)

const (
	minimalVIURL              = "https://89d64382-20df-4581-8cc7-80df331f67fa.selstorage.ru/test/test.qcow2"
	minimalCVIURL             = "https://89d64382-20df-4581-8cc7-80df331f67fa.selstorage.ru/test/test.iso"
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
	originalMemorySize        = "256Mi"
	changedCPUCores           = 2
	changedMemorySize         = "512Mi"
	fileDataPath              = "/mnt/value"
)

var _ = Describe("VirtualMachineOperationRestore", Label("Slow"), func() {
	DescribeTable("restores a virtual machine from a snapshot", func(restoreMode v1alpha2.SnapshotOperationMode, restartApprovalMode v1alpha2.RestartApprovalMode, runPolicy v1alpha2.RunPolicy, shouldDeleteBeforeRestore bool) {
		f := framework.NewFramework(fmt.Sprintf("vmop-restore-%s", strings.ToLower(string(restoreMode))))
		DeferCleanup(f.After)
		f.Before()
		t := newRestoreTest(f)

		By("Environment preparation", func() {
			t.GenerateResources(restoreMode, restartApprovalMode, runPolicy)
			err := f.CreateWithDeferredDeletion(
				context.Background(), t.CVI, t.VI, t.VDRoot, t.VDBlank, t.VM, t.VMBDA,
			)
			Expect(err).NotTo(HaveOccurred())
			if runPolicy == v1alpha2.ManualPolicy {
				util.UntilObjectPhase(string(v1alpha2.MachineStopped), framework.ShortTimeout, t.VM)
				util.StartVirtualMachine(f, t.VM)
			}
			util.UntilVMAgentReady(crclient.ObjectKeyFromObject(t.VM), framework.LongTimeout)
			util.UntilObjectPhase(string(v1alpha2.BlockDeviceAttachmentPhaseAttached), framework.MiddleTimeout, t.VMBDA)

			util.CreateBlockDeviceFilesystem(f, t.VM, v1alpha2.DiskDevice, t.VDBlank.Name, "ext4")
			util.MountBlockDevice(f, t.VM, v1alpha2.DiskDevice, t.VDBlank.Name)
			util.WriteFile(f, t.VM, fileDataPath, originalValueOnDisk)

			err = f.CreateWithDeferredDeletion(context.Background(), t.VMSnapshot)
			Expect(err).NotTo(HaveOccurred())
			util.UntilObjectPhase(string(v1alpha2.VirtualMachineSnapshotPhaseReady), framework.ShortTimeout, t.VMSnapshot)
		})
		By("Changing VM", func() {
			util.WriteFile(f, t.VM, fileDataPath, changedValueOnDisk)

			err := f.Clients.GenericClient().Get(context.Background(), crclient.ObjectKeyFromObject(t.VM), t.VM)
			Expect(err).NotTo(HaveOccurred())

			runningCondition, _ := conditions.GetCondition(vmcondition.TypeRunning, t.VM.Status.Conditions)
			// Save the last transition time of the Running status, so that we can verify
			// that the virtual machine was actually rebooted after changes.
			runningLastTransitionTime := runningCondition.LastTransitionTime.Time

			t.VM.Annotations[vmAnnotationName] = vmAnnotationChangedValue
			t.VM.Labels[vmLabelName] = vmLabelChangedValue
			t.VM.Spec.CPU.Cores = changedCPUCores
			t.VM.Spec.Memory.Size = resource.MustParse(changedMemorySize)
			err = f.Clients.GenericClient().Update(context.Background(), t.VM)
			Expect(err).NotTo(HaveOccurred())

			if util.IsRestartRequired(t.VM, 3*time.Second) {
				util.RebootVirtualMachineBySSH(f, t.VM)
			}

			util.UntilVirtualMachineRebooted(crclient.ObjectKeyFromObject(t.VM), runningLastTransitionTime, framework.LongTimeout)
			util.UntilVMAgentReady(crclient.ObjectKeyFromObject(t.VM), framework.ShortTimeout)
			util.UntilObjectPhase(string(v1alpha2.BlockDeviceAttachmentPhaseAttached), framework.MiddleTimeout, t.VMBDA)
		})
		By("Check that VM is in changed state", func() {
			Expect(util.ReadFile(f, t.VM, fileDataPath)).To(Equal(changedValueOnDisk))
			err := f.Clients.GenericClient().Get(context.Background(), crclient.ObjectKeyFromObject(t.VM), t.VM)
			Expect(err).NotTo(HaveOccurred())
			Expect(t.VM.Annotations[vmAnnotationName]).To(Equal(vmAnnotationChangedValue))
			Expect(t.VM.Labels[vmLabelName]).To(Equal(vmLabelChangedValue))
			Expect(t.VM.Status.Resources.CPU.Cores).To(Equal(changedCPUCores))
			Expect(t.VM.Status.Resources.Memory.Size).To(Equal(resource.MustParse(changedMemorySize)))
		})
		By("Resource preparation", func() {
			if shouldDeleteBeforeRestore {
				t.RemoveRecoverableResources()
			}
		})
		By("Restore VM from snapshot", func() {
			t.RestoreVM(t.VM, t.VMOPRestore)
		})
		By("Check VM after restore", func() {
			t.CheckVMAfterRestore(t.VM, t.VDRoot, t.VDBlank, t.VMBDA, t.VMOPRestore)
		})
		By("After restoration, verify that labels and annotations are preserved on the resources", func() {
			err := f.Clients.GenericClient().Get(context.Background(), crclient.ObjectKeyFromObject(t.VDRoot), t.VDRoot)
			Expect(err).NotTo(HaveOccurred())
			Expect(t.VDRoot.Annotations[resourceAnnotationName]).To(Equal(resourceAnnotationValue))
			Expect(t.VDRoot.Labels[resourceLabelName]).To(Equal(resourceLabelValue))

			err = f.Clients.GenericClient().Get(context.Background(), crclient.ObjectKeyFromObject(t.VDBlank), t.VDBlank)
			Expect(err).NotTo(HaveOccurred())
			Expect(t.VDBlank.Annotations[resourceAnnotationName]).To(Equal(resourceAnnotationValue))
			Expect(t.VDBlank.Labels[resourceLabelName]).To(Equal(resourceLabelValue))

			err = f.Clients.GenericClient().Get(context.Background(), crclient.ObjectKeyFromObject(t.VMBDA), t.VMBDA)
			Expect(err).NotTo(HaveOccurred())
			Expect(t.VMBDA.Annotations[resourceAnnotationName]).To(Equal(resourceAnnotationValue))
			Expect(t.VMBDA.Labels[resourceLabelName]).To(Equal(resourceLabelValue))
		})
	},
		Entry("DryRun restore mode with VM manual restart approval mode, always on unless stopped manually run policy", v1alpha2.SnapshotOperationModeDryRun, v1alpha2.Manual, v1alpha2.AlwaysOnUnlessStoppedManually, false),
		Entry("BestEffort restore mode with VM manual restart approval mode, always on unless stopped manually run policy", v1alpha2.SnapshotOperationModeBestEffort, v1alpha2.Manual, v1alpha2.AlwaysOnUnlessStoppedManually, false),
		Entry("Strict restore mode with VM manual restart approval mode, always on unless stopped manually run policy", v1alpha2.SnapshotOperationModeStrict, v1alpha2.Manual, v1alpha2.AlwaysOnUnlessStoppedManually, false),
		Entry("BestEffort restore mode with VM manual restart approval mode, always on unless stopped manually run policy with resource deletion", v1alpha2.SnapshotOperationModeBestEffort, v1alpha2.Manual, v1alpha2.AlwaysOnUnlessStoppedManually, true),
		Entry("Strict restore mode with VM manual restart approval mode, always on unless stopped manually run policy with resource deletion", v1alpha2.SnapshotOperationModeStrict, v1alpha2.Manual, v1alpha2.AlwaysOnUnlessStoppedManually, true),
		Entry("BestEffort restore mode with VM automatic restart approval mode, always on unless stopped manually run policy", v1alpha2.SnapshotOperationModeBestEffort, v1alpha2.Automatic, v1alpha2.AlwaysOnUnlessStoppedManually, false),
		Entry("BestEffort restore mode with VM automatic restart approval mode, manual run policy", v1alpha2.SnapshotOperationModeBestEffort, v1alpha2.Automatic, v1alpha2.ManualPolicy, false),
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

	Framework *framework.Framework
}

func newRestoreTest(f *framework.Framework) *restoreModeTest {
	return &restoreModeTest{
		Framework: f,
	}
}

func (t *restoreModeTest) GenerateResources(restoreMode v1alpha2.SnapshotOperationMode, restartApprovalMode v1alpha2.RestartApprovalMode, runPolicy v1alpha2.RunPolicy) {
	t.CVI = cvibuilder.New(
		cvibuilder.WithName(fmt.Sprintf("%s-cvi", t.Framework.Namespace().Name)),
		cvibuilder.WithDataSourceHTTP(minimalCVIURL, nil, nil),
	)

	t.VI = vibuilder.New(
		vibuilder.WithName("vi"),
		vibuilder.WithNamespace(t.Framework.Namespace().Name),
		vibuilder.WithDataSourceHTTP(minimalVIURL, nil, nil),
		vibuilder.WithStorage(v1alpha2.StorageContainerRegistry),
	)

	t.VDRoot = vdbuilder.New(
		vdbuilder.WithName("vd-root"),
		vdbuilder.WithNamespace(t.Framework.Namespace().Name),
		vdbuilder.WithDataSourceHTTP(&v1alpha2.DataSourceHTTP{
			URL: object.ImageURLUbuntu,
		}),
		vdbuilder.WithAnnotation(resourceAnnotationName, resourceAnnotationValue),
		vdbuilder.WithLabel(resourceLabelName, resourceLabelValue),
	)

	t.VDBlank = vdbuilder.New(
		vdbuilder.WithName("vd-blank"),
		vdbuilder.WithNamespace(t.Framework.Namespace().Name),
		vdbuilder.WithPersistentVolumeClaim(nil, ptr.To(resource.MustParse("51Mi"))),
		vdbuilder.WithAnnotation(resourceAnnotationName, resourceAnnotationValue),
		vdbuilder.WithLabel(resourceLabelName, resourceLabelValue),
	)

	t.VM = vmbuilder.New(
		vmbuilder.WithName("vm"),
		vmbuilder.WithNamespace(t.Framework.Namespace().Name),
		vmbuilder.WithAnnotation(vmAnnotationName, vmAnnotationOriginalValue),
		vmbuilder.WithLabel(vmLabelName, vmLabelOriginalValue),
		vmbuilder.WithCPU(originalCPUCores, ptr.To("5%")),
		vmbuilder.WithMemory(resource.MustParse(originalMemorySize)),
		vmbuilder.WithLiveMigrationPolicy(v1alpha2.AlwaysSafeMigrationPolicy),
		vmbuilder.WithVirtualMachineClass(object.DefaultVMClass),
		vmbuilder.WithProvisioningUserData(object.DefaultCloudInit),
		vmbuilder.WithBlockDeviceRefs(
			v1alpha2.BlockDeviceSpecRef{
				Kind: v1alpha2.DiskDevice,
				Name: t.VDRoot.Name,
			},
		),
		vmbuilder.WithBlockDeviceRefs(
			v1alpha2.BlockDeviceSpecRef{
				Kind: v1alpha2.ClusterImageDevice,
				Name: t.CVI.Name,
			},
		),
		vmbuilder.WithBlockDeviceRefs(
			v1alpha2.BlockDeviceSpecRef{
				Kind: v1alpha2.ImageDevice,
				Name: t.VI.Name,
			},
		),
		vmbuilder.WithRestartApprovalMode(restartApprovalMode),
		vmbuilder.WithRunPolicy(runPolicy),
	)

	t.VMBDA = vmbdabuilder.New(
		vmbdabuilder.WithName("vmbda"),
		vmbdabuilder.WithNamespace(t.VDBlank.Namespace),
		vmbdabuilder.WithVirtualMachineName(t.VM.Name),
		vmbdabuilder.WithBlockDeviceRef(v1alpha2.VMBDAObjectRefKindVirtualDisk, t.VDBlank.Name),
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

func (t *restoreModeTest) RemoveRecoverableResources() {
	GinkgoHelper()

	err := util.StopVirtualMachineFromOS(t.Framework, t.VM)
	Expect(err).NotTo(HaveOccurred())
	util.UntilObjectPhase(string(v1alpha2.MachineStopped), framework.ShortTimeout, t.VM)

	err = t.Framework.Delete(context.Background(), t.VDRoot, t.VDBlank, t.VMBDA)
	Expect(err).NotTo(HaveOccurred())

	// Wait for resources to be deleted before proceeding.
	Eventually(func(g Gomega) {
		var vdRootLocal v1alpha2.VirtualDisk
		err = t.Framework.Clients.GenericClient().Get(context.Background(), types.NamespacedName{
			Namespace: t.VDRoot.Namespace,
			Name:      t.VDRoot.Name,
		}, &vdRootLocal)
		g.Expect(k8serrors.IsNotFound(err)).Should(BeTrue())

		var vdBlankLocal v1alpha2.VirtualDisk
		err = t.Framework.Clients.GenericClient().Get(context.Background(), types.NamespacedName{
			Namespace: t.VDBlank.Namespace,
			Name:      t.VDBlank.Name,
		}, &vdBlankLocal)
		g.Expect(k8serrors.IsNotFound(err)).Should(BeTrue())

		var vmbdaLocal v1alpha2.VirtualMachineBlockDeviceAttachment
		err = t.Framework.Clients.GenericClient().Get(context.Background(), types.NamespacedName{
			Namespace: t.VMBDA.Namespace,
			Name:      t.VMBDA.Name,
		}, &vmbdaLocal)
		g.Expect(k8serrors.IsNotFound(err)).Should(BeTrue())
	}, framework.LongTimeout, time.Second).Should(Succeed())
}

func (t *restoreModeTest) CheckVMAfterRestore(
	vm *v1alpha2.VirtualMachine,
	vdRoot, vdBlank *v1alpha2.VirtualDisk,
	vmbda *v1alpha2.VirtualMachineBlockDeviceAttachment,
	vmopRestore *v1alpha2.VirtualMachineOperation,
) {
	GinkgoHelper()

	err := t.Framework.Clients.GenericClient().Get(context.Background(), crclient.ObjectKeyFromObject(vm), vm)
	Expect(err).NotTo(HaveOccurred())

	// In DryRun mode, the VM should remain unchanged and VMOPRestore should contain
	// information about resources ready for restore. In actual restore modes,
	// the VM should be restored to the snapshot state.
	switch vmopRestore.Spec.Restore.Mode {
	case v1alpha2.SnapshotOperationModeDryRun:
		err := t.Framework.Clients.GenericClient().Get(context.Background(), crclient.ObjectKeyFromObject(vmopRestore), vmopRestore)
		Expect(err).NotTo(HaveOccurred())
		restoreCompletedCondition, _ := conditions.GetCondition(vmopcondition.TypeRestoreCompleted, vmopRestore.Status.Conditions)
		Expect(restoreCompletedCondition.Status).To(Equal(metav1.ConditionTrue))
		Expect(restoreCompletedCondition.Reason).To(Equal(vmopcondition.ReasonDryRunOperationCompleted.String()))
		Expect(restoreCompletedCondition.Message).To(ContainSubstring("The virtual machine can be restored from the snapshot."))

		t.CheckResourceReadyForRestore(vmopRestore, v1alpha2.VirtualMachineKind, vm.Name)
		t.CheckResourceReadyForRestore(vmopRestore, v1alpha2.VirtualDiskKind, vdRoot.Name)
		t.CheckResourceReadyForRestore(vmopRestore, v1alpha2.VirtualDiskKind, vdBlank.Name)
		t.CheckResourceReadyForRestore(vmopRestore, v1alpha2.VirtualMachineBlockDeviceAttachmentKind, vmbda.Name)

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

func (t *restoreModeTest) RestoreVM(vm *v1alpha2.VirtualMachine, vmopRestore *v1alpha2.VirtualMachineOperation) {
	GinkgoHelper()

	err := t.Framework.CreateWithDeferredDeletion(context.Background(), vmopRestore)
	Expect(err).NotTo(HaveOccurred())
	util.UntilObjectPhase(string(v1alpha2.VMOPPhaseCompleted), framework.LongTimeout, t.VMOPRestore)

	if vmopRestore.Spec.Restore.Mode == v1alpha2.SnapshotOperationModeDryRun {
		return
	}

	// after restore, the VM is in the Stopped state
	// if runPolicy == ManualPolicy, the VM should be started
	// cannot use isRestartRequired here, because we might skip the stopped phase
	if t.VM.Spec.RunPolicy == v1alpha2.ManualPolicy {
		util.UntilObjectPhase(string(v1alpha2.MachineStopped), framework.ShortTimeout, t.VM)
		util.StartVirtualMachine(t.Framework, t.VM)
	}

	util.UntilVMAgentReady(crclient.ObjectKeyFromObject(t.VM), framework.LongTimeout)
	util.UntilObjectPhase(string(v1alpha2.BlockDeviceAttachmentPhaseAttached), framework.MiddleTimeout, t.VMBDA)
}
