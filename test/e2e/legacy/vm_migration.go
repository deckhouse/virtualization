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

package legacy

import (
	"fmt"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/test/e2e/internal/config"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	kc "github.com/deckhouse/virtualization/test/e2e/internal/kubectl"
)

var _ = Describe("VirtualMachineMigration", framework.CommonE2ETestDecorators(), func() {
	testCaseLabel := map[string]string{"testcase": "vm-migration"}
	var ns string

	BeforeAll(func() {
		kustomization := fmt.Sprintf("%s/%s", conf.TestData.VMMigration, "kustomization.yaml")
		var err error
		ns, err = kustomize.GetNamespace(kustomization)
		Expect(err).NotTo(HaveOccurred(), "%w", err)

		CreateNamespace(ns)
	})

	AfterEach(func() {
		if CurrentSpecReport().Failed() {
			SaveTestCaseDump(testCaseLabel, CurrentSpecReport().LeafNodeText, ns)
		}
	})

	Context("When resources are applied", func() {
		It("result should be succeeded", func() {
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
			WaitVMAgentReady(kc.WaitOptions{
				Labels:    testCaseLabel,
				Namespace: ns,
				Timeout:   MaxWaitTimeout,
			})
		})
	})

	Context("When virtual machine agents are ready", func() {
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
			Expect(res.WasSuccess()).To(Equal(true), res.StdErr())

			vms := strings.Split(res.StdOut(), " ")
			CheckCiliumAgents(kubectl, ns, vms...)
			CheckExternalConnection(externalHost, httpStatusOk, ns, vms...)
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

			DeleteTestCaseResources(ns, resourcesToDelete)
		})
	})
})

func MigrateVirtualMachines(label map[string]string, vmNamespace string, vmNames ...string) {
	GinkgoHelper()
	CreateAndApplyVMOPs(label, v1alpha2.VMOPTypeEvict, vmNamespace, vmNames...)
}
