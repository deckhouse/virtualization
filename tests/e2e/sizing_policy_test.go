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
	kc "github.com/deckhouse/virtualization/tests/e2e/kubectl"
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

var _ = Describe("Sizing policy", Ordered, ContinueOnFailure, func() {
	var (
		vmNotValidSizingPolicy   string
		phaseByVolumeBindingMode string
		vmClassDiscovery         string
		notExistingVmClass       = map[string]string{"vm": "not-existing-vmclass"}
		existingVmClass          = map[string]string{"vm": "existing-vmclass"}
		testcaseLabel            = map[string]string{"testcase": "sizing-policy"}
	)

	Context("Environment preparing", func() {
		vmNotValidSizingPolicy = fmt.Sprintf("%s-vm-%s", namePrefix, notExistingVmClass["vm"])
		vmClassDiscovery = fmt.Sprintf("%s-discovery", namePrefix)
		switch conf.StorageClass.VolumeBindingMode {
		case "Immediate":
			phaseByVolumeBindingMode = PhaseReady
		case "WaitForFirstConsumer":
			phaseByVolumeBindingMode = PhaseWaitForFirstConsumer
		default:
			phaseByVolumeBindingMode = PhaseReady
		}
	})

	Context("Resources", func() {
		When("Resources applied:", func() {
			It("result must have no error", func() {
				res := kubectl.Kustomize(conf.TestData.SizingPolicy, kc.KustomizeOptions{})
				Expect(res.WasSuccess()).To(Equal(true), res.StdErr())
			})
		})
	})

	Context("When virtual images are applied:", func() {
		It("checks VIs phases", func() {
			By(fmt.Sprintf("VIs should be in %s phases", PhaseReady))
			WaitPhase(kc.ResourceVI, PhaseReady, kc.GetOptions{
				Labels:    testcaseLabel,
				Namespace: conf.Namespace,
				Output:    "jsonpath='{.items[*].metadata.name}'",
			})
		})
	})

	Context("When virtual disks are applied:", func() {
		It(fmt.Sprintf("checks VDs phases with %s label", notExistingVmClass), func() {
			By(fmt.Sprintf("VDs should be in %s phases", phaseByVolumeBindingMode))
			WaitPhase(kc.ResourceVD, phaseByVolumeBindingMode, kc.GetOptions{
				Labels:    notExistingVmClass,
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
		It(fmt.Sprintf("checks VMs phases with %s label", notExistingVmClass), func() {
			By(fmt.Sprintf("VMs should be in %s phases", PhasePending))
			WaitPhase(kc.ResourceVM, PhasePending, kc.GetOptions{
				Labels:    notExistingVmClass,
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

	Context(fmt.Sprintf("When virtual machine with label %s in phase %s:", notExistingVmClass, PhasePending), func() {
		It("fixes VirtualMachineClassName in VM specification", func() {
			mergePatch := fmt.Sprintf("{\"spec\":{\"virtualMachineClassName\":\"%s\"}}", vmClassDiscovery)
			MergePatchResource(kc.ResourceVM, vmNotValidSizingPolicy, mergePatch)
		})

		It("checks VM phase after fixing", func() {
			By(fmt.Sprintf("VM should be in %s phase", PhaseRunning))
			WaitPhase(kc.ResourceVM, PhaseRunning, kc.GetOptions{
				Labels:    notExistingVmClass,
				Namespace: conf.Namespace,
				Output:    "jsonpath='{.items[*].metadata.name}'",
			})
		})
	})

	Context(fmt.Sprintf("When virtual machines in phase %s:", PhaseRunning), func() {
		It("check sizing policy match", func() {
			res := kubectl.List(kc.ResourceVM, kc.GetOptions{
				Labels:    testcaseLabel,
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
