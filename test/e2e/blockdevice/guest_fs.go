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
	"fmt"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	"github.com/deckhouse/virtualization/test/e2e/internal/util"
)

// These are blockdevice-local, root/no-sudo variants of the util.* guest
// filesystem helpers. The custom e2e-br image has no cloud user, no sudo and no
// bash, so we log in as root and use POSIX sh. The util.* originals stay as
// cloud+sudo for the other suites (e.g. vmop/restore) that rely on them.

// guestDeviceBySerial returns the in-guest device path (e.g. /dev/sda) of the
// block device backing (bdKind,bdName), resolved by its serial number.
func guestDeviceBySerial(ctx context.Context, f *framework.Framework, vm *v1alpha2.VirtualMachine, bdKind v1alpha2.BlockDeviceKind, bdName string) string {
	GinkgoHelper()
	serial, ok := util.GetBlockDeviceSerialNumber(ctx, vm, bdKind, bdName)
	Expect(ok).To(BeTrue(), "failed to get block device %s/%s serial number", bdKind, bdName)

	out, err := f.SSHCommand(vm.Name, vm.Namespace,
		fmt.Sprintf(`lsblk -o PATH,SERIAL | awk '$2=="%s"{print $1}'`, serial),
		framework.WithSSHUser("root"))
	Expect(err).NotTo(HaveOccurred())

	path := strings.TrimSpace(out)
	Expect(path).NotTo(BeEmpty(), "no device with serial %s found in guest", serial)
	return path
}

// guestCreateFilesystem formats the device backing (bdKind,bdName) with fsType.
func guestCreateFilesystem(ctx context.Context, f *framework.Framework, vm *v1alpha2.VirtualMachine, bdKind v1alpha2.BlockDeviceKind, bdName, fsType string) {
	GinkgoHelper()
	dev := guestDeviceBySerial(ctx, f, vm, bdKind, bdName)
	_, err := f.SSHCommand(vm.Name, vm.Namespace, fmt.Sprintf("mkfs.%s %s", fsType, dev), framework.WithSSHUser("root"))
	Expect(err).NotTo(HaveOccurred(), "failed to create %s filesystem on %s", fsType, dev)
}

// guestMount mounts the device backing (bdKind,bdName) at mountPoint.
func guestMount(ctx context.Context, f *framework.Framework, vm *v1alpha2.VirtualMachine, bdKind v1alpha2.BlockDeviceKind, bdName, mountPoint string) {
	GinkgoHelper()
	dev := guestDeviceBySerial(ctx, f, vm, bdKind, bdName)
	_, err := f.SSHCommand(vm.Name, vm.Namespace, fmt.Sprintf("mkdir -p %s && mount %s %s", mountPoint, dev, mountPoint), framework.WithSSHUser("root"))
	Expect(err).NotTo(HaveOccurred(), "failed to mount %s at %s", dev, mountPoint)
}

// guestUnmount unmounts mountPoint.
func guestUnmount(f *framework.Framework, vm *v1alpha2.VirtualMachine, mountPoint string) {
	GinkgoHelper()
	_, err := f.SSHCommand(vm.Name, vm.Namespace, fmt.Sprintf("umount %s", mountPoint), framework.WithSSHUser("root"))
	Expect(err).NotTo(HaveOccurred(), "failed to unmount %s", mountPoint)
}

// guestWriteFile writes value (a simple token) to path in the guest.
func guestWriteFile(f *framework.Framework, vm *v1alpha2.VirtualMachine, path, value string) {
	GinkgoHelper()
	_, err := f.SSHCommand(vm.Name, vm.Namespace, fmt.Sprintf("echo %s > %s", value, path), framework.WithSSHUser("root"))
	Expect(err).NotTo(HaveOccurred(), "failed to write %s", path)
}

// guestReadFile returns the trimmed content of path in the guest.
func guestReadFile(f *framework.Framework, vm *v1alpha2.VirtualMachine, path string) string {
	GinkgoHelper()
	out, err := f.SSHCommand(vm.Name, vm.Namespace, fmt.Sprintf("cat %s", path), framework.WithSSHUser("root"))
	Expect(err).NotTo(HaveOccurred(), "failed to read %s", path)
	return strings.TrimSpace(out)
}
