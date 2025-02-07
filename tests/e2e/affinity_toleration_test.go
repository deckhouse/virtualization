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
	v1 "k8s.io/api/core/v1"

	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/tests/e2e/config"
	"github.com/deckhouse/virtualization/tests/e2e/ginkgoutil"
	kc "github.com/deckhouse/virtualization/tests/e2e/kubectl"
)

var _ = Describe("Virtual machine affinity and toleration", ginkgoutil.CommonE2ETestDecorators(), func() {
	BeforeEach(func() {
		if config.IsReusable() {
			Skip("Test not available in REUSABLE mode: not supported yet.")
		}
	})

	var (
		testCaseLabel = map[string]string{"testcase": "affinity-toleration"}
		vmA           = map[string]string{"vm": "vm-a"}
		vmB           = map[string]string{"vm": "vm-b"}
		vmC           = map[string]string{"vm": "vm-c"}
		vmD           = map[string]string{"vm": "vm-d"}
	)

	Context("Preparing the environment", func() {
		It("sets the namespace", func() {
			kustomization := fmt.Sprintf("%s/%s", conf.TestData.AffinityToleration, "kustomization.yaml")
			ns, err := kustomize.GetNamespace(kustomization)
			Expect(err).NotTo(HaveOccurred(), "%w", err)
			conf.SetNamespace(ns)
		})
	})

	Context("When virtualization resources are applied:", func() {
		It("result should be succeeded", func() {
			res := kubectl.Apply(kc.ApplyOptions{
				Filename:       []string{conf.TestData.AffinityToleration},
				FilenameOption: kc.Kustomize,
			})
			Expect(res.Error()).NotTo(HaveOccurred(), res.StdErr())
		})
	})

	Context("When virtual images are applied:", func() {
		It("checks VIs phases", func() {
			By(fmt.Sprintf("VIs should be in %s phases", PhaseReady))
			WaitPhaseByLabel(kc.ResourceVI, PhaseReady, kc.WaitOptions{
				Labels:    testCaseLabel,
				Namespace: conf.Namespace,
				Timeout:   MaxWaitTimeout,
			})
		})
	})

	Context("When virtual machine classes are applied:", func() {
		It("checks VMClasses phases", func() {
			By(fmt.Sprintf("VMClasses should be in %s phases", PhaseReady))
			WaitPhaseByLabel(kc.ResourceVMClass, PhaseReady, kc.WaitOptions{
				Labels:    testCaseLabel,
				Namespace: conf.Namespace,
				Timeout:   MaxWaitTimeout,
			})
		})
	})

	Context("When virtual disks are applied:", func() {
		It("checks VDs phases", func() {
			By(fmt.Sprintf("VDs should be in %s phases", PhaseReady))
			WaitPhaseByLabel(kc.ResourceVD, PhaseReady, kc.WaitOptions{
				Labels:    testCaseLabel,
				Namespace: conf.Namespace,
				Timeout:   MaxWaitTimeout,
			})
		})
	})

	Context("When virtual machines are applied:", func() {
		It("checks VMs phases", func() {
			By("VMs should be running")
			WaitVmRunning(true, kc.WaitOptions{
				Labels:    testCaseLabel,
				Namespace: conf.Namespace,
				Timeout:   MaxWaitTimeout,
			})
		})
	})

	Context(fmt.Sprintf("When virtual machines in %s phase", PhaseRunning), func() {
		It("checks VMs `status.nodeName`", func() {
			vmObjects := virtv2.VirtualMachineList{}
			err := GetObjects(kc.ResourceVM, &vmObjects, kc.GetOptions{
				Labels:    vmA,
				Namespace: conf.Namespace,
			})
			Expect(err).NotTo(HaveOccurred(), "error: cannot get virtual machines with label %s\nstderr: %s", vmA, err)
			vmANodeName := vmObjects.Items[0].Status.Node
			err = GetObjects(kc.ResourceVM, &vmObjects, kc.GetOptions{
				Labels:    vmC,
				Namespace: conf.Namespace,
			})
			Expect(err).NotTo(HaveOccurred(), "error: cannot get virtual machines with label %s\nstderr: %s", vmC, err)
			vmCNodeName := vmObjects.Items[0].Status.Node
			By("Affinity: `vm-a` and `vm-c` should be running on the same node")
			Expect(vmANodeName).Should(Equal(vmCNodeName), "error: vm-a and vm-c should be running on the same node")
			err = GetObjects(kc.ResourceVM, &vmObjects, kc.GetOptions{
				Labels:    vmB,
				Namespace: conf.Namespace,
			})
			Expect(err).NotTo(HaveOccurred(), "error: cannot get virtual machines with label %s\nstderr: %s", vmB, err)
			vmBNodeName := vmObjects.Items[0].Status.Node
			By("AntiAffinity: `vm-a` and `vm-b` should be running on different nodes")
			Expect(vmANodeName).ShouldNot(Equal(vmBNodeName), "error: vm-a and vm-b should be running on different nodes")
			err = GetObjects(kc.ResourceVM, &vmObjects, kc.GetOptions{
				Labels:    vmD,
				Namespace: conf.Namespace,
			})
			Expect(err).NotTo(HaveOccurred(), "error: cannot get virtual machines with label %s\nstderr: %s", vmD, err)
			vmDNodeName := vmObjects.Items[0].Status.Node
			nodeObj := v1.Node{}
			err = GetObject(kc.ResourceNode, vmDNodeName, &nodeObj, kc.GetOptions{})
			Expect(err).NotTo(HaveOccurred(), "error: cannot get node %s:\nstderr: %s", vmDNodeName, err)
			By("Toleration: `vm-d` should be running on a master node")
			Expect(nodeObj.Labels).Should(HaveKeyWithValue("node.deckhouse.io/group", "master"))
		})
	})

	Context("When test is completed", func() {
		It("deletes test case resources", func() {
			DeleteTestCaseResources(ResourcesToDelete{KustomizationDir: conf.TestData.AffinityToleration})
		})
	})
})
