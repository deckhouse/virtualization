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
	"errors"
	"fmt"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	virtv1 "kubevirt.io/api/core/v1"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
)

func GetBlockDevicePath(f *framework.Framework, vm *v1alpha2.VirtualMachine, bdKind v1alpha2.BlockDeviceKind, bdName string) string {
	GinkgoHelper()

	serial, ok := GetBlockDeviceSerialNumber(vm, bdKind, bdName)
	Expect(ok).To(BeTrue(), "failed to get block device serial number")

	devicePath, err := GetBlockDeviceBySerial(f, vm, serial)
	Expect(err).NotTo(HaveOccurred(), fmt.Errorf("failed to get device by serial: %w", err))
	return devicePath
}

func CreateBlockDeviceFilesystem(f *framework.Framework, vm *v1alpha2.VirtualMachine, bdKind v1alpha2.BlockDeviceKind, bdName, fsType string) {
	GinkgoHelper()

	serial, ok := GetBlockDeviceSerialNumber(vm, bdKind, bdName)
	Expect(ok).To(BeTrue(), "failed to get block device serial number")

	devicePath, err := GetBlockDeviceBySerial(f, vm, serial)
	Expect(err).NotTo(HaveOccurred(), fmt.Errorf("failed to get device by serial: %w", err))

	_, err = f.SSHCommand(vm.Name, vm.Namespace, fmt.Sprintf("sudo mkfs.%s %s", fsType, devicePath))
	Expect(err).NotTo(HaveOccurred())
}

func MountBlockDevice(f *framework.Framework, vm *v1alpha2.VirtualMachine, bdKind v1alpha2.BlockDeviceKind, bdName string) {
	GinkgoHelper()

	serial, ok := GetBlockDeviceSerialNumber(vm, bdKind, bdName)
	Expect(ok).To(BeTrue(), "failed to get block device serial number")

	devicePath, err := GetBlockDeviceBySerial(f, vm, serial)
	Expect(err).NotTo(HaveOccurred(), fmt.Errorf("failed to get device by serial: %w", err))

	_, err = f.SSHCommand(vm.Name, vm.Namespace, fmt.Sprintf("sudo mount %s /mnt", devicePath))
	Expect(err).NotTo(HaveOccurred())

	cmd := fmt.Sprintf(`UUID=$(lsblk -o SERIAL,UUID | grep %s | awk "{print \$2}"); echo "UUID=$UUID /mnt ext4 defaults 0 0" | sudo tee -a /etc/fstab`, serial)
	_, err = f.SSHCommand(vm.Name, vm.Namespace, cmd)
	Expect(err).NotTo(HaveOccurred())
}

func GetBlockDeviceBySerial(f *framework.Framework, vm *v1alpha2.VirtualMachine, serial string) (string, error) {
	cmdOut, err := f.SSHCommand(vm.Name, vm.Namespace, fmt.Sprintf("sudo lsblk -o PATH,SERIAL | grep %s | awk \"{print \\$1, \\$2}\"", serial))
	if err != nil {
		return "", err
	}

	cmdLines := strings.Split(strings.TrimSpace(cmdOut), "\n")
	if len(cmdLines) == 0 {
		return "", errors.New("shell out is empty")
	}

	columns := strings.Split(strings.TrimSpace(cmdLines[0]), " ")
	if len(columns) != 2 {
		return "", errors.New("shell out columns mismatch")
	}

	if columns[1] == serial {
		return columns[0], nil
	}

	return "", errors.New("no block device found")
}

func GetBlockDeviceSerialNumber(vm *v1alpha2.VirtualMachine, bdKind v1alpha2.BlockDeviceKind, bdName string) (string, bool) {
	unstructuredVMI, err := framework.GetClients().DynamicClient().Resource(schema.GroupVersionResource{
		Group:    "internal.virtualization.deckhouse.io",
		Version:  "v1",
		Resource: "internalvirtualizationvirtualmachineinstances",
	}).Namespace(vm.Namespace).Get(context.Background(), vm.Name, metav1.GetOptions{})
	Expect(err).NotTo(HaveOccurred())

	var kvvmi virtv1.VirtualMachineInstance
	err = runtime.DefaultUnstructuredConverter.FromUnstructured(unstructuredVMI.Object, &kvvmi)
	Expect(err).NotTo(HaveOccurred())

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
	Expect(err).NotTo(HaveOccurred())
}

func ReadFile(f *framework.Framework, vm *v1alpha2.VirtualMachine, path string) string {
	GinkgoHelper()

	cmdOut, err := f.SSHCommand(vm.Name, vm.Namespace, fmt.Sprintf("sudo cat %s", path))
	Expect(err).NotTo(HaveOccurred())
	return strings.TrimSpace(cmdOut)
}
