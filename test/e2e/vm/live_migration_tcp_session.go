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
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/builder/vd"
	"github.com/deckhouse/virtualization-controller/pkg/builder/vm"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	"github.com/deckhouse/virtualization/test/e2e/internal/object"
	"github.com/deckhouse/virtualization/test/e2e/internal/util"
)

var _ = Describe("VirtualMachineLiveMigrationTCPSession", func() {
	var (
		iperfServer *v1alpha2.VirtualMachine
		iperfClient *v1alpha2.VirtualMachine
		report      *IPerfReport

		reportName      = "iperf-client-report.json"
		iperfServerName = "iperf-server"
		iperfClientName = "iperf-client"

		f            = framework.NewFramework("vm-live-migration-tcp-session")
		storageClass = framework.GetConfig().StorageClass.TemplateStorageClass
	)

	BeforeEach(func() {
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
			iperfServerDisk := vd.New(
				vd.WithName(iperfServerName),
				vd.WithNamespace(f.Namespace().Name),
				vd.WithStorageClass(&storageClass.Name),
				vd.WithDataSourceHTTP(&v1alpha2.DataSourceHTTP{
					URL: object.AlpineUEFIPerfHTTP,
				}),
			)

			iperfClientDisk := vd.New(
				vd.WithName(iperfClientName),
				vd.WithNamespace(f.Namespace().Name),
				vd.WithStorageClass(&storageClass.Name),
				vd.WithDataSourceHTTP(&v1alpha2.DataSourceHTTP{
					URL: object.AlpineUEFIPerfHTTP,
				}),
			)

			iperfServer = newVirtualMachine(iperfServerName, f.Namespace().Name, iperfServerDisk)
			iperfClient = newVirtualMachine(iperfClientName, f.Namespace().Name, iperfClientDisk)

			err := f.CreateWithDeferredDeletion(context.Background(), iperfServerDisk, iperfClientDisk, iperfServer, iperfClient)
			Expect(err).NotTo(HaveOccurred())

			util.UntilVMAgentReady(crclient.ObjectKeyFromObject(iperfServer), framework.LongTimeout)
			util.UntilVMAgentReady(crclient.ObjectKeyFromObject(iperfClient), framework.LongTimeout)
		})

		By("Wait for the iPerf server to start", func() {
			waitForIPerfServerToStart(iperfServer.Name, f.Namespace().Name, f)
		})

		By("Run the iPerf client", func() {
			iperfServer, err := f.Clients.VirtClient().VirtualMachines(f.Namespace().Name).Get(context.Background(), iperfServer.Name, metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred())
			cmd := fmt.Sprintf("nohup iperf3 --client %s --time 0 --json > ~/%s 2>&1 < /dev/null &", iperfServer.Status.IPAddress, reportName)
			_, err = f.SSHCommand(iperfClient.Name, iperfClient.Namespace, cmd)
			Expect(err).NotTo(HaveOccurred(), "failed to run iperf3 client")
		})

		By("Migrate the iPerf server", func() {
			util.MigrateVirtualMachine(f, iperfServer)
			util.UntilVMMigrationSucceeded(crclient.ObjectKeyFromObject(iperfServer), framework.LongTimeout)
		})

		By("Wait for packets to be transmitted after migration", func() {
			time.Sleep(10 * time.Second)
		})

		By("Check the iPerf client report", func() {
			stopIPerfClient(iperfClient.Name, f.Namespace().Name, iperfServer.Status.IPAddress, f)
			report = getIPerfClientReport(iperfClient.Name, f.Namespace().Name, reportName, f)
			Expect(report).NotTo(BeNil(), "iPerf report cannot be nil")

			iperfServer, err := f.Clients.VirtClient().VirtualMachines(f.Namespace().Name).Get(context.Background(), iperfServerName, metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred())

			iPerfClientStartTime, err := time.Parse(time.RFC1123, report.Start.Timestamp.Time)
			Expect(err).NotTo(HaveOccurred())
			Expect(iPerfClientStartTime.Before(iperfServer.Status.MigrationState.StartTimestamp.Time)).To(BeTrue(), "the iPerfClient connection test should start before the virtual machine is migrated")

			iPerfClientEndTimeSec := int64(report.Start.Timestamp.Timesecs) + int64(report.End.SumSent.End)
			iPerfClientEndTimeNSec := int64((report.End.SumSent.End - float64(int64(report.End.SumSent.End))) * 1e9)
			iPerfClientEndTime := time.Unix(iPerfClientEndTimeSec, iPerfClientEndTimeNSec).UTC()
			Expect(iPerfClientEndTime.After(iperfServer.Status.MigrationState.EndTimestamp.Time)).To(BeTrue(), "the iPerfClient connection test should stop after the virtual machine is migrated")

			zeroBytesIntervalCounter := 0
			for _, i := range report.Intervals {
				if i.Sum.Bytes == 0 {
					zeroBytesIntervalCounter++
				}
			}
			Expect(zeroBytesIntervalCounter).To(BeNumerically("<=", 1), "there should not be more than one zero-byte interval during the migration process")
		})
	})
})

