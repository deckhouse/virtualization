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

	// . "github.com/deckhouse/virtualization/tests/e2e/helper"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	kc "github.com/deckhouse/virtualization/tests/e2e/kubectl"
)

var _ = Describe("Virtual machine versions", ginkgoutil.CommonE2ETestDecorators(), func() {
	BeforeEach(func() {
		if config.IsReusable() {
			Skip("Test not available in REUSABLE mode: not supported yet.")
		}
	})

	testCaseLabel := map[string]string{"testcase": "vm-versions"}

	Context("Preparing the environment", func() {
		It("sets the namespace", func() {
			kustomization := fmt.Sprintf("%s/%s", conf.TestData.VmVersions, "kustomization.yaml")
			ns, err := kustomize.GetNamespace(kustomization)
			Expect(err).NotTo(HaveOccurred(), "%w", err)
			conf.SetNamespace(ns)
		})
	})

	Context("When virtualization resources are applied:", func() {
		It("result should be succeeded", func() {
			res := kubectl.Apply(kc.ApplyOptions{
				Filename:       []string{conf.TestData.VmVersions},
				FilenameOption: kc.Kustomize,
			})
			Expect(res.Error()).NotTo(HaveOccurred(), "cmd: %s\nstderr: %s", res.GetCmd(), res.StdErr())
		})
	})

	Context("When virtual images are applied:", func() {
		It("checks VIs phases", func() {
			By(fmt.Sprintf("VIs should be in %s phase", PhaseReady))
			WaitPhaseByLabel(kc.ResourceVI, PhaseReady, kc.WaitOptions{
				Labels:    testCaseLabel,
				Namespace: conf.Namespace,
				Timeout:   MaxWaitTimeout,
			})
		})
	})

	Context("When virtual machines are applied:", func() {
		It("checks VMs phases", func() {
			By(fmt.Sprintf("VM should be in %s phase", PhaseRunning))
			WaitPhaseByLabel(kc.ResourceVM, PhaseRunning, kc.WaitOptions{
				Labels:    testCaseLabel,
				Namespace: conf.Namespace,
				Timeout:   MaxWaitTimeout,
			})
		})
	})

	Context("When virtual machines are ready:", func() {
		Eventually(func() error {
			var vms virtv2.VirtualMachineList
			err := GetObjects(kc.ResourceVM, &vms, kc.GetOptions{
				Labels:    testCaseLabel,
				Namespace: conf.Namespace,
			})
			Expect(err).NotTo(HaveOccurred())

			It("has qemu version is status", func() {
				for _, vm := range vms.Items {
					Expect(vm.Status.Versions.Qemu).NotTo(BeEmpty())
				}
			})

			It("has libvirt version is status", func() {
				for _, vm := range vms.Items {
					Expect(vm.Status.Versions.Libvirt).NotTo(BeEmpty())
				}
			})

			return nil
		}).WithTimeout(Timeout).WithPolling(Interval).Should(Succeed())
	})

	Context("When test is completed", func() {
		It("deletes test case resources", func() {
			DeleteTestCaseResources(ResourcesToDelete{
				KustomizationDir: conf.TestData.VmVersions,
			})
		})
	})
})
