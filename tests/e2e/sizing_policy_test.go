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
	"slices"
	"strconv"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
	"github.com/deckhouse/virtualization/tests/e2e/framework"
	"github.com/deckhouse/virtualization/tests/e2e/helper"
	kc "github.com/deckhouse/virtualization/tests/e2e/kubectl"
)

var _ = Describe("SizingPolicy", framework.CommonE2ETestDecorators(), func() {
	var (
		vmNotValidSizingPolicyChanging string
		vmNotValidSizingPolicyCreating string
		vmClassDiscovery               string
		vmClassDiscoveryCopy           string
		newVMClassFilePath             string
		notExistingVMClassChanging     = map[string]string{"vm": "vmc-change"}
		notExistingVMClassCreating     = map[string]string{"vm": "vmc-create"}
		existingVMClass                = map[string]string{"vm": "vmc-exists"}
		testCaseLabel                  = map[string]string{"testcase": "sizing-policy"}
		ns                             string
		phaseByVolumeBindingMode       = GetPhaseByVolumeBindingModeForTemplateSc()
	)

	BeforeAll(func() {
		vmNotValidSizingPolicyChanging = fmt.Sprintf("%s-vm-%s", namePrefix, notExistingVMClassChanging["vm"])
		vmNotValidSizingPolicyCreating = fmt.Sprintf("%s-vm-%s", namePrefix, notExistingVMClassCreating["vm"])
		vmClassDiscovery = fmt.Sprintf("%s-sizing-policy-discovery", namePrefix)
		vmClassDiscoveryCopy = fmt.Sprintf("%s-sizing-policy-discovery-copy", namePrefix)
		newVMClassFilePath = fmt.Sprintf("%s/vmc-copy.yaml", conf.TestData.SizingPolicy)

		kustomization := fmt.Sprintf("%s/%s", conf.TestData.SizingPolicy, "kustomization.yaml")
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

	Context("When resources are applied", func() {
		It("result should be succeeded", func() {
			res := kubectl.Apply(kc.ApplyOptions{
				Filename:       []string{conf.TestData.SizingPolicy},
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
				Namespace: ns,
				Timeout:   MaxWaitTimeout,
			})
		})
	})

	Context("When virtual disks are applied", func() {
		It(fmt.Sprintf("checks VDs phases with %s and %s label", notExistingVMClassChanging, notExistingVMClassCreating), func() {
			By(fmt.Sprintf("VDs should be in %s phases", phaseByVolumeBindingMode))
			WaitPhaseByLabel(kc.ResourceVD, phaseByVolumeBindingMode, kc.WaitOptions{
				Labels:    notExistingVMClassChanging,
				Namespace: ns,
				Timeout:   MaxWaitTimeout,
			})
			WaitPhaseByLabel(kc.ResourceVD, phaseByVolumeBindingMode, kc.WaitOptions{
				Labels:    notExistingVMClassCreating,
				Namespace: ns,
				Timeout:   MaxWaitTimeout,
			})
		})

		It(fmt.Sprintf("checks VDs phases with %s label", existingVMClass), func() {
			By(fmt.Sprintf("VDs should be in %s phases", PhaseReady))
			WaitPhaseByLabel(kc.ResourceVD, PhaseReady, kc.WaitOptions{
				Labels:    existingVMClass,
				Namespace: ns,
				Timeout:   MaxWaitTimeout,
			})
		})
	})

	Context("When virtual machines are applied", func() {
		It(fmt.Sprintf("checks VMs phases with %s and %s label", notExistingVMClassChanging, notExistingVMClassCreating), func() {
			By(fmt.Sprintf("VMs should be in %s phases", PhasePending))
			WaitPhaseByLabel(kc.ResourceVM, PhasePending, kc.WaitOptions{
				Labels:    notExistingVMClassChanging,
				Namespace: ns,
				Timeout:   MaxWaitTimeout,
			})
			WaitPhaseByLabel(kc.ResourceVM, PhasePending, kc.WaitOptions{
				Labels:    notExistingVMClassCreating,
				Namespace: ns,
				Timeout:   MaxWaitTimeout,
			})
		})

		It(fmt.Sprintf("checks VMs phases with %s label", existingVMClass), func() {
			By("Virtual machine agents should be ready")
			WaitVMAgentReady(kc.WaitOptions{
				Labels:    existingVMClass,
				Namespace: ns,
				Timeout:   MaxWaitTimeout,
			})
		})
	})

	Describe("Not existing virtual machine class", func() {
		Context(fmt.Sprintf("When virtual machine with label %s in phase %s", notExistingVMClassChanging, PhasePending), func() {
			It("checks condition status before changing 'virtulaMachineCLass` field with existing class", func() {
				By(fmt.Sprintf("VirtualMachineClassReady status should be '%s' before changing", metav1.ConditionFalse))
				CompareVirtualMachineClassReadyStatus(ns, vmNotValidSizingPolicyChanging, metav1.ConditionFalse)
			})

			It("changes VMClassName in VM specification with existing VMClass", func() {
				mergePatch := fmt.Sprintf("{\"spec\":{\"virtualMachineClassName\":%q}}", vmClassDiscovery)
				err := MergePatchResource(kc.ResourceVM, ns, vmNotValidSizingPolicyChanging, mergePatch)
				Expect(err).NotTo(HaveOccurred())
			})

			It("checks VM phase and condition status after changing", func() {
				By("VM should be ready")
				WaitVMAgentReady(kc.WaitOptions{
					Labels:    notExistingVMClassChanging,
					Namespace: ns,
					Timeout:   MaxWaitTimeout,
				})
				By(fmt.Sprintf("VirtualMachineClassReady status should be '%s' after changing", metav1.ConditionTrue))
				CompareVirtualMachineClassReadyStatus(ns, vmNotValidSizingPolicyChanging, metav1.ConditionTrue)
			})
		})

		Context(fmt.Sprintf("When virtual machine with label %s in phase %s", notExistingVMClassCreating, PhasePending), func() {
			It("checks condition status before creating `VirtualMachineClass`", func() {
				By(fmt.Sprintf("VirtualMachineClassReady status should be '%s' before creating", metav1.ConditionFalse))
				CompareVirtualMachineClassReadyStatus(ns, vmNotValidSizingPolicyCreating, metav1.ConditionFalse)
			})

			It("changes VMClassName in VM specification with not existing VMClass which have correct prefix for creating", func() {
				mergePatch := fmt.Sprintf("{\"spec\":{\"virtualMachineClassName\":%q}}", vmClassDiscoveryCopy)
				err := MergePatchResource(kc.ResourceVM, ns, vmNotValidSizingPolicyCreating, mergePatch)
				Expect(err).NotTo(HaveOccurred())
			})

			It("creates new `VirtualMachineClass`", func() {
				vmClass := v1alpha2.VirtualMachineClass{}
				err := GetObject(kc.ResourceVMClass, vmClassDiscovery, &vmClass, kc.GetOptions{})
				Expect(err).NotTo(HaveOccurred())
				vmClass.Name = vmClassDiscoveryCopy
				vmClass.Labels = map[string]string{"id": namePrefix}
				writeErr := helper.WriteYamlObject(newVMClassFilePath, &vmClass)
				Expect(writeErr).NotTo(HaveOccurred(), writeErr)
				res := kubectl.Apply(kc.ApplyOptions{
					Filename:       []string{newVMClassFilePath},
					FilenameOption: kc.Filename,
				})
				Expect(res.Error()).NotTo(HaveOccurred(), res.StdErr())
			})

			It("checks VM phase and condition after creating", func() {
				By("VM should be ready")
				WaitVMAgentReady(kc.WaitOptions{
					Labels:    notExistingVMClassCreating,
					Namespace: ns,
					Timeout:   MaxWaitTimeout,
				})
				By(fmt.Sprintf("VirtualMachineClassReady status should be '%s' after creating", metav1.ConditionTrue))
				CompareVirtualMachineClassReadyStatus(ns, vmNotValidSizingPolicyCreating, metav1.ConditionTrue)
			})
		})
	})

	Context(fmt.Sprintf("When virtual machines in phase %s", PhaseRunning), func() {
		It("checks sizing policy match", func() {
			res := kubectl.List(kc.ResourceVM, kc.GetOptions{
				Labels:    testCaseLabel,
				Namespace: ns,
				Output:    "jsonpath='{.items[*].metadata.name}'",
			})
			Expect(res.Error()).NotTo(HaveOccurred(), res.StdErr())

			vms := strings.Split(res.StdOut(), " ")
			vmClass := v1alpha2.VirtualMachineClass{}
			err := GetObject(kc.ResourceVMClass, vmClassDiscovery, &vmClass, kc.GetOptions{})
			Expect(err).NotTo(HaveOccurred())

			for _, vm := range vms {
				By(fmt.Sprintf("Check virtual machine: %s", vm))
				vmObj := v1alpha2.VirtualMachine{}
				err := GetObject(kc.ResourceVM, vm, &vmObj, kc.GetOptions{Namespace: ns})
				Expect(err).NotTo(HaveOccurred())
				ValidateVirtualMachineByClass(&vmClass, &vmObj)
			}
		})
	})

	Context("When test is completed", func() {
		It("deletes test case resources", func() {
			DeleteTestCaseResources(ns, ResourcesToDelete{
				KustomizationDir: conf.TestData.SizingPolicy,
				Files:            []string{newVMClassFilePath},
			})
		})
	})
})

func ValidateVirtualMachineByClass(virtualMachineClass *v1alpha2.VirtualMachineClass, virtualMachine *v1alpha2.VirtualMachine) {
	var sizingPolicy v1alpha2.SizingPolicy
	for _, p := range virtualMachineClass.Spec.SizingPolicies {
		if virtualMachine.Spec.CPU.Cores >= p.Cores.Min && virtualMachine.Spec.CPU.Cores <= p.Cores.Max {
			sizingPolicy = *p.DeepCopy()
			break
		}
	}

	checkMinMemory := virtualMachine.Spec.Memory.Size.Value() >= sizingPolicy.Memory.Min.Value()
	checkMaxMemory := virtualMachine.Spec.Memory.Size.Value() <= sizingPolicy.Memory.Max.Value()
	checkMemory := checkMinMemory && checkMaxMemory
	Expect(checkMemory).To(BeTrue(), fmt.Errorf("memory size outside of possible interval '%v - %v': %v", sizingPolicy.Memory.Min, sizingPolicy.Memory.Max, virtualMachine.Spec.Memory.Size))

	coreFraction, err := strconv.Atoi(strings.ReplaceAll(virtualMachine.Spec.CPU.CoreFraction, "%", ""))
	Expect(err).NotTo(HaveOccurred(), "cannot convert CoreFraction value to integer: %s", err)
	checkCoreFraction := slices.Contains(sizingPolicy.CoreFractions, v1alpha2.CoreFractionValue(coreFraction))
	Expect(checkCoreFraction).To(BeTrue(), fmt.Errorf("sizing policy core fraction list does not contain value from spec: %s\n%v", virtualMachine.Spec.CPU.CoreFraction, sizingPolicy.CoreFractions))
}

func CompareVirtualMachineClassReadyStatus(vmNamespace, vmName string, expectedStatus metav1.ConditionStatus) {
	GinkgoHelper()
	vm := v1alpha2.VirtualMachine{}
	err := GetObject(kc.ResourceVM, vmName, &vm, kc.GetOptions{Namespace: vmNamespace})
	Expect(err).NotTo(HaveOccurred(), "%v", err)
	status, err := GetConditionStatus(&vm, vmcondition.TypeClassReady.String())
	Expect(err).NotTo(HaveOccurred(), "%v", err)
	Expect(status).To(Equal(expectedStatus), fmt.Sprintf("VirtualMachineClassReady status should be '%s'", expectedStatus))
}
