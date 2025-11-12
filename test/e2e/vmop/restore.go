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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	virtv1 "kubevirt.io/api/core/v1"
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
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	"github.com/deckhouse/virtualization/test/e2e/internal/object"
	"github.com/deckhouse/virtualization/test/e2e/internal/util"
)

const (
	minimalViURL       = "https://89d64382-20df-4581-8cc7-80df331f67fa.selstorage.ru/test/test.qcow2"
	minimalCviURL      = "https://89d64382-20df-4581-8cc7-80df331f67fa.selstorage.ru/test/test.iso"
	initialValue       = "initial-value"
	changedValue       = "changed-value"
	testAnnotationName = "e2e-annotation-for-check-restoring-vm"
	testLabelName      = "e2e-label-for-check-restoring-vm"
	initialCPUCores    = 1
	initialMemorySize  = "256Mi"
	changedCPUCores    = 2
	changedMemorySize  = "512Mi"
)

var _ = Describe("VirtualMachineOperationRestore", func() {
	DescribeTable("restores a virtual machine from a snapshot", func(restoreMode v1alpha2.VMOPRestoreMode, restartApprovalMode v1alpha2.RestartApprovalMode, runPolicy v1alpha2.RunPolicy) {
		f := framework.NewFramework(fmt.Sprintf("vmop-restore-%s", strings.ToLower(string(restoreMode))))
		f.Before()
		DeferCleanup(f.After)
		t := NewRestoreTest(f)

		By("Environment preparation", func() {
			t.GenerateEnvironmentResources(restoreMode, restartApprovalMode, runPolicy)
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

			vmbdaPath := t.GetVMBDADevicePath()
			t.CreateVMBDAFilesystem(vmbdaPath)
			t.MountVMBDA(vmbdaPath)
			t.GeneratedValue = strconv.Itoa(time.Now().UTC().Second())
			t.WriteDataToVMBDA(t.GeneratedValue)

			err = f.CreateWithDeferredDeletion(context.Background(), t.VMSnapshot)
			Expect(err).NotTo(HaveOccurred())
			util.UntilObjectPhase(string(v1alpha2.VirtualMachineSnapshotPhaseReady), framework.ShortTimeout, t.VMSnapshot)
		})
		By("Changing VM", func() {
			t.WriteDataToVMBDA(changedValue)

			err := f.Clients.GenericClient().Get(context.Background(), crclient.ObjectKeyFromObject(t.VM), t.VM)
			Expect(err).NotTo(HaveOccurred())

			runningCondition, _ := conditions.GetCondition(vmcondition.TypeRunning, t.VM.Status.Conditions)
			// need to verify that VM will be rebooted
			runningLastTransitionTime := runningCondition.LastTransitionTime.Time

			t.VM.Annotations[testAnnotationName] = changedValue
			t.VM.Labels[testLabelName] = changedValue
			t.VM.Spec.CPU.Cores = changedCPUCores
			t.VM.Spec.Memory.Size = resource.MustParse(changedMemorySize)
			err = f.Clients.GenericClient().Update(context.Background(), t.VM)
			Expect(err).NotTo(HaveOccurred())

			if t.VM.Spec.Disruptions.RestartApprovalMode == v1alpha2.Manual {
				err := util.RebootVirtualMachineFromOS(f, t.VM)
				Expect(err).NotTo(HaveOccurred())
			}

			util.UntilVirtualMachineRebooted(crclient.ObjectKeyFromObject(t.VM), runningLastTransitionTime, framework.LongTimeout)
			util.UntilVMAgentReady(crclient.ObjectKeyFromObject(t.VM), framework.ShortTimeout)
			util.UntilObjectPhase(string(v1alpha2.BlockDeviceAttachmentPhaseAttached), framework.MiddleTimeout, t.VMBDA)
			t.MountVMBDA(t.GetVMBDADevicePath())
		})
		By("Check that VM is in changed state", func() {
			Expect(t.GetDataFromVMBDA()).To(Equal(changedValue))
			Expect(t.VM.Annotations[testAnnotationName]).To(Equal(changedValue))
			Expect(t.VM.Labels[testLabelName]).To(Equal(changedValue))
			Expect(t.VM.Status.Resources.CPU.Cores).To(Equal(changedCPUCores))
			Expect(t.VM.Status.Resources.Memory.Size).To(Equal(resource.MustParse(changedMemorySize)))
		})
		By("Resource preparation", func() {
			if restoreMode == v1alpha2.VMOPRestoreModeStrict {
				t.RemoveDisks()
			}
		})
		By("Restore VM from snapshot", func() {
			err := f.CreateWithDeferredDeletion(context.Background(), t.VMOPRestore)
			Expect(err).NotTo(HaveOccurred())
			util.UntilObjectPhase(string(v1alpha2.VMOPPhaseCompleted), framework.LongTimeout, t.VMOPRestore)
			if restoreMode != v1alpha2.VMOPRestoreModeDryRun {
				if runPolicy == v1alpha2.ManualPolicy {
					util.UntilObjectPhase(string(v1alpha2.MachineStopped), framework.ShortTimeout, t.VM)
					util.StartVirtualMachine(f, t.VM)
				}

				util.UntilVMAgentReady(crclient.ObjectKeyFromObject(t.VM), framework.LongTimeout)
				util.UntilObjectPhase(string(v1alpha2.BlockDeviceAttachmentPhaseAttached), framework.MiddleTimeout, t.VMBDA)
				t.MountVMBDA(t.GetVMBDADevicePath())
			}
		})
		By("Check VM after restore", func() {
			err := f.Clients.GenericClient().Get(context.Background(), crclient.ObjectKeyFromObject(t.VM), t.VM)
			Expect(err).NotTo(HaveOccurred())

			if restoreMode == v1alpha2.VMOPRestoreModeDryRun {
				Expect(t.GetDataFromVMBDA()).To(Equal(changedValue))
				Expect(t.VM.Annotations[testAnnotationName]).To(Equal(changedValue))
				Expect(t.VM.Labels[testLabelName]).To(Equal(changedValue))
				Expect(t.VM.Status.Resources.CPU.Cores).To(Equal(changedCPUCores))
				Expect(t.VM.Status.Resources.Memory.Size).To(Equal(resource.MustParse(changedMemorySize)))
			} else {
				Expect(t.GetDataFromVMBDA()).To(Equal(t.GeneratedValue))
				Expect(t.VM.Annotations[testAnnotationName]).To(Equal(initialValue))
				Expect(t.VM.Labels[testLabelName]).To(Equal(initialValue))
				Expect(t.VM.Status.Resources.CPU.Cores).To(Equal(initialCPUCores))
				Expect(t.VM.Status.Resources.Memory.Size).To(Equal(resource.MustParse(initialMemorySize)))
			}
		})
	},
		Entry("DryRun restore mode with VM manual restart approval mode, always on unless stopped manually run policy", v1alpha2.VMOPRestoreModeDryRun, v1alpha2.Manual, v1alpha2.AlwaysOnUnlessStoppedManually),
		Entry("BestEffort restore mode with VM manual restart approval mode, always on unless stopped manually run policy", v1alpha2.VMOPRestoreModeBestEffort, v1alpha2.Manual, v1alpha2.AlwaysOnUnlessStoppedManually),
		Entry("Strict restore mode with VM manual restart approval mode, always on unless stopped manually run policy", v1alpha2.VMOPRestoreModeStrict, v1alpha2.Manual, v1alpha2.AlwaysOnUnlessStoppedManually),
		Entry("BestEffort restore mode with VM automatic restart approval mode, always on unless stopped manually run policy", v1alpha2.VMOPRestoreModeBestEffort, v1alpha2.Automatic, v1alpha2.AlwaysOnUnlessStoppedManually),
		Entry("BestEffort restore mode with VM automatic restart approval mode, manual run policy", v1alpha2.VMOPRestoreModeBestEffort, v1alpha2.Automatic, v1alpha2.ManualPolicy),
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
	Framework                 *framework.Framework
}

func NewRestoreTest(f *framework.Framework) *restoreModeTest {
	return &restoreModeTest{
		Framework: f,
	}
}

func (r *restoreModeTest) GenerateEnvironmentResources(restoreMode v1alpha2.VMOPRestoreMode, restartApprovalMode v1alpha2.RestartApprovalMode, runPolicy v1alpha2.RunPolicy) {
	r.CVI = cvibuilder.New(
		cvibuilder.WithName(fmt.Sprintf("%s-cvi", r.Framework.Namespace().Name)),
		cvibuilder.WithDataSourceHTTP(minimalCviURL, nil, nil),
	)

	r.VI = vibuilder.New(
		vibuilder.WithName("vi"),
		vibuilder.WithNamespace(r.Framework.Namespace().Name),
		vibuilder.WithDataSourceHTTP(minimalViURL, nil, nil),
		vibuilder.WithStorage(v1alpha2.StorageContainerRegistry),
	)

	r.VDRoot = vdbuilder.New(
		vdbuilder.WithName("vd-root"),
		vdbuilder.WithNamespace(r.Framework.Namespace().Name),
		vdbuilder.WithDataSourceHTTP(&v1alpha2.DataSourceHTTP{
			URL: object.ImageURLUbuntu,
		}),
	)

	r.VDBlank = vdbuilder.New(
		vdbuilder.WithName("vd-blank"),
		vdbuilder.WithNamespace(r.Framework.Namespace().Name),
		vdbuilder.WithPersistentVolumeClaim(nil, ptr.To(resource.MustParse("51Mi"))),
	)

	r.VM = vmbuilder.New(
		vmbuilder.WithName("vm"),
		vmbuilder.WithNamespace(r.Framework.Namespace().Name),
		vmbuilder.WithAnnotation(testAnnotationName, initialValue),
		vmbuilder.WithLabel(testLabelName, initialValue),
		vmbuilder.WithCPU(initialCPUCores, ptr.To("5%")),
		vmbuilder.WithMemory(resource.MustParse(initialMemorySize)),
		vmbuilder.WithLiveMigrationPolicy(v1alpha2.AlwaysSafeMigrationPolicy),
		vmbuilder.WithVirtualMachineClass(object.DefaultVMClass),
		vmbuilder.WithProvisioningUserData(object.DefaultCloudInit),
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
		vmbuilder.WithRestartApprovalMode(restartApprovalMode),
		vmbuilder.WithRunPolicy(runPolicy),
	)

	r.VMBDA = vmbdabuilder.New(
		vmbdabuilder.WithName("vmbda"),
		vmbdabuilder.WithNamespace(r.VDBlank.Namespace),
		vmbdabuilder.WithVirtualMachineName(r.VM.Name),
		vmbdabuilder.WithBlockDeviceRef(v1alpha2.VMBDAObjectRefKindVirtualDisk, r.VDBlank.Name),
	)

	r.VMSnapshot = vmsnapshotbuilder.New(
		vmsnapshotbuilder.WithName("vmsnapshot"),
		vmsnapshotbuilder.WithNamespace(r.Framework.Namespace().Name),
		vmsnapshotbuilder.WithVirtualMachineName(r.VM.Name),
		vmsnapshotbuilder.WithRequiredConsistency(true),
		vmsnapshotbuilder.WithKeepIPAddress(v1alpha2.KeepIPAddressAlways),
	)

	r.VMOPRestore = vmopbuilder.New(
		vmopbuilder.WithName("restore-strict"),
		vmopbuilder.WithNamespace(r.Framework.Namespace().Name),
		vmopbuilder.WithType(v1alpha2.VMOPTypeRestore),
		vmopbuilder.WithVirtualMachine(r.VM.Name),
		vmopbuilder.WithVMOPRestoreMode(restoreMode),
		vmopbuilder.WithVirtualMachineSnapshotName(r.VMSnapshot.Name),
	)
}

func (r *restoreModeTest) GetVMBDADevicePath() string {
	GinkgoHelper()

	serial, ok := r.getVMBDADiskSerialNumber(r.VDBlank.Name)
	Expect(ok).To(BeTrue(), "failed to get VMBDA disk serial number")

	devicePath, ok, err := r.getDeviceBySerial(serial)
	Expect(err).NotTo(HaveOccurred(), fmt.Errorf("failed to get device by serial: %w", err))
	Expect(ok).To(BeTrue(), "failed to get device by serial")
	return devicePath
}

func (r *restoreModeTest) CreateVMBDAFilesystem(devicePath string) {
	GinkgoHelper()

	_, err := r.Framework.SSHCommand(r.VM.Name, r.VM.Namespace, fmt.Sprintf("sudo mkfs.ext4 %s", devicePath))
	Expect(err).NotTo(HaveOccurred())
}

func (r *restoreModeTest) MountVMBDA(devicePath string) {
	GinkgoHelper()

	_, err := r.Framework.SSHCommand(r.VM.Name, r.VM.Namespace, fmt.Sprintf("sudo mount %s /mnt", devicePath))
	Expect(err).NotTo(HaveOccurred())
}

func (r *restoreModeTest) WriteDataToVMBDA(value string) {
	GinkgoHelper()

	_, err := r.Framework.SSHCommand(r.VM.Name, r.VM.Namespace, fmt.Sprintf("sudo bash -c \"echo %s > /mnt/value\"", value))
	Expect(err).NotTo(HaveOccurred())
}

func (r *restoreModeTest) GetDataFromVMBDA() string {
	GinkgoHelper()

	cmdOut, err := r.Framework.SSHCommand(r.VM.Name, r.VM.Namespace, "sudo cat /mnt/value")
	Expect(err).NotTo(HaveOccurred())
	return strings.TrimSpace(cmdOut)
}

func (r *restoreModeTest) RemoveDisks() {
	GinkgoHelper()

	err := util.StopVirtualMachineFromOS(r.Framework, r.VM)
	Expect(err).NotTo(HaveOccurred())
	util.UntilObjectPhase(string(v1alpha2.MachineStopped), framework.ShortTimeout, r.VM)

	err = r.Framework.Delete(context.Background(), r.VDRoot, r.VDBlank)
	Expect(err).NotTo(HaveOccurred())

	Eventually(func(g Gomega) {
		var vdRootLocal v1alpha2.VirtualDisk
		err = r.Framework.Clients.GenericClient().Get(context.Background(), types.NamespacedName{
			Namespace: r.VDRoot.Namespace,
			Name:      r.VDRoot.Name,
		}, &vdRootLocal)
		g.Expect(k8serrors.IsNotFound(err)).Should(BeTrue())

		var vdBlankLocal v1alpha2.VirtualDisk
		err = r.Framework.Clients.GenericClient().Get(context.Background(), types.NamespacedName{
			Namespace: r.VDBlank.Namespace,
			Name:      r.VDBlank.Name,
		}, &vdBlankLocal)
		g.Expect(k8serrors.IsNotFound(err)).Should(BeTrue())
	}, framework.LongTimeout, time.Second).Should(Succeed())
}

func (r *restoreModeTest) getDeviceBySerial(serial string) (string, bool, error) {
	cmdOut, err := r.Framework.SSHCommand(r.VM.Name, r.VM.Namespace, fmt.Sprintf("sudo lsblk -o PATH,SERIAL | grep %s | awk \"{print \\$1, \\$2}\"", serial))
	if err != nil {
		return "", false, err
	}

	cmdLines := strings.Split(strings.TrimSpace(cmdOut), "\n")
	if len(cmdLines) == 0 {
		return "", false, nil
	}

	columns := strings.Split(strings.TrimSpace(cmdLines[0]), " ")
	if len(columns) != 2 {
		return "", false, nil
	}

	if columns[1] == serial {
		return columns[0], true, nil
	}

	return "", false, nil
}

func (r *restoreModeTest) getVMBDADiskSerialNumber(vdName string) (string, bool) {
	unstructuredVMI, err := r.Framework.Clients.DynamicClient().Resource(schema.GroupVersionResource{
		Group:    "internal.virtualization.deckhouse.io",
		Version:  "v1",
		Resource: "internalvirtualizationvirtualmachineinstances",
	}).Namespace(r.VM.Namespace).Get(context.Background(), r.VM.Name, metav1.GetOptions{})
	Expect(err).NotTo(HaveOccurred())

	var kvvmi virtv1.VirtualMachineInstance
	err = runtime.DefaultUnstructuredConverter.FromUnstructured(unstructuredVMI.Object, &kvvmi)
	Expect(err).NotTo(HaveOccurred())

	for _, disk := range kvvmi.Spec.Domain.Devices.Disks {
		if disk.Name == fmt.Sprintf("vd-%s", vdName) {
			return disk.Serial, true
		}
	}

	return "", false
}
