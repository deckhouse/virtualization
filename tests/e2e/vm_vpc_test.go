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
	"fmt"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
	"github.com/deckhouse/virtualization/tests/e2e/config"
	"github.com/deckhouse/virtualization/tests/e2e/ginkgoutil"
	kc "github.com/deckhouse/virtualization/tests/e2e/kubectl"
)

func WaitVMNetworkReady(opts kc.WaitOptions) {
	GinkgoHelper()
	WaitPhaseByLabel(kc.ResourceVM, PhaseRunning, opts)
	WaitConditionIsTrueByLabel(kc.ResourceVM, vmcondition.TypeNetworkReady.String(), opts)
}

var _ = Describe("VirtualMachineAdditionalNetworkInterfaces", SIGMigration(), ginkgoutil.CommonE2ETestDecorators(), func() {
	testCaseLabel := map[string]string{"testcase": "vm-vpc"}
	var ns string

	BeforeAll(func() {
		sbdEnabled, err := isSdnModuleEnabled()
		if err != nil || !sbdEnabled {
			Skip("Module SDN is disabled. Skipping all tests for module SDN.")
		}

		kustomization := fmt.Sprintf("%s/%s", conf.TestData.VMVpc, "kustomization.yaml")
		ns, err = kustomize.GetNamespace(kustomization)
		Expect(err).NotTo(HaveOccurred(), "%w", err)
	})

	AfterAll(func() {
		if CurrentSpecReport().Failed() {
			SaveTestResources(testCaseLabel, CurrentSpecReport().LeafNodeText)
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
				Filename:       []string{conf.TestData.VMVpc},
				FilenameOption: kc.Kustomize,
			})
			Expect(res.WasSuccess()).To(Equal(true), res.StdErr())
		})
	})

	Context("When virtual machines are applied", func() {
		It("checks VMs phases", func() {
			By("Virtual machine agents should be ready")
			WaitVMAgentReady(kc.WaitOptions{
				Labels:    testCaseLabel,
				Namespace: ns,
				Timeout:   MaxWaitTimeout,
			})
		})
		It("checks network status", func() {
			By("Network should be true")
			WaitVMNetworkReady(kc.WaitOptions{
				Labels:    testCaseLabel,
				Namespace: ns,
				Timeout:   MaxWaitTimeout,
			})
		})
	})

	Context("When virtual machine agents and network are ready", func() {
		It("starts migrations", func() {
			res := kubectl.List(kc.ResourceVM, kc.GetOptions{
				Labels:    testCaseLabel,
				Namespace: ns,
				Output:    "jsonpath='{.items[*].metadata.name}'",
			})
			Expect(res.WasSuccess()).To(Equal(true), res.StdErr())

			vms := strings.Split(res.StdOut(), " ")
			MigrateVirtualMachines(testCaseLabel, ns, vms...)
		})
	})

	Context("When VMs migrations are applied", func() {
		It("checks VMs and VMOPs phases", func() {
			By(fmt.Sprintf("VMOPs should be in %s phases", virtv2.VMOPPhaseCompleted))
			WaitPhaseByLabel(kc.ResourceVMOP, string(virtv2.VMOPPhaseCompleted), kc.WaitOptions{
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
			Expect(res.WasSuccess()).To(Equal(true), res.StdErr())

			vms := strings.Split(res.StdOut(), " ")
			CheckCiliumAgents(kubectl, ns, vms...)
			CheckExternalConnection(externalHost, httpStatusOk, ns, vms...)
		})

		It("checks network status after migrations", func() {
			By("Network should be true")
			WaitVMAgentReady(kc.WaitOptions{
				Labels:    testCaseLabel,
				Namespace: ns,
				Timeout:   MaxWaitTimeout,
			})
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

			Eventually(func() error {
				return tryDeleteTestCaseResources(ns, resourcesToDelete)
			}).WithTimeout(LongWaitDuration).WithPolling(Interval).Should(Succeed())
		})
	})
})

func tryDeleteTestCaseResources(ns string, resources ResourcesToDelete) error {
	const errMessage = "cannot delete test case resources"

	if resources.KustomizationDir != "" {
		kustomizationFile := fmt.Sprintf("%s/%s", resources.KustomizationDir, "kustomization.yaml")
		err := kustomize.ExcludeResource(kustomizationFile, "ns.yaml")
		if err != nil {
			return fmt.Errorf("%s\nkustomizationDir: %s\nstderr: %w", errMessage, resources.KustomizationDir, err)
		}

		res := kubectl.Delete(kc.DeleteOptions{
			Filename:       []string{resources.KustomizationDir},
			FilenameOption: kc.Kustomize,
			IgnoreNotFound: true,
		})
		if res.Error() != nil {
			return fmt.Errorf("%s\nkustomizationDir: %s\ncmd: %s\nstderr: %s", errMessage, resources.KustomizationDir, res.GetCmd(), res.StdErr())
		}
	}

	for _, r := range resources.AdditionalResources {
		res := kubectl.Delete(kc.DeleteOptions{
			Labels:    r.Labels,
			Namespace: ns,
			Resource:  r.Resource,
		})
		if res.Error() != nil {
			return fmt.Errorf("%s\ncmd: %s\nstderr: %s", errMessage, res.GetCmd(), res.StdErr())
		}
	}

	if len(resources.Files) != 0 {
		res := kubectl.Delete(kc.DeleteOptions{
			Filename:       resources.Files,
			FilenameOption: kc.Filename,
		})
		if res.Error() != nil {
			return fmt.Errorf("%s\ncmd: %s\nstderr: %s", errMessage, res.GetCmd(), res.StdErr())
		}
	}

	return nil
}

func isSdnModuleEnabled() (bool, error) {
	sdnModule, err := config.GetModuleConfig("sdn")
	if err != nil {
		return false, err
	}

	return sdnModule.Spec.Enabled, nil
}
