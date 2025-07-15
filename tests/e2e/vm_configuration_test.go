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
	"strconv"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/tests/e2e/config"
	d8 "github.com/deckhouse/virtualization/tests/e2e/d8"
	"github.com/deckhouse/virtualization/tests/e2e/ginkgoutil"
	kc "github.com/deckhouse/virtualization/tests/e2e/kubectl"
)

const (
	AutomaticMode = "automatic"
	ManualMode    = "manual"
	StageBefore   = "before"
	StageAfter    = "after"
)

func ExecSSHCommand(vmName, cmd string) {
	GinkgoHelper()

	Eventually(func() error {
		res := d8Virtualization.SSHCommand(vmName, cmd, d8.SSHOptions{
			Namespace:   conf.Namespace,
			Username:    conf.TestData.SSHUser,
			IdenityFile: conf.TestData.Sshkey,
		})
		if res.Error() != nil {
			return fmt.Errorf("cmd: %s\nstderr: %s", res.GetCmd(), res.StdErr())
		}
		return nil
	}).WithTimeout(Timeout).WithPolling(Interval).ShouldNot(HaveOccurred())
}

func ExecStartCommand(vmName string) {
	GinkgoHelper()
	Eventually(func() error {
		res := d8Virtualization.StartVM(vmName, d8.SSHOptions{Namespace: conf.Namespace})
		if res.Error() != nil {
			return fmt.Errorf("cmd: %s\nstderr: %s", res.GetCmd(), res.StdErr())
		}
		return nil
	}).WithTimeout(Timeout).WithPolling(Interval).ShouldNot(HaveOccurred())
}

func ExecStopCommand(vmName string) {
	GinkgoHelper()
	Eventually(func() error {
		res := d8Virtualization.StopVM(vmName, d8.SSHOptions{Namespace: conf.Namespace})
		if res.Error() != nil {
			return fmt.Errorf("cmd: %s\nstderr: %s", res.GetCmd(), res.StdErr())
		}
		return nil
	}).WithTimeout(Timeout).WithPolling(Interval).ShouldNot(HaveOccurred())
}

func ExecRestartCommand(vmName string) {
	GinkgoHelper()
	Eventually(func() error {
		res := d8Virtualization.RestartVM(vmName, d8.SSHOptions{Namespace: conf.Namespace})
		if res.Error() != nil {
			return fmt.Errorf("cmd: %s\nstderr: %s", res.GetCmd(), res.StdErr())
		}
		return nil
	}).WithTimeout(Timeout).WithPolling(Interval).ShouldNot(HaveOccurred())
}

func ChangeCPUCoresNumber(cpuNumber int, virtualMachines ...string) {
	vms := strings.Join(virtualMachines, " ")
	cmd := fmt.Sprintf("patch %s --namespace %s %s --type merge --patch '{\"spec\":{\"cpu\":{\"cores\":%d}}}'", kc.ResourceVM, conf.Namespace, vms, cpuNumber)
	By("Patching virtual machine specification")
	patchRes := kubectl.RawCommand(cmd, ShortWaitDuration)
	Expect(patchRes.Error()).NotTo(HaveOccurred(), patchRes.StdErr())
}

func CheckCPUCoresNumber(approvalMode, stage string, requiredValue int, virtualMachines ...string) {
	for _, vm := range virtualMachines {
		By(fmt.Sprintf("Checking the number of processor cores %s changing", stage))
		vmResource := virtv2.VirtualMachine{}
		err := GetObject(kc.ResourceVM, vm, &vmResource, kc.GetOptions{Namespace: conf.Namespace})
		Expect(err).NotTo(HaveOccurred(), "%v", err)
		Expect(vmResource.Spec.CPU.Cores).To(Equal(requiredValue))
		switch {
		case approvalMode == ManualMode && stage == StageAfter:
			Expect(vmResource.Status.RestartAwaitingChanges).ShouldNot(BeNil())
		case approvalMode == AutomaticMode && stage == StageAfter:
			Expect(vmResource.Status.RestartAwaitingChanges).ShouldNot(BeNil())
		}
	}
}

