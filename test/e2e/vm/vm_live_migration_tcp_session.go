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

package vm

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	"github.com/deckhouse/virtualization-controller/pkg/builder/vd"
	"github.com/deckhouse/virtualization-controller/pkg/builder/vm"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/test/e2e/eventually"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	"github.com/deckhouse/virtualization/test/e2e/internal/label"
	"github.com/deckhouse/virtualization/test/e2e/internal/object"
	vmobs "github.com/deckhouse/virtualization/test/e2e/internal/observer/vm"
	"github.com/deckhouse/virtualization/test/e2e/internal/precheck"
	"github.com/deckhouse/virtualization/test/e2e/internal/util"
)

var _ = Describe("VirtualMachineLiveMigrationTCPSession", Label(label.SIGCompute, precheck.NoPrecheck), func() {
	var (
		iperfServer    *v1alpha2.VirtualMachine
		iperfServerObs vmobs.Observer
		iperfClient    *v1alpha2.VirtualMachine
		report         *IPerfReport

		reportName      = "iperf-client-report.json"
		iperfServerName = "iperf-server"
		iperfClientName = "iperf-client"

		f            *framework.Framework
		storageClass *storagev1.StorageClass
		ctx          context.Context
	)

	BeforeEach(func() {
		// TODO: Re-enable the suite.
		Skip("skipped as flaky: fix the instability, then remove this skip")

		ctx = context.Background()
		f = framework.NewFramework("vm-live-migration-tcp-session")
		storageClass = framework.GetConfig().StorageClass.DefaultStorageClass

		DeferCleanup(f.After)

		f.Before()
	})

	AfterEach(func() {
		if CurrentSpecReport().Failed() && report != nil {
			By("Failed: save iPerf client report", func() {
				saveIPerfClientReport(report)
			})
		}
	})

	It("checks the TCP session when the virtual machine is migrated", func() {
		By("Environment preparation", func() {
			// Both guests run the custom BIOS image: iperf3 is baked into it
			// and started over SSH as root, so no cloud-init is involved. The
			// firmware is irrelevant here, and BIOS boots on a fraction of the
			// memory OVMF needs.
			iperfServerDisk := object.NewVDFromCVI(iperfServerName, f.Namespace().Name, object.PrecreatedCVICustomBIOS,
				vd.WithSize(ptr.To(resource.MustParse(vdCustomImageSize))),
				vd.WithStorageClass(&storageClass.Name),
			)

			iperfClientDisk := object.NewVDFromCVI(iperfClientName, f.Namespace().Name, object.PrecreatedCVICustomBIOS,
				vd.WithSize(ptr.To(resource.MustParse(vdCustomImageSize))),
				vd.WithStorageClass(&storageClass.Name),
			)

			iperfServer = newVirtualMachine(iperfServerName, f.Namespace().Name, iperfServerDisk)
			iperfClient = newVirtualMachine(iperfClientName, f.Namespace().Name, iperfClientDisk)

			err := f.CreateWithDeferredDeletion(ctx, iperfServerDisk, iperfClientDisk, iperfServer, iperfClient)
			Expect(err).NotTo(HaveOccurred())

			iperfServerObs = vmobs.StartObserver(ctx, f, iperfServer)
			iperfServerObs.Never(vmobs.BeFailed())
			iperfClientObs := vmobs.StartObserver(ctx, f, iperfClient)
			iperfClientObs.Never(vmobs.BeFailed())
			err = iperfServerObs.WaitFor(vmobs.BeRunning(), framework.LongTimeout)
			Expect(err).NotTo(HaveOccurred())
			err = iperfClientObs.WaitFor(vmobs.BeRunning(), framework.LongTimeout)
			Expect(err).NotTo(HaveOccurred())
		})

		By("Start the iPerf server", func() {
			eventually.SSHReadyAsRoot(f, iperfServer, framework.MiddleTimeout)
			_, err := f.SSHCommand(iperfServer.Name, iperfServer.Namespace,
				"nohup iperf3 -s >/dev/null 2>&1 </dev/null &", framework.WithSSHUser("root"))
			Expect(err).NotTo(HaveOccurred(), "failed to start iperf3 server")
			waitForIPerfServerToStart(iperfServer.Name, f.Namespace().Name, f)
		})

		By("Run the iPerf client", func() {
			eventually.SSHReadyAsRoot(f, iperfClient, framework.MiddleTimeout)

			iperfServer, err := f.Clients.VirtClient().VirtualMachines(f.Namespace().Name).Get(ctx, iperfServer.Name, metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred())

			cmd := fmt.Sprintf("nohup iperf3 -c %s -t 0 --json > ~/%s 2>&1 < /dev/null &", iperfServer.Status.IPAddress, reportName)
			_, err = f.SSHCommand(iperfClient.Name, iperfClient.Namespace, cmd, framework.WithSSHUser("root"))
			Expect(err).NotTo(HaveOccurred(), "failed to run iperf3 client")

			waitForIPerfClientToStart(iperfClient.Name, iperfClient.Namespace, f)
		})

		By("Migrate the iPerf server", func() {
			vmop := util.MigrateVirtualMachine(f, iperfServer)
			util.UntilVMOPMigrationSucceeded(ctx, vmop, framework.MaxTimeout)
			err := iperfServerObs.WaitFor(vmobs.HaveMigrationSucceeded(), framework.MaxTimeout)
			if err != nil {
				// TODO: remove temporary migration skip logic when both known issues are
				// fixed: kubevirt "client socket is closed" and Volume(s)UpdateError.
				util.SkipIfKnownMigrationFailureWithContext(ctx, iperfServer)
			}
			Expect(err).NotTo(HaveOccurred())
		})

		By("Wait 10s for packets to be transmitted after migration", func() {
			time.Sleep(10 * time.Second)
		})

		By("Check the iPerf client report", func() {
			iperfServer, err := f.Clients.VirtClient().VirtualMachines(f.Namespace().Name).Get(ctx, iperfServerName, metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred())
			stopIPerfClient(iperfClient.Name, f.Namespace().Name, f)
			report = getIPerfClientReport(iperfClient.Name, f.Namespace().Name, reportName, f)
			Expect(report).NotTo(BeNil(), "iPerf report cannot be nil")

			iPerfClientStartTime, err := time.Parse(time.RFC1123, report.Start.Timestamp.Time)
			Expect(err).NotTo(HaveOccurred())
			Expect(iPerfClientStartTime.Before(iperfServer.Status.MigrationState.StartTimestamp.Time)).To(BeTrue(), "the iPerfClient connection test should start before the virtual machine is migrated")

			iPerfClientEndTimeSec := int64(report.Start.Timestamp.Timesecs) + int64(report.End.SumSent.End)
			iPerfClientEndTimeNSec := int64((report.End.SumSent.End - float64(int64(report.End.SumSent.End))) * 1e9)
			iPerfClientEndTime := time.Unix(iPerfClientEndTimeSec, iPerfClientEndTimeNSec).UTC()
			Expect(iPerfClientEndTime.After(iperfServer.Status.MigrationState.EndTimestamp.Time)).To(BeTrue(), "the iPerfClient connection test should stop after the virtual machine is migrated")

			// The session is allowed to stall exactly once, while the machine is
			// switching over, and not a second longer. How long that stall lasts is
			// a property of the storage class rather than of the TCP session: on a
			// ReadWriteOnce class the migration is a BlockMigration that copies the
			// disks to the target node, and its handover window runs to several
			// seconds, whereas on shared storage it is under one. So the check is
			// not on the number of silent intervals but on where they sit: a single
			// uninterrupted run of them, entirely inside the migration window.
			var silent []int
			for i, iv := range report.Intervals {
				if iv.Sum.Bytes == 0 {
					silent = append(silent, i)
				}
			}

			base := time.Unix(int64(report.Start.Timestamp.Timesecs), 0).UTC()
			at := func(offset float64) time.Time {
				return base.Add(time.Duration(offset * float64(time.Second)))
			}

			// The migration timestamps are metav1.Time, which keeps whole
			// seconds only, so the reported end stands for any instant up to a
			// second later. The window closes on the last instant it can mean
			// rather than on the truncated value, otherwise the verdict comes
			// down to milliseconds that were never measured to begin with.
			migration := iperfServer.Status.MigrationState
			from := migration.StartTimestamp.Add(-migrationStallGrace)
			to := migration.EndTimestamp.Add(time.Second + migrationStallGrace)

			if len(silent) > 0 {
				Expect(silent[len(silent)-1]-silent[0]).To(Equal(len(silent)-1),
					"the TCP session went silent more than once (intervals %v); a stall outside the switchover means the session did not survive the migration cleanly", silent)

				// Report the whole silent run, not the one interval that
				// happens to fall outside the window: a session that never
				// comes back reads as a momentary blip otherwise, which says
				// the opposite of what the report actually holds.
				first := report.Intervals[silent[0]].Sum
				last := report.Intervals[silent[len(silent)-1]].Sum
				Expect(at(last.Start).Before(to) && at(first.End).After(from)).To(BeTrue(),
					"the TCP session was silent for %s, from %s to %s (%d of %d intervals), which does not fit the migration window %s - %s widened by %s; a stall that outlives the switchover means the session did not recover",
					at(last.End).Sub(at(first.Start)), at(first.Start), at(last.End),
					len(silent), len(report.Intervals),
					migration.StartTimestamp.Time, migration.EndTimestamp.Time, migrationStallGrace)
			}
		})
	})
})

