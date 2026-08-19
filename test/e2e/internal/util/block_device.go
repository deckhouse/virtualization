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

package util

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	virtv1 "kubevirt.io/api/core/v1"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	vdobs "github.com/deckhouse/virtualization/test/e2e/internal/observer/vd"
)

// CreateBlockDeviceFilesystem formats the device backing (bdKind,bdName) with
// fsType, as root without sudo (custom image guests).
func CreateBlockDeviceFilesystem(ctx context.Context, f *framework.Framework, vm *v1alpha2.VirtualMachine, bdKind v1alpha2.BlockDeviceKind, bdName, fsType string) {
	GinkgoHelper()

	devicePath := GuestDeviceBySerial(ctx, f, vm, bdKind, bdName)

	_, err := f.SSHCommand(vm.Name, vm.Namespace, fmt.Sprintf("mkfs.%s %s", fsType, devicePath), framework.WithSSHUser("root"))
	Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("failed to create %s filesystem on block device %s/%s", fsType, bdKind, bdName))
}

// MountBlockDevice mounts the device backing (bdKind,bdName) at mountPoint,
// as root without sudo (custom image guests).
func MountBlockDevice(ctx context.Context, f *framework.Framework, vm *v1alpha2.VirtualMachine, bdKind v1alpha2.BlockDeviceKind, bdName, mountPoint string) {
	GinkgoHelper()

	devicePath := GuestDeviceBySerial(ctx, f, vm, bdKind, bdName)

	_, err := f.SSHCommand(vm.Name, vm.Namespace, fmt.Sprintf("mkdir -p %s && mount %s %s", mountPoint, devicePath, mountPoint), framework.WithSSHUser("root"))
	Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("failed to mount block device %s/%s to %s", bdKind, bdName, mountPoint))
}

// UnmountBlockDevice unmounts mountPoint, as root without sudo.
func UnmountBlockDevice(f *framework.Framework, vm *v1alpha2.VirtualMachine, mountPoint string) {
	GinkgoHelper()

	_, err := f.SSHCommand(vm.Name, vm.Namespace, fmt.Sprintf("umount %s", mountPoint), framework.WithSSHUser("root"))
	Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("failed to unmount %s", mountPoint))
}

// RegisterFstabEntry adds an fstab entry mounting (bdKind,bdName) at /mnt, as
// root without sudo. The filesystem UUID is read with blkid straight from the
// device (no udev in the custom image, so lsblk UUID/SERIAL columns are
// empty); BusyBox init runs "mount -a" at boot, so the entry survives reboots.
func RegisterFstabEntry(ctx context.Context, f *framework.Framework, vm *v1alpha2.VirtualMachine, bdKind v1alpha2.BlockDeviceKind, bdName string) {
	GinkgoHelper()

	devicePath := GuestDeviceBySerial(ctx, f, vm, bdKind, bdName)

	cmd := fmt.Sprintf(`UUID=$(blkid -s UUID -o value %s); echo "UUID=$UUID /mnt ext4 defaults 0 0" >> /etc/fstab`, devicePath)
	_, err := f.SSHCommand(vm.Name, vm.Namespace, cmd, framework.WithSSHUser("root"))
	Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("failed to register fstab entry for block device %s/%s", bdKind, bdName))
}

// GetBlockDeviceHash returns the sha256 of the whole device backing
// (bdKind,bdName), read as root without sudo.
func GetBlockDeviceHash(ctx context.Context, f *framework.Framework, vm *v1alpha2.VirtualMachine, bdKind v1alpha2.BlockDeviceKind, bdName string) string {
	GinkgoHelper()

	devicePath := GuestDeviceBySerial(ctx, f, vm, bdKind, bdName)

	// We use dd to ensure the entire disk is read.
	cmdOut, err := f.SSHCommand(vm.Name, vm.Namespace, fmt.Sprintf("dd if=%s bs=4M 2>/dev/null | sha256sum | cut -d \" \" -f 1", devicePath), framework.WithSSHUser("root"))
	Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("failed to get hash for block device %s/%s", bdKind, bdName))
	return strings.TrimSpace(cmdOut)
}

// getVMIDisk fetches the KubeVirt VMI backing the VM and returns the disk whose
// derived name matches the block device (e.g. "vd-<name>" / "vi-<name>" / "cvi-<name>").
func getVMIDisk(ctx context.Context, vm *v1alpha2.VirtualMachine, bdKind v1alpha2.BlockDeviceKind, bdName string) (virtv1.Disk, bool) {
	unstructuredVMI, err := framework.GetClients().DynamicClient().Resource(schema.GroupVersionResource{
		Group:    "internal.virtualization.deckhouse.io",
		Version:  "v1",
		Resource: "internalvirtualizationvirtualmachineinstances",
	}).Namespace(vm.Namespace).Get(ctx, vm.Name, metav1.GetOptions{})
	Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("failed to get InternalVirtualizationVirtualMachineInstance %s/%s", vm.Namespace, vm.Name))

	var kvvmi virtv1.VirtualMachineInstance
	err = runtime.DefaultUnstructuredConverter.FromUnstructured(unstructuredVMI.Object, &kvvmi)
	Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("failed to convert InternalVirtualizationVirtualMachineInstance %s/%s to kubevirt VMI", vm.Namespace, vm.Name))

	var blockDeviceName string
	switch bdKind {
	case v1alpha2.DiskDevice:
		blockDeviceName = fmt.Sprintf("vd-%s", bdName)
	case v1alpha2.ImageDevice:
		blockDeviceName = fmt.Sprintf("vi-%s", bdName)
	case v1alpha2.ClusterImageDevice:
		blockDeviceName = fmt.Sprintf("cvi-%s", bdName)
	default:
		Fail(fmt.Sprintf("unknown block device kind %q", bdKind))
	}

	for _, disk := range kvvmi.Spec.Domain.Devices.Disks {
		if disk.Name == blockDeviceName {
			return disk, true
		}
	}

	return virtv1.Disk{}, false
}

