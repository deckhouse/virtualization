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
	d8 "github.com/deckhouse/virtualization/tests/e2e/d8"
	kc "github.com/deckhouse/virtualization/tests/e2e/kubectl"
)

const (
	AutomaticMode = "automatic"
	ManualMode    = "manual"
	StageBefore   = "before"
	StageAfter    = "after"
)

var (
	AutomaticLabel = map[string]string{"vm": "automatic-conf"}
	ManualLabel    = map[string]string{"vm": "manual-conf"}
)

func ExecSshCommand(vmName, cmd string) {
	GinkgoHelper()
	Eventually(func(g Gomega) {
		res := d8Virtualization.SshCommand(vmName, cmd, d8.SshOptions{
			Namespace:   conf.Namespace,
			Username:    conf.TestData.SshUser,
			IdenityFile: conf.TestData.Sshkey,
		})
		g.Expect(res.Error()).NotTo(HaveOccurred(), "execution of SSH command failed for %s/%s.\n%s\n%s", conf.Namespace, vmName, res.StdErr(), key)
	}).WithTimeout(Timeout).WithPolling(Interval).Should(Succeed())
}

func ChangeCPUCoresNumber(namespace string, cpuNumber int, virtualMachines ...string) {
	vms := strings.Join(virtualMachines, " ")
	cmd := fmt.Sprintf("patch %s --namespace %s %s --type merge --patch '{\"spec\":{\"cpu\":{\"cores\":%d}}}'", kc.ResourceVM, conf.Namespace, vms, cpuNumber)
	By("Patching virtual machine specification")
	patchRes := kubectl.RawCommand(cmd, ShortWaitDuration)
	Expect(patchRes.WasSuccess()).To(Equal(true), patchRes.StdErr())
}

func CheckCPUCoresNumber(approvalMode, stage string, requiredValue int, virtualMachines ...string) {
	for _, vm := range virtualMachines {
		By(fmt.Sprintf("Checking the number of processor cores %s changing", stage))
		vmResource := virtv2.VirtualMachine{}
		err := GetObject(kc.ResourceVM, vm, conf.Namespace, &vmResource)
		Expect(err).NotTo(HaveOccurred(), err)
		Expect(vmResource.Spec.CPU.Cores).To(Equal(requiredValue))
		switch {
		case approvalMode == ManualMode && stage == StageAfter:
			Expect(vmResource.Status.RestartAwaitingChanges).ShouldNot(BeNil())
		case approvalMode == AutomaticMode && stage == StageAfter:
			Expect(vmResource.Status.RestartAwaitingChanges).Should(BeNil())
		}
	}
}

func CheckCPUCoresNumberFromVirtualMachine(requiredValue string, virtualMachines ...string) {
	By("Checking the number of processor cores after changing from virtual machine")
	for _, vm := range virtualMachines {
		cmd := "nproc --all"
		CheckResultSshCommand(vm, cmd, requiredValue)
	}
}

