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
	"sync"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"

	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/tests/e2e/config"
	"github.com/deckhouse/virtualization/tests/e2e/executor"
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
		Timeout:   ShortWaitDuration,
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
		vmPodLabel         = map[string]string{"kubevirt.internal.virtualization.deckhouse.io": "virt-launcher"}
		alwaysOnLabel      = map[string]string{"alwaysOn": "complex-test"}
		notAlwaysOnLabel   = map[string]string{"notAlwaysOn": "complex-test"}

		cmdResult executor.CMDResult
		wg        sync.WaitGroup
	)

	Context("Preparing the environment", func() {
		It("sets the namespace", func() {
			kustomization := fmt.Sprintf("%s/%s", conf.TestData.ComplexTest, "kustomization.yaml")
			ns, err := kustomize.GetNamespace(kustomization)
			Expect(err).NotTo(HaveOccurred(), "%w", err)
			conf.SetNamespace(ns)
		})
	})

	Context("When virtualization resources are applied", func() {
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
				Filename:       []string{conf.TestData.ComplexTest},
				FilenameOption: kc.Kustomize,
			})
			Expect(res.Error()).NotTo(HaveOccurred(), res.StdErr())
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

	Context("When cluster virtual images are applied", func() {
		It("checks CVIs phases", func() {
			By(fmt.Sprintf("CVIs should be in %s phases", PhaseReady))
			WaitPhaseByLabel(kc.ResourceCVI, PhaseReady, kc.WaitOptions{
				Labels:    testCaseLabel,
				Namespace: conf.Namespace,
				Timeout:   MaxWaitTimeout,
			})
		})
	})

	Context("When virtual machine classes are applied", func() {
		It("checks VMClasses phases", func() {
			By(fmt.Sprintf("VMClasses should be in %s phases", PhaseReady))
			WaitPhaseByLabel(kc.ResourceVMClass, PhaseReady, kc.WaitOptions{
				Labels:    testCaseLabel,
				Namespace: conf.Namespace,
				Timeout:   MaxWaitTimeout,
			})
		})
	})

	Context("When virtual machines IP addresses are applied", func() {
		It("patches custom VMIP with unassigned address", func() {
			vmipName := fmt.Sprintf("%s-%s", namePrefix, "vm-custom-ip")
			Eventually(func() error {
				return AssignIPToVMIP(vmipName)
			}).Should(Succeed())
		})

		It("checks VMIPs phases", func() {
			By(fmt.Sprintf("VMIPs should be in %s phases", PhaseAttached))
			WaitPhaseByLabel(kc.ResourceVMIP, PhaseAttached, kc.WaitOptions{
				Labels:    testCaseLabel,
				Namespace: conf.Namespace,
				Timeout:   MaxWaitTimeout,
			})
		})
	})

	Context("When virtual disks are applied", func() {
		It("checks VDs phases with consumers", func() {
			By(fmt.Sprintf("VDs should be in %s phases", PhaseReady))
			WaitPhaseByLabel(kc.ResourceVD, PhaseReady, kc.WaitOptions{
				ExcludedLabels: []string{"hasNoConsumer"},
				Labels:         testCaseLabel,
				Namespace:      conf.Namespace,
				Timeout:        MaxWaitTimeout,
			})
		})

		It("checks VDs phases with no consumers", func() {
			By(fmt.Sprintf("VDs should be in %s phases", phaseByVolumeBindingMode))
			WaitPhaseByLabel(kc.ResourceVD, phaseByVolumeBindingMode, kc.WaitOptions{
				Labels:    hasNoConsumerLabel,
				Namespace: conf.Namespace,
				Timeout:   MaxWaitTimeout,
			})
		})
	})

	Context("When virtual machines are applied", func() {
		It("checks VMs phases", func() {
			By("Virtual machine agents should be ready")
			WaitVmAgentReady(kc.WaitOptions{
				Labels:    testCaseLabel,
				Namespace: conf.Namespace,
				Timeout:   MaxWaitTimeout,
			})
		})
	})

	Context("When virtual machine block device attachments are applied", func() {
		It("checks VMBDAs phases", func() {
			By(fmt.Sprintf("VMBDAs should be in %s phases", PhaseAttached))
			WaitPhaseByLabel(kc.ResourceVMBDA, PhaseAttached, kc.WaitOptions{
				Labels:    testCaseLabel,
				Namespace: conf.Namespace,
				Timeout:   MaxWaitTimeout,
			})
		})
	})

	Describe("External connection", func() {
		Context("When Virtual machine agents are ready", func() {
			It("checks VMs external connectivity", func() {
				res := kubectl.List(kc.ResourceVM, kc.GetOptions{
					Labels:    testCaseLabel,
					Namespace: conf.Namespace,
					Output:    "jsonpath='{.items[*].metadata.name}'",
				})
				Expect(res.Error()).NotTo(HaveOccurred(), res.StdErr())

				vms := strings.Split(res.StdOut(), " ")
				CheckExternalConnection(externalHost, httpStatusOk, vms...)
			})
		})
	})

	Describe("Power state checks", func() {
		var (
			alwaysOnVMs          []string
			notAlwaysOnVMs       []string
			alwaysOnVMStopVMOPs  []string
			notAlwaysOnVMStopVMs []string
		)

		Context("Verify that the virtual machines are stopping by VMOPs", func() {
			It("stops VMs by VMOPs", func() {
				var vmList virtv2.VirtualMachineList
				err := GetObjects(kc.ResourceVM, &vmList, kc.GetOptions{
					Labels:    testCaseLabel,
					Namespace: conf.Namespace,
				})
				Expect(err).ShouldNot(HaveOccurred())

				for _, vmObj := range vmList.Items {
					if vmObj.Spec.RunPolicy == virtv2.AlwaysOnPolicy {
						alwaysOnVMs = append(alwaysOnVMs, vmObj.Name)
						alwaysOnVMStopVMOPs = append(alwaysOnVMStopVMOPs, fmt.Sprintf("%s-%s", vmObj.Name, strings.ToLower(string(virtv2.VMOPTypeStop))))
					} else {
						notAlwaysOnVMs = append(notAlwaysOnVMs, vmObj.Name)
						notAlwaysOnVMStopVMs = append(notAlwaysOnVMStopVMs, fmt.Sprintf("%s-%s", vmObj.Name, strings.ToLower(string(virtv2.VMOPTypeStop))))
					}
				}

				By("Trying to stop AlwaysOn VMs")
				StopVirtualMachinesByVMOP(alwaysOnLabel, alwaysOnVMs...)
				By("Trying to stop not AlwaysOn VMs")
				StopVirtualMachinesByVMOP(notAlwaysOnLabel, notAlwaysOnVMs...)
			})

			It("checks VMOPs and VMs phases", func() {
				By(fmt.Sprintf("AlwaysOn VM VMOPs should be in %s phases", virtv2.VMOPPhaseFailed))
				WaitResourcesByPhase(alwaysOnVMStopVMOPs, kc.ResourceVMOP, string(virtv2.VMOPPhaseFailed), kc.WaitOptions{
					Namespace: conf.Namespace,
					Timeout:   MaxWaitTimeout,
				})
				By(fmt.Sprintf("Not AlwaysOn VM VMOPs should be in %s phases", virtv2.VMOPPhaseCompleted))
				WaitResourcesByPhase(notAlwaysOnVMStopVMs, kc.ResourceVMOP, string(virtv2.VMOPPhaseCompleted), kc.WaitOptions{
					Namespace: conf.Namespace,
					Timeout:   MaxWaitTimeout,
				})
				By(fmt.Sprintf("AlwaysOn VMs should be in %s phases", virtv2.MachineRunning))
				WaitResourcesByPhase(alwaysOnVMs, kc.ResourceVM, string(virtv2.MachineRunning), kc.WaitOptions{
					Namespace: conf.Namespace,
					Timeout:   MaxWaitTimeout,
				})
				By(fmt.Sprintf("Not AlwaysOn VMs should be in %s phases", virtv2.MachineStopped))
				WaitResourcesByPhase(notAlwaysOnVMs, kc.ResourceVM, string(virtv2.MachineStopped), kc.WaitOptions{
					Namespace: conf.Namespace,
					Timeout:   MaxWaitTimeout,
				})
			})

			It("cleanup AlwaysOn VM VMOPs", func() {
				res := kubectl.Delete(kc.DeleteOptions{
					Namespace:      conf.Namespace,
					Labels:         alwaysOnLabel,
					IgnoreNotFound: true,
					Resource:       kc.ResourceVMOP,
				})
				Expect(res.Error()).NotTo(HaveOccurred(), "%v", res.StdErr())
			})
		})

		Context("Verify that the virtual machines are starting", func() {
			It("starts VMs by VMOP", func() {
				var vms virtv2.VirtualMachineList
				err := GetObjects(kc.ResourceVM, &vms, kc.GetOptions{
					Namespace: conf.Namespace,
					Labels:    testCaseLabel,
				})
				Expect(err).NotTo(HaveOccurred())

				var notAlwaysOnVMs []string
				for _, vm := range vms.Items {
					if vm.Spec.RunPolicy != virtv2.AlwaysOnPolicy {
						notAlwaysOnVMs = append(notAlwaysOnVMs, vm.Name)
					}
				}

				StartVirtualMachinesByVMOP(testCaseLabel, notAlwaysOnVMs...)
			})

			It("checks VMs and VMOPs phases", func() {
				By(fmt.Sprintf("VMOPs should be in %s phases", virtv2.VMOPPhaseCompleted))
				WaitPhaseByLabel(kc.ResourceVMOP, string(virtv2.VMOPPhaseCompleted), kc.WaitOptions{
					Labels:    testCaseLabel,
					Namespace: conf.Namespace,
					Timeout:   MaxWaitTimeout,
				})
				By("Virtual machine agents should be ready")
				WaitVmAgentReady(kc.WaitOptions{
					Labels:    testCaseLabel,
					Namespace: conf.Namespace,
					Timeout:   MaxWaitTimeout,
				})
			})
		})

		Context("Verify that the virtual machines are stopping by ssh", func() {
			It("stops VMs by ssh", func() {
				var vmList virtv2.VirtualMachineList
				err := GetObjects(kc.ResourceVM, &vmList, kc.GetOptions{
					Labels:    testCaseLabel,
					Namespace: conf.Namespace,
				})
				Expect(err).ShouldNot(HaveOccurred())

				alwaysOnVMs = []string{}
				notAlwaysOnVMs = []string{}
				for _, vmObj := range vmList.Items {
					if vmObj.Spec.RunPolicy == virtv2.AlwaysOnPolicy {
						alwaysOnVMs = append(alwaysOnVMs, vmObj.Name)
					} else {
						notAlwaysOnVMs = append(notAlwaysOnVMs, vmObj.Name)
					}
				}
				var vms []string
				vms = append(vms, alwaysOnVMs...)
				vms = append(vms, notAlwaysOnVMs...)

				StopVirtualMachinesBySSH(vms...)
			})

			It("checks VMs phases", func() {
				By(fmt.Sprintf("Not AlwaysOn VMs should be in %s phases", virtv2.MachineStopped))
				WaitResourcesByPhase(notAlwaysOnVMs, kc.ResourceVM, string(virtv2.MachineStopped), kc.WaitOptions{
					Namespace: conf.Namespace,
					Timeout:   MaxWaitTimeout,
				})
				By(fmt.Sprintf("AlwaysOn VMs should be in %s phases", virtv2.MachineRunning))
				WaitResourcesByPhase(alwaysOnVMs, kc.ResourceVM, string(virtv2.MachineRunning), kc.WaitOptions{
					Namespace: conf.Namespace,
					Timeout:   MaxWaitTimeout,
				})
			})

			It("start not AlwaysOn VMs", func() {
				CreateAndApplyVMOPsWithSuffix(testCaseLabel, "-after-ssh-stopping", virtv2.VMOPTypeStart, notAlwaysOnVMs...)
			})

			It("checks VMs and VMOPs phases", func() {
				By(fmt.Sprintf("VMOPs should be in %s phases", virtv2.VMOPPhaseCompleted))
				WaitPhaseByLabel(kc.ResourceVMOP, string(virtv2.VMOPPhaseCompleted), kc.WaitOptions{
					Labels:    testCaseLabel,
					Namespace: conf.Namespace,
					Timeout:   MaxWaitTimeout,
				})
				By("Virtual machine agents should be ready")
				WaitVmAgentReady(kc.WaitOptions{
					Labels:    testCaseLabel,
					Namespace: conf.Namespace,
					Timeout:   MaxWaitTimeout,
				})
			})
		})

		Context("Verify that the virtual machines are restarting by VMOP", func() {
			It("reboot VMs by VMOP", func() {
				res := kubectl.List(kc.ResourceVM, kc.GetOptions{
					Labels:    testCaseLabel,
					Namespace: conf.Namespace,
					Output:    "jsonpath='{.items[*].metadata.name}'",
				})
				Expect(res.Error()).NotTo(HaveOccurred(), res.StdErr())

				vms := strings.Split(res.StdOut(), " ")

				RebootVirtualMachinesByVMOP(testCaseLabel, vms...)
			})

			It("checks VMs and VMOPs phases", func() {
				By(fmt.Sprintf("VMOPs should be in %s phases", virtv2.VMOPPhaseCompleted))
				WaitPhaseByLabel(kc.ResourceVMOP, string(virtv2.VMOPPhaseCompleted), kc.WaitOptions{
					Labels:    testCaseLabel,
					Namespace: conf.Namespace,
					Timeout:   MaxWaitTimeout,
				})
				By("Virtual machine agents should be ready")
				WaitVmAgentReady(kc.WaitOptions{
					Labels:    testCaseLabel,
					Namespace: conf.Namespace,
					Timeout:   MaxWaitTimeout,
				})
			})
		})

		Context("Verify that the virtual machines are restarting by ssh", func() {
			It("reboot VMs by ssh", func() {
				res := kubectl.List(kc.ResourceVM, kc.GetOptions{
					Labels:    testCaseLabel,
					Namespace: conf.Namespace,
					Output:    "jsonpath='{.items[*].metadata.name}'",
				})
				Expect(res.Error()).NotTo(HaveOccurred(), res.StdErr())

				vms := strings.Split(res.StdOut(), " ")

				RebootVirtualMachinesBySSH(vms...)
			})

			It("checks VMs phases", func() {
				By("Virtual machine should be stopped")
				WaitPhaseByLabel(kc.ResourceVM, string(virtv2.MachineStopped), kc.WaitOptions{
					Labels:    testCaseLabel,
					Namespace: conf.Namespace,
					Timeout:   MaxWaitTimeout,
				})
				By("Virtual machine agents should be ready")
				WaitVmAgentReady(kc.WaitOptions{
					Labels:    testCaseLabel,
					Namespace: conf.Namespace,
					Timeout:   MaxWaitTimeout,
				})
			})
		})

		Context("Verify that the virtual machines are restarting after deleting pods", func() {
			It("reboot VMs by delete pods", func() {
				// kubectl may not return control for too long, and we may miss the Stopped phase and get stuck without using goroutines.
				wg.Add(1)
				go RebootVirtualMachinesByDeletePods(vmPodLabel, &cmdResult, &wg)
			})

			It("checks VMs phases", func() {
				By("Virtual machines should be stopped")
				WaitPhaseByLabel(kc.ResourceVM, string(virtv2.MachineStopped), kc.WaitOptions{
					Labels:    testCaseLabel,
					Namespace: conf.Namespace,
					Timeout:   MaxWaitTimeout,
				})
				By("Virtual machine agents should be ready")
				WaitVmAgentReady(kc.WaitOptions{
					Labels:    testCaseLabel,
					Namespace: conf.Namespace,
					Timeout:   MaxWaitTimeout,
				})
				wg.Wait()
				Expect(cmdResult.Error()).ShouldNot(HaveOccurred())
			})

			It("checks VMs external connection after reboot", func() {
				res := kubectl.List(kc.ResourceVM, kc.GetOptions{
					Labels:    testCaseLabel,
					Namespace: conf.Namespace,
					Output:    "jsonpath='{.items[*].metadata.name}'",
				})
				Expect(res.Error()).NotTo(HaveOccurred(), res.StdErr())

				vms := strings.Split(res.StdOut(), " ")

				CheckExternalConnection(externalHost, httpStatusOk, vms...)
			})
		})
	})

	Describe("Migrations", func() {
		skipConnectivityCheck := make(map[string]struct{})

		Context("When Virtual machine agents are ready", func() {
			It("starts migrations", func() {
				res := kubectl.List(kc.ResourceVM, kc.GetOptions{
					Labels:    testCaseLabel,
					Namespace: conf.Namespace,
					Output:    "jsonpath='{.items[*].metadata.name}'",
				})
				Expect(res.Error()).NotTo(HaveOccurred(), res.StdErr())

				vms := strings.Split(res.StdOut(), " ")

				// TODO: remove this temporary solution after the d8-cni-cilium fix for network accessibility of migrating virtual machines is merged.
				for _, name := range vms {
					vmObj := virtv2.VirtualMachine{}
					err := GetObject(kc.ResourceVM, name, &vmObj, kc.GetOptions{Namespace: conf.Namespace})
					Expect(err).NotTo(HaveOccurred(), "%w", err)

					nodeObj := corev1.Node{}
					err = GetObject("node", vmObj.Status.Node, &nodeObj, kc.GetOptions{})
					Expect(err).NotTo(HaveOccurred(), "%w", err)

					_, ok := nodeObj.Labels["node-role.kubernetes.io/control-plane"]
					if ok {
						skipConnectivityCheck[name] = struct{}{}
					}
				}
				// TODO ↥ to delete ↥.

				MigrateVirtualMachines(testCaseLabel, conf.TestData.ComplexTest, vms...)
			})
		})

		Context("When VMs migrations are applied", func() {
			It("checks VMs and VMOPs phases", func() {
				By(fmt.Sprintf("VMOPs should be in %s phases", virtv2.VMOPPhaseCompleted))
				WaitPhaseByLabel(kc.ResourceVMOP, string(virtv2.VMOPPhaseCompleted), kc.WaitOptions{
					Labels:    testCaseLabel,
					Namespace: conf.Namespace,
					Timeout:   MaxWaitTimeout,
				})
				By("Virtual machines should be migrated")
				WaitByLabel(kc.ResourceVM, kc.WaitOptions{
					Labels:    testCaseLabel,
					Namespace: conf.Namespace,
					Timeout:   MaxWaitTimeout,
					For:       "'jsonpath={.status.migrationState.result}=Succeeded'",
				})
			})

			It("checks VMs external connection after migrations", func() {
				res := kubectl.List(kc.ResourceVM, kc.GetOptions{
					Labels:    testCaseLabel,
					Namespace: conf.Namespace,
					Output:    "jsonpath='{.items[*].metadata.name}'",
				})
				Expect(res.Error()).NotTo(HaveOccurred(), res.StdErr())

				var vms []string
				for _, vm := range strings.Split(res.StdOut(), " ") {
					if _, skip := skipConnectivityCheck[vm]; skip {
						continue
					}

					vms = append(vms, vm)
				}

				CheckExternalConnection(externalHost, httpStatusOk, vms...)
			})
		})
	})

	Context("When test is completed", func() {
		It("deletes test case resources", func() {
			resourcesToDelete := ResourcesToDelete{
				AdditionalResources: []AdditionalResource{
					{
						kc.ResourceVMOP,
						testCaseLabel,
					},
				},
			}

			if config.IsCleanUpNeeded() {
				resourcesToDelete.KustomizationDir = conf.TestData.ComplexTest
			}

			DeleteTestCaseResources(resourcesToDelete)
		})
	})
})