func GetBlockDeviceSerialNumber(ctx context.Context, vm *v1alpha2.VirtualMachine, bdKind v1alpha2.BlockDeviceKind, bdName string) (string, bool) {
	disk, ok := getVMIDisk(ctx, vm, bdKind, bdName)
	if !ok {
		return "", false
	}
	return disk.Serial, true
}

// GetBlockDeviceBus returns the bus of a block device as recorded on the
// KubeVirt VMI (e.g. "scsi", "sata"), looked up by its derived disk name.
func GetBlockDeviceBus(ctx context.Context, vm *v1alpha2.VirtualMachine, bdKind v1alpha2.BlockDeviceKind, bdName string) (virtv1.DiskBus, bool) {
	disk, ok := getVMIDisk(ctx, vm, bdKind, bdName)
	if !ok {
		return "", false
	}
	switch {
	case disk.Disk != nil:
		return disk.Disk.Bus, true
	case disk.CDRom != nil:
		return disk.CDRom.Bus, true
	}
	return "", false
}

// WriteFile writes value to path in the guest, as root without sudo. The
// value is escaped for a double-quoted POSIX-sh string; the command must not
// contain single quotes (d8 wraps the guest command in them).
func WriteFile(f *framework.Framework, vm *v1alpha2.VirtualMachine, path, value string) {
	GinkgoHelper()

	escaped := strings.NewReplacer("\\", "\\\\", "\"", "\\\"", "$", "\\$", "`", "\\`").Replace(value)
	_, err := f.SSHCommand(vm.Name, vm.Namespace, fmt.Sprintf("echo \"%s\" > %s", escaped, path), framework.WithSSHUser("root"))
	Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("failed to write file %s on vm %s/%s", path, vm.Namespace, vm.Name))
}

func ReadFile(f *framework.Framework, vm *v1alpha2.VirtualMachine, path string) string {
	GinkgoHelper()

	cmdOut, err := f.SSHCommand(vm.Name, vm.Namespace, fmt.Sprintf("cat %s", path), framework.WithSSHUser("root"))
	Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("failed to read file %s on vm %s/%s", path, vm.Namespace, vm.Name))
	return strings.TrimSpace(cmdOut)
}

// GetExpectedDiskPhaseByVolumeBindingMode returns the expected disk phase based on the DefaultStorageClass VolumeBindingMode.
// For Immediate binding mode, disks become Ready immediately.
// For WaitForFirstConsumer binding mode, disks wait until attached to a VM.
func GetExpectedDiskPhaseByVolumeBindingMode() string {
	sc := framework.GetConfig().StorageClass.DefaultStorageClass
	if sc == nil || sc.VolumeBindingMode == nil {
		return string(v1alpha2.DiskReady)
	}
	switch *sc.VolumeBindingMode {
	case storagev1.VolumeBindingImmediate:
		return string(v1alpha2.DiskReady)
	case storagev1.VolumeBindingWaitForFirstConsumer:
		return string(v1alpha2.DiskWaitForFirstConsumer)
	default:
		return string(v1alpha2.DiskReady)
	}
}

// WaitDiskInExpectedPhase waits, via a VirtualDisk Observer, until vd reaches the
// phase expected for the default storage class' volume binding mode (Ready for
// Immediate, WaitForFirstConsumer otherwise).
func WaitDiskInExpectedPhase(ctx context.Context, f *framework.Framework, vd *v1alpha2.VirtualDisk) {
	GinkgoHelper()
	expected := GetExpectedDiskPhaseByVolumeBindingMode()
	obs := vdobs.StartObserver(ctx, f, vd)
	obs.Never(vdobs.BeFailed())
	err := obs.WaitFor(vdobs.BeInPhase(v1alpha2.DiskPhase(expected)), framework.LongTimeout)
	Expect(err).NotTo(HaveOccurred())
}

