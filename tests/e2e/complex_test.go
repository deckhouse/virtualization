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

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/tests/e2e/config"
	"github.com/deckhouse/virtualization/tests/e2e/framework"
	kc "github.com/deckhouse/virtualization/tests/e2e/kubectl"
)

const VirtualMachineCount = 12

var _ = Describe("ComplexTest", Serial, framework.CommonE2ETestDecorators(), func() {
	var (
		testCaseLabel            = map[string]string{"testcase": "complex-test"}
		hasNoConsumerLabel       = map[string]string{"hasNoConsumer": "complex-test"}
		alwaysOnLabel            = map[string]string{"alwaysOn": "complex-test"}
		notAlwaysOnLabel         = map[string]string{"notAlwaysOn": "complex-test"}
		ns                       string
		phaseByVolumeBindingMode = GetPhaseByVolumeBindingModeForTemplateSc()
		f                        = framework.NewFramework("")
	)

	AfterEach(func() {
		if CurrentSpecReport().Failed() {
			SaveTestCaseDump(testCaseLabel, CurrentSpecReport().LeafNodeText, ns)
		}
	})

	BeforeAll(func() {
		kustomization := fmt.Sprintf("%s/%s", conf.TestData.ComplexTest, "kustomization.yaml")
		var err error
		ns, err = kustomize.GetNamespace(kustomization)
		Expect(err).NotTo(HaveOccurred(), "%w", err)

		CreateNamespace(ns)
	})

	Context("When virtualization resources are applied", func() {
		It("result should be succeeded", func() {
			res := kubectl.Apply(kc.ApplyOptions{
				Filename:       []string{conf.TestData.ComplexTest},
				FilenameOption: kc.Kustomize,
			})
			Expect(res.Error()).NotTo(HaveOccurred(), res.StdErr())
		})

		It("should fill empty virtualMachineClassName with the default class name", func() {
			defaultVMLabels := make(map[string]string, len(testCaseLabel)+1)
			for k, v := range testCaseLabel {
				defaultVMLabels[k] = v
			}
			defaultVMLabels["vm"] = "default"
			res := kubectl.List(kc.ResourceVM, kc.GetOptions{
				Labels:    testCaseLabel,
				Namespace: ns,
				Output:    "jsonpath='{.items[*].spec.virtualMachineClassName}'",
			})
			Expect(res.Error()).NotTo(HaveOccurred(), res.StdErr())

			Expect(res.StdOut()).Should(ContainSubstring(config.DefaultVirtualMachineClassName), "should fill empty .spec.virtualMachineClassName value")
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

	Context("When cluster virtual images are applied", func() {
		It("checks CVIs phases", func() {
			By(fmt.Sprintf("CVIs should be in %s phases", PhaseReady))
			WaitPhaseByLabel(kc.ResourceCVI, PhaseReady, kc.WaitOptions{
				Labels:    testCaseLabel,
				Namespace: ns,
				Timeout:   MaxWaitTimeout,
			})
		})
	})

	Context("When virtual machine classes are applied", func() {
		It("checks VMClasses phases", func() {
			By(fmt.Sprintf("VMClasses should be in %s phases", PhaseReady))
			WaitPhaseByLabel(kc.ResourceVMClass, PhaseReady, kc.WaitOptions{
				Labels:    testCaseLabel,
				Namespace: ns,
				Timeout:   MaxWaitTimeout,
			})
		})
	})

	Context("When virtual machines IP addresses are applied", func() {
		It("patches custom VMIP with unassigned address", func() {
			vmipName := fmt.Sprintf("%s-%s", namePrefix, "vm-custom-ip")
			Eventually(func() error {
				return AssignIPToVMIP(f, ns, vmipName)
			}).WithTimeout(LongWaitDuration).WithPolling(Interval).Should(Succeed())
		})

		It("checks VMIPs phases", func() {
			By(fmt.Sprintf("VMIPs should be in %s phases", PhaseAttached))
			WaitPhaseByLabel(kc.ResourceVMIP, PhaseAttached, kc.WaitOptions{
				Labels:    testCaseLabel,
				Namespace: ns,
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
				Namespace:      ns,
				Timeout:        MaxWaitTimeout,
			})
		})

		It("checks VDs phases with no consumers", func() {
			By(fmt.Sprintf("VDs should be in %s phases", phaseByVolumeBindingMode))
			WaitPhaseByLabel(kc.ResourceVD, phaseByVolumeBindingMode, kc.WaitOptions{
				Labels:    hasNoConsumerLabel,
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

	Context("When virtual machine block device attachments are applied", func() {
		It("checks VMBDAs phases", func() {
			By(fmt.Sprintf("VMBDAs should be in %s phases", PhaseAttached))
			WaitPhaseByLabel(kc.ResourceVMBDA, PhaseAttached, kc.WaitOptions{
				Labels:    testCaseLabel,
				Namespace: ns,
				Timeout:   MaxWaitTimeout,
			})
		})
	})

	Describe("External connection", func() {
		Context("When Virtual machine agents are ready", func() {
			It("checks VMs external connectivity", func() {
				res := kubectl.List(kc.ResourceVM, kc.GetOptions{
					Labels:    testCaseLabel,
					Namespace: ns,
					Output:    "jsonpath='{.items[*].metadata.name}'",
				})
				Expect(res.Error()).NotTo(HaveOccurred(), res.StdErr())

				vms := strings.Split(res.StdOut(), " ")
				CheckCiliumAgents(kubectl, ns, vms...)
				CheckExternalConnection(externalHost, httpStatusOk, ns, vms...)
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
				var vmList v1alpha2.VirtualMachineList
				err := GetObjects(kc.ResourceVM, &vmList, kc.GetOptions{
					Labels:    testCaseLabel,
					Namespace: ns,
				})
				Expect(err).ShouldNot(HaveOccurred())
				Expect(len(vmList.Items)).To(Equal(VirtualMachineCount))

				for _, vmObj := range vmList.Items {
					if vmObj.Spec.RunPolicy == v1alpha2.AlwaysOnPolicy {
						alwaysOnVMs = append(alwaysOnVMs, vmObj.Name)
						alwaysOnVMStopVMOPs = append(alwaysOnVMStopVMOPs, fmt.Sprintf("%s-%s", vmObj.Name, strings.ToLower(string(v1alpha2.VMOPTypeStop))))
					} else {
						notAlwaysOnVMs = append(notAlwaysOnVMs, vmObj.Name)
						notAlwaysOnVMStopVMs = append(notAlwaysOnVMStopVMs, fmt.Sprintf("%s-%s", vmObj.Name, strings.ToLower(string(v1alpha2.VMOPTypeStop))))
					}
				}

				By("Trying to stop AlwaysOn VMs")
				StopVirtualMachinesByVMOP(alwaysOnLabel, ns, alwaysOnVMs...)
				By("Trying to stop not AlwaysOn VMs")
				StopVirtualMachinesByVMOP(notAlwaysOnLabel, ns, notAlwaysOnVMs...)
			})

			It("checks VMOPs and VMs phases", func() {
				By(fmt.Sprintf("AlwaysOn VM VMOPs should be in %s phases", v1alpha2.VMOPPhaseFailed))
				WaitResourcesByPhase(alwaysOnVMStopVMOPs, kc.ResourceVMOP, string(v1alpha2.VMOPPhaseFailed), kc.WaitOptions{
					Namespace: ns,
					Timeout:   MaxWaitTimeout,
				})
				By(fmt.Sprintf("Not AlwaysOn VM VMOPs should be in %s phases", v1alpha2.VMOPPhaseCompleted))
				WaitResourcesByPhase(notAlwaysOnVMStopVMs, kc.ResourceVMOP, string(v1alpha2.VMOPPhaseCompleted), kc.WaitOptions{
					Namespace: ns,
					Timeout:   MaxWaitTimeout,
				})
				By(fmt.Sprintf("AlwaysOn VMs should be in %s phases", v1alpha2.MachineRunning))
				WaitResourcesByPhase(alwaysOnVMs, kc.ResourceVM, string(v1alpha2.MachineRunning), kc.WaitOptions{
					Namespace: ns,
					Timeout:   MaxWaitTimeout,
				})
				By(fmt.Sprintf("Not AlwaysOn VMs should be in %s phases", v1alpha2.MachineStopped))
				WaitResourcesByPhase(notAlwaysOnVMs, kc.ResourceVM, string(v1alpha2.MachineStopped), kc.WaitOptions{
					Namespace: ns,
					Timeout:   MaxWaitTimeout,
				})
			})

			It("cleanup AlwaysOn VM VMOPs", func() {
				res := kubectl.Delete(kc.DeleteOptions{
					Namespace:      ns,
					Labels:         alwaysOnLabel,
					IgnoreNotFound: true,
					Resource:       kc.ResourceVMOP,
				})
				Expect(res.Error()).NotTo(HaveOccurred(), "%v", res.StdErr())
			})
		})

		Context("Verify that the virtual machines are starting", func() {
			It("starts VMs by VMOP", func() {
				var vms v1alpha2.VirtualMachineList
				err := GetObjects(kc.ResourceVM, &vms, kc.GetOptions{
					Namespace: ns,
					Labels:    testCaseLabel,
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(len(vms.Items)).To(Equal(VirtualMachineCount))

				var notAlwaysOnVMs []string
				for _, vm := range vms.Items {
					if vm.Spec.RunPolicy != v1alpha2.AlwaysOnPolicy {
						notAlwaysOnVMs = append(notAlwaysOnVMs, vm.Name)
					}
				}

				StartVirtualMachinesByVMOP(testCaseLabel, ns, notAlwaysOnVMs...)
			})

			It("checks VMs and VMOPs phases", func() {
				By(fmt.Sprintf("VMOPs should be in %s phases", v1alpha2.VMOPPhaseCompleted))
				WaitPhaseByLabel(kc.ResourceVMOP, string(v1alpha2.VMOPPhaseCompleted), kc.WaitOptions{
					Labels:    testCaseLabel,
					Namespace: ns,
					Timeout:   MaxWaitTimeout,
				})
				By("Virtual machine agents should be ready")
				WaitVMAgentReady(kc.WaitOptions{
					Labels:    testCaseLabel,
					Namespace: ns,
					Timeout:   MaxWaitTimeout,
				})
			})
		})

		Context("Verify that the virtual machines are stopping by ssh", func() {
			It("stops VMs by ssh", func() {
				var vmList v1alpha2.VirtualMachineList
				err := GetObjects(kc.ResourceVM, &vmList, kc.GetOptions{
					Labels:    testCaseLabel,
					Namespace: ns,
				})
				Expect(err).ShouldNot(HaveOccurred())
				Expect(len(vmList.Items)).To(Equal(VirtualMachineCount))

				alwaysOnVMs = []string{}
				notAlwaysOnVMs = []string{}
				for _, vmObj := range vmList.Items {
					if vmObj.Spec.RunPolicy == v1alpha2.AlwaysOnPolicy {
						alwaysOnVMs = append(alwaysOnVMs, vmObj.Name)
					} else {
						notAlwaysOnVMs = append(notAlwaysOnVMs, vmObj.Name)
					}
				}
				var vms []string
				vms = append(vms, alwaysOnVMs...)
				vms = append(vms, notAlwaysOnVMs...)

				StopVirtualMachinesBySSH(ns, vms...)
			})

			It("checks VMs phases", func() {
				By(fmt.Sprintf("Not AlwaysOn VMs should be in %s phases", v1alpha2.MachineStopped))
				WaitResourcesByPhase(notAlwaysOnVMs, kc.ResourceVM, string(v1alpha2.MachineStopped), kc.WaitOptions{
					Namespace: ns,
					Timeout:   MaxWaitTimeout,
				})
				By(fmt.Sprintf("AlwaysOn VMs should be in %s phases", v1alpha2.MachineRunning))
				WaitResourcesByPhase(alwaysOnVMs, kc.ResourceVM, string(v1alpha2.MachineRunning), kc.WaitOptions{
					Namespace: ns,
					Timeout:   MaxWaitTimeout,
				})
			})

			It("start not AlwaysOn VMs", func() {
				CreateAndApplyVMOPsWithSuffix(testCaseLabel, "-after-ssh-stopping", v1alpha2.VMOPTypeStart, ns, notAlwaysOnVMs...)
			})

			It("checks VMs and VMOPs phases", func() {
				By(fmt.Sprintf("VMOPs should be in %s phases", v1alpha2.VMOPPhaseCompleted))
				WaitPhaseByLabel(kc.ResourceVMOP, string(v1alpha2.VMOPPhaseCompleted), kc.WaitOptions{
					Labels:    testCaseLabel,
					Namespace: ns,
					Timeout:   MaxWaitTimeout,
				})
				By("Virtual machine agents should be ready")
				WaitVMAgentReady(kc.WaitOptions{
					Labels:    testCaseLabel,
					Namespace: ns,
					Timeout:   MaxWaitTimeout,
				})
			})
		})

		Context("Verify that the virtual machines are restarting by VMOP", func() {
			It("reboot VMs by VMOP", func() {
				res := kubectl.List(kc.ResourceVM, kc.GetOptions{
					Labels:    testCaseLabel,
					Namespace: ns,
					Output:    "jsonpath='{.items[*].metadata.name}'",
				})
				Expect(res.Error()).NotTo(HaveOccurred(), res.StdErr())

				vms := strings.Split(res.StdOut(), " ")

				RebootVirtualMachinesByVMOP(testCaseLabel, ns, vms...)
			})

			It("checks VMs and VMOPs phases", func() {
				By(fmt.Sprintf("VMOPs should be in %s phases", v1alpha2.VMOPPhaseCompleted))
				WaitPhaseByLabel(kc.ResourceVMOP, string(v1alpha2.VMOPPhaseCompleted), kc.WaitOptions{
					Labels:    testCaseLabel,
					Namespace: ns,
					Timeout:   MaxWaitTimeout,
				})
				By("Virtual machine agents should be ready")
				WaitVMAgentReady(kc.WaitOptions{
					Labels:    testCaseLabel,
					Namespace: ns,
					Timeout:   MaxWaitTimeout,
				})
			})
		})

		Context("Verify that the virtual machines are restarting by ssh", func() {
			It("reboot VMs by ssh", func() {
				wg := &sync.WaitGroup{}

				By("Virtual machines should be stopped", func() {
					wg.Add(1)
					go func() {
						defer GinkgoRecover()
						defer wg.Done()
						WaitPhaseByLabel(kc.ResourceVM, string(v1alpha2.MachineStopped), kc.WaitOptions{
							Labels:    testCaseLabel,
							Namespace: ns,
							Timeout:   MaxWaitTimeout,
						})
					}()
				})

				res := kubectl.List(kc.ResourceVM, kc.GetOptions{
					Labels:    testCaseLabel,
					Namespace: ns,
					Output:    "jsonpath='{.items[*].metadata.name}'",
				})
				Expect(res.Error()).NotTo(HaveOccurred(), res.StdErr())

				vms := strings.Split(res.StdOut(), " ")

				RebootVirtualMachinesBySSH(ns, vms...)

				By("Virtual machines agent should be ready", func() {
					WaitVMAgentReady(kc.WaitOptions{
						Labels:    testCaseLabel,
						Namespace: ns,
						Timeout:   MaxWaitTimeout,
					})
				})
				wg.Wait()
			})
		})

		Context("Verify that the virtual machines are restarting after deleting their pods", func() {
			It("reboots the VMs by deleting their pods", func() {
				wg := &sync.WaitGroup{}

				By("Virtual machines should be stopped", func() {
					wg.Add(1)
					go func() {
						defer GinkgoRecover()
						defer wg.Done()
						WaitPhaseByLabel(kc.ResourceVM, string(v1alpha2.MachineStopped), kc.WaitOptions{
							Labels:    testCaseLabel,
							Namespace: ns,
							Timeout:   MaxWaitTimeout,
						})
					}()
				})

				res := kubectl.Delete(kc.DeleteOptions{
					IgnoreNotFound: true,
					Labels:         testCaseLabel,
					Namespace:      ns,
					Resource:       kc.ResourcePod,
				})
				Expect(res.Error()).NotTo(HaveOccurred())

				By("Virtual machines agent should be ready", func() {
					WaitVMAgentReady(kc.WaitOptions{
						Labels:    testCaseLabel,
						Namespace: ns,
						Timeout:   MaxWaitTimeout,
					})
				})

				wg.Wait()
			})

			It("checks VMs external connection after reboot", func() {
				res := kubectl.List(kc.ResourceVM, kc.GetOptions{
					Labels:    testCaseLabel,
					Namespace: ns,
					Output:    "jsonpath='{.items[*].metadata.name}'",
				})
				Expect(res.Error()).NotTo(HaveOccurred(), res.StdErr())

				vms := strings.Split(res.StdOut(), " ")

				CheckCiliumAgents(kubectl, ns, vms...)
				CheckExternalConnection(externalHost, httpStatusOk, ns, vms...)
			})
		})
	})

	Describe("Migrations", func() {
		Context("When Virtual machine agents are ready", func() {
			It("starts migrations", func() {
				res := kubectl.List(kc.ResourceVM, kc.GetOptions{
					Labels:    testCaseLabel,
					Namespace: ns,
					Output:    "jsonpath='{.items[*].metadata.name}'",
				})
				Expect(res.Error()).NotTo(HaveOccurred(), res.StdErr())

				vms := strings.Split(res.StdOut(), " ")

				MigrateVirtualMachines(testCaseLabel, ns, vms...)
			})
		})

		Context("When VMs migrations are applied", func() {
			It("checks VMs and VMOPs phases", func() {
				By(fmt.Sprintf("VMOPs should be in %s phases", v1alpha2.VMOPPhaseCompleted))
				WaitPhaseByLabel(kc.ResourceVMOP, string(v1alpha2.VMOPPhaseCompleted), kc.WaitOptions{
					Labels:    testCaseLabel,
					Namespace: ns,
					Timeout:   MaxWaitTimeout,
				})
				By("Virtual machines should be migrated")
				WaitByLabel(kc.ResourceVM, kc.WaitOptions{
					Labels:    testCaseLabel,
					Namespace: ns,
					Timeout:   MaxWaitTimeout,
					For:       "'jsonpath={.status.migrationState.result}=Succeeded'",
				})
			})

			It("checks VMs external connection after migrations", func() {
				res := kubectl.List(kc.ResourceVM, kc.GetOptions{
					Labels:    testCaseLabel,
					Namespace: ns,
					Output:    "jsonpath='{.items[*].metadata.name}'",
				})
				Expect(res.Error()).NotTo(HaveOccurred(), res.StdErr())

				vms := strings.Split(res.StdOut(), " ")

				CheckCiliumAgents(kubectl, ns, vms...)
				CheckExternalConnection(externalHost, httpStatusOk, ns, vms...)
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

			DeleteTestCaseResources(ns, resourcesToDelete)
		})
	})
})

func AssignIPToVMIP(f *framework.Framework, vmipNamespace, vmipName string) error {
	mc, err := f.GetVirtualizationModuleConfig()
	if err != nil {
		return err
	}

	assignErr := fmt.Sprintf("cannot patch VMIP %q with unnassigned IP address", vmipName)
	unassignedIP, err := FindUnassignedIP(mc.Spec.Settings.VirtualMachineCIDRs)
	if err != nil {
		return fmt.Errorf("%s\n%w", assignErr, err)
	}

	patch := fmt.Sprintf(`{"spec":{"staticIP":%q}}`, unassignedIP)
	err = MergePatchResource(kc.ResourceVMIP, vmipNamespace, vmipName, patch)
	if err != nil {
		return fmt.Errorf("%s\n%w", assignErr, err)
	}

	vmip := v1alpha2.VirtualMachineIPAddress{}
	err = GetObject(kc.ResourceVMIP, vmipName, &vmip, kc.GetOptions{
		Namespace: vmipNamespace,
	})
	if err != nil {
		return fmt.Errorf("%s\n%w", assignErr, err)
	}

	jsonPath := fmt.Sprintf("'jsonpath={.status.phase}=%s'", PhaseAttached)
	waitOpts := kc.WaitOptions{
		Namespace: vmipNamespace,
		For:       jsonPath,
		Timeout:   ShortWaitDuration,
	}
	res := kubectl.WaitResources(kc.ResourceVMIP, waitOpts, vmipName)
	if res.Error() != nil {
		return fmt.Errorf("%s\n%s", assignErr, res.StdErr())
	}

	return nil
}
