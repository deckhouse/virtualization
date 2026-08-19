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

package blockdevice

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	virtv1 "kubevirt.io/api/core/v1"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"

	vdbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vd"
	vibuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vi"
	vmbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vm"
	"github.com/deckhouse/virtualization-controller/pkg/builder/vmbda"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/test/e2e/eventually"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	"github.com/deckhouse/virtualization/test/e2e/internal/label"
	"github.com/deckhouse/virtualization/test/e2e/internal/object"
	"github.com/deckhouse/virtualization/test/e2e/internal/observer"
	vmobs "github.com/deckhouse/virtualization/test/e2e/internal/observer/vm"
	"github.com/deckhouse/virtualization/test/e2e/internal/precheck"
	"github.com/deckhouse/virtualization/test/e2e/internal/util"
)

const (
	hotplugImagesCount  = 4
	hotplugPollInterval = 5 * time.Second
)

var _ = Describe("VirtualMachineImageHotplug", Label(label.SIGStorage, precheck.NoPrecheck), func() {
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
			object.PrecreatedCVICustomBIOS,
			vdbuilder.WithSize(ptr.To(resource.MustParse(vdCreationImageSize))),
		)

		viHotplugQCOW := object.NewVI(
			vibuilder.WithName("vi-hotplug-qcow"),
			vibuilder.WithNamespace(f.Namespace().Name),
			vibuilder.WithDataSourceObjectRef(v1alpha2.VirtualImageObjectRefKindClusterVirtualImage, object.PrecreatedCVICustomBIOS),
			vibuilder.WithStorage(v1alpha2.StorageContainerRegistry),
		)

		viHotplugPVC := object.NewVI(
			vibuilder.WithName("vi-hotplug-pvc"),
			vibuilder.WithNamespace(f.Namespace().Name),
			vibuilder.WithDataSourceObjectRef(v1alpha2.VirtualImageObjectRefKindClusterVirtualImage, object.PrecreatedCVICustomBIOS),
			vibuilder.WithStorage(v1alpha2.StoragePersistentVolumeClaim),
		)

		vm := vmbuilder.New(
			vmbuilder.WithName("vm"),
			vmbuilder.WithNamespace(f.Namespace().Name),
			vmbuilder.WithCPU(1, ptr.To(object.CustomImageVMCoreFraction)),
			vmbuilder.WithMemory(resource.MustParse(object.CustomImageVMMemory)),
			vmbuilder.WithLiveMigrationPolicy(v1alpha2.AlwaysSafeMigrationPolicy),
			// The custom image has no cloud-init; the guest agent and lsblk
			// are baked in.
			vmbuilder.WithBlockDeviceRefs(v1alpha2.BlockDeviceSpecRef{
				Kind: v1alpha2.DiskDevice,
				Name: vdRoot.Name,
			}),
		)

		err := f.CreateWithDeferredDeletion(ctx, vdRoot, viHotplugQCOW, viHotplugPVC)
		Expect(err).NotTo(HaveOccurred())

		util.UntilObjectPhase(ctx, string(v1alpha2.ImageReady), framework.LongTimeout, viHotplugQCOW, viHotplugPVC)

		// On node-local storage (e.g. LINSTOR with one replica) the PVC-backed
		// VirtualImage is provisioned on an arbitrary node and its PV is usable
		// only there: hotplugging it into a VM running on another node is
		// impossible and the VMBDA webhook rejects the attachment. Pin the VM to
		// the node the image's PV allows. TODO: remove the pinning if the storage
		// layer ever allows attaching such volumes across nodes; this is a storage
		// topology limitation, not a controller bug.
		if node := nodeNameForImagePV(ctx, f, viHotplugPVC.Name); node != "" {
			vmbuilder.ApplyOptions(vm, []vmbuilder.Option{vmbuilder.WithNodeSelector(map[string]string{corev1.LabelHostname: node})})
		}

		err = f.CreateWithDeferredDeletion(ctx, vm)
		Expect(err).NotTo(HaveOccurred())

		util.UntilObjectPhase(ctx, string(v1alpha2.MachineRunning), framework.LongTimeout, vm)
		eventually.SSHReadyAsRoot(f, vm, framework.MiddleTimeout)

		By("Getting initial block devices count")
		initialDiskCount, err := util.GetDiskCountAsRoot(f, vm.Name, vm.Namespace)
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
				vmbda.WithName("attach-vi-hotplug-pvc"),
				vmbda.WithNamespace(f.Namespace().Name),
				vmbda.WithVirtualMachineName(vm.Name),
				vmbda.WithBlockDeviceRef(v1alpha2.VMBDAObjectRefKindVirtualImage, viHotplugPVC.Name),
			),
			vmbda.New(
				vmbda.WithName("attach-cvi-hotplug-bios"),
				vmbda.WithNamespace(f.Namespace().Name),
				vmbda.WithVirtualMachineName(vm.Name),
				vmbda.WithBlockDeviceRef(v1alpha2.VMBDAObjectRefKindClusterVirtualImage, object.PrecreatedCVICustomBIOS),
			),
			vmbda.New(
				vmbda.WithName("attach-cvi-hotplug-iso"),
				vmbda.WithNamespace(f.Namespace().Name),
				vmbda.WithVirtualMachineName(vm.Name),
				vmbda.WithBlockDeviceRef(v1alpha2.VMBDAObjectRefKindClusterVirtualImage, object.PrecreatedCVICustomISO),
			),
		}

		err = f.CreateWithDeferredDeletion(ctx, util.ToObjects(vmbdas)...)
		Expect(err).NotTo(HaveOccurred())

		By("Waiting for VMBDAs to become attached")
		util.UntilObjectPhase(ctx, string(v1alpha2.BlockDeviceAttachmentPhaseAttached), framework.LongTimeout, util.ToObjects(vmbdas)...)
		waitBlockDeviceRefsAttached(ctx, f, vm, hotplugImagesCount)

		By("Verifying disk count increased inside guest OS")
		eventually.UntilDiskCountAsRoot(f, vm.Name, vm.Namespace,
			Equal(initialDiskCount+hotplugImagesCount), framework.LongTimeout,
			eventually.WithPolling(hotplugPollInterval),
			eventually.WithExplanation("expected guest disk count to increase by %d after image hotplug", hotplugImagesCount))

		By("Checking that exactly one hotplugged ISO is attached as CD-ROM")
		vmi, err := util.GetInternalVirtualMachineInstance(ctx, vm)
		Expect(err).NotTo(HaveOccurred())
		Expect(vmi).NotTo(BeNil())
		isoDisk := findHotplugISO(vmi)
		// The controller deliberately assigns no serial to CD-ROM attachments,
		// so the device cannot be resolved by serial; the ISO is the VM's only
		// CD-ROM, so assert the guest exposes exactly one "rom" device instead.
		romCount, cdErr := guestCdRomCount(f, vm)
		Expect(cdErr).NotTo(HaveOccurred())
		Expect(romCount).To(Equal(1), "expected %q to be the single CD-ROM block device in the guest", isoDisk.Name)

		By("Checking all hotplugged images are mounted as read-only devices")
		hotplugged := getHotpluggedImageDisks(vmi)
		Expect(hotplugged).To(HaveLen(hotplugImagesCount), "expected %d hotplug image disks", hotplugImagesCount)

		for _, disk := range hotplugged {
			readOnly, roErr := isBlockDeviceReadOnly(f, vm, disk)
			Expect(roErr).NotTo(HaveOccurred(), "failed to validate read-only mode for %q", disk.Name)
			Expect(readOnly).To(BeTrue(), "expected disk %q to be mounted read-only", disk.Name)
		}

		By("Detaching hotplugged images and waiting for baseline disk count")
		err = f.Delete(ctx, util.ToObjects(vmbdas)...)
		Expect(err).NotTo(HaveOccurred())

		eventually.UntilDiskCountAsRoot(f, vm.Name, vm.Namespace,
			Equal(initialDiskCount), framework.LongTimeout,
			eventually.WithPolling(hotplugPollInterval),
			eventually.WithExplanation("expected disk count to return to initial value after detaching hotplugged images"))
	})
})

