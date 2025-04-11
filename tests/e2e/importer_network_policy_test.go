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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/tests/e2e/config"
	"github.com/deckhouse/virtualization/tests/e2e/ginkgoutil"
	kc "github.com/deckhouse/virtualization/tests/e2e/kubectl"
)

var _ = Describe("Importer network policy", ginkgoutil.CommonE2ETestDecorators(), func() {
	testCaseLabel := map[string]string{"testcase": "importer-network-policy"}

	AfterAll(func() {
		By("Delete manifests")
		DeleteTestCaseResources(ResourcesToDelete{KustomizationDir: conf.TestData.ImporterNetworkPolicy})
	})

	BeforeEach(func() {
		if config.IsReusable() {
			Skip("Test not available in REUSABLE mode: not supported yet.")
		}
	})

	AfterEach(func() {
		if CurrentSpecReport().Failed() {
			SaveTestResources(testCaseLabel, CurrentSpecReport().LeafNodeText)
		}
	})

	Context("Preparing the environment", func() {
		It("sets the namespace", func() {
			kustomization := fmt.Sprintf("%s/%s", conf.TestData.ImporterNetworkPolicy, "kustomization.yaml")
			ns, err := kustomize.GetNamespace(kustomization)
			Expect(err).NotTo(HaveOccurred(), "%w", err)
			conf.SetNamespace(ns)
		})

		It("project apply", func() {
			config.PrepareProject(conf.TestData.ImporterNetworkPolicy)

			res := kubectl.Apply(kc.ApplyOptions{
				Filename:       []string{conf.TestData.ImporterNetworkPolicy + "/project"},
				FilenameOption: kc.Kustomize,
			})
			Expect(res.WasSuccess()).To(Equal(true), res.StdErr())
		})

		It("checks project readiness", func() {
			By("Project should be deployed")
			WaitByLabel(kc.ResourceProject, kc.WaitOptions{
				Labels:  testCaseLabel,
				Timeout: MaxWaitTimeout,
				For:     "'jsonpath={.status.state}=Deployed'",
			})
		})
	})

	Context("When resources are applied", func() {
		It("result should be succeeded", func() {
			res := kubectl.Apply(kc.ApplyOptions{
				Filename:       []string{conf.TestData.ImporterNetworkPolicy},
				FilenameOption: kc.Kustomize,
			})
			Expect(res.WasSuccess()).To(Equal(true), res.StdErr())
		})
	})

	DescribeTable("When resources are applied",
		func(resourceShortName string, resource kc.Resource, phase string) {
			By(fmt.Sprintf("%ss should be in %s phases", resourceShortName, phase))
			WaitPhaseByLabel(resource, phase, kc.WaitOptions{
				Labels:    testCaseLabel,
				Namespace: conf.Namespace,
				Timeout:   MaxWaitTimeout,
			})
		},
		Entry("When virtual images are applied", "VI", kc.ResourceVI, string(virtv2.ImageReady)),
		Entry("When virtual disks are applied", "VD", kc.ResourceVD, string(virtv2.DiskReady)),
		Entry("When virtual machines are applied", "VM", kc.ResourceVM, string(virtv2.MachineRunning)),
	)
})
