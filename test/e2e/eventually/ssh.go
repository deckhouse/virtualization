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

package eventually

import (
	"context"
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	gomegatypes "github.com/onsi/gomega/types"
	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	"github.com/deckhouse/virtualization/test/e2e/internal/util"
)

// Guest-side readiness probes: they poll the guest OS over SSH, so there is
// no Kubernetes resource to observe via an internal/observer Observer.

// SSHReady waits until the guest accepts SSH commands as the cloud user.
func SSHReady(f *framework.Framework, vm *v1alpha2.VirtualMachine, timeout time.Duration) {
	GinkgoHelper()

	UntilAssertion(func(g Gomega) {
		result, err := f.SSHCommand(vm.Name, vm.Namespace, "echo 'test'", framework.WithSSHTimeout(5*time.Second))
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(result).To(ContainSubstring("test"))
	}, timeout)
}

// SSHReadyAsRoot waits until the guest accepts SSH commands as root. The
// custom image has no cloud user and no sudo, so the check logs in as
// root with the baked key.
func SSHReadyAsRoot(f *framework.Framework, vm *v1alpha2.VirtualMachine, timeout time.Duration) {
	GinkgoHelper()

	UntilAssertion(func(g Gomega) {
		out, err := f.SSHCommand(vm.Name, vm.Namespace, "echo ok",
			framework.WithSSHUser("root"), framework.WithSSHTimeout(5*time.Second))
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(out).To(ContainSubstring("ok"))
	}, timeout)
}

// GuestCommandsReady waits until every command in commands is available in
// the guest PATH (e.g. installed by cloud-init).
func GuestCommandsReady(f *framework.Framework, vm *v1alpha2.VirtualMachine, commands []string, timeout time.Duration) {
	GinkgoHelper()

	cmd := fmt.Sprintf(`
		missing=""
		for command in %s; do
			command -v "$command" >/dev/null 2>&1 || missing="$missing $command"
		done
		[ -z "$missing" ] || { echo "missing commands:$missing"; exit 1; }
	`, shellArgs(commands))

	Until(func() error {
		_, err := f.SSHCommand(vm.Name, vm.Namespace, cmd, framework.WithSSHTimeout(5*time.Second))
		return err
	}, timeout)
}

func shellArgs(args []string) string {
	quoted := make([]string, 0, len(args))
	for _, arg := range args {
		quoted = append(quoted, fmt.Sprintf("%q", arg))
	}

	return strings.Join(quoted, " ")
}

// LsblkSizeGrows waits until the guest-reported size (lsblk over SSH as root)
// of the VirtualDisk vdName grows beyond oldSize.
//
// This is a guest-side wait, not a Kubernetes resource, so there is nothing to
// observe via an Observer: the new size becomes visible in the guest
// asynchronously (CSI expansion + qemu block-device refresh finish after the
// VirtualDisk reports Ready).
func LsblkSizeGrows(ctx context.Context, f *framework.Framework, vm *v1alpha2.VirtualMachine, vdName string, oldSize resource.Quantity) {
	GinkgoHelper()

	UntilMatch(func() int {
		size := util.GetBlockDeviceLsblkSizeAsRoot(ctx, f, vm, vdName)
		return size.Cmp(oldSize)
	}, BeNumerically(">", 0), framework.MiddleTimeout,
		WithPolling(5*time.Second),
		WithExplanation("the guest should observe the increased size of the %q disk", vdName))
}

// UntilDiskCount waits until the number of block devices the guest reports
// (lsblk over SSH) satisfies matcher, e.g. Equal(initialDiskCount+1).
func UntilDiskCount(f *framework.Framework, vmName, vmNamespace string, matcher gomegatypes.GomegaMatcher, timeout time.Duration, options ...Option) {
	GinkgoHelper()

	UntilMatch(func() (int, error) {
		return util.GetDiskCount(f, vmName, vmNamespace)
	}, matcher, timeout, options...)
}

// UntilDiskCountAsRoot is the root/no-sudo variant of [UntilDiskCount] for
// custom image guests, which have no cloud user and no sudo.
func UntilDiskCountAsRoot(f *framework.Framework, vmName, vmNamespace string, matcher gomegatypes.GomegaMatcher, timeout time.Duration, options ...Option) {
	GinkgoHelper()

	UntilMatch(func() (int, error) {
		return util.GetDiskCountAsRoot(f, vmName, vmNamespace)
	}, matcher, timeout, options...)
}