// nodeNameForImagePV returns the node a PVC-backed VirtualImage's
// PersistentVolume is pinned to, or "" when the PV has no node constraint
// (replicated/shared storage). Node-local provisioners record the node in the
// PV's required node-affinity terms under a provisioner-specific hostname key
// (kubernetes.io/hostname, linbit.com/hostname, ...), so the node is resolved
// by matching the terms' values against the cluster's node names.
func nodeNameForImagePV(ctx context.Context, f *framework.Framework, viName string) string {
	GinkgoHelper()

	vi, err := f.VirtClient().VirtualImages(f.Namespace().Name).Get(ctx, viName, metav1.GetOptions{})
	Expect(err).NotTo(HaveOccurred())
	if vi.Status.Target.PersistentVolumeClaim == "" {
		return ""
	}

	pvc, err := f.KubeClient().CoreV1().PersistentVolumeClaims(f.Namespace().Name).Get(ctx, vi.Status.Target.PersistentVolumeClaim, metav1.GetOptions{})
	Expect(err).NotTo(HaveOccurred())
	if pvc.Spec.VolumeName == "" {
		return ""
	}

	pv, err := f.KubeClient().CoreV1().PersistentVolumes().Get(ctx, pvc.Spec.VolumeName, metav1.GetOptions{})
	Expect(err).NotTo(HaveOccurred())
	if pv.Spec.NodeAffinity == nil || pv.Spec.NodeAffinity.Required == nil {
		return ""
	}

	nodes, err := f.KubeClient().CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	Expect(err).NotTo(HaveOccurred())
	nodeNames := make(map[string]struct{}, len(nodes.Items))
	for _, node := range nodes.Items {
		nodeNames[node.Name] = struct{}{}
	}

	for _, term := range pv.Spec.NodeAffinity.Required.NodeSelectorTerms {
		for _, expr := range term.MatchExpressions {
			if expr.Operator != corev1.NodeSelectorOpIn {
				continue
			}
			for _, value := range expr.Values {
				if _, ok := nodeNames[value]; ok {
					return value
				}
			}
		}
	}
	return ""
}

