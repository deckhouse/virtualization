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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"

	cvibuilder "github.com/deckhouse/virtualization-controller/pkg/builder/cvi"
	vdbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vd"
	vibuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vi"
	vmbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vm"
	vmbdabuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vmbda"
	vmsnapshotbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vmsnapshot"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/tests/e2e/framework"
	"github.com/deckhouse/virtualization/tests/e2e/ginkgoutil"
)

const ubuntuUrl = "https://cloud-images.ubuntu.com/jammy/current/jammy-server-cloudimg-amd64.img"

var _ = Describe("VirtualMachineRestoreOperation", Serial, ginkgoutil.CommonE2ETestDecorators(), func() {
	frameworkEntity := framework.NewFramework("virtual-machine-restore-operation")
	helper := NewVMOPRestoreTestHelper(frameworkEntity)

	BeforeAll(func() {
		frameworkEntity.Before()
	})

	AfterAll(func() {
		frameworkEntity.After()
	})

	Context("Preparing resources", func() {
		It("Applying resources", func() {
			helper.GenerateAndCreateOriginalResources()
		})
		It("Resorces should be applied", func() {
			Eventually(func(g Gomega) {
				helper.UpdateState(g)
				helper.CheckIfResourcesReady(g)
			}, 180*time.Second, 1*time.Second).Should(Succeed())
		})
	})

	Context("Creating snapshot", func() {
		It("Applying snapshot resource", func() {
			helper.VMSnapshot = helper.GenerateVMSnapshot(
				"vmsnapshot",
				frameworkEntity.Namespace().Name,
				helper.VM.Name,
				true,
				v1alpha2.KeepIPAddressAlways,
			)
			err := frameworkEntity.Clients.GenericClient().Create(context.Background(), helper.VMSnapshot)
			Expect(err).ShouldNot(HaveOccurred())
		})

		It("Snapshot should be ready", func() {
			Eventually(func(g Gomega) {
				helper.UpdateState(g)

				// Expect(helper.VMSnapshot.)
			}, 120*time.Second, 1*time.Second).Should(Succeed())
		})
	})

	Context("kek", func() {
		time.Sleep(30 * time.Second)
	})
})

type VMOPRestoreTestHelper struct {
	FrameworkEntity *framework.Framework
	VM              *v1alpha2.VirtualMachine
	CVI             *v1alpha2.ClusterVirtualImage
	VI              *v1alpha2.VirtualImage
	VDRoot, VDBlank *v1alpha2.VirtualDisk
	VMBDA           *v1alpha2.VirtualMachineBlockDeviceAttachment
	VMSnapshot      *v1alpha2.VirtualMachineSnapshot
}

func NewVMOPRestoreTestHelper(frameworkEntity *framework.Framework) *VMOPRestoreTestHelper {
	return &VMOPRestoreTestHelper{
		FrameworkEntity: frameworkEntity,
	}
}

func (h *VMOPRestoreTestHelper) GenerateAndCreateOriginalResources() {
	GinkgoHelper()
	h.CVI = h.GenerateCVI("ubuntu-cvi", ubuntuUrl)
	h.FrameworkEntity.AddResourceToDelete(h.CVI)
	h.VI = h.GenerateVI("ubuntu-vi", h.FrameworkEntity.Namespace().Name, ubuntuUrl)
	h.VDRoot = h.GenerateVDFromHttp("vd-root", h.FrameworkEntity.Namespace().Name, "10Gi", ubuntuUrl)
	h.VDBlank = h.GenerateVDBlank("vd-blank", h.FrameworkEntity.Namespace().Name, "10Gi")
	h.VM = h.GenerateVM(
		"ubuntu-vm",
		h.FrameworkEntity.Namespace().Name,
		v1alpha2.AlwaysSafeMigrationPolicy,
		1,
		"10%",
		"1Gi",
		v1alpha2.BlockDeviceSpecRef{
			Kind: v1alpha2.ClusterImageDevice,
			Name: h.CVI.Name,
		},
		v1alpha2.BlockDeviceSpecRef{
			Kind: v1alpha2.ImageDevice,
			Name: h.VI.Name,
		},
		v1alpha2.BlockDeviceSpecRef{
			Kind: v1alpha2.DiskDevice,
			Name: h.VDRoot.Name,
		},
	)
	h.VMBDA = h.GenerateVMBDA(
		"vmbda", h.FrameworkEntity.Namespace().Name, h.VM.Name,
		v1alpha2.VMBDAObjectRef{
			Kind: v1alpha2.VMBDAObjectRefKindVirtualDisk,
			Name: h.VDBlank.Name,
		},
	)

	By(fmt.Sprintf("Creating cvi: %s", h.CVI.Name))
	err := h.FrameworkEntity.GenericClient().Create(context.Background(), h.CVI)
	Expect(err).ShouldNot(HaveOccurred())
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
	memoreySize string,
	blockDeviceRefs ...v1alpha2.BlockDeviceSpecRef,
) *v1alpha2.VirtualMachine {
	return vmbuilder.New(
		vmbuilder.WithName(fmt.Sprintf("%s-%s", name, framework.GetCommitHash())),
		vmbuilder.WithNamespace(namespace),
		vmbuilder.WithBlockDeviceRefs(blockDeviceRefs...),
		vmbuilder.WithLiveMigrationPolicy(liveMigrationPolicy),
		vmbuilder.WithCPU(cores, ptr.To(coreFraction)),
		vmbuilder.WithMemory(resource.MustParse(memoreySize)),
	)
}

