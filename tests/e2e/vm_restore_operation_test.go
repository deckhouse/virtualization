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

package e2e

import (
	"context"
	"fmt"
	"strconv"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"

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
	"github.com/deckhouse/virtualization/tests/e2e/d8"
	"github.com/deckhouse/virtualization/tests/e2e/framework"
	"github.com/deckhouse/virtualization/tests/e2e/ginkgoutil"
)

const ubuntuUrl = "https://cloud-images.ubuntu.com/noble/20250704/noble-server-cloudimg-amd64.img"
const viUrl = "https://89d64382-20df-4581-8cc7-80df331f67fa.selstorage.ru/test/test.qcow2"
const cviUrl = "https://89d64382-20df-4581-8cc7-80df331f67fa.selstorage.ru/test/test.iso"

var _ = Describe("VirtualMachineRestoreOperation", Serial, ginkgoutil.CommonE2ETestDecorators(), func() {
	frameworkEntity := framework.NewFramework("virtual-machine-restore-operation")
	helper := NewVMOPRestoreTestHelper(frameworkEntity)

	frameworkEntity.BeforeAll()
	frameworkEntity.AfterAll()

	Context("Preparing resources", func() {
		It("Applying resources", func() {
			helper.GenerateAndCreateOriginalResources()
		})
		It("Resorces should be ready", func() {
			Eventually(func(g Gomega) {
				helper.UpdateState(g)
				helper.CheckIfResourcesReady(g)
			}, 600*time.Second, 1*time.Second).Should(Succeed())
		})

		It("Exec ssh", func() {
			Eventually(func(g Gomega) {
				helper.UpdateState(g)

				var devicePath string
				for _, bd := range helper.VM.Status.BlockDeviceRefs {
					fmt.Println(bd.Name)
					if bd.Name == helper.VDBlank.Name {
						devicePath = fmt.Sprintf("/dev/%s", bd.Target)
					}
				}
				g.Expect(devicePath).ShouldNot(BeEmpty())

				value := strconv.Itoa(time.Now().UTC().Second())
				cmdCreate := fmt.Sprintf("sudo mkfs.ext4 %[1]s && sudo mount %[1]s /mnt && sudo bash -c \"echo %s > /mnt/value\"", devicePath, value)
				fmt.Println(cmdCreate)
				cmdGet := "cat /mnt/value"

				res := d8Virtualization.SSHCommand(helper.VM.Name, cmdCreate, d8.SSHOptions{
					Namespace:   helper.VM.Namespace,
					Username:    conf.TestData.SSHUser,
					IdenityFile: conf.TestData.Sshkey,
				})
				if res.Error() != nil {
					fmt.Println(res.Error())
				}
				g.Expect(res.Error()).ShouldNot(HaveOccurred())
				By(res.StdOut())

				res = d8Virtualization.SSHCommand(helper.VM.Name, cmdGet, d8.SSHOptions{
					Namespace:   helper.VM.Namespace,
					Username:    conf.TestData.SSHUser,
					IdenityFile: conf.TestData.Sshkey,
				})
				g.Expect(res.Error()).ShouldNot(HaveOccurred())
				By(res.StdOut())
			}, 600*time.Second, time.Second).Should(Succeed())
		})
	})

	// Context("Creating snapshot", func() {
	// 	It("Applying snapshot resource", func() {
	// 		helper.VMSnapshot = helper.GenerateVMSnapshot(
	// 			"vmsnapshot",
	// 			frameworkEntity.Namespace().Name,
	// 			helper.VM.Name,
	// 			true,
	// 			v1alpha2.KeepIPAddressAlways,
	// 		)
	// 		By(fmt.Sprintf("Creating vm snapshot: %s/%s", helper.VMSnapshot.Namespace, helper.VMSnapshot.Name))
	// 		err := frameworkEntity.Clients.GenericClient().Create(context.Background(), helper.VMSnapshot)
	// 		Expect(err).ShouldNot(HaveOccurred())
	// 	})

	// 	It("Snapshot should be ready", func() {
	// 		Eventually(func(g Gomega) {
	// 			helper.UpdateState(g)

	// 			g.Expect(helper.VMSnapshot.Status.Phase).Should(Equal(v1alpha2.VirtualMachineSnapshotPhaseReady))
	// 		}, 300*time.Second, 1*time.Second).Should(Succeed())
	// 	})
	// })

	// Context("Restore dry run", func() {
	// 	It("Creating VMOP", func() {
	// 		helper.VMOPDryRun = helper.GenerateRestoreVMOP(
	// 			"vmop-dryrun", frameworkEntity.Namespace().Name,
	// 			helper.VMSnapshot.Name,
	// 			helper.VM.Name,
	// 			v1alpha2.VMOPRestoreModeDryRun,
	// 		)
	// 		err := frameworkEntity.Clients.GenericClient().Create(context.Background(), helper.VMOPDryRun)
	// 		Expect(err).ShouldNot(HaveOccurred())
	// 	})

	// 	It("Dry run restore VMOP must be Completed", func() {
	// 		Eventually(func(g Gomega) {
	// 			helper.UpdateState(g)
	// 			g.Expect(helper.VMOPDryRun.Status.Phase).Should(Equal(v1alpha2.VMOPPhaseCompleted))
	// 		}, 120*time.Second, 1*time.Second).Should(Succeed())
	// 	})
	// })

	// Context("Removing VM", func() {
	// 	It("Remove VM", func() {
	// 		err := helper.FrameworkEntity.Clients.GenericClient().Delete(context.Background(), helper.VM)
	// 		Expect(err).ShouldNot(HaveOccurred())
	// 	})

	// 	It("VM should not exists", func() {
	// 		Eventually(func(g Gomega) {
	// 			var vm v1alpha2.VirtualMachine
	// 			err := helper.FrameworkEntity.Clients.GenericClient().Get(
	// 				context.Background(),
	// 				types.NamespacedName{
	// 					Namespace: helper.VM.Namespace,
	// 					Name:      helper.VM.Name,
	// 				},
	// 				&vm,
	// 			)
	// 			g.Expect(k8serrors.IsNotFound(err)).Should(BeTrue())
	// 		}, 60*time.Second, time.Second).Should(Succeed())
	// 	})
	// })

	// Context("Restore in BestEffort mode", func() {
	// 	It("Creating VMOP", func() {
	// 		helper.VMOPBestEffort = helper.GenerateRestoreVMOP(
	// 			"vmop-besteffort", frameworkEntity.Namespace().Name,
	// 			helper.VMSnapshot.Name,
	// 			helper.VM.Name,
	// 			v1alpha2.VMOPRestoreModeBestEffort,
	// 		)
	// 		err := frameworkEntity.Clients.GenericClient().Create(context.Background(), helper.VMOPBestEffort)
	// 		Expect(err).ShouldNot(HaveOccurred())
	// 	})

	// 	It("BestEffort restore VMOP must be Completed", func() {
	// 		Eventually(func(g Gomega) {
	// 			helper.UpdateState(g)
	// 			g.Expect(helper.VMOPBestEffort.Status.Phase).Should(Equal(v1alpha2.VMOPPhaseCompleted))
	// 		}, 120*time.Second, 1*time.Second).Should(Succeed())
	// 	})
	// })

	// Context("Do something", func() {
	// 	// ???
	// })

	// Context("Restore in ??? mode", func() {
	// 	It("Creating VMOP", func() {
	// 		helper.VMOPDryRun = helper.GenerateRestoreVMOP(
	// 			"vmop-dryrun", frameworkEntity.Namespace().Name,
	// 			helper.VMSnapshot.Name,
	// 			v1alpha2.VMOPRestoreModeDryRun,
	// 		)
	// 		err := frameworkEntity.Clients.GenericClient().Create(context.Background(), helper.VMOPDryRun)
	// 		Expect(err).ShouldNot(HaveOccurred())
	// 	})

	// 	It("Dry run restore VMOP must be ???", func() {
	// 		Eventually(func(g Gomega) {
	// 			// ???
	// 		}, 120*time.Second, 1*time.Second).Should(Succeed())
	// 	})
	// })

	// Context("kek", func() {
	// 	time.Sleep(200 * time.Second)
	// })
})

