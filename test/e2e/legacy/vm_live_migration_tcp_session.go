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

package legacy

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/watch"

	"github.com/deckhouse/virtualization-controller/pkg/builder/vd"
	"github.com/deckhouse/virtualization-controller/pkg/builder/vm"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/test/e2e/internal/d8"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	kc "github.com/deckhouse/virtualization/test/e2e/internal/kubectl"
	"github.com/deckhouse/virtualization/test/e2e/internal/object"
	"github.com/deckhouse/virtualization/test/e2e/internal/util"
)

var _ = Describe("VirtualMachineLiveMigrationTCPSession", framework.CommonE2ETestDecorators(), func() {
	var (
		ctx    context.Context
		cancel context.CancelFunc

		iperfClientVM *v1alpha2.VirtualMachine
		iperfServerVM *v1alpha2.VirtualMachine

		rawReport  = new(string)
		reportName = "iperf-client-report.json"

		testCaseLabel      = "testcase"
		testCaseLabelValue = "vm-live-migration-tcp-session"

		iperfClientName = "iperf-client"
		iperfServerName = "iperf-server"

		alpineVirtualImageURL = "https://89d64382-20df-4581-8cc7-80df331f67fa.selstorage.ru/alpine/alpine-3-21-uefi-perf.qcow2"

		f            = framework.NewFramework(testCaseLabelValue)
		storageClass = framework.GetConfig().StorageClass.TemplateStorageClass
		testSkipped  bool
	)

	BeforeAll(func() {
		// TODO: The test is being disabled because running it with the ginkgo `--race` option detects a race condition.
		// This leads to unstable test execution. Remove Skip after fixing the issue.
		testSkipped = true
		Skip("This test case is not working everytime. Should be fixed.")
	})

	f.BeforeAll()
	f.AfterAll()

	AfterEach(func() {
		if !testSkipped {
			if CurrentSpecReport().Failed() {
				SaveTestCaseDump(map[string]string{testCaseLabel: testCaseLabelValue}, CurrentSpecReport().LeafNodeText, f.Namespace().Name)
				SaveIPerfClientReport(testCaseLabelValue, rawReport)
			}

			cancel()
		}
	})

	It("checks TCP connection", func() {
		By("Environment preparation", func() {
			iperfClientDisk := newVirtualDisk(iperfClientName, f.Namespace().Name, alpineVirtualImageURL, &storageClass.Name, map[string]string{testCaseLabel: testCaseLabelValue})
			iperfServerDisk := newVirtualDisk(iperfServerName, f.Namespace().Name, alpineVirtualImageURL, &storageClass.Name, map[string]string{testCaseLabel: testCaseLabelValue})
			virtualDisks := []*v1alpha2.VirtualDisk{iperfClientDisk, iperfServerDisk}

			iperfClientVM = newVirtualMachine(iperfClientName, f.Namespace().Name, iperfClientDisk, map[string]string{testCaseLabel: testCaseLabelValue})
			iperfServerVM = newVirtualMachine(iperfServerName, f.Namespace().Name, iperfServerDisk, map[string]string{testCaseLabel: testCaseLabelValue})

			ctx, cancel = context.WithTimeout(context.Background(), framework.MaxTimeout)

			wg := &sync.WaitGroup{}

			for _, vd := range virtualDisks {
				wg.Add(1)
				go func() {
					defer GinkgoRecover()
					defer wg.Done()
					_ = CreateVirtualDisk(ctx, vd)
				}()
			}

			iperfServerVM = CreateVirtualMachine(ctx, iperfServerVM)
			iperfClientVM = CreateVirtualMachine(ctx, iperfClientVM)

			wg.Wait()
		})

		By("Wait for the iPerf server to start", func() {
			WaitForIPerfServerToStart(iperfServerName, f.Namespace().Name)
		})

		By("Run the iPerf client", func() {
			cmd := fmt.Sprintf("nohup iperf3 --client %s --time 0 --json > ~/%s 2>&1 < /dev/null &", iperfServerVM.Status.IPAddress, reportName)
			ExecSSHCommand(f.Namespace().Name, iperfClientVM.Name, cmd)
		})

		By("Migrate the iPerf server", func() {
			MigrateVirtualMachines(map[string]string{testCaseLabel: testCaseLabelValue}, f.Namespace().Name, iperfServerVM.Name)
			WaitMigrationEnd(iperfServerVM.Name, f.Namespace().Name)
		})

		By("Wait for packets to be transmitted after migration", func() {
			time.Sleep(10 * time.Second)
		})

		By("Check the iPerf client report", func() {
			StopIPerfClient(iperfClientVM.Name, f.Namespace().Name, iperfServerVM.Status.IPAddress)
			GetIPerfClientReport(iperfClientVM.Name, f.Namespace().Name, reportName, rawReport)

			report := &IPerfReport{}
			err := json.Unmarshal([]byte(*rawReport), report)
			Expect(err).NotTo(HaveOccurred())

			iperfServerVMAfterMigration := &v1alpha2.VirtualMachine{}
			err = GetObject(kc.ResourceVM, iperfServerVM.Name, iperfServerVMAfterMigration, kc.GetOptions{Namespace: f.Namespace().Name})
			Expect(err).NotTo(HaveOccurred())

			iPerfClientStartTime, err := time.Parse(time.RFC1123, report.Start.Timestamp.Time)
			Expect(err).NotTo(HaveOccurred())
			Expect(iPerfClientStartTime.Before(iperfServerVMAfterMigration.Status.MigrationState.StartTimestamp.Time)).To(BeTrue(), "the iPerfClient connection test should start before the virtual machine is migrated")

			iPerfClientEndTimeSec := int64(report.Start.Timestamp.Timesecs) + int64(report.End.SumSent.End)
			iPerfClientEndTimeNSec := int64((report.End.SumSent.End - float64(int64(report.End.SumSent.End))) * 1e9)
			iPerfClientEndTime := time.Unix(iPerfClientEndTimeSec, iPerfClientEndTimeNSec).UTC()
			Expect(iPerfClientEndTime.After(iperfServerVMAfterMigration.Status.MigrationState.EndTimestamp.Time)).To(BeTrue(), "the iPerfClient connection test should stop after the virtual machine is migrated")

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

func WaitMigrationEnd(vmName, namespace string) {
	GinkgoHelper()

	Eventually(func() error {
		vmAfterMigration := &v1alpha2.VirtualMachine{}
		err := GetObject(kc.ResourceVM, vmName, vmAfterMigration, kc.GetOptions{Namespace: namespace})
		if err != nil {
			return err
		}

		if vmAfterMigration.Status.MigrationState != nil {
			if vmAfterMigration.Status.MigrationState.Result == v1alpha2.MigrationResultSucceeded {
				return nil
			}
		}

		return fmt.Errorf("failed to get `VirtualMachine.Status.MigrationState`: %s", vmName)
	}).WithTimeout(LongWaitDuration).WithPolling(Interval).Should(Succeed())
}

func WaitForIPerfServerToStart(vmName, namespace string) {
	GinkgoHelper()

	var pid string

	iPerfServerPidCmd := "ps aux | grep \"iperf3 -s\" | grep -v grep | awk \"{print \\$1}\""
	Eventually(func() error {
		res := framework.GetClients().D8Virtualization().SSHCommand(vmName, iPerfServerPidCmd, d8.SSHOptions{
			Namespace:    namespace,
			Username:     conf.TestData.SSHUser,
			IdentityFile: conf.TestData.Sshkey,
		})
		if res.Error() != nil {
			return fmt.Errorf("cmd: %s\nstderr: %s", res.GetCmd(), res.StdErr())
		}
		pid = strings.TrimSuffix(res.StdOut(), "\n")

		re := regexp.MustCompile(`^\d+$`)
		if !re.MatchString(pid) {
			return fmt.Errorf("failed to find iPerf server PID: %s", pid)
		}

		return nil
	}).WithTimeout(Timeout).WithPolling(Interval).ShouldNot(HaveOccurred())
}

func StopIPerfClient(vmName, namespace, ip string) {
	GinkgoHelper()

	var pid string

	iPerfClientPidCmd := fmt.Sprintf("ps aux | grep \"iperf3 --client %s\" | grep -v grep | awk \"{print \\$1}\"", ip)
	Eventually(func() error {
		res := framework.GetClients().D8Virtualization().SSHCommand(vmName, iPerfClientPidCmd, d8.SSHOptions{
			Namespace:    namespace,
			Username:     conf.TestData.SSHUser,
			IdentityFile: conf.TestData.Sshkey,
		})
		if res.Error() != nil {
			return fmt.Errorf("cmd: %s\nstderr: %s", res.GetCmd(), res.StdErr())
		}
		pid = res.StdOut()
		return nil
	}).WithTimeout(Timeout).WithPolling(Interval).ShouldNot(HaveOccurred())

	stopIPerfClientCmd := fmt.Sprintf("kill %s", pid)
	Eventually(func() error {
		res := framework.GetClients().D8Virtualization().SSHCommand(vmName, stopIPerfClientCmd, d8.SSHOptions{
			Namespace:    namespace,
			Username:     conf.TestData.SSHUser,
			IdentityFile: conf.TestData.Sshkey,
		})
		if res.Error() != nil {
			return fmt.Errorf("cmd: %s\nstderr: %s", res.GetCmd(), res.StdErr())
		}
		return nil
	}).WithTimeout(Timeout).WithPolling(Interval).ShouldNot(HaveOccurred())
}

func GetIPerfClientReport(vmName, namespace, reportName string, report *string) {
	GinkgoHelper()

	cmd := fmt.Sprintf("jq . ~/%s", reportName)
	Eventually(func() error {
		res := framework.GetClients().D8Virtualization().SSHCommand(vmName, cmd, d8.SSHOptions{
			Namespace:    namespace,
			Username:     conf.TestData.SSHUser,
			IdentityFile: conf.TestData.Sshkey,
		})
		if res.Error() != nil {
			return fmt.Errorf("cmd: %s\nstderr: %s", res.GetCmd(), res.StdErr())
		}

		*report = res.StdOut()

		return nil
	}).WithTimeout(Timeout).WithPolling(Interval).ShouldNot(HaveOccurred())
}

func SaveIPerfClientReport(testCaseName string, rawReport *string) {
	GinkgoHelper()

	tmpDir := os.Getenv("RUNNER_TEMP")
	if tmpDir == "" {
		tmpDir = "/tmp"
	}

	var jsonObject map[string]any
	err := json.Unmarshal([]byte(*rawReport), &jsonObject)
	Expect(err).NotTo(HaveOccurred())

	r, err := json.MarshalIndent(&jsonObject, "", "  ")
	Expect(err).NotTo(HaveOccurred())

	name := fmt.Sprintf("%s/e2e_failed__%s__iperf_client_report.json", tmpDir, testCaseName)
	err = os.WriteFile(name, r, 0o644)
	Expect(err).NotTo(HaveOccurred())
}

type IPerfReport struct {
	Start struct {
		Timestamp struct {
			Time     string `json:"time"`
			Timesecs int    `json:"timesecs"`
		} `json:"timestamp"`
	} `json:"start"`
	Intervals []IPerfInterval `json:"intervals"`
	End       struct {
		SumSent struct {
			End float64 `json:"end"`
		} `json:"sum_sent"`
	} `json:"end"`
	Error string `json:"error,omitempty"`
}

type IPerfInterval struct {
	Sum struct {
		Bytes int64 `json:"bytes"`
	} `json:"sum"`
}

func newVirtualMachine(name, namespace string, disk *v1alpha2.VirtualDisk, labels map[string]string) *v1alpha2.VirtualMachine {
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
		vm.WithLabels(labels),
	)
}

func newVirtualDisk(name, namespace, image string, storageClass *string, labels map[string]string) *v1alpha2.VirtualDisk {
	return vd.New(
		vd.WithName(name),
		vd.WithNamespace(namespace),
		vd.WithStorageClass(storageClass),
		vd.WithDataSourceHTTP(&v1alpha2.DataSourceHTTP{
			URL: image,
		}),
		vd.WithLabels(labels),
	)
}

func WaitForVirtualMachine(ctx context.Context, namespace, name string, h util.EventHandler[*v1alpha2.VirtualMachine]) *v1alpha2.VirtualMachine {
	GinkgoHelper()

	virtualMachine, err := util.WaitFor(ctx, framework.GetClients().VirtClient().VirtualMachines(namespace), h, metav1.ListOptions{
		FieldSelector: fields.OneTermEqualSelector("metadata.name", name).String(),
	})
	Expect(err).NotTo(HaveOccurred())

	return virtualMachine
}

func WaitForVirtualDisk(ctx context.Context, namespace, name string, h util.EventHandler[*v1alpha2.VirtualDisk]) *v1alpha2.VirtualDisk {
	GinkgoHelper()

	virtualDisk, err := util.WaitFor(ctx, framework.GetClients().VirtClient().VirtualDisks(namespace), h, metav1.ListOptions{
		FieldSelector: fields.OneTermEqualSelector("metadata.name", name).String(),
	})
	Expect(err).NotTo(HaveOccurred())

	return virtualDisk
}

func CreateVirtualMachine(ctx context.Context, virtualMachine *v1alpha2.VirtualMachine) *v1alpha2.VirtualMachine {
	GinkgoHelper()

	CreateResource(ctx, virtualMachine)
	virtualMachine = WaitForVirtualMachine(ctx, virtualMachine.Namespace, virtualMachine.Name, func(_ watch.EventType, e *v1alpha2.VirtualMachine) (bool, error) {
		return e.Status.Phase == v1alpha2.MachineRunning, nil
	})

	return virtualMachine
}

func CreateVirtualDisk(ctx context.Context, virtualDisk *v1alpha2.VirtualDisk) *v1alpha2.VirtualDisk {
	GinkgoHelper()

	CreateResource(ctx, virtualDisk)
	virtualDisk = WaitForVirtualDisk(ctx, virtualDisk.Namespace, virtualDisk.Name, func(_ watch.EventType, e *v1alpha2.VirtualDisk) (bool, error) {
		return e.Status.Phase == v1alpha2.DiskReady, nil
	})

	return virtualDisk
}