var _ = Describe("Virtual machine configuration", Ordered, ContinueOnFailure, func() {
	Context("When resources are applied:", func() {
		It("must have no errors", func() {
			res := kubectl.Kustomize(conf.TestData.VmConfiguration, kc.KustomizeOptions{})
			Expect(res.WasSuccess()).To(Equal(true), res.StdErr())
		})
	})

	Context("When virtual disks are applied:", func() {
		It(fmt.Sprintf("should be in %s phase", PhaseReady), func() {
			WaitPhase(kc.ResourceVD, PhaseReady, kc.GetOptions{
				Labels:    map[string]string{"testcase": "vm-configuration"},
				Namespace: conf.Namespace,
				Output:    "jsonpath='{.items[*].metadata.name}'",
			})
		})
	})

	Context("When virtual machines are applied:", func() {
		It(fmt.Sprintf("should be in %s phase", PhaseRunning), func() {
			WaitPhase(kc.ResourceVM, PhaseRunning, kc.GetOptions{
				Labels:    map[string]string{"testcase": "vm-configuration"},
				Namespace: conf.Namespace,
				Output:    "jsonpath='{.items[*].metadata.name}'",
			})
		})
	})

	Describe("Manual restart approval mode", func() {
		Context(fmt.Sprintf("When virtual machine is in %s phase:", PhaseRunning), func() {
			It("changes the number of processor cores", func() {
				res := kubectl.List(kc.ResourceVM, kc.GetOptions{
					Labels:    ManualLabel,
					Namespace: conf.Namespace,
					Output:    "jsonpath='{.items[*].metadata.name}'",
				})
				Expect(res.WasSuccess()).To(Equal(true), res.StdErr())

				vms := strings.Split(res.StdOut(), " ")
				CheckCPUCoresNumber(ManualMode, StageBefore, 1, vms...)
				ChangeCPUCoresNumber(conf.Namespace, 2, vms...)
			})
		})

		Context("When virtual machine is patched:", func() {
			It("checks the number of processor cores in specification", func() {
				res := kubectl.List(kc.ResourceVM, kc.GetOptions{
					Labels:    ManualLabel,
					Namespace: conf.Namespace,
					Output:    "jsonpath='{.items[*].metadata.name}'",
				})
				Expect(res.WasSuccess()).To(Equal(true), res.StdErr())

				vms := strings.Split(res.StdOut(), " ")
				CheckCPUCoresNumber(ManualMode, StageAfter, 2, vms...)
			})
		})

		Context("When virtual machine is restarted:", func() {
			It(fmt.Sprintf("should be in %s phase", PhaseRunning), func() {
				res := kubectl.List(kc.ResourceVM, kc.GetOptions{
					Labels:    ManualLabel,
					Namespace: conf.Namespace,
					Output:    "jsonpath='{.items[*].metadata.name}'",
				})
				Expect(res.WasSuccess()).To(Equal(true), res.StdErr())

				vms := strings.Split(res.StdOut(), " ")
				for _, vm := range vms {
					cmd := "sudo reboot"
					ExecSshCommand(vm, cmd)
				}
				WaitPhase(kc.ResourceVM, PhaseRunning, kc.GetOptions{
					Labels:    ManualLabel,
					Namespace: conf.Namespace,
					Output:    "jsonpath='{.items[*].metadata.name}'",
				})
			})
		})

		Context(fmt.Sprintf("When virtual machine is in %s phase:", PhaseRunning), func() {
			It("checks that the number of processor cores was changed", func() {
				res := kubectl.List(kc.ResourceVM, kc.GetOptions{
					Labels:    ManualLabel,
					Namespace: conf.Namespace,
					Output:    "jsonpath='{.items[*].metadata.name}'",
				})
				Expect(res.WasSuccess()).To(Equal(true), res.StdErr())

				vms := strings.Split(res.StdOut(), " ")
				CheckCPUCoresNumberFromVirtualMachine("2", vms...)
			})
		})
	})

	Describe("Automatic restart approval mode", func() {
		Context(fmt.Sprintf("When virtual machine is in %s phase:", PhaseRunning), func() {
			It("changes the number of processor cores", func() {
				res := kubectl.List(kc.ResourceVM, kc.GetOptions{
					Labels:    AutomaticLabel,
					Namespace: conf.Namespace,
					Output:    "jsonpath='{.items[*].metadata.name}'",
				})
				Expect(res.WasSuccess()).To(Equal(true), res.StdErr())

				vms := strings.Split(res.StdOut(), " ")
				CheckCPUCoresNumber(AutomaticMode, StageBefore, 1, vms...)
				ChangeCPUCoresNumber(conf.Namespace, 2, vms...)
			})
		})

		Context("When virtual machine is patched:", func() {
			It("checks the number of processor cores in specification", func() {
				res := kubectl.List(kc.ResourceVM, kc.GetOptions{
					Labels:    AutomaticLabel,
					Namespace: conf.Namespace,
					Output:    "jsonpath='{.items[*].metadata.name}'",
				})
				Expect(res.WasSuccess()).To(Equal(true), res.StdErr())

				vms := strings.Split(res.StdOut(), " ")
				CheckCPUCoresNumber(AutomaticMode, StageAfter, 2, vms...)
			})
		})

		Context("When virtual machine is restarted:", func() {
			It(fmt.Sprintf("should be in %s phase", PhaseRunning), func() {
				WaitPhase(kc.ResourceVM, PhaseRunning, kc.GetOptions{
					Labels:    AutomaticLabel,
					Namespace: conf.Namespace,
					Output:    "jsonpath='{.items[*].metadata.name}'",
				})
			})
		})

		Context(fmt.Sprintf("When virtual machine is in %s phase:", PhaseRunning), func() {
			It("checks that the number of processor cores was changed", func() {
				res := kubectl.List(kc.ResourceVM, kc.GetOptions{
					Labels:    AutomaticLabel,
					Namespace: conf.Namespace,
					Output:    "jsonpath='{.items[*].metadata.name}'",
				})
				Expect(res.WasSuccess()).To(Equal(true), res.StdErr())

				vms := strings.Split(res.StdOut(), " ")
				CheckCPUCoresNumberFromVirtualMachine("2", vms...)
			})
		})
	})
})