type VMOPRestoreTestHelper struct {
	FrameworkEntity *framework.Framework
	VM              *v1alpha2.VirtualMachine
	CVI             *v1alpha2.ClusterVirtualImage
	VI              *v1alpha2.VirtualImage
	VDRoot, VDBlank *v1alpha2.VirtualDisk
	VMBDA           *v1alpha2.VirtualMachineBlockDeviceAttachment
	VMSnapshot      *v1alpha2.VirtualMachineSnapshot
	VMOPDryRun      *v1alpha2.VirtualMachineOperation
	VMOPStrict      *v1alpha2.VirtualMachineOperation
	VMOPBestEffort  *v1alpha2.VirtualMachineOperation
}

func NewVMOPRestoreTestHelper(frameworkEntity *framework.Framework) *VMOPRestoreTestHelper {
	return &VMOPRestoreTestHelper{
		FrameworkEntity: frameworkEntity,
	}
}

func (h *VMOPRestoreTestHelper) GenerateAndCreateOriginalResources() {
	GinkgoHelper()
	h.CVI = h.GenerateCVI("ubuntu-cvi", cviUrl)

	// for getting real cvi name
	err := h.FrameworkEntity.GenericClient().Create(context.Background(), h.CVI)
	By(fmt.Sprintf("Created cvi: %s", h.CVI.Name))
	Expect(err).ShouldNot(HaveOccurred())

	h.FrameworkEntity.AddResourceToDelete(h.CVI)
	h.VI = h.GenerateVI("ubuntu-vi", h.FrameworkEntity.Namespace().Name, viUrl)
	h.VDRoot = h.GenerateVDFromHttp("vd-root", h.FrameworkEntity.Namespace().Name, "10Gi", ubuntuUrl)
	h.VDBlank = h.GenerateVDBlank("vd-blank", h.FrameworkEntity.Namespace().Name, "200Mi")
	h.VM = h.GenerateVM(
		"ubuntu-vm",
		h.FrameworkEntity.Namespace().Name,
		v1alpha2.AlwaysSafeMigrationPolicy,
		1,
		"10%",
		"1Gi",
		v1alpha2.BlockDeviceSpecRef{
			Kind: v1alpha2.DiskDevice,
			Name: h.VDRoot.Name,
		},
		v1alpha2.BlockDeviceSpecRef{
			Kind: v1alpha2.ClusterImageDevice,
			Name: h.CVI.Name,
		},
		v1alpha2.BlockDeviceSpecRef{
			Kind: v1alpha2.ImageDevice,
			Name: h.VI.Name,
		},
	)
	h.VMBDA = h.GenerateVMBDA(
		"vmbda", h.FrameworkEntity.Namespace().Name, h.VM.Name,
		v1alpha2.VMBDAObjectRef{
			Kind: v1alpha2.VMBDAObjectRefKindVirtualDisk,
			Name: h.VDBlank.Name,
		},
	)

	By(fmt.Sprintf("Creating vi: %s/%s", h.VI.Namespace, h.VI.Name))
	err = h.FrameworkEntity.GenericClient().Create(context.Background(), h.VI)
	Expect(err).ShouldNot(HaveOccurred())
	By(fmt.Sprintf("Creating vd blank: %s/%s", h.VDBlank.Namespace, h.VDBlank.Name))
	err = h.FrameworkEntity.GenericClient().Create(context.Background(), h.VDBlank)
	Expect(err).ShouldNot(HaveOccurred())
	By(fmt.Sprintf("Creating vd root: %s/%s", h.VDRoot.Namespace, h.VDRoot.Name))
	err = h.FrameworkEntity.GenericClient().Create(context.Background(), h.VDRoot)
	Expect(err).ShouldNot(HaveOccurred())
	By(fmt.Sprintf("Creating vm: %s/%s", h.VM.Namespace, h.VM.Name))
	err = h.FrameworkEntity.GenericClient().Create(context.Background(), h.VM)
	Expect(err).ShouldNot(HaveOccurred())
	By(fmt.Sprintf("Creating vmbda: %s/%s", h.VMBDA.Namespace, h.VMBDA.Name))
	err = h.FrameworkEntity.GenericClient().Create(context.Background(), h.VMBDA)
	Expect(err).ShouldNot(HaveOccurred())
}

