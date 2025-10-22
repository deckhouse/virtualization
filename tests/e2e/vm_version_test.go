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
)

var _ = Describe("VirtualMachineVersions", framework.CommonE2ETestDecorators(), func() {
	testCaseLabel := map[string]string{"testcase": "vm-versions"}
	var ns string

	BeforeAll(func() {
		kustomization := fmt.Sprintf("%s/%s", conf.TestData.VMVersions, "kustomization.yaml")
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

	Context("When virtualization resources are applied:", func() {
		It("result should be succeeded", func() {
			res := kubectl.Apply(kc.ApplyOptions{
				Filename:       []string{conf.TestData.VMVersions},
				FilenameOption: kc.Kustomize,
			})
			Expect(res.Error()).NotTo(HaveOccurred(), "cmd: %s\nstderr: %s", res.GetCmd(), res.StdErr())
		})
	})

	Context("When virtual disks are applied:", func() {
		It("checks VDs phases", func() {
			By(fmt.Sprintf("VDs should be in %s phase", PhaseReady))
			WaitPhaseByLabel(kc.ResourceVD, PhaseReady, kc.WaitOptions{
				Labels:    testCaseLabel,
				Namespace: ns,
				Timeout:   MaxWaitTimeout,
			})
		})
	})

	Context("When virtual machines are applied:", func() {
		It("checks VMs phases", func() {
			By(fmt.Sprintf("VM should be in %s phase", PhaseRunning))
			WaitPhaseByLabel(kc.ResourceVM, PhaseRunning, kc.WaitOptions{
				Labels:    testCaseLabel,
				Namespace: ns,
				Timeout:   MaxWaitTimeout,
			})
		})
	})

	Context("When virtual machines are ready:", func() {
		Eventually(func() error {
			var vms v1alpha2.VirtualMachineList
			err := GetObjects(kc.ResourceVM, &vms, kc.GetOptions{
				Labels:    testCaseLabel,
				Namespace: ns,
			})
			Expect(err).NotTo(HaveOccurred())

			It("has qemu version in the status", func() {
				for _, vm := range vms.Items {
					Expect(vm.Status.Versions.Qemu).NotTo(BeEmpty())
				}
			})

			It("has libvirt version in the status", func() {
				for _, vm := range vms.Items {
					Expect(vm.Status.Versions.Libvirt).NotTo(BeEmpty())
				}
			})

			return nil
		}).WithTimeout(Timeout).WithPolling(Interval).Should(Succeed())
	})

	Context("When test is completed", func() {
		It("deletes test case resources", func() {
			DeleteTestCaseResources(ns, ResourcesToDelete{
				KustomizationDir: conf.TestData.VMVersions,
			})
		})
	})
})
