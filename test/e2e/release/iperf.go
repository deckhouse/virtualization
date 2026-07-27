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
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmopcondition"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
)

// migrationWindow describes the time span of the migration (firmware-update)
// triggered against the iperf server VM, derived from its migration VMOP.
//
// start is when the migration VMOP was created (i.e. the migration was
// triggered); end is when the VMOP reached its terminal Completed condition.
// These come from the VMOP resource, which persists after the migration, so —
// unlike vm.Status.MigrationState — they remain reliably available in the
// post-upgrade phase even after the VMI has been recreated.
type migrationWindow struct {
	start time.Time
	end   time.Time
}

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
	Start         float64 `json:"start,omitempty"`
	End           float64 `json:"end,omitempty"`
}

type releaseUpgradeContext struct {
	Namespace       string `json:"namespace"`
	IPerfClientVM   string `json:"iperfClientVM"`
	IPerfServerVM   string `json:"iperfServerVM"`
	IPerfReportPath string `json:"iperfReportPath"`
}

// startTime returns the wall-clock time at which the iperf test started, taken
// from the report header timestamp (RFC1123).
func (r *iperfReport) startTime() (time.Time, error) {
	return time.Parse(time.RFC1123, r.Start.Timestamp.Time)
}

// endTime returns the wall-clock time at which the iperf test stopped. iperf
// reports the run duration as seconds elapsed since the start timestamp
// (End.SumSent.End), so the absolute stop time is startTimesecs + duration.
func (r *iperfReport) endTime() time.Time {
	endSec := int64(r.Start.Timestamp.Timesecs) + int64(r.End.SumSent.End)
	frac := r.End.SumSent.End - float64(int64(r.End.SumSent.End))
	endNSec := int64(frac * 1e9)
	return time.Unix(endSec, endNSec).UTC()
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

// getIPerfClientReport reads and parses the long-running iperf3 client report
// from the guest.
//
// Unlike the live-migration test (test/e2e/vm/live_migration_tcp_session.go),
// this release smoke test does not trigger an explicit VM migration and does not
// wait for it to complete. The VM survives a *module upgrade*, during which the
// iperf server VM's Status.MigrationState may be nil: either no KubeVirt VMI
// migration record was ever created, or the VMI was recreated afterwards and the
// controller reset MigrationState back to nil (see MigratingHandler.wrapMigrationState).
// Therefore this function must NOT depend on MigrationState. The continuity of
// the TCP session across the upgrade window is validated separately against the
// upgrade timestamp in verifyIPerfContinuityAfterUpgrade.
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

	// A zero-byte interval (sum.bytes == 0, equivalently sum.bits_per_second == 0)
	// means the TCP session stalled for that second. At most one such interval is
	// tolerated for the brief switchover pause; more than one indicates the session
	// was dropped.
	zeroBytesIntervalCounter := 0
	for _, i := range result.Intervals {
		if i.Sum.Bytes == 0 {
			zeroBytesIntervalCounter++
		}
	}
	Expect(zeroBytesIntervalCounter).To(BeNumerically("<=", 1), "there should not be more than one zero-byte interval during the migration process")

	return result
}

// getMigrationWindow discovers the newest migration (Evict/Migrate) VMOP for the
// given VM and returns its [start, end] window. It expects exactly the migration
// triggered by the firmware-update step to be present and completed.
func getMigrationWindow(f *framework.Framework, vmName, namespace string) migrationWindow {
	GinkgoHelper()

	vmops, err := f.Clients.VirtClient().VirtualMachineOperations(namespace).List(context.Background(), metav1.ListOptions{})
	Expect(err).NotTo(HaveOccurred())

	var newest *v1alpha2.VirtualMachineOperation
	for i := range vmops.Items {
		vmop := &vmops.Items[i]
		if vmop.Spec.VirtualMachine != vmName {
			continue
		}
		if vmop.Spec.Type != v1alpha2.VMOPTypeEvict {
			continue
		}
		if newest == nil || vmop.CreationTimestamp.After(newest.CreationTimestamp.Time) {
			newest = vmop
		}
	}

	Expect(newest).NotTo(BeNil(), "a migration VMOP for %s/%s must exist so the iperf window can be validated against the migration", namespace, vmName)

	window := migrationWindow{start: newest.CreationTimestamp.Time}
	for _, c := range newest.Status.Conditions {
		if c.Type == vmopcondition.TypeCompleted.String() {
			window.end = c.LastTransitionTime.Time
			break
		}
	}

	Expect(window.end.IsZero()).To(BeFalse(), "migration VMOP %s/%s must have a Completed condition so the migration end time is known", newest.Namespace, newest.Name)
	Expect(window.end.Before(window.start)).To(BeFalse(), "migration end must not precede migration start")

	return window
}

// continuityWindowBounds returns the index range [lower, upper] of iperf intervals
// around the upgrade timestamp.
func continuityWindowBounds(startedAt, upgradeStartedAt int64, intervals []iperfReportInterval) (int, int) {
	if len(intervals) == 0 {
		return 1, 0
	}

	upgradeOffset := float64(upgradeStartedAt - startedAt)
	index := len(intervals) - 1
	for idx, interval := range intervals {
		if upgradeOffset < interval.Sum.Start {
			index = idx
			break
		}
		if upgradeOffset >= interval.Sum.Start && upgradeOffset < interval.Sum.End {
			index = idx
			break
		}
	}

	lower := max(index-1, 0)
	upper := min(index+1, len(intervals)-1)
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
