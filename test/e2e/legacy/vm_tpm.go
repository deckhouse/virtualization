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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	kc "github.com/deckhouse/virtualization/test/e2e/internal/kubectl"
)

var _ = Describe("VMCheckTPM", Ordered, func() {
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

		CreateNamespace(ns)
		res := kubectl.Apply(kc.ApplyOptions{
			Filename:       []string{conf.TestData.VMTpm},
			FilenameOption: kc.Kustomize,
		})
		Expect(res.Error()).NotTo(HaveOccurred(), res.StdErr())
	})

	It("Checks if tpm exists in VM", func() {
		By("Waits qemu agent to be ready")
		WaitVMAgentReady(kc.WaitOptions{
			Labels:    testCaseLabel,
			Namespace: ns,
			Timeout:   MaxWaitTimeout,
		})
		By("Checks from OS that VM has tpm module version 2.0")
		CheckResultSSHCommand(ns, vmName, `sudo tpm2_getcap properties-fixed | grep -A2 TPM2_PT_FAMILY_INDICATOR | grep value | awk -F"\"" "{print \$2}"`, "2.0")
	})
})