func (h *VMOPRestoreTestHelper) GenerateVDBlank(name, namespace, size string) *v1alpha2.VirtualDisk {
	return vdbuilder.New(
		vdbuilder.WithName(fmt.Sprintf("%s-%s", name, framework.GetCommitHash())),
		vdbuilder.WithNamespace(namespace),
		vdbuilder.WithSize(ptr.To(resource.MustParse(size))),
	)
}

func (h *VMOPRestoreTestHelper) GenerateVDFromHttp(name, namespace, size, url string) *v1alpha2.VirtualDisk {
	return vdbuilder.New(
		vdbuilder.WithName(fmt.Sprintf("%s-%s", name, framework.GetCommitHash())),
		vdbuilder.WithNamespace(namespace),
		vdbuilder.WithSize(ptr.To(resource.MustParse(size))),
		vdbuilder.WithDataSourceHTTPWithOnlyURL(url),
	)
}

func (h *VMOPRestoreTestHelper) GenerateVI(name, namespace, url string) *v1alpha2.VirtualImage {
	return vibuilder.New(
		vibuilder.WithName(fmt.Sprintf("%s-%s", name, framework.GetCommitHash())),
		vibuilder.WithNamespace(namespace),
		vibuilder.WithDataSourceHTTPWithOnlyURL(url),
		vibuilder.WithStorageType(ptr.To(v1alpha2.StorageContainerRegistry)),
	)
}

func (h *VMOPRestoreTestHelper) GenerateCVI(name, url string) *v1alpha2.ClusterVirtualImage {
	return cvibuilder.New(
		cvibuilder.WithName(fmt.Sprintf("%s-%s", name, framework.GetCommitHash())),
		cvibuilder.WithDataSourceHTTPWithOnlyURL(url),
	)
}

func (h *VMOPRestoreTestHelper) GenerateVMBDA(name, namespace, vmName string, bdRef v1alpha2.VMBDAObjectRef) *v1alpha2.VirtualMachineBlockDeviceAttachment {
	return vmbdabuilder.New(
		vmbdabuilder.WithName(fmt.Sprintf("%s-%s", name, framework.GetCommitHash())),
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
		vmsnapshotbuilder.WithName(fmt.Sprintf("%s-%s", name, framework.GetCommitHash())),
		vmsnapshotbuilder.WithNamespace(namespace),
		vmsnapshotbuilder.WithVm(vmName),
		vmsnapshotbuilder.WithKeepIpAddress(keepIpAddress),
		vmsnapshotbuilder.WithRequiredConsistency(requiredConsistency),
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
}

func (h *VMOPRestoreTestHelper) CheckIfResourcesReady(g Gomega) {
	g.Expect(h.CVI.Status.Phase).Should(Equal(v1alpha2.ImageReady))
	g.Expect(h.VI.Status.Phase).Should(Equal(v1alpha2.ImageReady))
	g.Expect(h.VDBlank.Status.Phase).Should(Equal(v1alpha2.DiskReady))
	g.Expect(h.VDRoot.Status.Phase).Should(Equal(v1alpha2.DiskReady))
	g.Expect(h.VMBDA.Status.Phase).Should(Equal(v1alpha2.BlockDeviceAttachmentPhaseAttached))
	g.Expect(h.VM.Status.Phase).Should(Equal(v1alpha2.MachineRunning))
}
