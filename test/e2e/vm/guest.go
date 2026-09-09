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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
)

// guestPingExternalCommand probes outbound connectivity from the minimal
// custom guest: it has no curl (BusyBox userspace), so ping is used
// instead of network.CheckExternalConnectivity. The command deliberately
// contains no single quotes (d8 wraps the guest command in '...').
const guestPingExternalCommand = `for h in flant.ru google.com ya.ru; do ping -c1 -W3 $h >/dev/null 2>&1 && echo $h && exit 0; done; exit 1`

// expectExternalConnectivityAsRoot asserts the custom guest can reach
// at least one well-known external host, probing with ping as root.
func expectExternalConnectivityAsRoot(f *framework.Framework, vmName, namespace string) {
	GinkgoHelper()
	reachableHost, err := f.SSHCommand(vmName, namespace, guestPingExternalCommand, framework.WithSSHUser("root"))
	Expect(err).NotTo(HaveOccurred(), "VM %s should have outbound connectivity", vmName)
	Expect(reachableHost).NotTo(BeEmpty())
}

// vdCustomImageSize is the size for disks backed by the custom image
// and for blank/data disks in the specs migrated to it. The BIOS flavor is
// ~47Mi and the EFI flavor ~56Mi virtual (they grow their root filesystem to
// the disk on first boot), so 64Mi fits both with headroom.
//
// The root/no-sudo guest helpers for the specs migrated to the custom
// image (no cloud user, no sudo, no bash: guest commands log in as root over
// dropbear with the baked key and use POSIX sh) live in the shared eventually
// and util packages: eventually.SSHReadyAsRoot, eventually.UntilDiskCountAsRoot,
// util.GetDiskCountAsRoot. The cloud+sudo originals stay for the suites that
// still boot Alpine/Ubuntu with cloud-init (e.g. usb, tpm, iperf and SDN tests).
const vdCustomImageSize = "64Mi"
