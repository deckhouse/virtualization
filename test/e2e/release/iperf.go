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
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/test/e2e/eventually"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	"github.com/deckhouse/virtualization/test/e2e/internal/label"
	"github.com/deckhouse/virtualization/test/e2e/internal/util"
)

// An endless iperf3 session started before the upgrade must survive the live
// migration of its server.
var _ = Describe("ModuleUpgradeNetworkContinuity", Label(label.SIGCompute), func() {
	var (
		f   *framework.Framework
		ctx context.Context
	)

	BeforeEach(func() {
		f = framework.NewFramework("")
		ctx = context.Background()
	})

	It("keeps the TCP session between the iperf virtual machines across the module upgrade", func() {
		server := findVMScenario(iperfServerVM).newVMRef()
		client := findVMScenario(iperfClientVM).newVMRef()

		switch GetPhase() {
		case PhasePreUpgrade:
			startIPerfSession(ctx, f, server, client)
		case PhasePostUpgrade:
			expectIPerfSessionSurvivedUpgrade(ctx, f, server, client)
		}
	})
})

func startIPerfSession(ctx context.Context, f *framework.Framework, server, client *v1alpha2.VirtualMachine) {
	GinkgoHelper()

	By("Waiting for the iperf virtual machines to be Running and reachable")
	util.UntilObjectPhase(ctx, string(v1alpha2.MachineRunning), framework.LongTimeout, server, client)
	eventually.SSHReadyAsRoot(f, server, framework.LongTimeout)
	eventually.SSHReadyAsRoot(f, client, framework.LongTimeout)

	By("Starting the iperf3 server")
	eventually.StartIPerfServer(f, server)

	By("Starting the endless iperf3 client")

	serverIP := getVirtualMachine(ctx, f, server).Status.IPAddress
	Expect(serverIP).NotTo(BeEmpty(), "the iperf server %s/%s has no IP address", server.Namespace, server.Name)
	eventually.StartIPerfClient(f, client, serverIP, iperfReportPath)
}

func expectIPerfSessionSurvivedUpgrade(ctx context.Context, f *framework.Framework, server, client *v1alpha2.VirtualMachine) {
	GinkgoHelper()

	By("Checking that the iperf3 client is still running")
	eventually.UntilIPerfRunning(f, client)

	By("Stopping the iperf3 client and reading its report")
	eventually.StopIPerfClient(f, client)
	report := eventually.ReadIPerfClientReport(f, client, iperfReportPath)
	Expect(report.IsInterruptedByClient()).To(BeTrue(), "the iperf3 report contains an unexpected error: %q", report.Error)
	Expect(report.End.SumSent.Bytes).To(BeNumerically(">", 0), "the iperf3 client should have sent data")
	Expect(report.Intervals).NotTo(BeEmpty(), "the iperf3 report must include intervals")

	By("Verifying the session was not interrupted")
	// A single silent interval is the switchover itself.
	Expect(len(report.FindSilentIntervals())).To(BeNumerically("<=", 1),
		"the iperf3 session went silent in intervals %v of %d; a live migration must not interrupt it", report.FindSilentIntervals(), len(report.Intervals))

	if !migratesVMs() {
		By(fmt.Sprintf("Skipping the migration window check: %s=false, the upgrade does not migrate virtual machines", migratesVMsEnv))
		return
	}

	By("Verifying the session brackets the migration of the iperf server")

	window := findMigrationWindow(ctx, f, server)
	iperfStart, err := report.GetStartTime()
	Expect(err).NotTo(HaveOccurred(), "the iperf3 report start timestamp must be parseable")
	Expect(iperfStart).To(BeTemporally("<", window.start),
		"the iperf3 session must start before the migration (session start %s, migration start %s)", iperfStart, window.start)
	Expect(report.GetEndTime()).To(BeTemporally(">", window.end),
		"the iperf3 session must stop after the migration (session end %s, migration end %s)", report.GetEndTime(), window.end)
}
