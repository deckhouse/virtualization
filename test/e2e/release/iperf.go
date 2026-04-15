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

package release

import (
	"encoding/json"
	"fmt"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
)

const (
	releaseIPerfReportPath = "/tmp/release-upgrade-iperf-client-report.json"
)

type iperfReport struct {
	Start     iperfReportStart      `json:"start"`
	Intervals []iperfReportInterval `json:"intervals"`
	End       iperfReportEnd        `json:"end"`
	Error     string                `json:"error,omitempty"`
}

type iperfReportStart struct {
	Timestamp iperfReportTimestamp `json:"timestamp"`
}

type iperfReportTimestamp struct {
	Time     string `json:"time"`
	Timesecs int    `json:"timesecs"`
}

type iperfReportEnd struct {
	SumSent     iperfReportSummary `json:"sum_sent"`
	SumReceived iperfReportSummary `json:"sum_received"`
}

type iperfReportInterval struct {
	Sum iperfReportSummary `json:"sum"`
}

type iperfReportSummary struct {
	Bytes         int64   `json:"bytes"`
	BitsPerSecond float64 `json:"bits_per_second"`
	End           float64 `json:"end,omitempty"`
}

type releaseUpgradeContext struct {
	Namespace       string `json:"namespace"`
	IPerfClientVM   string `json:"iperfClientVM"`
	IPerfServerVM   string `json:"iperfServerVM"`
	IPerfReportPath string `json:"iperfReportPath"`
}

func waitForIPerfServerToStart(f *framework.Framework, vm *v1alpha2.VirtualMachine) {
	GinkgoHelper()

	command := "rc-service iperf3 status --nocolor"
	Eventually(func() error {
		stdout, err := f.SSHCommand(vm.Name, vm.Namespace, command)
		if err != nil {
			return fmt.Errorf("cmd: %s\nstderr: %w", command, err)
		}
		if strings.Contains(stdout, "status: started") {
			return nil
		}
		return fmt.Errorf("iperf3 server is not started yet: %s", stdout)
	}).WithTimeout(framework.MiddleTimeout).WithPolling(framework.PollingInterval).Should(Succeed())
}

func waitForIPerfClientToStart(f *framework.Framework, vm *v1alpha2.VirtualMachine) {
	GinkgoHelper()

	command := "pgrep -x iperf3"
	Eventually(func() error {
		stdout, err := f.SSHCommand(vm.Name, vm.Namespace, command)
		if err != nil {
			return fmt.Errorf("cmd: %s\nstderr: %w", command, err)
		}
		if strings.TrimSpace(stdout) == "" {
			return fmt.Errorf("iperf3 client is not running yet")
		}
		return nil
	}).WithTimeout(framework.MiddleTimeout).WithPolling(framework.PollingInterval).Should(Succeed())
}

func stopIPerfClient(f *framework.Framework, vm *v1alpha2.VirtualMachine) {
	GinkgoHelper()

	command := "pkill -INT -x iperf3"
	Eventually(func() error {
		_, err := f.SSHCommand(vm.Name, vm.Namespace, command)
		if err != nil {
			return fmt.Errorf("cmd: %s\nstderr: %w", command, err)
		}
		return nil
	}).WithTimeout(framework.MiddleTimeout).WithPolling(framework.PollingInterval).Should(Succeed())
}

func getIPerfClientReport(f *framework.Framework, vm *v1alpha2.VirtualMachine, reportPath string) *iperfReport {
	GinkgoHelper()

	command := fmt.Sprintf("cat %s", reportPath)
	var result *iperfReport
	Eventually(func() error {
		stdout, err := f.SSHCommand(vm.Name, vm.Namespace, command)
		if err != nil {
			return fmt.Errorf("cmd: %s\nstderr: %w", command, err)
		}
		report, err := parseIPerfReport(stdout)
		if err != nil {
			return err
		}
		if report.End.SumSent.End <= 0 {
			return fmt.Errorf("iperf3 report is incomplete")
		}
		result = report
		return nil
	}).WithTimeout(framework.LongTimeout).WithPolling(framework.PollingInterval).Should(Succeed())

	Expect(result).NotTo(BeNil())
	return result
}

// continuityWindowBounds returns the index range [lower, upper] of iperf intervals
// around the upgrade timestamp. Assumes default 1-second reporting intervals.
func continuityWindowBounds(startedAt, upgradeStartedAt int64, intervalCount int) (int, int) {
	if intervalCount == 0 {
		return 1, 0
	}

	index := int(upgradeStartedAt - startedAt)
	if index < 0 {
		index = 0
	}
	if index >= intervalCount {
		index = intervalCount - 1
	}

	lower := max(index-1, 0)
	upper := min(index+1, intervalCount-1)
	return lower, upper
}

func parseIPerfReport(raw string) (*iperfReport, error) {
	var report iperfReport
	if err := json.Unmarshal([]byte(raw), &report); err != nil {
		return nil, fmt.Errorf("parse iperf3 json: %w", err)
	}

	return &report, nil
}

func isExpectedIPerfReportError(errMsg string) bool {
	if errMsg == "" {
		return true
	}

	return strings.Contains(errMsg, "interrupt - the client has terminated by signal Interrupt(2)")
}