func waitForIPerfServerToStart(vmName, vmNamespace string, f *framework.Framework) {
	GinkgoHelper()

	// EXCEPTION: guest-side wait (iperf3 server process over SSH), not a
	// Kubernetes resource — nothing to observe via an Observer.
	// BusyBox has no pgrep; pidof is the baked-in equivalent.
	iPerfServerPidCmd := "pidof iperf3"
	eventually.Until(func() error {
		stdout, err := f.SSHCommand(vmName, vmNamespace, iPerfServerPidCmd, framework.WithSSHUser("root"))
		if err != nil {
			return fmt.Errorf("cmd: %s\nstderr: %w", iPerfServerPidCmd, err)
		}
		pid := strings.TrimSpace(stdout)

		re := regexp.MustCompile(`^[\d ]+$`)
		if !re.MatchString(pid) {
			return fmt.Errorf("failed to find iPerf server PID: %s", pid)
		}

		return nil
	}, framework.MiddleTimeout, eventually.WithPolling(framework.PollingInterval))
}

func waitForIPerfClientToStart(vmName, vmNamespace string, f *framework.Framework) {
	GinkgoHelper()
	// EXCEPTION: guest-side wait (iperf3 client process over SSH), not a
	// Kubernetes resource — nothing to observe via an Observer.
	// BusyBox has no pgrep; pidof is the baked-in equivalent.
	iPerfClientPidCmd := "pidof iperf3"
	eventually.Until(func() error {
		stdout, err := f.SSHCommand(vmName, vmNamespace, iPerfClientPidCmd, framework.WithSSHUser("root"))
		if err != nil {
			return fmt.Errorf("cmd: %s\nstderr: %w", iPerfClientPidCmd, err)
		}
		pid := strings.TrimSpace(strings.TrimSuffix(stdout, "\n"))
		re := regexp.MustCompile(`^[\d ]+$`)
		if !re.MatchString(pid) {
			return fmt.Errorf("iperf3 client not running yet (no PID found): %q", stdout)
		}
		return nil
	}, framework.MiddleTimeout, eventually.WithPolling(framework.PollingInterval))
}