func CheckCPUCoresNumberFromVirtualMachine(requiredValue string, virtualMachines ...string) {
	By("Checking the number of processor cores after changing from virtual machine")
	for _, vm := range virtualMachines {
		cmd := "nproc --all"
		CheckResultSSHCommand(vm, cmd, requiredValue)
	}
}

var _ = Describe("Virtual machine configuration", ginkgoutil.CommonE2ETestDecorators(), func() {
	var (
		testCaseLabel  = map[string]string{"testcase": "vm-configuration"}
		automaticLabel = map[string]string{"vm": "automatic-conf"}
		manualLabel    = map[string]string{"vm": "manual-conf"}
	)

	AfterEach(func() {
		if CurrentSpecReport().Failed() {
			SaveTestResources(testCaseLabel, CurrentSpecReport().LeafNodeText)
		}
	})

	Context("Preparing the environment", func() {
		It("sets the namespace", func() {
			kustomization := fmt.Sprintf("%s/%s", conf.TestData.VMConfiguration, "kustomization.yaml")
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
				Filename:       []string{conf.TestData.VMConfiguration},
				FilenameOption: kc.Kustomize,
			})
			Expect(res.WasSuccess()).To(Equal(true), res.StdErr())
		})
	})

	Context("When virtual images are applied", func() {
		It("checks VIs phases", func() {
			By(fmt.Sprintf("VIs should be in %s phases", PhaseReady))
			WaitPhaseByLabel(kc.ResourceVI, PhaseReady, kc.WaitOptions{
				Labels:    testCaseLabel,
				Namespace: conf.Namespace,
				Timeout:   MaxWaitTimeout,
			})
		})
	})

	Context("When virtual disks are applied", func() {
		It(fmt.Sprintf("should be in %s phase", PhaseReady), func() {
			WaitPhaseByLabel(kc.ResourceVD, PhaseReady, kc.WaitOptions{
				Labels:    testCaseLabel,
				Namespace: conf.Namespace,
				Timeout:   MaxWaitTimeout,
			})
		})
	})

	Context("When virtual machines are applied", func() {
		It("should be ready", func() {
			WaitVMAgentReady(kc.WaitOptions{
				Labels:    testCaseLabel,
				Namespace: conf.Namespace,
				Timeout:   MaxWaitTimeout,
			})
		})
	})

	Describe("Manual restart approval mode", func() {
		var oldCpuCores int
		var newCPUCores int

		Context("When virtual machine agents are ready", func() {
			It("changes the number of processor cores", func() {
				res := kubectl.List(kc.ResourceVM, kc.GetOptions{
					Labels:    manualLabel,
					Namespace: conf.Namespace,
					Output:    "jsonpath='{.items[*].metadata.name}'",
				})
				Expect(res.Error()).NotTo(HaveOccurred(), res.StdErr())

				vms := strings.Split(res.StdOut(), " ")
				Expect(vms).NotTo(BeEmpty())

				vmResource := virtv2.VirtualMachine{}
				err := GetObject(kc.ResourceVM, vms[0], &vmResource, kc.GetOptions{Namespace: conf.Namespace})
				Expect(err).NotTo(HaveOccurred())

				oldCpuCores = vmResource.Spec.CPU.Cores
				newCPUCores = 1 + (vmResource.Spec.CPU.Cores & 1)

				CheckCPUCoresNumber(ManualMode, StageBefore, oldCpuCores, vms...)
				ChangeCPUCoresNumber(newCPUCores, vms...)
			})
		})

		Context("When virtual machine is patched", func() {
			It("checks the number of processor cores in specification", func() {
				res := kubectl.List(kc.ResourceVM, kc.GetOptions{
					Labels:    manualLabel,
					Namespace: conf.Namespace,
					Output:    "jsonpath='{.items[*].metadata.name}'",
				})
				Expect(res.WasSuccess()).To(Equal(true), res.StdErr())

				vms := strings.Split(res.StdOut(), " ")
				CheckCPUCoresNumber(ManualMode, StageAfter, newCPUCores, vms...)
			})
		})

		Context("When virtual machine is restarted", func() {
			It("should be ready", func() {
				res := kubectl.List(kc.ResourceVM, kc.GetOptions{
					Labels:    manualLabel,
					Namespace: conf.Namespace,
					Output:    "jsonpath='{.items[*].metadata.name}'",
				})
				Expect(res.WasSuccess()).To(Equal(true), res.StdErr())

				vms := strings.Split(res.StdOut(), " ")
				for _, vm := range vms {
					cmd := "sudo nohup reboot -f > /dev/null 2>&1 &"
					ExecSSHCommand(vm, cmd)
				}
				WaitVMAgentReady(kc.WaitOptions{
					Labels:    manualLabel,
					Namespace: conf.Namespace,
					Timeout:   MaxWaitTimeout,
				})
			})
		})

		Context("When virtual machine agents are ready", func() {
			It("checks that the number of processor cores was changed", func() {
				res := kubectl.List(kc.ResourceVM, kc.GetOptions{
					Labels:    manualLabel,
					Namespace: conf.Namespace,
					Output:    "jsonpath='{.items[*].metadata.name}'",
				})
				Expect(res.WasSuccess()).To(Equal(true), res.StdErr())

				vms := strings.Split(res.StdOut(), " ")
				CheckCPUCoresNumberFromVirtualMachine(strconv.FormatInt(int64(newCPUCores), 10), vms...)
			})
		})
	})

	Describe("Automatic restart approval mode", func() {
		var oldCpuCores int
		var newCPUCores int

		Context(fmt.Sprintf("When virtual machine is in %s phase", PhaseRunning), func() {
			It("changes the number of processor cores", func() {
				res := kubectl.List(kc.ResourceVM, kc.GetOptions{
					Labels:    automaticLabel,
					Namespace: conf.Namespace,
					Output:    "jsonpath='{.items[*].metadata.name}'",
				})
				Expect(res.WasSuccess()).To(Equal(true), res.StdErr())

				vms := strings.Split(res.StdOut(), " ")
				Expect(vms).NotTo(BeEmpty())

				vmResource := virtv2.VirtualMachine{}
				err := GetObject(kc.ResourceVM, vms[0], &vmResource, kc.GetOptions{Namespace: conf.Namespace})
				Expect(err).NotTo(HaveOccurred(), "%v", err)

				oldCpuCores = vmResource.Spec.CPU.Cores
				newCPUCores = 1 + (vmResource.Spec.CPU.Cores & 1)

				CheckCPUCoresNumber(AutomaticMode, StageBefore, oldCpuCores, vms...)
				ChangeCPUCoresNumber(newCPUCores, vms...)
			})
		})

		Context("When virtual machine is patched", func() {
			It("checks the number of processor cores in specification", func() {
				res := kubectl.List(kc.ResourceVM, kc.GetOptions{
					Labels:    automaticLabel,
					Namespace: conf.Namespace,
					Output:    "jsonpath='{.items[*].metadata.name}'",
				})
				Expect(res.WasSuccess()).To(Equal(true), res.StdErr())

				vms := strings.Split(res.StdOut(), " ")
				CheckCPUCoresNumber(AutomaticMode, StageAfter, newCPUCores, vms...)
			})
		})

		Context("When virtual machine is restarted", func() {
			It("should be ready", func() {
				WaitVMAgentReady(kc.WaitOptions{
					Labels:    automaticLabel,
					Namespace: conf.Namespace,
					Timeout:   MaxWaitTimeout,
				})
			})
		})

		Context("When virtual machine agents are ready", func() {
			It("checks that the number of processor cores was changed", func() {
				res := kubectl.List(kc.ResourceVM, kc.GetOptions{
					Labels:    automaticLabel,
					Namespace: conf.Namespace,
					Output:    "jsonpath='{.items[*].metadata.name}'",
				})
				Expect(res.Error()).NotTo(HaveOccurred(), res.StdErr())

				vms := strings.Split(res.StdOut(), " ")
				CheckCPUCoresNumberFromVirtualMachine(strconv.FormatInt(int64(newCPUCores), 10), vms...)
			})
		})
	})

	Context("When test is completed", func() {
		It("deletes test case resources", func() {
			var resourcesToDelete ResourcesToDelete

			if config.IsCleanUpNeeded() {
				resourcesToDelete.KustomizationDir = conf.TestData.VMConfiguration
			}

			DeleteTestCaseResources(resourcesToDelete)
		})
	})
})
