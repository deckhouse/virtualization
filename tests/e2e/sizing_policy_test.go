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

	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	. "github.com/deckhouse/virtualization/tests/e2e/helper"
	kc "github.com/deckhouse/virtualization/tests/e2e/kubectl"
)

const (
	ReadyStatusTrue  = "True"
	ReadyStatusFalse = "False"
)

func ValidateVirtualMachineByClass(virtualMachineClass *virtv2.VirtualMachineClass, virtualMachine *virtv2.VirtualMachine) {
	var sizingPolicy virtv2.SizingPolicy
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
	checkCoreFraction := slices.Contains(sizingPolicy.CoreFractions, virtv2.CoreFractionValue(coreFraction))
	Expect(checkCoreFraction).To(BeTrue(), fmt.Errorf("sizing policy core fraction list does not contain value from spec: %s\n%v", virtualMachine.Spec.CPU.CoreFraction, sizingPolicy.CoreFractions))
}

func CompareVirtualMachineClassReadyStatus(vmName, expectedStatus string) {
	GinkgoHelper()
	vm := virtv2.VirtualMachine{}
	err := GetObject(kc.ResourceVM, vmName, &vm, kc.GetOptions{Namespace: conf.Namespace})
	Expect(err).NotTo(HaveOccurred(), err)
	status, err := GetConditionStatus(&vm, "VirtualMachineClassReady")
	Expect(err).NotTo(HaveOccurred(), err)
	Expect(status).To(Equal(expectedStatus), fmt.Sprintf("VirtualMachineClassReady status should be '%s'", expectedStatus))
}

