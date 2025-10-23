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

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/tests/e2e/framework"
	kc "github.com/deckhouse/virtualization/tests/e2e/kubectl"
	"github.com/deckhouse/virtualization/tests/e2e/util"
)

var _ = Describe("ImporterNetworkPolicy", framework.CommonE2ETestDecorators(), func() {
	testCaseLabel := map[string]string{"testcase": "importer-network-policy"}
	var ns string

	BeforeAll(func() {
		kustomization := fmt.Sprintf("%s/%s", conf.TestData.ImporterNetworkPolicy, "kustomization.yaml")
		var err error
		ns, err = kustomize.GetNamespace(kustomization)
		Expect(err).NotTo(HaveOccurred(), "%w", err)
	})

	AfterAll(func() {
		By("Delete manifests")
		DeleteTestCaseResources(ns, ResourcesToDelete{KustomizationDir: conf.TestData.ImporterNetworkPolicy})
	})

	AfterEach(func() {
		if CurrentSpecReport().Failed() {
			SaveTestCaseDump(testCaseLabel, CurrentSpecReport().LeafNodeText, ns)
		}
	})

	Context("Project", func() {
		It("creates project", func() {
			//nolint:staticcheck // deprecated function is temporarily used
			util.PrepareProject(conf.TestData.ImporterNetworkPolicy, conf.StorageClass.TemplateStorageClass)

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
				Namespace: ns,
				Timeout:   MaxWaitTimeout,
			})
		},
		Entry("When virtual images are applied", "VI", kc.ResourceVI, string(v1alpha2.ImageReady)),
		Entry("When virtual disks are applied", "VD", kc.ResourceVD, string(v1alpha2.DiskReady)),
		Entry("When virtual machines are applied", "VM", kc.ResourceVM, string(v1alpha2.MachineRunning)),
	)
})
