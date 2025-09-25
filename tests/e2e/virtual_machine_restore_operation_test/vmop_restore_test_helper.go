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

package virtualmachinerestoreoperationtest

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"

	cvibuilder "github.com/deckhouse/virtualization-controller/pkg/builder/cvi"
	vdbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vd"
	vibuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vi"
	vmbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vm"
	vmbdabuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vmbda"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
	"github.com/deckhouse/virtualization/tests/e2e/framework"
)

const (
	ubuntuURL          = "https://89d64382-20df-4581-8cc7-80df331f67fa.selstorage.ru/ubuntu/jammy-minimal-cloudimg-amd64.img"
	viURL              = "https://89d64382-20df-4581-8cc7-80df331f67fa.selstorage.ru/test/test.qcow2"
	cviURL             = "https://89d64382-20df-4581-8cc7-80df331f67fa.selstorage.ru/test/test.iso"
	defaultValue       = "value"
	changedValue       = "changed"
	testAnnotationName = "test-annotation"
	testLabelName      = "test-label"
)

type VMOPRestoreTestHelper struct {
	FrameworkEntity           *framework.Framework
	VM                        *v1alpha2.VirtualMachine
	CVI                       *v1alpha2.ClusterVirtualImage
	VI                        *v1alpha2.VirtualImage
	VDRoot, VDBlank           *v1alpha2.VirtualDisk
	VMBDA                     *v1alpha2.VirtualMachineBlockDeviceAttachment
	VMSnapshot                *v1alpha2.VirtualMachineSnapshot
	VMOPDryRun                *v1alpha2.VirtualMachineOperation
	VMOPStrict                *v1alpha2.VirtualMachineOperation
	VMOPBestEffort            *v1alpha2.VirtualMachineOperation
	GeneratedValue            string
	RunningLastTransitionTime time.Time
}

func NewVMOPRestoreTestHelper(frameworkEntity *framework.Framework) *VMOPRestoreTestHelper {
	return &VMOPRestoreTestHelper{
		FrameworkEntity: frameworkEntity,
	}
}

