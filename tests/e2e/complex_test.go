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
	"github.com/deckhouse/virtualization/tests/e2e/ginkgoutil"
	kc "github.com/deckhouse/virtualization/tests/e2e/kubectl"
)

func AssignIPToVMIP(name string) error {
	assignErr := fmt.Sprintf("cannot patch VMIP %q with unnassigned IP address", name)
	unassignedIP, err := FindUnassignedIP(mc.Spec.Settings.VirtualMachineCIDRs)
	if err != nil {
		return fmt.Errorf("%s\n%s", assignErr, err)
	}
	patch := fmt.Sprintf("{\"spec\":{\"staticIP\":%q}}", unassignedIP)
	err = MergePatchResource(kc.ResourceVMIP, name, patch)
	if err != nil {
		return fmt.Errorf("%s\n%s", assignErr, err)
	}
	vmip := virtv2.VirtualMachineIPAddress{}
	err = GetObject(kc.ResourceVMIP, name, &vmip, kc.GetOptions{
		Namespace: conf.Namespace,
	})
	if err != nil {
		return fmt.Errorf("%s\n%s", assignErr, err)
	}
	jsonPath := fmt.Sprintf("'jsonpath={.status.phase}=%s'", PhaseAttached)
	waitOpts := kc.WaitOptions{
		Namespace: conf.Namespace,
		For:       jsonPath,
		Timeout:   30,
	}
	res := kubectl.WaitResources(kc.ResourceVMIP, waitOpts, name)
	if res.Error() != nil {
		return fmt.Errorf("%s\n%s", assignErr, res.StdErr())
	}
	return nil
}

