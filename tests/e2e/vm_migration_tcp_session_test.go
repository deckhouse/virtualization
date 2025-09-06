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

package e2e

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/tests/e2e/config"
	"github.com/deckhouse/virtualization/tests/e2e/d8"
	"github.com/deckhouse/virtualization/tests/e2e/ginkgoutil"
	kc "github.com/deckhouse/virtualization/tests/e2e/kubectl"
)

var _ = Describe("VirtualMachineMigrationTCPSession", SIGMigration(), ginkgoutil.CommonE2ETestDecorators(), func() {
	var (
		testCaseLabel    = map[string]string{"testcase": "vm-migration-tcp-session"}
		iperfClientLabel = map[string]string{"vm": "iperf-client"}
		iperfServerLabel = map[string]string{"vm": "iperf-server"}
		rawReport        = new(string)
		ns               string
	)

	BeforeAll(func() {
		kustomization := fmt.Sprintf("%s/%s", conf.TestData.VMMigrationTCPSession, "kustomization.yaml")
		var err error
		ns, err = kustomize.GetNamespace(kustomization)
		Expect(err).NotTo(HaveOccurred(), "%w", err)

		CreateNamespace(ns)
	})

	AfterEach(func() {
		if CurrentSpecReport().Failed() {
			SaveTestResources(testCaseLabel, CurrentSpecReport().LeafNodeText)
			SaveIPerfClientReport(testCaseLabel, rawReport)
		}
	})

	Context("When resources are applied", func() {
		It("result should be succeeded", func() {
			if config.IsReusable() {
				res := kubectl.List(kc.ResourceVM, kc.GetOptions{
					Labels:    testCaseLabel,
					Namespace: ns,
					Output:    "jsonpath='{.items[*].metadata.name}'",
				})
				Expect(res.Error()).NotTo(HaveOccurred(), res.StdErr())

				if res.StdOut() != "" {
					return
				}
			}

			res := kubectl.Apply(kc.ApplyOptions{
				Filename:       []string{conf.TestData.VMMigrationTCPSession},
				FilenameOption: kc.Kustomize,
			})
			Expect(res.Error()).NotTo(HaveOccurred(), res.StdErr())

			WaitVMAgentReady(kc.WaitOptions{
				Labels:    testCaseLabel,
				Namespace: ns,
				Timeout:   MaxWaitTimeout,
			})
		})
	})

	Context("Virtual Machine Migration", func() {
		It("checks TCP connection", func() {
			reportName := "report.json"
			iperfClientVM, iperfServerVM, err := getVirtualMachinesByLabel(iperfClientLabel, iperfServerLabel, testCaseLabel, ns)

			Expect(err).NotTo(HaveOccurred())
			Expect(iperfClientVM).NotTo(BeNil())
			Expect(iperfServerVM).NotTo(BeNil())

			By("Run the iPerf client.", func() {
				cmd := fmt.Sprintf("nohup iperf3 --client %s --time 0 --json > ~/%s 2>&1 &", iperfServerVM.Status.IPAddress, reportName)
				ExecSSHCommand(ns, iperfClientVM.Name, cmd)
			})
			By("Migrate the iPerf server.", func() {
				MigrateVirtualMachines(testCaseLabel, ns, iperfServerVM.Name)
				WaitMigrationEnd(iperfServerVM.Name, ns)
			})

			By("Wait for packets to be transmitted after migration.", func() {
				time.Sleep(10 * time.Second)
			})

			By("Check the iPerf client report.", func() {
				StopIPerfClient(iperfClientVM.Name, ns, iperfServerVM.Status.IPAddress)
				GetIPerfClientReport(iperfClientVM.Name, ns, reportName, rawReport)

				report := &IPerfReport{}
				err := json.Unmarshal([]byte(*rawReport), report)
				Expect(err).NotTo(HaveOccurred())

				iperfServerVMAfterMigration := &v1alpha2.VirtualMachine{}
				err = GetObject(kc.ResourceVM, iperfServerVM.Name, iperfServerVMAfterMigration, kc.GetOptions{Namespace: ns})
				Expect(err).NotTo(HaveOccurred())

				iPerfClientStartTime, err := time.Parse(time.RFC1123, report.Start.Timestamp.Time)
				Expect(err).NotTo(HaveOccurred())
				Expect(iPerfClientStartTime.Before(iperfServerVMAfterMigration.Status.MigrationState.StartTimestamp.Time)).To(BeTrue(), "the iPerfClient connection test should start before the virtual machine is migrated")

				iPerfClientEndTimeUnix := int64(report.Start.Timestamp.Timesecs) + int64(report.End.SumSent.End)
				iPerfClientEndTime := time.Unix(iPerfClientEndTimeUnix, int64((report.End.SumSent.End-float64(int64(report.End.SumSent.End)))*1e9)).UTC()
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

	Context("When test is completed", func() {
		It("deletes test case resources", func() {
			var resourcesToDelete ResourcesToDelete

			if config.IsCleanUpNeeded() {
				resourcesToDelete.KustomizationDir = conf.TestData.VMMigrationTCPSession
			}

			DeleteTestCaseResources(ns, resourcesToDelete)
		})
	})
})

func getVirtualMachinesByLabel(iperfClientLabel, iperfServerLabel, testCaseLabel map[string]string, namespace string) (iperfClientVM, iperfServerVM *v1alpha2.VirtualMachine, err error) {
	vms := &v1alpha2.VirtualMachineList{}
	err = GetObjects(kc.ResourceVM, vms, kc.GetOptions{
		Labels:    testCaseLabel,
		Namespace: namespace,
	})
	if err != nil {
		return iperfClientVM, iperfServerVM, err
	}

	for virtualMachineKey, iperfClientValue := range iperfClientLabel {
		for _, vm := range vms.Items {
			if v, ok := vm.Labels[virtualMachineKey]; ok {
				if v == iperfClientValue {
					iperfClientVM = &vm
					break
				}
			}
		}
	}

	for virtualMachineKey, iperfServerValue := range iperfServerLabel {
		for _, vm := range vms.Items {
			if v, ok := vm.Labels[virtualMachineKey]; ok {
				if v == iperfServerValue {
					iperfServerVM = &vm
					break
				}
			}
		}
	}

	return iperfClientVM, iperfServerVM, nil
}

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

func StopIPerfClient(vmName, namespace, ip string) {
	GinkgoHelper()

	var pid string

	iPerfClientPidCmd := fmt.Sprintf("ps aux | grep \"iperf3 --client %s\" | grep -v grep | awk \"{print \\$1}\"", ip)
	Eventually(func() error {
		res := d8Virtualization.SSHCommand(vmName, iPerfClientPidCmd, d8.SSHOptions{
			Namespace:   namespace,
			Username:    conf.TestData.SSHUser,
			IdenityFile: conf.TestData.Sshkey,
		})
		if res.Error() != nil {
			return fmt.Errorf("cmd: %s\nstderr: %s", res.GetCmd(), res.StdErr())
		}
		pid = res.StdOut()
		return nil
	}).WithTimeout(Timeout).WithPolling(Interval).ShouldNot(HaveOccurred())

	stopIPerfClientCmd := fmt.Sprintf("kill %s", pid)
	Eventually(func() error {
		res := d8Virtualization.SSHCommand(vmName, stopIPerfClientCmd, d8.SSHOptions{
			Namespace:   namespace,
			Username:    conf.TestData.SSHUser,
			IdenityFile: conf.TestData.Sshkey,
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
		res := d8Virtualization.SSHCommand(vmName, cmd, d8.SSHOptions{
			Namespace:   namespace,
			Username:    conf.TestData.SSHUser,
			IdenityFile: conf.TestData.Sshkey,
		})
		if res.Error() != nil {
			return fmt.Errorf("cmd: %s\nstderr: %s", res.GetCmd(), res.StdErr())
		}

		*report = res.StdOut()

		return nil
	}).WithTimeout(Timeout).WithPolling(Interval).ShouldNot(HaveOccurred())
}

func SaveIPerfClientReport(labels map[string]string, rawReport *string) {
	GinkgoHelper()

	var jsonObject map[string]any
	err := json.Unmarshal([]byte(*rawReport), &jsonObject)
	Expect(err).NotTo(HaveOccurred())

	r, err := json.MarshalIndent(&jsonObject, "", "  ")
	Expect(err).NotTo(HaveOccurred())

	name := fmt.Sprintf("/tmp/e2e_failed__%s__iperf_client_report.json", labels["testcase"])
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