func (h *VMOPRestoreTestHelper) GenerateVM(
	name, namespace string,
	liveMigrationPolicy v1alpha2.LiveMigrationPolicy,
	cores int,
	coreFraction string,
	memorySize string,
	blockDeviceRefs ...v1alpha2.BlockDeviceSpecRef,
) *v1alpha2.VirtualMachine {
	cloudInit :=
		`#cloud-config
users:
- name: cloud
  passwd: $6$rounds=4096$vln/.aPHBOI7BMYR$bBMkqQvuGs5Gyd/1H5DP4m9HjQSy.kgrxpaGEHwkX7KEFV8BS.HZWPitAtZ2Vd8ZqIZRqmlykRCagTgPejt1i.
  shell: /bin/bash
  sudo: ALL=(ALL) NOPASSWD:ALL
  chpasswd: { expire: False }
  lock_passwd: false
  ssh_authorized_keys:
      - ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIFxcXHmwaGnJ8scJaEN5RzklBPZpVSic4GdaAsKjQoeA your_email@example.com

runcmd:
- [bash, -c, "apt update"]
- [bash, -c, "apt install qemu-guest-agent -y"]
- [bash, -c, "systemctl enable qemu-guest-agent"]
- [bash, -c, "systemctl start qemu-guest-agent"]`

	return vmbuilder.New(
		vmbuilder.WithName(name),
		vmbuilder.WithNamespace(namespace),
		vmbuilder.WithBlockDeviceRefs(blockDeviceRefs...),
		vmbuilder.WithLiveMigrationPolicy(liveMigrationPolicy),
		vmbuilder.WithCPU(cores, ptr.To(coreFraction)),
		vmbuilder.WithMemory(resource.MustParse(memorySize)),
		vmbuilder.WithProvisioning(
			&v1alpha2.Provisioning{
				Type:     v1alpha2.ProvisioningTypeUserData,
				UserData: cloudInit,
			},
		),
	)
}

