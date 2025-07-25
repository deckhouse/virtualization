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
	v1 "k8s.io/api/core/v1"

	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/tests/e2e/config"
	"github.com/deckhouse/virtualization/tests/e2e/ginkgoutil"
	kc "github.com/deckhouse/virtualization/tests/e2e/kubectl"
)

var _ = Describe("VirtualMachineLabelAndAnnotation", ginkgoutil.CommonE2ETestDecorators(), func() {
	BeforeEach(func() {
		if config.IsReusable() {
			Skip("Test not available in REUSABLE mode: not supported yet.")
		}
	})

	const (
		specialKey   = "specialKey"
		specialValue = "specialValue"
	)
	testCaseLabel := map[string]string{"testcase": "vm-label-annotation"}
	specialKeyValue := map[string]string{specialKey: specialValue}
	var ns string

	AfterEach(func() {
		if CurrentSpecReport().Failed() {
			SaveTestResources(testCaseLabel, CurrentSpecReport().LeafNodeText)
		}
	})

	Context("Preparing the environment", func() {
		It("sets the namespace", func() {
			kustomization := fmt.Sprintf("%s/%s", conf.TestData.VMLabelAnnotation, "kustomization.yaml")
			var err error
			ns, err = kustomize.GetNamespace(kustomization)
			Expect(err).NotTo(HaveOccurred(), "%w", err)
		})
	})

	Context("When resources are applied", func() {
		It("result should be succeeded", func() {
			res := kubectl.Apply(kc.ApplyOptions{
				Filename:       []string{conf.TestData.VMLabelAnnotation},
				FilenameOption: kc.Kustomize,
			})
			Expect(res.Error()).NotTo(HaveOccurred(), "cmd: %s\nstderr: %s", res.GetCmd(), res.StdErr())
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
		It("checks VDs phases", func() {
			By(fmt.Sprintf("VDs should be in %s phases", PhaseReady))
			WaitPhaseByLabel(kc.ResourceVD, PhaseReady, kc.WaitOptions{
				Labels:    testCaseLabel,
				Namespace: ns,
				Timeout:   MaxWaitTimeout,
			})
		})
	})

	Context("When virtual machines are applied", func() {
		It("checks VMs phases", func() {
			By("Virtual machine agents should be ready")
			WaitVMAgentReady(kc.WaitOptions{
				Labels:    testCaseLabel,
				Namespace: ns,
				Timeout:   MaxWaitTimeout,
			})
		})
	})

	Context("When virtual machine agents are ready", func() {
		It(fmt.Sprintf("marks VMs with label %q", specialKeyValue), func() {
			res := kubectl.List(kc.ResourceVM, kc.GetOptions{
				Labels:    testCaseLabel,
				Namespace: ns,
				Output:    "jsonpath='{.items[*].metadata.name}'",
			})
			Expect(res.Error()).NotTo(HaveOccurred(), "cmd: %s\nstderr: %s", res.GetCmd(), res.StdErr())

			vms := strings.Split(res.StdOut(), " ")
			err := AddLabel(kc.ResourceVM, specialKeyValue, ns, vms...)
			Expect(err).NotTo(HaveOccurred())
		})

		It("checks VMs and pods labels after VMs labeling", func() {
			Eventually(func() error {
				var vms virtv2.VirtualMachineList
				err := GetObjects(kc.ResourceVM, &vms, kc.GetOptions{
					Labels:    testCaseLabel,
					Namespace: ns,
				})
				if err != nil {
					return err
				}

				for _, vm := range vms.Items {
					if !IsContainsLabelWithValue(&vm, specialKey, specialValue) {
						return fmt.Errorf("vm label %q with value %q was not found in %s", specialKey, specialValue, vm.Name)
					}

					activePodName := GetActiveVirtualMachinePod(&vm)
					vmPod := v1.Pod{}
					err = GetObject(kc.ResourcePod, activePodName, &vmPod, kc.GetOptions{Namespace: ns})
					if err != nil {
						return err
					}

					if !IsContainsLabelWithValue(&vmPod, specialKey, specialValue) {
						return fmt.Errorf("vm pod label %q with value %q was not found in %s", specialKey, specialValue, vmPod.Name)
					}
				}

				return nil
			}).WithTimeout(Timeout).WithPolling(Interval).Should(Succeed())
		})

		It(fmt.Sprintf("removes label %s from VMs", specialKeyValue), func() {
			res := kubectl.List(kc.ResourceVM, kc.GetOptions{
				Labels:    testCaseLabel,
				Namespace: ns,
				Output:    "jsonpath='{.items[*].metadata.name}'",
			})
			Expect(res.Error()).NotTo(HaveOccurred(), "cmd: %s\nstderr: %s", res.GetCmd(), res.StdErr())

			vms := strings.Split(res.StdOut(), " ")
			err := RemoveLabel(kc.ResourceVM, specialKeyValue, ns, vms...)
			Expect(err).NotTo(HaveOccurred())
		})

		It("checks VMs and pods labels after VMs unlabeling", func() {
			Eventually(func() error {
				var vms virtv2.VirtualMachineList
				err := GetObjects(kc.ResourceVM, &vms, kc.GetOptions{
					Labels:    testCaseLabel,
					Namespace: ns,
				})
				if err != nil {
					return err
				}

				for _, vm := range vms.Items {
					if IsContainsLabel(&vm, specialKey) {
						return fmt.Errorf("vm label %q was found in %s", specialKey, vm.Name)
					}

					activePodName := GetActiveVirtualMachinePod(&vm)
					vmPod := v1.Pod{}
					err = GetObject(kc.ResourcePod, activePodName, &vmPod, kc.GetOptions{Namespace: ns})
					if err != nil {
						return err
					}

					if IsContainsLabel(&vmPod, specialKey) {
						return fmt.Errorf("vm pod label %q was found in %s", specialKey, vmPod.Name)
					}
				}

				return nil
			}).WithTimeout(Timeout).WithPolling(Interval).Should(Succeed())
		})
	})

	Context(fmt.Sprintf("Annotate `VirtualMachines` in %s phase", PhaseRunning), func() {
		It(fmt.Sprintf("marks VMs with annotation %q", specialKeyValue), func() {
			res := kubectl.List(kc.ResourceVM, kc.GetOptions{
				Labels:    testCaseLabel,
				Namespace: ns,
				Output:    "jsonpath='{.items[*].metadata.name}'",
			})
			Expect(res.Error()).NotTo(HaveOccurred(), "cmd: %s\nstderr: %s", res.GetCmd(), res.StdErr())

			vms := strings.Split(res.StdOut(), " ")
			err := AddAnnotation(kc.ResourceVM, specialKeyValue, ns, vms...)
			Expect(err).NotTo(HaveOccurred())
		})

		It("checks VMs and pods annotations after VMs annotating", func() {
			Eventually(func() error {
				var vms virtv2.VirtualMachineList
				err := GetObjects(kc.ResourceVM, &vms, kc.GetOptions{
					Labels:    testCaseLabel,
					Namespace: ns,
				})
				if err != nil {
					return err
				}

				for _, vm := range vms.Items {
					if !IsContainsAnnotationWithValue(&vm, specialKey, specialValue) {
						return fmt.Errorf("vm annotation %q with value %q was not found in %s", specialKey, specialValue, vm.Name)
					}

					activePodName := GetActiveVirtualMachinePod(&vm)
					vmPod := v1.Pod{}
					err = GetObject(kc.ResourcePod, activePodName, &vmPod, kc.GetOptions{Namespace: ns})
					if err != nil {
						return err
					}

					if !IsContainsAnnotationWithValue(&vmPod, specialKey, specialValue) {
						return fmt.Errorf("vm pod annotation %q with value %q was not found in %s", specialKey, specialValue, vmPod.Name)
					}
				}

				return nil
			}).WithTimeout(Timeout).WithPolling(Interval).Should(Succeed())
		})

		It(fmt.Sprintf("removes annotation %s from VMs", specialKeyValue), func() {
			res := kubectl.List(kc.ResourceVM, kc.GetOptions{
				Labels:    testCaseLabel,
				Namespace: ns,
				Output:    "jsonpath='{.items[*].metadata.name}'",
			})
			Expect(res.Error()).NotTo(HaveOccurred(), "cmd: %s\nstderr: %s", res.GetCmd(), res.StdErr())

			vms := strings.Split(res.StdOut(), " ")
			err := RemoveAnnotation(kc.ResourceVM, specialKeyValue, ns, vms...)
			Expect(err).NotTo(HaveOccurred())
		})

		It("checks VMs and pods annotations after VMs unannotating", func() {
			Eventually(func() error {
				var vms virtv2.VirtualMachineList
				err := GetObjects(kc.ResourceVM, &vms, kc.GetOptions{
					Labels:    testCaseLabel,
					Namespace: ns,
				})
				if err != nil {
					return err
				}

				for _, vm := range vms.Items {
					if IsContainsAnnotation(&vm, specialKey) {
						return fmt.Errorf("vm annotation %q was found in %s", specialKey, vm.Name)
					}

					activePodName := GetActiveVirtualMachinePod(&vm)
					vmPod := v1.Pod{}
					err = GetObject(kc.ResourcePod, activePodName, &vmPod, kc.GetOptions{Namespace: ns})
					if err != nil {
						return err
					}

					if IsContainsAnnotation(&vmPod, specialKey) {
						return fmt.Errorf("vm pod annotation %q was found in %s", specialKey, vmPod.Name)
					}
				}

				return nil
			}).WithTimeout(Timeout).WithPolling(Interval).Should(Succeed())
		})
	})

	Context("When test is completed", func() {
		It("deletes test case resources", func() {
			DeleteTestCaseResources(ns, ResourcesToDelete{
				KustomizationDir: conf.TestData.VMLabelAnnotation,
			})
		})
	})
})

func AddLabel(resource kc.Resource, labels map[string]string, ns string, names ...string) error {
	formattedLabels := make([]string, 0, len(labels))
	for k, v := range labels {
		formattedLabels = append(formattedLabels, fmt.Sprintf("%s=%s", k, v))
	}
	rawResources := strings.Join(names, " ")
	rawLabels := strings.Join(formattedLabels, "")
	subCmd := fmt.Sprintf("label %s %s --namespace %s %s", resource, rawResources, ns, rawLabels)
	res := kubectl.RawCommand(subCmd, kc.MediumTimeout)
	if res.Error() != nil {
		return fmt.Errorf("cmd: %s\nstderr: %s", res.GetCmd(), res.StdErr())
	}
	return nil
}

func RemoveLabel(resource kc.Resource, labels map[string]string, ns string, names ...string) error {
	formattedLabels := make([]string, 0, len(labels))
	for k := range labels {
		formattedLabels = append(formattedLabels, fmt.Sprintf("%s-", k))
	}
	rawResources := strings.Join(names, " ")
	rawLabels := strings.Join(formattedLabels, "")
	subCmd := fmt.Sprintf("label %s %s --namespace %s %s", resource, rawResources, ns, rawLabels)
	res := kubectl.RawCommand(subCmd, kc.MediumTimeout)
	if res.Error() != nil {
		return fmt.Errorf("cmd: %s\nstderr: %s", res.GetCmd(), res.StdErr())
	}
	return nil
}

func AddAnnotation(resource kc.Resource, annotations map[string]string, ns string, names ...string) error {
	formattedAnnotations := make([]string, 0, len(annotations))
	for k, v := range annotations {
		formattedAnnotations = append(formattedAnnotations, fmt.Sprintf("%s=%s", k, v))
	}
	rawResources := strings.Join(names, " ")
	rawAnnotations := strings.Join(formattedAnnotations, "")
	subCmd := fmt.Sprintf("annotate %s %s --namespace %s %s", resource, rawResources, ns, rawAnnotations)
	res := kubectl.RawCommand(subCmd, kc.MediumTimeout)
	if res.Error() != nil {
		return fmt.Errorf("cmd: %s\nstderr: %s", res.GetCmd(), res.StdErr())
	}
	return nil
}

func RemoveAnnotation(resource kc.Resource, annotations map[string]string, ns string, names ...string) error {
	formattedAnnotations := make([]string, 0, len(annotations))
	for k := range annotations {
		formattedAnnotations = append(formattedAnnotations, fmt.Sprintf("%s-", k))
	}
	rawResources := strings.Join(names, " ")
	rawAnnotations := strings.Join(formattedAnnotations, "")
	subCmd := fmt.Sprintf("annotate %s %s --namespace %s %s", resource, rawResources, ns, rawAnnotations)
	res := kubectl.RawCommand(subCmd, kc.MediumTimeout)
	if res.Error() != nil {
		return fmt.Errorf("cmd: %s\nstderr: %s", res.GetCmd(), res.StdErr())
	}
	return nil
}

func GetActiveVirtualMachinePod(vmObj *virtv2.VirtualMachine) string {
	for _, pod := range vmObj.Status.VirtualMachinePods {
		if pod.Active {
			return pod.Name
		}
	}
	return ""
}