func stopIPerfClient(vmName, vmNamespace string, f *framework.Framework) {
	GinkgoHelper()

	// EXCEPTION: guest-side action retried over SSH, not a Kubernetes
	// resource — nothing to observe via an Observer.
	// BusyBox has no pkill; signal the PIDs that pidof reports.
	stopIPerfClientCmd := "kill -INT $(pidof iperf3)"
	eventually.Until(func() error {
		_, err := f.SSHCommand(vmName, vmNamespace, stopIPerfClientCmd, framework.WithSSHUser("root"))
		if err != nil {
			return fmt.Errorf("cmd: %s\nstderr: %w", stopIPerfClientCmd, err)
		}
		return nil
	}, framework.MiddleTimeout, eventually.WithPolling(framework.PollingInterval))
}

func getIPerfClientReport(vmName, vmNamespace, reportName string, f *framework.Framework) *IPerfReport {
	GinkgoHelper()

	rawReport := new(string)
	// The report is read raw (the custom image has no jq); json.Unmarshal
	// below is the validity check.
	cmd := fmt.Sprintf("cat ~/%s", reportName)
	// EXCEPTION: guest-side wait (the iperf3 report file over SSH), not a
	// Kubernetes resource — nothing to observe via an Observer.
	eventually.Until(func() error {
		stdout, err := f.SSHCommand(vmName, vmNamespace, cmd, framework.WithSSHUser("root"))
		if err != nil {
			return fmt.Errorf("cmd: %s\nstderr: %w", cmd, err)
		}
		if len(strings.TrimSpace(stdout)) < 100 || !strings.Contains(stdout, `"start"`) {
			return fmt.Errorf("iperf3 report empty or incomplete (len=%d)", len(stdout))
		}
		*rawReport = stdout
		return nil
	}, framework.MiddleTimeout, eventually.WithPolling(framework.PollingInterval))

	report := &IPerfReport{}
	err := json.Unmarshal([]byte(*rawReport), report)
	Expect(err).NotTo(HaveOccurred(), "iperf3 report must be valid JSON (file may be truncated or iperf3 did not run)")

	return report
}