func (h *VMOPRestoreTestHelper) GenerateVDBlank(name, namespace, size string) *v1alpha2.VirtualDisk {
	return vdbuilder.New(
		vdbuilder.WithName(name),
		vdbuilder.WithNamespace(namespace),
		vdbuilder.WithSize(ptr.To(resource.MustParse(size))),
	)
}

func (h *VMOPRestoreTestHelper) GenerateVDFromHttp(name, namespace, size, url string) *v1alpha2.VirtualDisk {
	return vdbuilder.New(
		vdbuilder.WithName(name),
		vdbuilder.WithNamespace(namespace),
		vdbuilder.WithSize(ptr.To(resource.MustParse(size))),
		vdbuilder.WithDataSourceHTTPWithOnlyURL(url),
	)
}

func (h *VMOPRestoreTestHelper) GenerateVI(name, namespace, url string) *v1alpha2.VirtualImage {
	return vibuilder.New(
		vibuilder.WithName(name),
		vibuilder.WithNamespace(namespace),
		vibuilder.WithDataSourceHTTPWithOnlyURL(url),
		vibuilder.WithStorageType(ptr.To(v1alpha2.StorageContainerRegistry)),
	)
}

func (h *VMOPRestoreTestHelper) GenerateCVI(name, url string) *v1alpha2.ClusterVirtualImage {
	return cvibuilder.New(
		cvibuilder.WithGenerateName(fmt.Sprintf("%s-", name)),
		cvibuilder.WithDataSourceHTTPWithOnlyURL(url),
	)
}