var _ = Describe("Sizing policy", Ordered, ContinueOnFailure, func() {
	var (
		vmNotValidSizingPolicyChanging string
		vmNotValidSizingPolicyCreating string
		vmClassDiscovery               string
		vmClassDiscoveryCopy           string
		notExistingVmClassChanging     = map[string]string{"vm": "not-existing-vmclass-with-changing"}
		notExistingVmClassCreating     = map[string]string{"vm": "not-existing-vmclass-with-creating"}
		existingVmClass                = map[string]string{"vm": "existing-vmclass"}
		testCaseLabel                  = map[string]string{"testcase": "sizing-policy"}
	)

	Context("Preparing the environment", func() {
		vmNotValidSizingPolicyChanging = fmt.Sprintf("%s-vm-%s", namePrefix, notExistingVmClassChanging["vm"])
		vmNotValidSizingPolicyCreating = fmt.Sprintf("%s-vm-%s", namePrefix, notExistingVmClassCreating["vm"])
		vmClassDiscovery = fmt.Sprintf("%s-discovery", namePrefix)
		vmClassDiscoveryCopy = fmt.Sprintf("%s-discovery-copy", namePrefix)
	})

	Context("When resources are applied:", func() {
		It("result should be succeeded", func() {
			res := kubectl.Kustomize(conf.TestData.SizingPolicy, kc.KustomizeOptions{})
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

	Context("When virtual disks are applied:", func() {
		It(fmt.Sprintf("checks VDs phases with %s and %s label", notExistingVmClassChanging, notExistingVmClassCreating), func() {
			By(fmt.Sprintf("VDs should be in %s phases", phaseByVolumeBindingMode))
			WaitPhase(kc.ResourceVD, phaseByVolumeBindingMode, kc.GetOptions{
				Labels:    notExistingVmClassChanging,
				Namespace: conf.Namespace,
				Output:    "jsonpath='{.items[*].metadata.name}'",
			})
			WaitPhase(kc.ResourceVD, phaseByVolumeBindingMode, kc.GetOptions{
				Labels:    notExistingVmClassCreating,
				Namespace: conf.Namespace,
				Output:    "jsonpath='{.items[*].metadata.name}'",
			})
		})

		It(fmt.Sprintf("checks VDs phases with %s label", existingVmClass), func() {
			By(fmt.Sprintf("VDs should be in %s phases", PhaseReady))
			WaitPhase(kc.ResourceVD, PhaseReady, kc.GetOptions{
				Labels:    existingVmClass,
				Namespace: conf.Namespace,
				Output:    "jsonpath='{.items[*].metadata.name}'",
			})
		})
	})

	Context("When virtual machines are applied:", func() {
		It(fmt.Sprintf("checks VMs phases with %s and %s label", notExistingVmClassChanging, notExistingVmClassCreating), func() {
			By(fmt.Sprintf("VMs should be in %s phases", PhasePending))
			WaitPhase(kc.ResourceVM, PhasePending, kc.GetOptions{
				Labels:    notExistingVmClassChanging,
				Namespace: conf.Namespace,
				Output:    "jsonpath='{.items[*].metadata.name}'",
			})
			WaitPhase(kc.ResourceVM, PhasePending, kc.GetOptions{
				Labels:    notExistingVmClassCreating,
				Namespace: conf.Namespace,
				Output:    "jsonpath='{.items[*].metadata.name}'",
			})
		})

		It(fmt.Sprintf("checks VMs phases with %s label", existingVmClass), func() {
			By(fmt.Sprintf("VMs should be in %s phases", PhaseRunning))
			WaitPhase(kc.ResourceVM, PhaseRunning, kc.GetOptions{
				Labels:    existingVmClass,
				Namespace: conf.Namespace,
				Output:    "jsonpath='{.items[*].metadata.name}'",
			})
		})
	})

	Describe("Not existing virtual machine class", func() {
		Context(fmt.Sprintf("When virtual machine with label %s in phase %s:", notExistingVmClassChanging, PhasePending), func() {
			It("checks condition status before changing 'virtulaMachineCLass` field with existing class", func() {
				By(fmt.Sprintf("VirtualMachineClassReady status should be '%s' before changing", ReadyStatusFalse))
				CompareVirtualMachineClassReadyStatus(vmNotValidSizingPolicyChanging, ReadyStatusFalse)
			})

			It("changes VMClassName in VM specification with existing VMClass", func() {
				mergePatch := fmt.Sprintf("{\"spec\":{\"virtualMachineClassName\":%q}}", vmClassDiscovery)
				MergePatchResource(kc.ResourceVM, vmNotValidSizingPolicyChanging, mergePatch)
			})

			It("checks VM phase and condition status after changing", func() {
				By(fmt.Sprintf("VM should be in %s phase", PhaseRunning))
				WaitPhase(kc.ResourceVM, PhaseRunning, kc.GetOptions{
					Labels:    notExistingVmClassChanging,
					Namespace: conf.Namespace,
					Output:    "jsonpath='{.items[*].metadata.name}'",
				})
				By(fmt.Sprintf("VirtualMachineClassReady status should be '%s' after changing", ReadyStatusTrue))
				CompareVirtualMachineClassReadyStatus(vmNotValidSizingPolicyChanging, ReadyStatusTrue)
			})
		})

		Context(fmt.Sprintf("When virtual machine with label %s in phase %s:", notExistingVmClassCreating, PhasePending), func() {
			It("checks condition status before creating `VirtualMachineClass`", func() {
				By(fmt.Sprintf("VirtualMachineClassReady status should be '%s' before creating", ReadyStatusFalse))
				CompareVirtualMachineClassReadyStatus(vmNotValidSizingPolicyCreating, ReadyStatusFalse)
			})

			It("changes VMClassName in VM specification with not existing VMClass which have correct prefix for creating", func() {
				mergePatch := fmt.Sprintf("{\"spec\":{\"virtualMachineClassName\":%q}}", vmClassDiscoveryCopy)
				MergePatchResource(kc.ResourceVM, vmNotValidSizingPolicyCreating, mergePatch)
			})

			It("creates new `VirtualMachineClass`", func() {
				vmClass := virtv2.VirtualMachineClass{}
				err := GetObject(kc.ResourceVMClass, vmClassDiscovery, &vmClass, kc.GetOptions{})
				Expect(err).NotTo(HaveOccurred(), err)
				filePath := fmt.Sprintf("%s/vmc.yaml", conf.TestData.SizingPolicy)
				vmClass.Name = vmClassDiscoveryCopy
				vmClass.Labels = map[string]string{"id": namePrefix}
				writeErr := WriteYamlObject(filePath, &vmClass)
				Expect(writeErr).NotTo(HaveOccurred(), writeErr)
				res := kubectl.Apply(filePath, kc.ApplyOptions{})
				Expect(res.Error()).NotTo(HaveOccurred(), res.StdErr())
			})

			It("checks VM phase and condition after creating", func() {
				By(fmt.Sprintf("VM should be in %s phase", PhaseRunning))
				WaitPhase(kc.ResourceVM, PhaseRunning, kc.GetOptions{
					Labels:    notExistingVmClassCreating,
					Namespace: conf.Namespace,
					Output:    "jsonpath='{.items[*].metadata.name}'",
				})
				By(fmt.Sprintf("VirtualMachineClassReady status should be '%s' after creating", ReadyStatusTrue))
				CompareVirtualMachineClassReadyStatus(vmNotValidSizingPolicyCreating, ReadyStatusTrue)
			})
		})
	})

	Context(fmt.Sprintf("When virtual machines in phase %s:", PhaseRunning), func() {
		It("checks sizing policy match", func() {
			res := kubectl.List(kc.ResourceVM, kc.GetOptions{
				Labels:    testCaseLabel,
				Namespace: conf.Namespace,
				Output:    "jsonpath='{.items[*].metadata.name}'",
			})
			Expect(res.Error()).NotTo(HaveOccurred(), res.StdErr())

			vms := strings.Split(res.StdOut(), " ")
			vmClass := virtv2.VirtualMachineClass{}
			err := GetObject(kc.ResourceVMClass, vmClassDiscovery, &vmClass, kc.GetOptions{})
			Expect(err).NotTo(HaveOccurred(), err)

			for _, vm := range vms {
				By(fmt.Sprintf("Check virtual machine: %s", vm))
				vmObj := virtv2.VirtualMachine{}
				err := GetObject(kc.ResourceVM, vm, &vmObj, kc.GetOptions{Namespace: conf.Namespace})
				Expect(err).NotTo(HaveOccurred(), err)
				ValidateVirtualMachineByClass(&vmClass, &vmObj)
			}
		})
	})
})
