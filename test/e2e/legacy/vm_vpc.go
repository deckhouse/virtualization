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
	"fmt"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
	"github.com/deckhouse/virtualization/test/e2e/internal/config"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	kc "github.com/deckhouse/virtualization/test/e2e/internal/kubectl"
)

func WaitForVMNetworkReady(opts kc.WaitOptions) {
	GinkgoHelper()
	WaitConditionIsTrueByLabel(kc.ResourceVM, vmcondition.TypeNetworkReady.String(), opts)
}

func WaitForVMRunningPhase(opts kc.WaitOptions) {
	GinkgoHelper()
	WaitPhaseByLabel(kc.ResourceVM, PhaseRunning, opts)
}

var _ = Describe("VirtualMachineAdditionalNetworkInterfaces", Ordered, func() {
	testCaseLabel := map[string]string{"testcase": "vm-vpc"}
	var ns string

	BeforeAll(func() {
		sdnEnabled, err := isSdnModuleEnabled()
		if err != nil || !sdnEnabled {
			Skip("Module SDN is disabled. Skipping all tests for module SDN.")
		}

		kustomization := fmt.Sprintf("%s/%s", conf.TestData.VMVpc, "kustomization.yaml")
		ns, err = kustomize.GetNamespace(kustomization)
		Expect(err).NotTo(HaveOccurred(), "%w", err)

		CreateNamespace(ns)
	})

	AfterAll(func() {
		if CurrentSpecReport().Failed() {
			SaveTestCaseDump(testCaseLabel, CurrentSpecReport().LeafNodeText, ns)
		}
	})

	Context("When resources are applied", func() {
		It("result should be succeeded", func() {
			res := kubectl.Apply(kc.ApplyOptions{
				Filename:       []string{conf.TestData.VMVpc},
				FilenameOption: kc.Kustomize,
			})
			Expect(res.Error()).NotTo(HaveOccurred(), res.StdErr())
		})
	})

	Context("When virtual machines are applied", func() {
		It("checks VMs phases", func() {
			By("Virtual machine should be running")
			WaitForVMRunningPhase(kc.WaitOptions{
				Labels:    testCaseLabel,
				Namespace: ns,
				Timeout:   MaxWaitTimeout,
			})
		})
		It("checks network availability", func() {
			By("Network condition should be true")
			WaitForVMNetworkReady(kc.WaitOptions{
				Labels:    testCaseLabel,
				Namespace: ns,
				Timeout:   MaxWaitTimeout,
			})

			CheckVMConnectivityToTargetIPs(ns, testCaseLabel)
		})
	})

	Context("When virtual machine agents and network are ready", func() {
		It("starts migrations", func() {
			res := kubectl.List(kc.ResourceVM, kc.GetOptions{
				Labels:    testCaseLabel,
				Namespace: ns,
				Output:    "jsonpath='{.items[*].metadata.name}'",
			})
			Expect(res.Error()).NotTo(HaveOccurred(), res.StdErr())

			vms := strings.Split(res.StdOut(), " ")
			MigrateVirtualMachines(testCaseLabel, ns, vms...)
		})
	})

	Context("When VMs migrations are applied", func() {
		It("checks VMs and VMOPs phases", func() {
			By(fmt.Sprintf("VMOPs should be in %s phases", v1alpha2.VMOPPhaseCompleted))
			WaitPhaseByLabel(kc.ResourceVMOP, string(v1alpha2.VMOPPhaseCompleted), kc.WaitOptions{
				Labels:    testCaseLabel,
				Namespace: ns,
				Timeout:   MaxWaitTimeout,
			})
			By("Virtual machines should be migrated")
			WaitByLabel(kc.ResourceVM, kc.WaitOptions{
				Labels:    testCaseLabel,
				Namespace: ns,
				Timeout:   MaxWaitTimeout,
				For:       "'jsonpath={.status.migrationState.result}=Succeeded'",
			})
		})

		It("checks VMs external connection after migrations", func() {
			res := kubectl.List(kc.ResourceVM, kc.GetOptions{
				Labels:    testCaseLabel,
				Namespace: ns,
				Output:    "jsonpath='{.items[*].metadata.name}'",
			})
			Expect(res.Error()).NotTo(HaveOccurred(), res.StdErr())

			vms := strings.Split(res.StdOut(), " ")
			Expect(vms).NotTo(BeEmpty())

			CheckCiliumAgents(kubectl, ns, vms...)
			CheckExternalConnection(externalHost, httpStatusOk, ns, vms...)
		})

		It("checks network availability after migrations", func() {
			By("Network condition should be true")
			WaitForVMNetworkReady(kc.WaitOptions{
				Labels:    testCaseLabel,
				Namespace: ns,
				Timeout:   MaxWaitTimeout,
			})

			CheckVMConnectivityToTargetIPs(ns, testCaseLabel)
		})
	})

	Context("When test is completed", func() {
		It("deletes test case resources", func() {
			resourcesToDelete := ResourcesToDelete{
				AdditionalResources: []AdditionalResource{
					{
						Resource: kc.ResourceVMOP,
						Labels:   testCaseLabel,
					},
				},
			}

			if config.IsCleanUpNeeded() {
				resourcesToDelete.KustomizationDir = conf.TestData.VMVpc
			}

			DeleteTestCaseResources(ns, resourcesToDelete)
		})
	})
})

func isSdnModuleEnabled() (bool, error) {
	sdnModule, err := framework.NewFramework("").GetModuleConfig("sdn")
	if err != nil {
		return false, err
	}
	enabled := sdnModule.Spec.Enabled

	return enabled != nil && *enabled, nil
}

func CheckVMConnectivityToTargetIPs(ns string, testCaseLabel map[string]string) {
	var vmList v1alpha2.VirtualMachineList
	err := GetObjects(kc.ResourceVM, &vmList, kc.GetOptions{
		Labels:    testCaseLabel,
		Namespace: ns,
	})
	Expect(err).ShouldNot(HaveOccurred())

	for _, vm := range vmList.Items {
		switch {
		case strings.Contains(vm.Name, "foo"):
			By(fmt.Sprintf("VM %q should have connectivity to 192.168.1.10 (target: vm-bar)", vm.Name))
			CheckResultSSHCommand(ns, vm.Name, `ping -c 2 -W 2 -w 5 -q 192.168.1.10 2>&1 | grep -o "[0-9]\+%\s*packet loss"`, "0% packet loss")
		case strings.Contains(vm.Name, "bar"):
			By(fmt.Sprintf("VM %q should have connectivity to 192.168.1.11 (target: vm-foo)", vm.Name))
			CheckResultSSHCommand(ns, vm.Name, `ping -c 2 -W 2 -w 5 -q 192.168.1.11 2>&1 | grep -o "[0-9]\+%\s*packet loss"`, "0% packet loss")
		}
	}
}