func (h *VMOPRestoreTestHelper) GenerateVMBDA(name, namespace, vmName string, bdRef v1alpha2.VMBDAObjectRef) *v1alpha2.VirtualMachineBlockDeviceAttachment {
	return vmbdabuilder.New(
		vmbdabuilder.WithName(name),
		vmbdabuilder.WithNamespace(namespace),
		vmbdabuilder.WithVMName(vmName),
		vmbdabuilder.WithBlockDeviceRef(bdRef),
	)
}

func (h *VMOPRestoreTestHelper) GenerateVMSnapshot(
	name, namespace, vmName string,
	requiredConsistency bool,
	keepIpAddress v1alpha2.KeepIPAddress,
) *v1alpha2.VirtualMachineSnapshot {
	return vmsnapshotbuilder.New(
		vmsnapshotbuilder.WithName(name),
		vmsnapshotbuilder.WithNamespace(namespace),
		vmsnapshotbuilder.WithVm(vmName),
		vmsnapshotbuilder.WithKeepIpAddress(keepIpAddress),
		vmsnapshotbuilder.WithRequiredConsistency(requiredConsistency),
	)
}

func (h *VMOPRestoreTestHelper) GenerateRestoreVMOP(name, namespace, vmSnapshotName, vmName string, restoreMode v1alpha2.VMOPRestoreMode) *v1alpha2.VirtualMachineOperation {
	restoreSpec := &v1alpha2.VirtualMachineOperationRestoreSpec{
		VirtualMachineSnapshotName: vmSnapshotName,
		Mode:                       restoreMode,
	}

	return vmopbuilder.New(
		vmopbuilder.WithName(name),
		vmopbuilder.WithNamespace(namespace),
		vmopbuilder.WithType(v1alpha2.VMOPTypeRestore),
		vmopbuilder.WithRestoreSpec(restoreSpec),
		vmopbuilder.WithVirtualMachine(vmName),
	)
}