// waitBlockDeviceRefsAttached observes the VirtualMachine through a watch
// until its status reports the expected number of attached block devices
// (the hotplugged images plus the root disk).
func waitBlockDeviceRefsAttached(ctx context.Context, f *framework.Framework, vm *v1alpha2.VirtualMachine, expectedAttached int) {
	GinkgoHelper()

	obs, err := observer.New[*v1alpha2.VirtualMachine](
		ctx,
		f.VirtClient().VirtualMachines(vm.Namespace),
		vm.Name, vm.Namespace,
	)
	Expect(err).NotTo(HaveOccurred(), "failed to start observer for VirtualMachine %s/%s", vm.Namespace, vm.Name)
	defer obs.Stop()

	err = obs.WaitFor(vmobs.HaveAttachedBlockDeviceCount(expectedAttached+1), framework.LongTimeout)
	Expect(err).NotTo(HaveOccurred(),
		"expected %d attached block devices: %d hotplug images plus one root disk",
		expectedAttached+1, expectedAttached,
	)

	// The former polling implementation refreshed the caller's object on every
	// attempt; keep that contract.
	Expect(f.GenericClient().Get(ctx, crclient.ObjectKeyFromObject(vm), vm)).To(Succeed())
}

func findHotplugISO(vmi *virtv1.VirtualMachineInstance) virtv1.Disk {
	GinkgoHelper()

	isoCount := 0
	var isoDisk virtv1.Disk

	for _, disk := range vmi.Spec.Domain.Devices.Disks {
		if disk.CDRom == nil {
			continue
		}
		if !strings.HasPrefix(disk.Name, "vi-") && !strings.HasPrefix(disk.Name, "cvi-") {
			continue
		}
		isoCount++
		isoDisk = disk
	}

	Expect(isoCount).To(Equal(1), "expected exactly one hotplugged ISO disk in VMI spec")
	return isoDisk
}

func getHotpluggedImageDisks(vmi *virtv1.VirtualMachineInstance) []virtv1.Disk {
	GinkgoHelper()

	disks := make([]virtv1.Disk, 0, hotplugImagesCount)
	for _, disk := range vmi.Spec.Domain.Devices.Disks {
		if !strings.HasPrefix(disk.Name, "vi-") && !strings.HasPrefix(disk.Name, "cvi-") {
			continue
		}
		disks = append(disks, disk)
	}

	return disks
}

// guestCdRomCount returns how many block devices the guest exposes as
// CD-ROMs (lsblk type "rom"). CD-ROM attachments carry no KubeVirt serial (the
// controller deliberately omits it), so unlike disks they cannot be resolved
// through the SCSI VPD serial in sysfs.
func guestCdRomCount(f *framework.Framework, vm *v1alpha2.VirtualMachine) (int, error) {
	output, err := f.SSHCommand(vm.Name, vm.Namespace, "lsblk --json --nodeps --output name,type", framework.WithSSHUser("root"))
	if err != nil {
		return 0, err
	}

	var disks util.Disks
	if err = json.Unmarshal([]byte(output), &disks); err != nil {
		return 0, err
	}

	count := 0
	for _, device := range disks.BlockDevices {
		if device.Type == "rom" {
			count++
		}
	}
	return count, nil
}

// isBlockDeviceReadOnly reports whether the guest kernel exposes the block
// device as read-only. It reads the device read-only flag directly (lsblk RO)
// instead of trying to mount it: a genuinely read-only device with a
// filesystem that needs journal recovery cannot be mounted even with -o ro,
// which would produce a false negative for exactly the devices under test.
// The device is resolved by its KubeVirt serial via sysfs (no udev by-id
// symlinks in the custom image); CD-ROMs are read-only by nature.
func isBlockDeviceReadOnly(f *framework.Framework, vm *v1alpha2.VirtualMachine, disk virtv1.Disk) (bool, error) {
	if disk.CDRom != nil {
		return true, nil
	}

	devicePath := util.GuestDeviceBySerialString(f, vm, disk.Serial)
	cmd := fmt.Sprintf("lsblk --nodeps --noheadings --output RO %q", devicePath)
	out, err := f.SSHCommand(vm.Name, vm.Namespace, cmd, framework.WithSSHUser("root"))
	if err != nil {
		return false, fmt.Errorf("failed to read the read-only flag for %q: %w", devicePath, err)
	}

	switch strings.TrimSpace(out) {
	case "1":
		return true, nil
	case "0":
		return false, nil
	default:
		return false, fmt.Errorf("unexpected lsblk RO output for %q: %q", devicePath, out)
	}
}
