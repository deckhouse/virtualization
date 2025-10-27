/*
Copyright 2025 Flant JSC

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

package legacy

import (
	"context"
	"fmt"
	"slices"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/test/e2e/internal/config"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	kc "github.com/deckhouse/virtualization/test/e2e/internal/kubectl"
)

var _ = Describe("VirtualMachineEvacuation", framework.CommonE2ETestDecorators(), func() {
	testCaseLabel := map[string]string{"testcase": "vm-evacuation"}
	kubeClient := framework.GetClients().KubeClient()
	var ns string

	BeforeAll(func() {
		kustomization := fmt.Sprintf("%s/%s", conf.TestData.VMEvacuation, "kustomization.yaml")
		var err error
		ns, err = kustomize.GetNamespace(kustomization)
		Expect(err).NotTo(HaveOccurred(), "%w", err)

		CreateNamespace(ns)
	})

	BeforeEach(func() {
		res := kubectl.Apply(kc.ApplyOptions{
			Filename:       []string{conf.TestData.VMEvacuation},
			FilenameOption: kc.Kustomize,
		})
		Expect(res.WasSuccess()).To(Equal(true), res.StdErr())
	})

	AfterEach(func() {
		if CurrentSpecReport().Failed() {
			SaveTestCaseDump(testCaseLabel, CurrentSpecReport().LeafNodeText, ns)
		}
		resourcesToDelete := ResourcesToDelete{
			AdditionalResources: []AdditionalResource{
				{
					Resource: kc.ResourceVMOP,
					Labels:   testCaseLabel,
				},
			},
		}

		if config.IsCleanUpNeeded() {
			resourcesToDelete.KustomizationDir = conf.TestData.VMEvacuation
		}
		DeleteTestCaseResources(ns, resourcesToDelete)
	})

	evacuate := func(vms []string) {
		pods := &corev1.PodList{}
		err := GetObjects(kc.ResourcePod, pods, kc.GetOptions{Labels: testCaseLabel, Namespace: ns})
		Expect(err).NotTo(HaveOccurred())
		Expect(pods.Items).Should(HaveLen(len(vms)))

		for _, pod := range pods.Items {
			err := kubeClient.CoreV1().Pods(pod.GetNamespace()).EvictV1(context.Background(), &policyv1.Eviction{
				ObjectMeta: metav1.ObjectMeta{
					Name:      pod.GetName(),
					Namespace: pod.GetNamespace(),
				},
			})
			Expect(err).To(HaveOccurred())
			Expect(err).To(MatchError(ContainSubstring("Eviction triggered evacuation of VMI")))
		}
	}

	It("Evacuation", func() {
		By("Virtual machine agents should be ready")
		WaitVMAgentReady(kc.WaitOptions{
			Labels:    testCaseLabel,
			Namespace: ns,
			Timeout:   MaxWaitTimeout,
		})

		vms := &v1alpha2.VirtualMachineList{}
		err := GetObjects(kc.ResourceVM, vms, kc.GetOptions{Labels: testCaseLabel, Namespace: ns})
		Expect(err).NotTo(HaveOccurred())

		vmNames := make([]string, len(vms.Items))
		for i, vm := range vms.Items {
			vmNames[i] = vm.GetName()
		}

		By("Evacuate virtual machines")
		evacuate(vmNames)

		By("Waiting for all VMOPs to be finished")
		Eventually(func() error {
			vmops := &v1alpha2.VirtualMachineOperationList{}
			err := GetObjects(kc.ResourceVMOP, vmops, kc.GetOptions{Namespace: ns})
			if err != nil {
				return err
			}

			finishedVMOPs := 0

			for _, vmop := range vmops.Items {
				if !slices.Contains(vmNames, vmop.Spec.VirtualMachine) {
					continue
				}
				if _, exists := vmop.GetAnnotations()["virtualization.deckhouse.io/evacuation"]; !exists {
					continue
				}
				if vmop.Status.Phase == v1alpha2.VMOPPhaseFailed || vmop.Status.Phase == v1alpha2.VMOPPhaseCompleted {
					finishedVMOPs++
				}

			}

			if len(vmNames) != finishedVMOPs {
				return fmt.Errorf("expected %d finished VMOPs, got %d", len(vmNames), finishedVMOPs)
			}
			return nil
		}).WithTimeout(MaxWaitTimeout).WithPolling(time.Second).ShouldNot(HaveOccurred())
	})
})
