/*
Copyright 2024 Flant JSC

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
	"fmt"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	"github.com/deckhouse/virtualization/test/e2e/internal/util"
)

// The iperf helpers drive iperf3 inside custom-image guests as root: iperf3 is
// baked into the image but nothing starts it at boot. BusyBox has no pgrep or
// pkill, so pidof is used for both.
//
// EXCEPTION: these wait on a guest-side process or file over SSH, not on a
// Kubernetes resource, so there is nothing to observe and a polling wait is
// used deliberately.

// StartIPerfServer starts an iperf3 server in the guest and waits for it to run.
func StartIPerfServer(f *framework.Framework, vm *v1alpha2.VirtualMachine) {
	GinkgoHelper()

	_, err := f.SSHCommand(vm.Name, vm.Namespace, "nohup iperf3 -s >/dev/null 2>&1 </dev/null &", framework.WithSSHUser("root"))
	Expect(err).NotTo(HaveOccurred(), "failed to start the iperf3 server on %s/%s", vm.Namespace, vm.Name)

	UntilIPerfRunning(f, vm)
}

// StartIPerfClient starts an endless iperf3 client in the guest, writing its
// JSON report to reportPath, and waits for it to run.
func StartIPerfClient(f *framework.Framework, vm *v1alpha2.VirtualMachine, serverIP, reportPath string) {
	GinkgoHelper()

	command := fmt.Sprintf("nohup iperf3 -c %s -t 0 --json > %s 2>&1 </dev/null &", serverIP, reportPath)
	_, err := f.SSHCommand(vm.Name, vm.Namespace, command, framework.WithSSHUser("root"))
	Expect(err).NotTo(HaveOccurred(), "failed to start the iperf3 client on %s/%s", vm.Namespace, vm.Name)

	UntilIPerfRunning(f, vm)
}

// IPerfRunning waits until an iperf3 process is running in the guest.
func UntilIPerfRunning(f *framework.Framework, vm *v1alpha2.VirtualMachine) {
	GinkgoHelper()

	command := "pidof iperf3"
	Until(func() error {
		stdout, err := f.SSHCommand(vm.Name, vm.Namespace, command, framework.WithSSHUser("root"))
		if err != nil {
			return fmt.Errorf("cmd: %s\nstderr: %w", command, err)
		}
		if strings.TrimSpace(stdout) == "" {
			return fmt.Errorf("iperf3 is not running on %s/%s yet", vm.Namespace, vm.Name)
		}
		return nil
	}, framework.MiddleTimeout, WithPolling(framework.PollingInterval))
}

// StopIPerfClient interrupts the iperf3 client in the guest so that it writes
// its final report.
func StopIPerfClient(f *framework.Framework, vm *v1alpha2.VirtualMachine) {
	GinkgoHelper()

	command := "kill -INT $(pidof iperf3)"
	Until(func() error {
		_, err := f.SSHCommand(vm.Name, vm.Namespace, command, framework.WithSSHUser("root"))
		if err != nil {
			return fmt.Errorf("cmd: %s\nstderr: %w", command, err)
		}
		return nil
	}, framework.MiddleTimeout, WithPolling(framework.PollingInterval))
}

// IPerfClientReport waits for the iperf3 client report at reportPath to be
// complete and returns it parsed.
func ReadIPerfClientReport(f *framework.Framework, vm *v1alpha2.VirtualMachine, reportPath string) *util.IPerfReport {
	GinkgoHelper()

	command := "cat " + reportPath
	var result *util.IPerfReport
	Until(func() error {
		stdout, err := f.SSHCommand(vm.Name, vm.Namespace, command, framework.WithSSHUser("root"))
		if err != nil {
			return fmt.Errorf("cmd: %s\nstderr: %w", command, err)
		}
		report, err := util.ParseIPerfReport(stdout)
		if err != nil {
			return err
		}
		if report.End.SumSent.End <= 0 {
			return fmt.Errorf("iperf3 report on %s/%s is incomplete", vm.Namespace, vm.Name)
		}
		result = report
		return nil
	}, framework.LongTimeout, WithPolling(framework.PollingInterval))

	Expect(result).NotTo(BeNil())
	return result
}