func (h *VMOPRestoreTestHelper) GenerateAndCreateOriginalResources() {
	GinkgoHelper()
	h.CVI = cvibuilder.New(
		cvibuilder.WithGenerateName("ubuntu-cvi-"),
		cvibuilder.WithDataSourceHTTPWithOnlyURL(cviURL),
	)

	// for getting real cvi name
	err := h.FrameworkEntity.GenericClient().Create(context.Background(), h.CVI)
	By(fmt.Sprintf("Created cvi: %s", h.CVI.Name))
	Expect(err).ShouldNot(HaveOccurred())

	h.FrameworkEntity.AddResourceToDelete(h.CVI)
	h.VI = vibuilder.New(
		vibuilder.WithName("ubuntu-vi"),
		vibuilder.WithNamespace(h.FrameworkEntity.Namespace().Name),
		vibuilder.WithDataSourceHTTPWithOnlyURL(viURL),
		vibuilder.WithStorageType(ptr.To(v1alpha2.StoragePersistentVolumeClaim)),
	)
	h.VDRoot = vdbuilder.New(
		vdbuilder.WithName("vd-root"),
		vdbuilder.WithNamespace(h.FrameworkEntity.Namespace().Name),
		vdbuilder.WithSize(ptr.To(resource.MustParse("10Gi"))),
		vdbuilder.WithDataSourceHTTPWithOnlyURL(ubuntuURL),
	)
	h.VDBlank = vdbuilder.New(
		vdbuilder.WithName("vd-blank"),
		vdbuilder.WithNamespace(h.FrameworkEntity.Namespace().Name),
		vdbuilder.WithSize(ptr.To(resource.MustParse("51Mi"))),
	)
	h.VM = vmbuilder.New(
		vmbuilder.WithAnnotation(h.GetTestAnnotationName(), h.GetDefaultValue()),
		vmbuilder.WithLabel(h.GetTestLabelName(), h.GetDefaultValue()),
		vmbuilder.WithName("ubuntu-vm"),
		vmbuilder.WithNamespace(h.FrameworkEntity.Namespace().Name),
		vmbuilder.WithLiveMigrationPolicy(v1alpha2.AlwaysSafeMigrationPolicy),
		vmbuilder.WithCPU(1, ptr.To("10%")),
		vmbuilder.WithMemory(resource.MustParse("1Gi")),
		vmbuilder.WithProvisioning(
			&v1alpha2.Provisioning{
				Type:     v1alpha2.ProvisioningTypeUserData,
				UserData: cloudInit,
			},
		),
		vmbuilder.WithBlockDeviceRefs(
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
		),
	)
	h.VMBDA = vmbdabuilder.New(
		vmbdabuilder.WithName("vmbda"),
		vmbdabuilder.WithNamespace(h.FrameworkEntity.Namespace().Name),
		vmbdabuilder.WithVMName(h.VM.Name),
		vmbdabuilder.WithBlockDeviceRef(v1alpha2.VMBDAObjectRef{
			Kind: v1alpha2.VMBDAObjectRefKindVirtualDisk,
			Name: h.VDBlank.Name,
		}),
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

func (h *VMOPRestoreTestHelper) UpdateState() {
	GinkgoHelper()

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
		if err == nil {
			h.CVI = &cvi
		} else {
			Expect(k8serrors.IsNotFound(err)).Should(BeTrue())
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
		if err == nil {
			h.VI = &vi
		} else {
			Expect(k8serrors.IsNotFound(err)).Should(BeTrue())
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
		if err == nil {
			h.VDBlank = &vdBlank
		} else {
			Expect(k8serrors.IsNotFound(err)).Should(BeTrue())
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
		if err == nil {
			h.VDRoot = &vdRoot
		} else {
			Expect(k8serrors.IsNotFound(err)).Should(BeTrue())
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
		if err == nil {
			h.VMBDA = &vmbda
		} else {
			Expect(k8serrors.IsNotFound(err)).Should(BeTrue())
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
		if err == nil {
			h.VM = &vm
		} else {
			Expect(k8serrors.IsNotFound(err)).Should(BeTrue())
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
		if err == nil {
			h.VMSnapshot = &vmSnapshot
		} else {
			Expect(k8serrors.IsNotFound(err)).Should(BeTrue())
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
		if err == nil {
			h.VMOPDryRun = &vmopDryRun
		} else {
			Expect(k8serrors.IsNotFound(err)).Should(BeTrue())
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
		if err == nil {
			h.VMOPStrict = &vmopStrict
		} else {
			Expect(k8serrors.IsNotFound(err)).Should(BeTrue())
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
		if err == nil {
			h.VMOPBestEffort = &vmopBestEffort
		} else {
			Expect(k8serrors.IsNotFound(err)).Should(BeTrue())
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

func (h *VMOPRestoreTestHelper) GetDefaultValue() string {
	return defaultValue
}

func (h *VMOPRestoreTestHelper) GetChangedValue() string {
	return changedValue
}

func (h *VMOPRestoreTestHelper) GetTestAnnotationName() string {
	return testAnnotationName
}

func (h *VMOPRestoreTestHelper) GetTestLabelName() string {
	return testLabelName
}

func (h *VMOPRestoreTestHelper) CreateFsAndSetValueOnDiskShell(value string) string {
	return fmt.Sprintf("umount /mnt &>/dev/null || DEV=/dev/$(sudo lsblk | grep disk | tail -n 1 | awk \"{print \\$1}\") && sudo mkfs.ext4 $DEV && sudo mount $DEV /mnt && sudo bash -c \"echo %s > /mnt/value\"", value)
}

func (h *VMOPRestoreTestHelper) ChangeValueOnDiskShell(value string) string {
	return fmt.Sprintf("sudo bash -c \"echo %s > /mnt/value\"", value)
}

func (h *VMOPRestoreTestHelper) MountAndGetDiskFileContentShell() string {
	return "umount /mnt &>/dev/null || DEV=/dev/$(sudo lsblk | grep disk | tail -n 1 | awk \"{print \\$1}\") && sudo mount $DEV /mnt && cat /mnt/value"
}
