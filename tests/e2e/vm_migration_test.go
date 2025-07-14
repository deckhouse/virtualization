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

package e2e

import (
	"fmt"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/tests/e2e/config"
	"github.com/deckhouse/virtualization/tests/e2e/ginkgoutil"
	kc "github.com/deckhouse/virtualization/tests/e2e/kubectl"
)

func MigrateVirtualMachines(label map[string]string, virtualMachines ...string) {
	GinkgoHelper()
	CreateAndApplyVMOPs(label, virtv2.VMOPTypeEvict, virtualMachines...)
}

var _ = Describe("Virtual machine migration", SIGMigration(), ginkgoutil.CommonE2ETestDecorators(), func() {
	testCaseLabel := map[string]string{"testcase": "vm-migration"}

	AfterEach(func() {
		if CurrentSpecReport().Failed() {
			SaveTestResources(testCaseLabel, CurrentSpecReport().LeafNodeText)
		}
	})

	Context("Preparing the environment", func() {
		It("sets the namespace", func() {
			kustomization := fmt.Sprintf("%s/%s", conf.TestData.VMMigration, "kustomization.yaml")
			ns, err := kustomize.GetNamespace(kustomization)
			Expect(err).NotTo(HaveOccurred(), "%w", err)
			conf.SetNamespace(ns)
		})
	})

	Context("When resources are applied", func() {
		It("result should be succeeded", func() {
			if config.IsReusable() {
				res := kubectl.List(kc.ResourceVM, kc.GetOptions{
					Labels:    testCaseLabel,
					Namespace: conf.Namespace,
					Output:    "jsonpath='{.items[*].metadata.name}'",
				})
				Expect(res.Error()).NotTo(HaveOccurred(), res.StdErr())

				if res.StdOut() != "" {
					return
				}
			}

			res := kubectl.Apply(kc.ApplyOptions{
				Filename:       []string{conf.TestData.VMMigration},
				FilenameOption: kc.Kustomize,
			})
			Expect(res.WasSuccess()).To(Equal(true), res.StdErr())
		})
	})

	Context("When virtual machines are applied", func() {
		It("checks VMs phases", func() {
			By("Virtual machine agents should be ready")
			WaitVmAgentReady(kc.WaitOptions{
				Labels:    testCaseLabel,
				Namespace: conf.Namespace,
				Timeout:   MaxWaitTimeout,
			})
		})
	})

	Context("When virtual machine agents are ready", func() {
		It("starts migrations", func() {
			res := kubectl.List(kc.ResourceVM, kc.GetOptions{
				Labels:    testCaseLabel,
				Namespace: conf.Namespace,
				Output:    "jsonpath='{.items[*].metadata.name}'",
			})
			Expect(res.WasSuccess()).To(Equal(true), res.StdErr())

			vms := strings.Split(res.StdOut(), " ")
			MigrateVirtualMachines(testCaseLabel, vms...)
		})
	})

	Context("When VMs migrations are applied", func() {
		It("checks VMs and VMOPs phases", func() {
			By(fmt.Sprintf("VMOPs should be in %s phases", virtv2.VMOPPhaseCompleted))
			WaitPhaseByLabel(kc.ResourceVMOP, string(virtv2.VMOPPhaseCompleted), kc.WaitOptions{
				Labels:    testCaseLabel,
				Namespace: conf.Namespace,
				Timeout:   MaxWaitTimeout,
			})
			By("Virtual machines should be migrated")
			WaitByLabel(kc.ResourceVM, kc.WaitOptions{
				Labels:    testCaseLabel,
				Namespace: conf.Namespace,
				Timeout:   MaxWaitTimeout,
				For:       "'jsonpath={.status.migrationState.result}=Succeeded'",
			})
		})

		It("checks VMs external connection after migrations", func() {
			res := kubectl.List(kc.ResourceVM, kc.GetOptions{
				Labels:    testCaseLabel,
				Namespace: conf.Namespace,
				Output:    "jsonpath='{.items[*].metadata.name}'",
			})
			Expect(res.WasSuccess()).To(Equal(true), res.StdErr())

			vms := strings.Split(res.StdOut(), " ")
			CheckCiliumAgents(kubectl, vms...)
			CheckExternalConnection(externalHost, httpStatusOk, vms...)
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
				resourcesToDelete.KustomizationDir = conf.TestData.VMMigration
			}

			DeleteTestCaseResources(resourcesToDelete)
		})
	})
})
