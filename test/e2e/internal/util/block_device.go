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
	"errors"
	"fmt"
	"strings"
	"time"

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
)

func GetBlockDevicePath(ctx context.Context, f *framework.Framework, vm *v1alpha2.VirtualMachine, bdKind v1alpha2.BlockDeviceKind, bdName string) string {
	GinkgoHelper()

	serial, ok := GetBlockDeviceSerialNumber(ctx, vm, bdKind, bdName)
	Expect(ok).To(BeTrue(), fmt.Sprintf("failed to get block device %s/%s serial number", bdKind, bdName))

	devicePath, err := GetBlockDeviceBySerial(f, vm, serial)
	Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("failed to get device %s/%s by serial", bdKind, bdName))
	return devicePath
}

func CreateBlockDeviceFilesystem(ctx context.Context, f *framework.Framework, vm *v1alpha2.VirtualMachine, bdKind v1alpha2.BlockDeviceKind, bdName, fsType string) {
	GinkgoHelper()

	serial, ok := GetBlockDeviceSerialNumber(ctx, vm, bdKind, bdName)
	Expect(ok).To(BeTrue(), fmt.Sprintf("failed to get block device %s/%s serial number", bdKind, bdName))

	devicePath, err := GetBlockDeviceBySerial(f, vm, serial)
	Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("failed to get device %s/%s by serial", bdKind, bdName))

	_, err = f.SSHCommand(vm.Name, vm.Namespace, fmt.Sprintf("sudo mkfs.%s %s", fsType, devicePath))
	Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("failed to create %s filesystem on block device %s/%s", fsType, bdKind, bdName))
}

func MountBlockDevice(ctx context.Context, f *framework.Framework, vm *v1alpha2.VirtualMachine, bdKind v1alpha2.BlockDeviceKind, bdName, mountPoint string) {
	GinkgoHelper()

	serial, ok := GetBlockDeviceSerialNumber(ctx, vm, bdKind, bdName)
	Expect(ok).To(BeTrue(), fmt.Sprintf("failed to get block device %s/%s serial number", bdKind, bdName))

	devicePath, err := GetBlockDeviceBySerial(f, vm, serial)
	Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("failed to get device %s/%s by serial", bdKind, bdName))

	_, err = f.SSHCommand(vm.Name, vm.Namespace, fmt.Sprintf("sudo mount %s %s", devicePath, mountPoint))
	Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("failed to mount block device %s/%s to %s", bdKind, bdName, mountPoint))
}

func UnmountBlockDevice(f *framework.Framework, vm *v1alpha2.VirtualMachine, mountPoint string) {
	GinkgoHelper()

	_, err := f.SSHCommand(vm.Name, vm.Namespace, fmt.Sprintf("sudo umount %s", mountPoint))
	Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("failed to unmount %s", mountPoint))
}

func RegisterFstabEntry(ctx context.Context, f *framework.Framework, vm *v1alpha2.VirtualMachine, bdKind v1alpha2.BlockDeviceKind, bdName string) {
	GinkgoHelper()

	serial, ok := GetBlockDeviceSerialNumber(ctx, vm, bdKind, bdName)
	Expect(ok).To(BeTrue(), fmt.Sprintf("failed to get block device %s/%s serial number", bdKind, bdName))

	cmd := fmt.Sprintf(`UUID=$(lsblk -o SERIAL,UUID | grep %s | awk "{print \$2}"); echo "UUID=$UUID /mnt ext4 defaults 0 0" | sudo tee -a /etc/fstab`, serial)
	_, err := f.SSHCommand(vm.Name, vm.Namespace, cmd)
	Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("failed to register fstab entry for block device %s/%s", bdKind, bdName))
}

func GetBlockDeviceHash(ctx context.Context, f *framework.Framework, vm *v1alpha2.VirtualMachine, bdKind v1alpha2.BlockDeviceKind, bdName string) string {
	GinkgoHelper()

	serial, ok := GetBlockDeviceSerialNumber(ctx, vm, bdKind, bdName)
	Expect(ok).To(BeTrue(), fmt.Sprintf("failed to get block device %s/%s serial number", bdKind, bdName))

	devicePath, err := GetBlockDeviceBySerial(f, vm, serial)
	Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("failed to get device %s/%s by serial", bdKind, bdName))

	// We use dd to ensure the entire disk is read.
	cmdOut, err := f.SSHCommand(vm.Name, vm.Namespace, fmt.Sprintf("sudo dd if=%s bs=4M | sha256sum | awk \"{print \\$1}\"", devicePath))
	Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("failed to get hash for block device %s/%s", bdKind, bdName))
	return strings.TrimSpace(cmdOut)
}

