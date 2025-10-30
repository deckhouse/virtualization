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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/utils/ptr"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"

	cvibuilder "github.com/deckhouse/virtualization-controller/pkg/builder/cvi"
	vdbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vd"
	vibuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vi"
	vmbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vm"
	vmbdabuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vmbda"
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
	)

	var (
		cvi        *v1alpha2.ClusterVirtualImage
		vi         *v1alpha2.VirtualImage
		vdRoot     *v1alpha2.VirtualDisk
		vdBlank    *v1alpha2.VirtualDisk
		vm         *v1alpha2.VirtualMachine
		vmbda      *v1alpha2.VirtualMachineBlockDeviceAttachment
		vmsnapshot *v1alpha2.VirtualMachineSnapshot

		generatedValue            string
		runningLastTransitionTime time.Time

		f = framework.NewFramework("vmop-restore")

		createEnvironmentResources     func(namespace string)
		shellCreateFsAndSetValueOnDisk func(value string) string
		// shellChangeValueOnDisk         func(value string) string
		// shellMountAndGetValueFromDisk  func() string
	)

	BeforeEach(func() {
		DeferCleanup(f.After)

		f.Before()
	})

	It("restores a virtual machine from a snapshot", func() {
		By("Environment preparation", func() {
			createEnvironmentResources(f.Namespace().Name)

			// vm.Annotations[testAnnotationName] = changedValue
			vm.Labels[testLabelName] = changedValue

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
			err := f.UpdateFromCluster(context.Background(), vm)
			Expect(err).NotTo(HaveOccurred())
			vm.Annotations[testAnnotationName] = changedValue
			vm.Labels[testLabelName] = changedValue
			vm.Spec.CPU.Cores = 2
			vm.Spec.Memory.Size = resource.MustParse("2Gi")
			err = f.Clients.GenericClient().Update(context.Background(), vm)
			Expect(err).NotTo(HaveOccurred())
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

			Eventually(func(g Gomega) {
				err = f.UpdateFromCluster(context.Background(), vm)
				Expect(err).NotTo(HaveOccurred())

				runningCondition, _ := conditions.GetCondition(vmcondition.TypeRunning, vm.Status.Conditions)
				g.Expect(runningCondition.LastTransitionTime.Time.After(runningLastTransitionTime)).Should(BeTrue())
			}, framework.LongTimeout, time.Second).Should(Succeed())
			util.UntilVMAgentReady(crclient.ObjectKeyFromObject(vm), framework.LongTimeout)
			util.UntilVMBDAttached(crclient.ObjectKeyFromObject(vmbda), framework.ShortTimeout)
		})
		By("Rebooted")
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
		vdRoot = vdbuilder.New(
			vdbuilder.WithName("vd-root"),
			vdbuilder.WithNamespace(namespace),
			vdbuilder.WithDataSourceHTTP(&v1alpha2.DataSourceHTTP{
				URL: object.UbuntuHTTP,
			}),
			vdbuilder.WithSize(ptr.To(resource.MustParse("10Gi"))),
		)
		vdBlank = vdbuilder.New(
			vdbuilder.WithName("vd-blank"),
			vdbuilder.WithNamespace(namespace),
			vdbuilder.WithPersistentVolumeClaim(nil, ptr.To(resource.MustParse("51Mi"))),
		)
		err = f.CreateWithDeferredDeletion(context.Background(), vi, vdRoot, vdBlank)
		Expect(err).NotTo(HaveOccurred())

		vm = object.NewMinimalVM(
			"", namespace,
			vmbuilder.WithName("vm"),
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

		vmbda = vmbdabuilder.New(
			vmbdabuilder.WithName("vmbda"),
			vmbdabuilder.WithNamespace(namespace),
			vmbdabuilder.WithVirtualMachineName(vm.Name),
			vmbdabuilder.WithBlockDeviceRef(v1alpha2.VMBDAObjectRefKindVirtualDisk, vdBlank.Name),
		)
		err = f.CreateWithDeferredDeletion(context.Background(), vmbda)
		Expect(err).NotTo(HaveOccurred())
	}

	shellCreateFsAndSetValueOnDisk = func(value string) string {
		return fmt.Sprintf("umount /mnt &>/dev/null || DEV=/dev/$(sudo lsblk | grep disk | tail -n 1 | awk \"{print \\$1}\") && sudo mkfs.ext4 $DEV && sudo mount $DEV /mnt && sudo bash -c \"echo %s > /mnt/value\"", value)
	}

	// shellChangeValueOnDisk = func(value string) string {
	// 	return fmt.Sprintf("sudo bash -c \"echo %s > /mnt/value\"", value)
	// }

	// shellMountAndGetValueFromDisk = func() string {
	// 	return "umount /mnt &>/dev/null || DEV=/dev/$(sudo lsblk | grep disk | tail -n 1 | awk \"{print \\$1}\") && sudo mount $DEV /mnt && cat /mnt/value"
	// }
})
