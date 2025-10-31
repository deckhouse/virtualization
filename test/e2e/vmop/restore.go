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

var _ = Describe("VirtualMachineOperationRestore", func() {
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

	var (
		cvi        *v1alpha2.ClusterVirtualImage
		vi         *v1alpha2.VirtualImage
		vdRoot     *v1alpha2.VirtualDisk
		vdBlank    *v1alpha2.VirtualDisk
		vm         *v1alpha2.VirtualMachine
		vmbda      *v1alpha2.VirtualMachineBlockDeviceAttachment
		vmsnapshot *v1alpha2.VirtualMachineSnapshot

		vmoRestoreDryRun     *v1alpha2.VirtualMachineOperation
		vmoRestoreBestEffort *v1alpha2.VirtualMachineOperation
		vmoRestoreStrict     *v1alpha2.VirtualMachineOperation

		generatedValue            string
		runningLastTransitionTime time.Time

		f = framework.NewFramework("vmop-restore")

		createEnvironmentResources     func(namespace string)
		changeVMConfiguration          func()
		checkVMInInitialState          func()
		checkVMInChangedState          func()
		removeDisks                    func()
		shellCreateFsAndSetValueOnDisk func(value string) string
		shellChangeValueOnDisk         func(value string) string
		shellMountAndGetValueFromDisk  func() string
	)

	BeforeEach(func() {
		DeferCleanup(f.After)

		f.Before()
	})

	It("restores a virtual machine from a snapshot", func() {
		By("Environment preparation", func() {
			createEnvironmentResources(f.Namespace().Name)

			util.UntilVMAgentReady(crclient.ObjectKeyFromObject(vm), framework.LongTimeout)
			util.UntilVMBDAttached(crclient.ObjectKeyFromObject(vmbda), framework.ShortTimeout)
		})
		By("Create file on last disk", func() {
			generatedValue = strconv.Itoa(time.Now().UTC().Second())
			_, err := f.SSHCommand(vm.Name, vm.Namespace, shellCreateFsAndSetValueOnDisk(generatedValue))
			Expect(err).NotTo(HaveOccurred())
		})
		By("Snapshot creation", func() {
			vmsnapshot = vmsnapshotbuilder.New(
				vmsnapshotbuilder.WithName("vmsnapshot"),
				vmsnapshotbuilder.WithNamespace(f.Namespace().Name),
				vmsnapshotbuilder.WithVirtualMachineName(vm.Name),
				vmsnapshotbuilder.WithRequiredConsistency(true),
				vmsnapshotbuilder.WithKeepIPAddress(v1alpha2.KeepIPAddressAlways),
			)
			err := f.CreateWithDeferredDeletion(context.Background(), vmsnapshot)
			Expect(err).NotTo(HaveOccurred())
			util.UntilVMSnapshotReady(crclient.ObjectKeyFromObject(vmsnapshot), framework.ShortTimeout)
		})
		By("Changing VM", func() {
			changeVMConfiguration()
			util.UntilVMAgentReady(crclient.ObjectKeyFromObject(vm), framework.ShortTimeout)
			util.UntilVMBDAttached(crclient.ObjectKeyFromObject(vmbda), framework.ShortTimeout)
		})
		By("Reboot VM", func() {
			err := f.UpdateFromCluster(context.Background(), vm)
			Expect(err).NotTo(HaveOccurred())
			runningCondition, _ := conditions.GetCondition(vmcondition.TypeRunning, vm.Status.Conditions)
			runningLastTransitionTime = runningCondition.LastTransitionTime.Time
			err = util.RebootVirtualMachineFromOS(f, vm)
			Expect(err).NotTo(HaveOccurred())

			util.UntilVirtualMachineRebooted(crclient.ObjectKeyFromObject(vm), runningLastTransitionTime, framework.LongTimeout)
			util.UntilVMBDAttached(crclient.ObjectKeyFromObject(vmbda), framework.ShortTimeout)
		})
		By("Create restore DryRun operation", func() {
			vmoRestoreDryRun = vmopbuilder.New(
				vmopbuilder.WithName("restore-dry-run"),
				vmopbuilder.WithNamespace(f.Namespace().Name),
				vmopbuilder.WithType(v1alpha2.VMOPTypeRestore),
				vmopbuilder.WithVirtualMachine(vm.Name),
				vmopbuilder.WithVMOPRestoreMode(v1alpha2.VMOPRestoreModeDryRun),
				vmopbuilder.WithVirtualMachineSnapshotName(vmsnapshot.Name),
			)
			err := f.CreateWithDeferredDeletion(context.Background(), vmoRestoreDryRun)
			Expect(err).NotTo(HaveOccurred())
			util.UntilVMOPCompleted(crclient.ObjectKeyFromObject(vmoRestoreDryRun), framework.ShortTimeout)
		})
		By("Check VM in changed state", func() {
			checkVMInChangedState()
		})
		By("Create restore BestEffort operation", func() {
			vmoRestoreBestEffort = vmopbuilder.New(
				vmopbuilder.WithName("restore-best-effort"),
				vmopbuilder.WithNamespace(f.Namespace().Name),
				vmopbuilder.WithType(v1alpha2.VMOPTypeRestore),
				vmopbuilder.WithVirtualMachine(vm.Name),
				vmopbuilder.WithVMOPRestoreMode(v1alpha2.VMOPRestoreModeBestEffort),
				vmopbuilder.WithVirtualMachineSnapshotName(vmsnapshot.Name),
			)
			err := f.CreateWithDeferredDeletion(context.Background(), vmoRestoreBestEffort)
			Expect(err).NotTo(HaveOccurred())
			util.UntilVMOPCompleted(crclient.ObjectKeyFromObject(vmoRestoreBestEffort), framework.LongTimeout)
		})
		By("Check VM in restored state", func() {
			util.UntilVMAgentReady(crclient.ObjectKeyFromObject(vm), framework.LongTimeout)
			util.UntilVMBDAttached(crclient.ObjectKeyFromObject(vmbda), framework.ShortTimeout)

			checkVMInInitialState()
		})
		By("Changing VM", func() {
			changeVMConfiguration()
			util.UntilVMAgentReady(crclient.ObjectKeyFromObject(vm), framework.ShortTimeout)
			util.UntilVMBDAttached(crclient.ObjectKeyFromObject(vmbda), framework.ShortTimeout)
		})
		By("Reboot VM", func() {
			err := f.UpdateFromCluster(context.Background(), vm)
			Expect(err).NotTo(HaveOccurred())
			runningCondition, _ := conditions.GetCondition(vmcondition.TypeRunning, vm.Status.Conditions)
			runningLastTransitionTime = runningCondition.LastTransitionTime.Time
			err = util.RebootVirtualMachineFromOS(f, vm)
			Expect(err).NotTo(HaveOccurred())

			util.UntilVirtualMachineRebooted(crclient.ObjectKeyFromObject(vm), runningLastTransitionTime, framework.LongTimeout)
			util.UntilVMBDAttached(crclient.ObjectKeyFromObject(vmbda), framework.ShortTimeout)
		})
		By("Remove resources", func() {
			removeDisks()
		})
		By("Create restore Strict operation", func() {
			vmoRestoreStrict = vmopbuilder.New(
				vmopbuilder.WithName("restore-strict"),
				vmopbuilder.WithNamespace(f.Namespace().Name),
				vmopbuilder.WithType(v1alpha2.VMOPTypeRestore),
				vmopbuilder.WithVirtualMachine(vm.Name),
				vmopbuilder.WithVMOPRestoreMode(v1alpha2.VMOPRestoreModeStrict),
				vmopbuilder.WithVirtualMachineSnapshotName(vmsnapshot.Name),
			)
			err := f.CreateWithDeferredDeletion(context.Background(), vmoRestoreStrict)
			Expect(err).NotTo(HaveOccurred())
			util.UntilVMOPCompleted(crclient.ObjectKeyFromObject(vmoRestoreStrict), framework.LongTimeout)
		})
		By("Check VM in restored state", func() {
			util.UntilVMAgentReady(crclient.ObjectKeyFromObject(vm), framework.LongTimeout)
			util.UntilVMBDAttached(crclient.ObjectKeyFromObject(vmbda), framework.ShortTimeout)

			checkVMInInitialState()
		})
	})

	createEnvironmentResources = func(namespace string) {
		GinkgoHelper()

		cvi = cvibuilder.New(
			cvibuilder.WithGenerateName("cvi-"),
			cvibuilder.WithDataSourceHTTP(cviURL, nil, nil),
		)
		err := f.CreateWithDeferredDeletion(context.Background(), cvi)
		Expect(err).NotTo(HaveOccurred())

		vi = vibuilder.New(
			vibuilder.WithName("vi"),
			vibuilder.WithNamespace(namespace),
			vibuilder.WithDataSourceHTTP(viURL, nil, nil),
			vibuilder.WithStorage(v1alpha2.StorageContainerRegistry),
		)
		vdRoot = object.NewGeneratedHTTPVDUbuntu("", namespace, vdbuilder.WithName("vd-root"))
		vdBlank = object.NewBlankVD("", namespace, nil, ptr.To(resource.MustParse("51Mi")), vdbuilder.WithName("vd-blank"))
		err = f.CreateWithDeferredDeletion(context.Background(), vi, vdRoot, vdBlank)
		Expect(err).NotTo(HaveOccurred())

		vm = object.NewMinimalVM(
			"", namespace,
			vmbuilder.WithName("vm"),
			vmbuilder.WithAnnotation(testAnnotationName, defaultValue),
			vmbuilder.WithLabel(testLabelName, defaultValue),
			vmbuilder.WithCPU(defaultCPUCores, ptr.To("5%")),
			vmbuilder.WithMemory(resource.MustParse(defaultMemorySize)),
			vmbuilder.WithBlockDeviceRefs(
				v1alpha2.BlockDeviceSpecRef{
					Kind: v1alpha2.DiskDevice,
					Name: vdRoot.Name,
				},
			),
			vmbuilder.WithBlockDeviceRefs(
				v1alpha2.BlockDeviceSpecRef{
					Kind: v1alpha2.ClusterImageDevice,
					Name: cvi.Name,
				},
			),
			vmbuilder.WithBlockDeviceRefs(
				v1alpha2.BlockDeviceSpecRef{
					Kind: v1alpha2.ImageDevice,
					Name: vi.Name,
				},
			),
		)
		err = f.CreateWithDeferredDeletion(context.Background(), vm)
		Expect(err).NotTo(HaveOccurred())

		vmbda = object.NewVMBDAFromDisk("vmbda", vm.Name, vdBlank)
		err = f.CreateWithDeferredDeletion(context.Background(), vmbda)
		Expect(err).NotTo(HaveOccurred())
	}

	changeVMConfiguration = func() {
		_, err := f.SSHCommand(vm.Name, vm.Namespace, shellChangeValueOnDisk(changedValue))
		Expect(err).NotTo(HaveOccurred())
		err = f.UpdateFromCluster(context.Background(), vm)
		Expect(err).NotTo(HaveOccurred())
		Expect(vm.Annotations).ToNot(BeNil()) // otherwise a panic may potentially occur
		Expect(vm.Labels).ToNot(BeNil())      // otherwise a panic may potentially occur
		vm.Annotations[testAnnotationName] = changedValue
		vm.Labels[testLabelName] = changedValue
		vm.Spec.CPU.Cores = changedCPUCores
		vm.Spec.Memory.Size = resource.MustParse(changedMemorySize)
		err = f.Clients.GenericClient().Update(context.Background(), vm)
		Expect(err).NotTo(HaveOccurred())
	}

	checkVMInInitialState = func() {
		value, err := f.SSHCommand(vm.Name, vm.Namespace, shellMountAndGetValueFromDisk())
		Expect(err).NotTo(HaveOccurred())
		Expect(strings.TrimSpace(value)).To(Equal(generatedValue))
		err = f.UpdateFromCluster(context.Background(), vm)
		Expect(err).NotTo(HaveOccurred())
		Expect(vm.Annotations[testAnnotationName]).To(Equal(defaultValue))
		Expect(vm.Labels[testLabelName]).To(Equal(defaultValue))
		Expect(vm.Status.Resources.CPU.Cores).To(Equal(defaultCPUCores))
		Expect(vm.Status.Resources.Memory.Size).To(Equal(resource.MustParse(defaultMemorySize)))
	}

	checkVMInChangedState = func() {
		value, err := f.SSHCommand(vm.Name, vm.Namespace, shellMountAndGetValueFromDisk())
		Expect(err).NotTo(HaveOccurred())
		Expect(strings.TrimSpace(value)).To(Equal(changedValue))
		err = f.UpdateFromCluster(context.Background(), vm)
		Expect(err).NotTo(HaveOccurred())
		Expect(vm.Annotations[testAnnotationName]).To(Equal(changedValue))
		Expect(vm.Labels[testLabelName]).To(Equal(changedValue))
		Expect(vm.Status.Resources.CPU.Cores).To(Equal(changedCPUCores))
		Expect(vm.Status.Resources.Memory.Size).To(Equal(resource.MustParse(changedMemorySize)))
	}

	removeDisks = func() {
		err := util.StopVirtualMachineFromOS(f, vm)
		Expect(err).NotTo(HaveOccurred())
		util.UntilVirtualMachineStopped(crclient.ObjectKeyFromObject(vm), framework.ShortTimeout)

		err = f.Delete(context.Background(), vdRoot)
		Expect(err).NotTo(HaveOccurred())
		err = f.Delete(context.Background(), vdBlank)
		Expect(err).NotTo(HaveOccurred())

		Eventually(func(g Gomega) {
			var vdRootLocal v1alpha2.VirtualDisk
			err = f.Clients.GenericClient().Get(context.Background(), types.NamespacedName{
				Namespace: vdRoot.Namespace,
				Name:      vdRoot.Name,
			}, &vdRootLocal)
			g.Expect(k8serrors.IsNotFound(err)).Should(BeTrue())

			var vdBlankLocal v1alpha2.VirtualDisk
			err = f.Clients.GenericClient().Get(context.Background(), types.NamespacedName{
				Namespace: vdBlank.Namespace,
				Name:      vdBlank.Name,
			}, &vdBlankLocal)
			g.Expect(k8serrors.IsNotFound(err)).Should(BeTrue())
		}, framework.LongTimeout, time.Second).Should(Succeed())
	}

	shellCreateFsAndSetValueOnDisk = func(value string) string {
		return fmt.Sprintf("umount /mnt &>/dev/null || DEV=/dev/$(sudo lsblk | grep disk | tail -n 1 | awk \"{print \\$1}\") && sudo mkfs.ext4 $DEV && sudo mount $DEV /mnt && sudo bash -c \"echo %s > /mnt/value\"", value)
	}

	shellChangeValueOnDisk = func(value string) string {
		return fmt.Sprintf("sudo bash -c \"echo %s > /mnt/value\"", value)
	}

	shellMountAndGetValueFromDisk = func() string {
		return "umount /mnt &>/dev/null || DEV=/dev/$(sudo lsblk | grep disk | tail -n 1 | awk \"{print \\$1}\") && sudo mount $DEV /mnt && cat /mnt/value"
	}
})