func GetBlockDeviceLsblkSize(ctx context.Context, f *framework.Framework, vm *v1alpha2.VirtualMachine, bdKind v1alpha2.BlockDeviceKind, bdName string) resource.Quantity {
	GinkgoHelper()

	var size resource.Quantity
	Eventually(func(g Gomega) {
		quantity, err := tryGetBlockDeviceLsblkSize(ctx, f, vm, bdKind, bdName)
		g.Expect(err).NotTo(HaveOccurred(), "failed to get lsblk size for block device %s/%s", bdKind, bdName)
		size = quantity
	}).WithTimeout(framework.MiddleTimeout).WithPolling(time.Second).Should(Succeed())

	return size
}

func tryGetBlockDeviceLsblkSize(ctx context.Context, f *framework.Framework, vm *v1alpha2.VirtualMachine, bdKind v1alpha2.BlockDeviceKind, bdName string) (resource.Quantity, error) {
	serial, ok := GetBlockDeviceSerialNumber(ctx, vm, bdKind, bdName)
	if !ok {
		return resource.Quantity{}, fmt.Errorf("failed to get block device %s/%s serial number", bdKind, bdName)
	}

	devicePath, err := GetBlockDeviceBySerial(f, vm, serial, framework.WithSSHTimeout(10*time.Second))
	if err != nil {
		return resource.Quantity{}, fmt.Errorf("failed to get device %s/%s by serial: %w", bdKind, bdName, err)
	}

	cmdOut, err := f.SSHCommand(vm.Name, vm.Namespace, fmt.Sprintf("sudo lsblk --json -o SIZE %s", devicePath), framework.WithSSHTimeout(10*time.Second))
	if err != nil {
		return resource.Quantity{}, fmt.Errorf("failed to get lsblk size for block device %s/%s: %w", bdKind, bdName, err)
	}

	var disks Disks
	if err = json.Unmarshal([]byte(cmdOut), &disks); err != nil {
		return resource.Quantity{}, fmt.Errorf("failed to parse lsblk output: %w", err)
	}
	if len(disks.BlockDevices) == 0 {
		return resource.Quantity{}, fmt.Errorf("lsblk output does not contain block devices")
	}

	quantity, err := resource.ParseQuantity(strings.TrimSpace(disks.BlockDevices[0].Size))
	if err != nil {
		return resource.Quantity{}, fmt.Errorf("failed to parse lsblk size: %w", err)
	}

	return quantity, nil
}

func GetBlockDeviceBySerial(f *framework.Framework, vm *v1alpha2.VirtualMachine, serial string, options ...framework.SSHCommandOption) (string, error) {
	cmdOut, err := f.SSHCommand(vm.Name, vm.Namespace, fmt.Sprintf("sudo lsblk --nodeps --noheadings -o PATH,SERIAL | awk \"\\$2 == \\\"%s\\\" {print \\$1, \\$2; exit}\"", serial), options...)
	if err != nil {
		return "", err
	}

	cmdLines := strings.Split(strings.TrimSpace(cmdOut), "\n")
	if len(cmdLines) == 0 {
		return "", errors.New("shell out is empty")
	}

	columns := strings.Fields(cmdLines[0])
	if len(columns) != 2 {
		return "", errors.New("shell out columns mismatch")
	}

	if columns[1] == serial {
		return columns[0], nil
	}

	return "", errors.New("no block device found")
}

func GetBlockDeviceSerialNumber(ctx context.Context, vm *v1alpha2.VirtualMachine, bdKind v1alpha2.BlockDeviceKind, bdName string) (string, bool) {
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
			return disk.Serial, true
		}
	}

	return "", false
}

func WriteFile(f *framework.Framework, vm *v1alpha2.VirtualMachine, path, value string) {
	GinkgoHelper()

	// Escape single quotes in value to prevent command injection.
	escapedValue := strings.ReplaceAll(value, "'", "'\"'\"'")
	_, err := f.SSHCommand(vm.Name, vm.Namespace, fmt.Sprintf("sudo bash -c \"echo '%s' > %s\"", escapedValue, path))
	Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("failed to write file %s on vm %s/%s", path, vm.Namespace, vm.Name))
}

func ReadFile(f *framework.Framework, vm *v1alpha2.VirtualMachine, path string) string {
	GinkgoHelper()

	cmdOut, err := f.SSHCommand(vm.Name, vm.Namespace, fmt.Sprintf("sudo cat %s", path))
	Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("failed to read file %s on vm %s/%s", path, vm.Namespace, vm.Name))
	return strings.TrimSpace(cmdOut)
}

// GetExpectedDiskPhaseByVolumeBindingMode returns the expected disk phase based on the TemplateStorageClass VolumeBindingMode.
// For Immediate binding mode, disks become Ready immediately.
// For WaitForFirstConsumer binding mode, disks wait until attached to a VM.
func GetExpectedDiskPhaseByVolumeBindingMode() string {
	sc := framework.GetConfig().StorageClass.TemplateStorageClass
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
	Name string `json:"name"`
	Size string `json:"size"`
	Type string `json:"type"`
}
