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

	"github.com/deckhouse/virtualization/tests/e2e/config"
	"github.com/deckhouse/virtualization/tests/e2e/ginkgoutil"
	kc "github.com/deckhouse/virtualization/tests/e2e/kubectl"
)

var _ = Describe("VMCheckTPM", ginkgoutil.CommonE2ETestDecorators(), func() {
	testCaseLabel := map[string]string{"testcase": "vm-check-tpm"}
	var (
		ns     string
		vmName = fmt.Sprintf("%s-vm-check-tpm", namePrefix)
	)
	BeforeAll(func() {
		kustomization := fmt.Sprintf("%s/%s", conf.TestData.VMTpm, "kustomization.yaml")
		var err error
		ns, err = kustomize.GetNamespace(kustomization)
		Expect(err).NotTo(HaveOccurred(), "%w", err)
	})

	It("checks if tpm exists in VM", func() {
		By("checks if vm already exists in cluster. Reusable flag.", func() {
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

			CreateNamespace(ns)
			res := kubectl.Apply(kc.ApplyOptions{
				Filename:       []string{conf.TestData.VMTpm},
				FilenameOption: kc.Kustomize,
			})
			Expect(res.Error()).NotTo(HaveOccurred(), res.StdErr())
		})

		By("waits qemu agent to be ready")
		WaitVMAgentReady(kc.WaitOptions{
			Labels:    testCaseLabel,
			Namespace: ns,
			Timeout:   MaxWaitTimeout,
		})
		By("checks from OS that VM has tpm module version 2.0")
		CheckResultSSHCommand(ns, vmName, `sudo tpm2_getcap properties-fixed | grep -A2 TPM2_PT_FAMILY_INDICATOR | grep value | awk -F"\"" "{print \$2}"`, "2.0")
	})
})