var _ = Describe("Complex test", ginkgoutil.CommonE2ETestDecorators(), func() {
	var (
		testCaseLabel      = map[string]string{"testcase": "complex-test"}
		hasNoConsumerLabel = map[string]string{"hasNoConsumer": "complex-test"}
	)

	Context("When virtualization resources are applied:", func() {
		It("result should be succeeded", func() {
			res := kubectl.Kustomize(conf.TestData.ComplexTest, kc.KustomizeOptions{})
			Expect(res.WasSuccess()).To(Equal(true), res.StdErr())
		})
	})

	Context("When virtual images are applied:", func() {
		It("checks VIs phases", func() {
			By(fmt.Sprintf("VIs should be in %s phases", PhaseReady))
			WaitPhase(kc.ResourceVI, PhaseReady, kc.GetOptions{
				Labels:    testCaseLabel,
				Namespace: conf.Namespace,
				Output:    "jsonpath='{.items[*].metadata.name}'",
			})
		})
	})

	Context("When cluster virtual images are applied:", func() {
		It("checks CVIs phases", func() {
			By(fmt.Sprintf("CVIs should be in %s phases", PhaseReady))
			WaitPhase(kc.ResourceCVI, PhaseReady, kc.GetOptions{
				Labels:    testCaseLabel,
				Namespace: conf.Namespace,
				Output:    "jsonpath='{.items[*].metadata.name}'",
			})
		})
	})

	Context("When virtual machine classes are applied:", func() {
		It("checks VMClasses phases", func() {
			By(fmt.Sprintf("VMClasses should be in %s phases", PhaseReady))
			WaitPhase(kc.ResourceVMClass, PhaseReady, kc.GetOptions{
				Labels:    testCaseLabel,
				Namespace: conf.Namespace,
				Output:    "jsonpath='{.items[*].metadata.name}'",
			})
		})
	})

	Context("When virtual machines IP addresses are applied:", func() {
		It("patches custom VMIP with unassigned address", func() {
			vmipName := fmt.Sprintf("%s-%s", namePrefix, "vm-custom-ip")
			Eventually(func() error {
				return AssignIPToVMIP(vmipName)
			}).Should(Succeed())
		})

		It("checks VMIPs phases", func() {
			By(fmt.Sprintf("VMIPs should be in %s phases", PhaseAttached))
			WaitPhase(kc.ResourceVMIP, PhaseAttached, kc.GetOptions{
				Labels:    testCaseLabel,
				Namespace: conf.Namespace,
				Output:    "jsonpath='{.items[*].metadata.name}'",
			})
		})
	})

	Context("When virtual disks are applied:", func() {
		It("checks VDs phases with consumers", func() {
			By(fmt.Sprintf("VDs should be in %s phases", PhaseReady))
			WaitPhase(kc.ResourceVD, PhaseReady, kc.GetOptions{
				ExcludeLabels: []string{"hasNoConsumer"},
				Labels:        testCaseLabel,
				Namespace:     conf.Namespace,
				Output:        "jsonpath='{.items[*].metadata.name}'",
			})
		})

		It("checks VDs phases with no consumers", func() {
			By(fmt.Sprintf("VDs should be in %s phases", phaseByVolumeBindingMode))
			WaitPhase(kc.ResourceVD, phaseByVolumeBindingMode, kc.GetOptions{
				Labels:    hasNoConsumerLabel,
				Namespace: conf.Namespace,
				Output:    "jsonpath='{.items[*].metadata.name}'",
			})
		})
	})

	Context("When virtual machines are applied:", func() {
		It("checks VMs phases", func() {
			By(fmt.Sprintf("VMs should be in %s phases", PhaseRunning))
			WaitPhase(kc.ResourceVM, PhaseRunning, kc.GetOptions{
				Labels:    testCaseLabel,
				Namespace: conf.Namespace,
				Output:    "jsonpath='{.items[*].metadata.name}'",
			})
		})
	})

	Context("When virtual machine block device attachments are applied:", func() {
		It("checks VMBDAs phases", func() {
			By(fmt.Sprintf("VMBDAs should be in %s phases", PhaseAttached))
			WaitPhase(kc.ResourceVMBDA, PhaseAttached, kc.GetOptions{
				Labels:    testCaseLabel,
				Namespace: conf.Namespace,
				Output:    "jsonpath='{.items[*].metadata.name}'",
			})
		})
	})

	Describe("External connection", func() {
		Context(fmt.Sprintf("When VMs are in %s phases", PhaseRunning), func() {
			It("checks VMs external connectivity", func() {
				res := kubectl.List(kc.ResourceVM, kc.GetOptions{
					Labels:    testCaseLabel,
					Namespace: conf.Namespace,
					Output:    "jsonpath='{.items[*].metadata.name}'",
				})
				Expect(res.WasSuccess()).To(Equal(true), res.StdErr())

				vms := strings.Split(res.StdOut(), " ")
				CheckExternalConnection(externalHost, httpStatusOk, vms...)
			})
		})
	})

	Describe("Migrations", func() {
		Context(fmt.Sprintf("When VMs are in %s phases:", PhaseRunning), func() {
			It("starts migrations", func() {
				res := kubectl.List(kc.ResourceVM, kc.GetOptions{
					Labels:    testCaseLabel,
					Namespace: conf.Namespace,
					Output:    "jsonpath='{.items[*].metadata.name}'",
				})
				Expect(res.WasSuccess()).To(Equal(true), res.StdErr())

				vms := strings.Split(res.StdOut(), " ")
				MigrateVirtualMachines(testCaseLabel, conf.TestData.ComplexTest, vms...)
			})
		})

		Context("When VMs migrations are applied:", func() {
			It("checks VMs and KubevirtVMIMs phases", func() {
				By(fmt.Sprintf("KubevirtVMIMs should be in %s phases", PhaseSucceeded))
				WaitPhase(kc.ResourceKubevirtVMIM, PhaseSucceeded, kc.GetOptions{
					Labels:    testCaseLabel,
					Namespace: conf.Namespace,
					Output:    "jsonpath='{.items[*].metadata.name}'",
				})
				By(fmt.Sprintf("VMs should be in %s phase", PhaseRunning))
				WaitPhase(kc.ResourceVM, PhaseRunning, kc.GetOptions{
					Labels:    testCaseLabel,
					Namespace: conf.Namespace,
					Output:    "jsonpath='{.items[*].metadata.name}'",
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
				CheckExternalConnection(externalHost, httpStatusOk, vms...)
			})
		})
	})
})