// GetDiskCount returns the number of block devices attached to a VM.
// Uses lsblk --nodeps --json to get the list of block devices.
func GetDiskCount(f *framework.Framework, vmName, vmNamespace string) (int, error) {
	cmd := "lsblk --nodeps --json"
	result, err := f.SSHCommand(vmName, vmNamespace, cmd)
	if err != nil {
		return 0, fmt.Errorf("failed to execute command: %w: %s", err, result)
	}

	var disks Disks
	err = json.Unmarshal([]byte(result), &disks)
	if err != nil {
		return 0, fmt.Errorf("failed to parse lsblk output: %w", err)
	}

	return len(disks.BlockDevices), nil
}

// Disks represents the JSON output of lsblk --nodeps --json command.
// It contains a list of block devices attached to the VM.
type Disks struct {
	BlockDevices []BlockDevice `json:"blockdevices"`
}

// BlockDevice represents a single block device in the lsblk JSON output.
type BlockDevice struct {
	Name   string `json:"name"`
	Serial string `json:"serial"`
	Size   string `json:"size"`
	Type   string `json:"type"`
}

// guestSerialByDeviceCmd prints one line per SCSI disk as "<devpath> <serial>",
// logging in as root without sudo (the custom image has no cloud user).
//
// The minimal custom image runs no udev, so lsblk's SERIAL column and the
// /dev/disk/by-id symlinks are empty. The serial KubeVirt assigns is still
// readable straight from each disk's SCSI VPD page 0x80 in sysfs: a 4-byte
// header followed by the ASCII serial, hence "tail -c +5".
//
// The command deliberately contains no single quotes: d8 wraps the guest
// command in '...' (see internal/d8), so an embedded single quote would break
// argument parsing and d8 would reject the extra tokens.
const guestSerialByDeviceCmd = `for d in /sys/block/sd* /sys/block/sr*; do [ -e $d ] || continue; echo /dev/$(basename $d) $(tail -c +5 $d/device/vpd_pg80 2>/dev/null); done`

// GuestDeviceBySerial returns the in-guest device path (e.g. /dev/sda) of the
// block device backing (bdKind,bdName), resolved by its serial number. It logs
// in as root without sudo (custom image guests).
func GuestDeviceBySerial(ctx context.Context, f *framework.Framework, vm *v1alpha2.VirtualMachine, bdKind v1alpha2.BlockDeviceKind, bdName string) string {
	GinkgoHelper()
	serial, ok := GetBlockDeviceSerialNumber(ctx, vm, bdKind, bdName)
	Expect(ok).To(BeTrue(), "failed to get block device %s/%s serial number", bdKind, bdName)

	return GuestDeviceBySerialString(f, vm, serial)
}

// GuestDeviceBySerialString returns the in-guest device path (e.g. /dev/sda or
// /dev/sr0) of the block device carrying the given KubeVirt serial. It logs in
// as root without sudo (custom image guests).
func GuestDeviceBySerialString(f *framework.Framework, vm *v1alpha2.VirtualMachine, serial string) string {
	GinkgoHelper()

	out, err := f.SSHCommand(vm.Name, vm.Namespace, guestSerialByDeviceCmd, framework.WithSSHUser("root"))
	Expect(err).NotTo(HaveOccurred())

	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[1] == serial {
			return fields[0]
		}
	}
	Fail(fmt.Sprintf("no block device with serial %s found in guest; device/serial map:\n%s", serial, out))
	return ""
}

// GetBlockDeviceLsblkSizeAsRoot returns the lsblk-reported size (in bytes) of
// the VirtualDisk bdName, logging in as root without sudo.
//
// The custom image has no cloud user and no sudo, and runs no udev, so
// lsblk cannot populate the SERIAL column. The device is instead resolved by
// serial through GuestDeviceBySerial (which reads the SCSI VPD from sysfs), and
// its size is read with "lsblk -b" (fed from sysfs, so it needs no udev either).
func GetBlockDeviceLsblkSizeAsRoot(ctx context.Context, f *framework.Framework, vm *v1alpha2.VirtualMachine, bdName string) resource.Quantity {
	GinkgoHelper()

	dev := GuestDeviceBySerial(ctx, f, vm, v1alpha2.VirtualDiskKind, bdName)

	out, err := f.SSHCommand(vm.Name, vm.Namespace, "lsblk --nodeps -bno SIZE "+dev, framework.WithSSHUser("root"))
	Expect(err).NotTo(HaveOccurred())

	return resource.MustParse(strings.TrimSpace(out))
}

// GetDiskCountAsRoot returns the number of block devices the guest sees,
// logging in as root without sudo (custom image guests: no cloud user,
// no sudo). lsblk is fed from sysfs, so it needs no udev; the custom image
// bakes it in (util-linux), so there is no cloud-init installation to wait for.
func GetDiskCountAsRoot(f *framework.Framework, vmName, vmNamespace string) (int, error) {
	out, err := f.SSHCommand(vmName, vmNamespace, "lsblk --nodeps --json", framework.WithSSHUser("root"))
	if err != nil {
		return 0, fmt.Errorf("failed to execute command: %w: %s", err, out)
	}

	var disks Disks
	err = json.Unmarshal([]byte(out), &disks)
	if err != nil {
		return 0, fmt.Errorf("failed to parse lsblk output: %w", err)
	}

	return len(disks.BlockDevices), nil
}
