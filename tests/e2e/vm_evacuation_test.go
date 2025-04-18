package e2e

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

	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/tests/e2e/config"
	"github.com/deckhouse/virtualization/tests/e2e/ginkgoutil"
	kc "github.com/deckhouse/virtualization/tests/e2e/kubectl"
)

var _ = Describe("Virtual machine evacuation", SIGMigration(), ginkgoutil.CommonE2ETestDecorators(), func() {
	testCaseLabel := map[string]string{"testcase": "vm-evacuation"}

	BeforeEach(func() {
		kustomization := fmt.Sprintf("%s/%s", conf.TestData.VmEvacuation, "kustomization.yaml")
		ns, err := kustomize.GetNamespace(kustomization)
		Expect(err).NotTo(HaveOccurred(), "%w", err)
		conf.SetNamespace(ns)

		res := kubectl.Apply(kc.ApplyOptions{
			Filename:       []string{conf.TestData.VmEvacuation},
			FilenameOption: kc.Kustomize,
		})
		Expect(res.WasSuccess()).To(Equal(true), res.StdErr())
	})

	AfterEach(func() {
		if CurrentSpecReport().Failed() {
			SaveTestResources(testCaseLabel, CurrentSpecReport().LeafNodeText)
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
			resourcesToDelete.KustomizationDir = conf.TestData.VmEvacuation
		}
		DeleteTestCaseResources(resourcesToDelete)
	})

	evacuate := func(vms []string) {
		pods := &corev1.PodList{}
		err := GetObjects(kc.ResourcePod, pods, kc.GetOptions{Labels: testCaseLabel, Namespace: conf.Namespace})
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
		WaitVmAgentReady(kc.WaitOptions{
			Labels:    testCaseLabel,
			Namespace: conf.Namespace,
			Timeout:   MaxWaitTimeout,
		})

		vms := &virtv2.VirtualMachineList{}
		err := GetObjects(kc.ResourceVM, vms, kc.GetOptions{Labels: testCaseLabel, Namespace: conf.Namespace})
		Expect(err).NotTo(HaveOccurred())

		vmNames := make([]string, len(vms.Items))
		for i, vm := range vms.Items {
			vmNames[i] = vm.GetName()
		}

		By("Evacuate virtual machines")
		evacuate(vmNames)

		By("Waiting for all VMOPs to be finished")
		Eventually(func() error {
			vmops := &virtv2.VirtualMachineOperationList{}
			err := GetObjects(kc.ResourceVMOP, vmops, kc.GetOptions{Namespace: conf.Namespace})
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
				if vmop.Status.Phase == virtv2.VMOPPhaseFailed || vmop.Status.Phase == virtv2.VMOPPhaseCompleted {
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