func (h *VMOPRestoreTestHelper) UpdateState(g Gomega) {
	var err error

	if h.CVI != nil {
		var cvi v1alpha2.ClusterVirtualImage
		err = h.FrameworkEntity.Clients.GenericClient().Get(
			context.Background(),
			types.NamespacedName{
				Name: h.CVI.Name,
			},
			&cvi,
		)
		g.Expect(err).ShouldNot(HaveOccurred())
		if err == nil {
			h.CVI = &cvi
		}
	}

	if h.VI != nil {
		var vi v1alpha2.VirtualImage
		err = h.FrameworkEntity.Clients.GenericClient().Get(
			context.Background(),
			types.NamespacedName{
				Namespace: h.VI.Namespace,
				Name:      h.VI.Name,
			},
			&vi,
		)
		g.Expect(err).ShouldNot(HaveOccurred())
		if err == nil {
			h.VI = &vi
		}
	}

	if h.VDBlank != nil {
		var vdBlank v1alpha2.VirtualDisk
		err = h.FrameworkEntity.Clients.GenericClient().Get(
			context.Background(),
			types.NamespacedName{
				Namespace: h.VDBlank.Namespace,
				Name:      h.VDBlank.Name,
			},
			&vdBlank,
		)
		g.Expect(err).ShouldNot(HaveOccurred())
		if err == nil {
			h.VDBlank = &vdBlank
		}
	}

	if h.VDRoot != nil {
		var vdRoot v1alpha2.VirtualDisk
		err = h.FrameworkEntity.Clients.GenericClient().Get(
			context.Background(),
			types.NamespacedName{
				Namespace: h.VDRoot.Namespace,
				Name:      h.VDRoot.Name,
			},
			&vdRoot,
		)
		g.Expect(err).ShouldNot(HaveOccurred())
		if err == nil {
			h.VDRoot = &vdRoot
		}
	}

	if h.VMBDA != nil {
		var vmbda v1alpha2.VirtualMachineBlockDeviceAttachment
		err = h.FrameworkEntity.Clients.GenericClient().Get(
			context.Background(),
			types.NamespacedName{
				Namespace: h.VMBDA.Namespace,
				Name:      h.VMBDA.Name,
			},
			&vmbda,
		)
		g.Expect(err).ShouldNot(HaveOccurred())
		if err == nil {
			h.VMBDA = &vmbda
		}
	}

	if h.VM != nil {
		var vm v1alpha2.VirtualMachine
		err = h.FrameworkEntity.Clients.GenericClient().Get(
			context.Background(),
			types.NamespacedName{
				Namespace: h.VM.Namespace,
				Name:      h.VM.Name,
			},
			&vm,
		)
		g.Expect(err).ShouldNot(HaveOccurred())
		if err == nil {
			h.VM = &vm
		}
	}

	if h.VMSnapshot != nil {
		var vmSnapshot v1alpha2.VirtualMachineSnapshot
		err = h.FrameworkEntity.Clients.GenericClient().Get(
			context.Background(),
			types.NamespacedName{
				Namespace: h.VMSnapshot.Namespace,
				Name:      h.VMSnapshot.Name,
			},
			&vmSnapshot,
		)
		g.Expect(err).ShouldNot(HaveOccurred())
		if err == nil {
			h.VMSnapshot = &vmSnapshot
		}
	}

	if h.VMOPDryRun != nil {
		var vmopDryRun v1alpha2.VirtualMachineOperation
		err = h.FrameworkEntity.Clients.GenericClient().Get(
			context.Background(),
			types.NamespacedName{
				Namespace: h.VMOPDryRun.Namespace,
				Name:      h.VMOPDryRun.Name,
			},
			&vmopDryRun,
		)
		g.Expect(err).ShouldNot(HaveOccurred())
		if err == nil {
			h.VMOPDryRun = &vmopDryRun
		}
	}

	if h.VMOPStrict != nil {
		var vmopStrict v1alpha2.VirtualMachineOperation
		err = h.FrameworkEntity.Clients.GenericClient().Get(
			context.Background(),
			types.NamespacedName{
				Namespace: h.VMOPStrict.Namespace,
				Name:      h.VMOPStrict.Name,
			},
			&vmopStrict,
		)
		g.Expect(err).ShouldNot(HaveOccurred())
		if err == nil {
			h.VMOPStrict = &vmopStrict
		}
	}

	if h.VMOPBestEffort != nil {
		var vmopBestEffort v1alpha2.VirtualMachineOperation
		err = h.FrameworkEntity.Clients.GenericClient().Get(
			context.Background(),
			types.NamespacedName{
				Namespace: h.VMOPBestEffort.Namespace,
				Name:      h.VMOPBestEffort.Name,
			},
			&vmopBestEffort,
		)
		g.Expect(err).ShouldNot(HaveOccurred())
		if err == nil {
			h.VMOPBestEffort = &vmopBestEffort
		}
	}
}

func (h *VMOPRestoreTestHelper) CheckIfResourcesReady(g Gomega) {
	g.Expect(h.CVI.Status.Phase).Should(Equal(v1alpha2.ImageReady))
	g.Expect(h.VI.Status.Phase).Should(Equal(v1alpha2.ImageReady))
	g.Expect(h.VDBlank.Status.Phase).Should(Equal(v1alpha2.DiskReady))
	g.Expect(h.VDRoot.Status.Phase).Should(Equal(v1alpha2.DiskReady))
	g.Expect(h.VMBDA.Status.Phase).Should(Equal(v1alpha2.BlockDeviceAttachmentPhaseAttached))
	g.Expect(h.VM.Status.Phase).Should(Equal(v1alpha2.MachineRunning))

	agentReady, _ := conditions.GetCondition(vmcondition.TypeAgentReady, h.VM.Status.Conditions)
	g.Expect(agentReady.Status).Should(Equal(metav1.ConditionTrue))
}