func saveIPerfClientReport(report *IPerfReport) {
	GinkgoHelper()

	ft := framework.GetFormattedTestCaseFullText()
	tmpDir := framework.GetTMPDir()

	r, err := json.MarshalIndent(report, "", "  ")
	Expect(err).NotTo(HaveOccurred())

	name := fmt.Sprintf("%s/e2e_failed__%s__iperf_client_report.json", tmpDir, ft)
	err = os.WriteFile(name, r, 0o644)
	Expect(err).NotTo(HaveOccurred())
}

type Timestamp struct {
	Time     string `json:"time"`
	Timesecs int    `json:"timesecs"`
}

type Start struct {
	Timestamp Timestamp `json:"timestamp"`
}

type SumSent struct {
	End float64 `json:"end"`
}

type End struct {
	SumSent SumSent `json:"sum_sent"`
}

type Sum struct {
	Bytes int64 `json:"bytes"`
	// Start and End are offsets in seconds from the start of the iPerf run,
	// used to place an interval on the wall clock and tell the intervals that
	// overlap the migration apart from the ones that do not.
	Start float64 `json:"start"`
	End   float64 `json:"end"`
}

type IPerfInterval struct {
	Sum Sum `json:"sum"`
}

type IPerfReport struct {
	Start     Start           `json:"start"`
	Intervals []IPerfInterval `json:"intervals"`
	End       End             `json:"end"`
	Error     string          `json:"error,omitempty"`
}

// migrationStallGrace is how far outside the reported migration window a silent
// interval may still be attributed to the switchover. The window is taken from
// the VM status, while the last packets are carried by a network path that is
// rewired slightly after the migration is reported complete. It covers the
// rewiring, nothing more: a session that stays silent past it did not survive
// the migration, and widening the grace would only hide that.
const migrationStallGrace = 5 * time.Second

func newVirtualMachine(name, namespace string, disk *v1alpha2.VirtualDisk) *v1alpha2.VirtualMachine {
	return vm.New(
		vm.WithName(name),
		vm.WithNamespace(namespace),
		vm.WithBootloader(v1alpha2.BIOS),
		// This suite is the one place where the guests are not sized for merely
		// booting: they carry a saturated TCP session across a migration, and
		// have to drain it again the moment the machine lands on the target. A
		// guest that cannot get the CPU to do that leaves its peer backing off
		// its retransmits, which reads as a session that stalls and never
		// recovers rather than one that paused for the switchover.
		vm.WithCPU(1, ptr.To("100%")),
		vm.WithMemory(*resource.NewQuantity(object.Mi256, resource.BinarySI)),
		vm.WithDisks(disk),
		vm.WithLiveMigrationPolicy(v1alpha2.AlwaysSafeMigrationPolicy),
		// The custom image has no cloud-init; the guest agent is baked in.
	)
}