func waitForIPerfServerToStart(vmName, vmNamespace string, f *framework.Framework) {
	GinkgoHelper()

	var pid string

	iPerfServerPidCmd := "ps aux | grep \"iperf3 -s\" | grep -v grep | awk \"{print \\$1}\""
	Eventually(func() error {
		stdout, err := f.SSHCommand(vmName, vmNamespace, iPerfServerPidCmd)
		if err != nil {
			return fmt.Errorf("cmd: %s\nstderr: %w", iPerfServerPidCmd, err)
		}
		pid = strings.TrimSuffix(stdout, "\n")

		re := regexp.MustCompile(`^\d+$`)
		if !re.MatchString(pid) {
			return fmt.Errorf("failed to find iPerf server PID: %s", pid)
		}

		return nil
	}).WithTimeout(framework.MiddleTimeout).WithPolling(framework.PollingInterval).ShouldNot(HaveOccurred())
}

func stopIPerfClient(vmName, vmNamespace, ip string, f *framework.Framework) {
	GinkgoHelper()

	var pid string

	iPerfClientPidCmd := fmt.Sprintf("ps aux | grep \"iperf3 --client %s\" | grep -v grep | awk \"{print \\$1}\"", ip)
	Eventually(func() error {
		stdout, err := f.SSHCommand(vmName, vmNamespace, iPerfClientPidCmd)
		if err != nil {
			return fmt.Errorf("cmd: %s\nstderr: %w", iPerfClientPidCmd, err)
		}
		pid = stdout
		return nil
	}).WithTimeout(framework.MiddleTimeout).WithPolling(framework.PollingInterval).ShouldNot(HaveOccurred())

	stopIPerfClientCmd := fmt.Sprintf("kill %s", pid)
	Eventually(func() error {
		_, err := f.SSHCommand(vmName, vmNamespace, stopIPerfClientCmd)
		if err != nil {
			return fmt.Errorf("cmd: %s\nstderr: %w", stopIPerfClientCmd, err)
		}
		return nil
	}).WithTimeout(framework.MiddleTimeout).WithPolling(framework.PollingInterval).ShouldNot(HaveOccurred())
}

func getIPerfClientReport(vmName, vmNamespace, reportName string, f *framework.Framework) *IPerfReport {
	GinkgoHelper()

	rawReport := new(string)
	cmd := fmt.Sprintf("jq . ~/%s", reportName)
	Eventually(func() error {
		stdout, err := f.SSHCommand(vmName, vmNamespace, cmd)
		if err != nil {
			return fmt.Errorf("cmd: %s\nstderr: %w", cmd, err)
		}

		*rawReport = stdout

		return nil
	}).WithTimeout(framework.MiddleTimeout).WithPolling(framework.PollingInterval).Should(Succeed())

	report := &IPerfReport{}
	err := json.Unmarshal([]byte(*rawReport), report)
	Expect(err).NotTo(HaveOccurred())

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

func newVirtualMachine(name, namespace string, disk *v1alpha2.VirtualDisk) *v1alpha2.VirtualMachine {
	cpuCount := 1
	coreFraction := "10%"

	return vm.New(
		vm.WithName(name),
		vm.WithNamespace(namespace),
		vm.WithBootloader(v1alpha2.EFI),
		vm.WithCPU(cpuCount, &coreFraction),
		vm.WithMemory(*resource.NewQuantity(object.Mi256, resource.BinarySI)),
		vm.WithDisks(disk),
		vm.WithLiveMigrationPolicy(v1alpha2.AlwaysSafeMigrationPolicy),
		vm.WithProvisioning(&v1alpha2.Provisioning{
			Type:     v1alpha2.ProvisioningTypeUserData,
			UserData: object.DefaultCloudInit,
		}),
	)
}
