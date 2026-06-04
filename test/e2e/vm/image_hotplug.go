/*
Copyright 2026 Flant JSC

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

package vm

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/utils/ptr"
	virtv1 "kubevirt.io/api/core/v1"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"

	vibuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vi"
	vmbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vm"
	"github.com/deckhouse/virtualization-controller/pkg/builder/vmbda"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	"github.com/deckhouse/virtualization/test/e2e/internal/object"
	"github.com/deckhouse/virtualization/test/e2e/internal/precheck"
	"github.com/deckhouse/virtualization/test/e2e/internal/util"
)

const (
	diskByIDPrefix      = "scsi-0QEMU_QEMU_HARDDISK"
	cdRomByIDPrefix     = "scsi-0QEMU_QEMU_CD-ROM_drive-ua"
	hotplugImagesCount  = 4
	hotplugPollInterval = 5 * time.Second
)

var _ = Describe("VirtualMachineImageHotplug", Label(precheck.NoPrecheck), func() {
	var (
		f   *framework.Framework
		ctx context.Context
	)

	BeforeEach(func() {
		f = framework.NewFramework("vm-image-hotplug")
		ctx = context.Background()
		DeferCleanup(f.After)
		f.Before()
	})

	It("should hotplug images as read-only devices and detach them back", func() {
		By("Environment preparation")
		vdRoot := object.NewVDFromCVI(
			"vd-root",
			f.Namespace().Name,
			object.PrecreatedCVIAlpineBIOSPerf,
		)

		viHotplugQCOW := vibuilder.New(
			vibuilder.WithName("vi-hotplug-qcow"),
			vibuilder.WithNamespace(f.Namespace().Name),
			vibuilder.WithDataSourceObjectRef(v1alpha2.VirtualImageObjectRefKindClusterVirtualImage, object.PrecreatedCVITestDataQCOW),
			vibuilder.WithStorage(v1alpha2.StorageContainerRegistry),
		)

		viHotplugBIOS := vibuilder.New(
			vibuilder.WithName("vi-hotplug-bios"),
			vibuilder.WithNamespace(f.Namespace().Name),
			vibuilder.WithDataSourceObjectRef(v1alpha2.VirtualImageObjectRefKindClusterVirtualImage, object.PrecreatedCVIAlpineBIOSPerf),
			vibuilder.WithStorage(v1alpha2.StorageContainerRegistry),
		)

		vm := vmbuilder.New(
			vmbuilder.WithName("vm"),
			vmbuilder.WithNamespace(f.Namespace().Name),
			vmbuilder.WithCPU(1, ptr.To("100%")),
			vmbuilder.WithMemory(resource.MustParse("256Mi")),
			vmbuilder.WithLiveMigrationPolicy(v1alpha2.AlwaysSafeMigrationPolicy),
			vmbuilder.WithProvisioningUserData(object.AlpineCloudInit),
			vmbuilder.WithBlockDeviceRefs(v1alpha2.BlockDeviceSpecRef{
				Kind: v1alpha2.DiskDevice,
				Name: vdRoot.Name,
			}),
		)

		err := f.CreateWithDeferredDeletion(ctx, vdRoot, viHotplugQCOW, viHotplugBIOS, vm)
		Expect(err).NotTo(HaveOccurred())

		util.UntilObjectPhase(ctx, string(v1alpha2.ImageReady), framework.LongTimeout, viHotplugQCOW, viHotplugBIOS)
		util.UntilObjectPhase(ctx, string(v1alpha2.MachineRunning), framework.LongTimeout, vm)
		util.UntilSSHReady(f, vm, framework.MiddleTimeout)
		util.UntilGuestCommandsReady(f, vm, []string{"lsblk"}, framework.ShortTimeout)

		By("Calculating initial block devices count")
		initialDiskCnt, err := util.GetDiskCount(f, vm.Name, vm.Namespace)
		Expect(err).NotTo(HaveOccurred())

		By("Attaching VirtualImages and ClusterVirtualImages via VMBDA resources")
		vmbdas := []*v1alpha2.VirtualMachineBlockDeviceAttachment{
			vmbda.New(
				vmbda.WithName("attach-vi-hotplug-qcow"),
				vmbda.WithNamespace(f.Namespace().Name),
				vmbda.WithVirtualMachineName(vm.Name),
				vmbda.WithBlockDeviceRef(v1alpha2.VMBDAObjectRefKindVirtualImage, viHotplugQCOW.Name),
			),
			vmbda.New(
				vmbda.WithName("attach-vi-hotplug-bios"),
				vmbda.WithNamespace(f.Namespace().Name),
				vmbda.WithVirtualMachineName(vm.Name),
				vmbda.WithBlockDeviceRef(v1alpha2.VMBDAObjectRefKindVirtualImage, viHotplugBIOS.Name),
			),
			vmbda.New(
				vmbda.WithName("attach-cvi-hotplug-bios"),
				vmbda.WithNamespace(f.Namespace().Name),
				vmbda.WithVirtualMachineName(vm.Name),
				vmbda.WithBlockDeviceRef(v1alpha2.VMBDAObjectRefKindClusterVirtualImage, object.PrecreatedCVIAlpineBIOSPerf),
			),
			vmbda.New(
				vmbda.WithName("attach-cvi-hotplug-iso"),
				vmbda.WithNamespace(f.Namespace().Name),
				vmbda.WithVirtualMachineName(vm.Name),
				vmbda.WithBlockDeviceRef(v1alpha2.VMBDAObjectRefKindClusterVirtualImage, object.PrecreatedCVIUbuntuISO),
			),
		}

		err = f.CreateWithDeferredDeletion(ctx, util.ToObjects(vmbdas)...)
		Expect(err).NotTo(HaveOccurred())

		By("Waiting for VMBDAs to become attached")
		util.UntilObjectPhase(ctx, string(v1alpha2.BlockDeviceAttachmentPhaseAttached), framework.LongTimeout, util.ToObjects(vmbdas)...)
		waitBlockDeviceRefsAttached(ctx, f, vm, hotplugImagesCount)

		By("Verifying disk count increased inside guest OS")
		Eventually(func(g Gomega) {
			count, diskErr := util.GetDiskCount(f, vm.Name, vm.Namespace)
			g.Expect(diskErr).NotTo(HaveOccurred())
			g.Expect(count).To(Equal(initialDiskCnt + hotplugImagesCount))
		}).WithTimeout(framework.LongTimeout).WithPolling(hotplugPollInterval).Should(Succeed())

		By("Checking that exactly one hotplugged ISO is attached as CD-ROM")
		vmi, err := util.GetInternalVirtualMachineInstance(ctx, vm)
		Expect(err).NotTo(HaveOccurred())
		Expect(vmi).NotTo(BeNil())
		isoDiskName := findHotplugISO(vmi)
		isCDRom, cdErr := isBlockDeviceCdRom(f, vm, isoDiskName)
		Expect(cdErr).NotTo(HaveOccurred())
		Expect(isCDRom).To(BeTrue(), "expected %q to be a CD-ROM block device", isoDiskName)

		By("Checking all hotplugged images are mounted as read-only devices")
		hotplugged := getHotpluggedImageDiskIDs(vmi)
		Expect(hotplugged).To(HaveLen(hotplugImagesCount), "expected %d hotplug image disks", hotplugImagesCount)

		for diskName, diskByID := range hotplugged {
			readOnly, roErr := isBlockDeviceReadOnly(f, vm, diskByID)
			Expect(roErr).NotTo(HaveOccurred(), "failed to validate read-only mode for %q", diskName)
			Expect(readOnly).To(BeTrue(), "expected disk %q to be mounted read-only", diskName)
		}

		By("Detaching hotplugged images and waiting for baseline disk count")
		err = f.Delete(ctx, util.ToObjects(vmbdas)...)
		Expect(err).NotTo(HaveOccurred())

		Eventually(func(g Gomega) {
			count, diskErr := util.GetDiskCount(f, vm.Name, vm.Namespace)
			g.Expect(diskErr).NotTo(HaveOccurred())
			g.Expect(count).To(Equal(initialDiskCnt))
		}).WithTimeout(framework.LongTimeout).WithPolling(hotplugPollInterval).Should(Succeed())
	})
})

func waitBlockDeviceRefsAttached(ctx context.Context, f *framework.Framework, vm *v1alpha2.VirtualMachine, expectedAttached int) {
	GinkgoHelper()

	Eventually(func(g Gomega) {
		err := f.GenericClient().Get(ctx, crclient.ObjectKeyFromObject(vm), vm)
		g.Expect(err).NotTo(HaveOccurred())

		attached := 0
		for _, ref := range vm.Status.BlockDeviceRefs {
			if ref.Attached {
				attached++
			}
		}
		g.Expect(attached).To(BeNumerically(">=", expectedAttached+1))
	}).WithTimeout(framework.LongTimeout).WithPolling(hotplugPollInterval).Should(Succeed())
}

func findHotplugISO(vmi *virtv1.VirtualMachineInstance) string {
	GinkgoHelper()

	isoCount := 0
	isoName := ""

	for _, disk := range vmi.Spec.Domain.Devices.Disks {
		if disk.CDRom == nil {
			continue
		}
		if !strings.HasPrefix(disk.Name, "vi-") && !strings.HasPrefix(disk.Name, "cvi-") {
			continue
		}
		isoCount++
		isoName = disk.Name
	}

	Expect(isoCount).To(Equal(1), "expected exactly one hotplugged ISO disk in VMI spec")
	return isoName
}

func getHotpluggedImageDiskIDs(vmi *virtv1.VirtualMachineInstance) map[string]string {
	GinkgoHelper()

	diskIDs := make(map[string]string, hotplugImagesCount)
	for _, disk := range vmi.Spec.Domain.Devices.Disks {
		if !strings.HasPrefix(disk.Name, "vi-") && !strings.HasPrefix(disk.Name, "cvi-") {
			continue
		}

		if disk.CDRom != nil {
			diskIDs[disk.Name] = fmt.Sprintf("%s-%s", cdRomByIDPrefix, disk.Name)
			continue
		}

		diskIDs[disk.Name] = fmt.Sprintf("%s_%s", diskByIDPrefix, disk.Serial)
	}

	return diskIDs
}

func isBlockDeviceCdRom(f *framework.Framework, vm *v1alpha2.VirtualMachine, blockDeviceName string) (bool, error) {
	bdByIDPath := fmt.Sprintf("/dev/disk/by-id/%s-%s", cdRomByIDPrefix, blockDeviceName)
	cmd := fmt.Sprintf("lsblk --json --nodeps --output name,type %s", bdByIDPath)

	output, err := f.SSHCommand(vm.Name, vm.Namespace, cmd)
	if err != nil {
		return false, err
	}

	var disks util.Disks
	if err = json.Unmarshal([]byte(output), &disks); err != nil {
		return false, err
	}
	if len(disks.BlockDevices) != 1 {
		return false, fmt.Errorf("expected a single block device for path %q", bdByIDPath)
	}

	return disks.BlockDevices[0].Type == "rom", nil
}

func isBlockDeviceReadOnly(f *framework.Framework, vm *v1alpha2.VirtualMachine, blockDeviceByID string) (bool, error) {
	devicePath := fmt.Sprintf("/dev/disk/by-id/%s", blockDeviceByID)
	cmd := fmt.Sprintf("lsblk --json --nodeps --output ro,type %s", devicePath)

	optionsOut, err := f.SSHCommand(vm.Name, vm.Namespace, cmd)
	if err != nil {
		return false, err
	}

	var disks lsblkReadOnlyOutput
	if err = json.Unmarshal([]byte(optionsOut), &disks); err != nil {
		return false, err
	}
	if len(disks.BlockDevices) != 1 {
		return false, fmt.Errorf("expected a single block device for path %q", devicePath)
	}

	bd := disks.BlockDevices[0]
	return bd.ReadOnly || bd.Type == "rom", nil
}

type lsblkReadOnlyOutput struct {
	BlockDevices []struct {
		ReadOnly bool   `json:"ro"`
		Type     string `json:"type"`
	} `json:"blockdevices"`
}
