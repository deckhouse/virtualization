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
	"encoding/json"
	"fmt"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
	"github.com/deckhouse/virtualization/tests/e2e/config"
	"github.com/deckhouse/virtualization/tests/e2e/ginkgoutil"
	kc "github.com/deckhouse/virtualization/tests/e2e/kubectl"
)

func ExpectVirtualMachineIsMigratable(vmObj *virtv2.VirtualMachine) {
	GinkgoHelper()
	for _, c := range vmObj.Status.Conditions {
		if c.Type == string(vmcondition.TypeMigratable) {
			Expect(c.Status).Should(Equal(metav1.ConditionTrue),
				"the `VirtualMachine` %s should be %q",
				vmObj.Name,
				vmcondition.TypeMigratable,
			)
		}
	}
}

func DefineTargetNode(sourceNode string, targetLabel map[string]string) (string, error) {
	nodes := &corev1.NodeList{}
	err := GetObjects(kc.ResourceNode, nodes, kc.GetOptions{
		Labels: targetLabel,
	})
	if err != nil {
		return "", err
	}
	for _, n := range nodes.Items {
		if n.Name != sourceNode {
			for _, c := range n.Status.Conditions {
				if c.Type == corev1.NodeReady && c.Status == corev1.ConditionTrue {
					return n.Name, nil
				}
			}
		}
	}
	return "", fmt.Errorf("failed to define a target node")
}

func GetVirtualMachineObjByLabel(namespace string, label map[string]string) (*virtv2.VirtualMachine, error) {
	vmObjects := virtv2.VirtualMachineList{}
	err := GetObjects(kc.ResourceVM, &vmObjects, kc.GetOptions{
		Labels:    label,
		Namespace: namespace,
	})
	if len(vmObjects.Items) != 1 {
		return nil, fmt.Errorf("there is only one `VirtualMachine` with the %q label in this case", label)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to obtain the %q `VirtualMachine`", label)
	}
	return &vmObjects.Items[0], nil
}

func GenerateNodeAffinityPatch(key string, operator corev1.NodeSelectorOperator, values []string) ([]byte, error) {
	vmAffinity := &virtv2.VMAffinity{
		NodeAffinity: &corev1.NodeAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
				NodeSelectorTerms: []corev1.NodeSelectorTerm{
					{
						MatchExpressions: []corev1.NodeSelectorRequirement{
							{
								Key:      key,
								Operator: operator,
								Values:   values,
							},
						},
					},
				},
			},
		},
	}

	b, err := json.Marshal(vmAffinity)
	if err != nil {
		return nil, err
	}
	return b, nil
}

func GenerateVirtualMachineAndPodAntiAffinityPatch(key, topologyKey string, operator metav1.LabelSelectorOperator, values []string) ([]byte, error) {
	vmAndPodAntiAffinity := &virtv2.VirtualMachineAndPodAntiAffinity{
		RequiredDuringSchedulingIgnoredDuringExecution: []virtv2.VirtualMachineAndPodAffinityTerm{
			{
				LabelSelector: &metav1.LabelSelector{
					MatchExpressions: []metav1.LabelSelectorRequirement{
						{
							Key:      key,
							Operator: operator,
							Values:   values,
						},
					},
				},
				TopologyKey: topologyKey,
			},
		},
	}

	b, err := json.Marshal(vmAndPodAntiAffinity)
	if err != nil {
		return nil, err
	}
	return b, nil
}

func GenerateVirtualMachineAndPodAffinityPatch(key, topologyKey string, operator metav1.LabelSelectorOperator, values []string) ([]byte, error) {
	vmAndPodAffinity := &virtv2.VirtualMachineAndPodAffinity{
		RequiredDuringSchedulingIgnoredDuringExecution: []virtv2.VirtualMachineAndPodAffinityTerm{
			{
				LabelSelector: &metav1.LabelSelector{
					MatchExpressions: []metav1.LabelSelectorRequirement{
						{
							Key:      key,
							Operator: operator,
							Values:   values,
						},
					},
				},
				TopologyKey: topologyKey,
			},
		},
	}

	b, err := json.Marshal(vmAndPodAffinity)
	if err != nil {
		return nil, err
	}
	return b, nil
}

var _ = Describe("Virtual machine affinity and toleration", ginkgoutil.CommonE2ETestDecorators(), func() {
	BeforeAll(func() {
		if config.IsReusable() {
			Skip("Test not available in REUSABLE mode: not supported yet.")
		}

		kustomization := fmt.Sprintf("%s/%s", conf.TestData.AffinityToleration, "kustomization.yaml")
		ns, err := kustomize.GetNamespace(kustomization)
		Expect(err).NotTo(HaveOccurred(), "%w", err)
		conf.SetNamespace(ns)
	})

	const (
		nodeLabelKey   = "kubernetes.io/hostname"
		masterLabelKey = "node.deckhouse.io/group"
		vmKey          = "vm"
	)

	var (
		migratingStatusPollingInterval = 1 * time.Second
		testCaseLabel                  = map[string]string{"testcase": "affinity-toleration"}
		vmA                            = map[string]string{"vm": "vm-a"}
		vmB                            = map[string]string{"vm": "vm-b"}
		vmC                            = map[string]string{"vm": "vm-c"}
		vmD                            = map[string]string{"vm": "vm-d"}
		vmNodeSelector                 = map[string]string{"vm": "vm-node-selector"}
		vmNodeAffinity                 = map[string]string{"vm": "vm-node-affinity"}
		workerNodeLabel                = map[string]string{"node.deckhouse.io/group": "worker"}
		masterNodeLabel                = map[string]string{"node.deckhouse.io/group": "master"}
	)

	AfterEach(func() {
		if CurrentSpecReport().Failed() {
			SaveTestResources(testCaseLabel)
		}
	})

	Context("When the virtualization resources are applied:", func() {
		It("result should be succeeded", func() {
			res := kubectl.Apply(kc.ApplyOptions{
				Filename:       []string{conf.TestData.AffinityToleration},
				FilenameOption: kc.Kustomize,
			})
			Expect(res.Error()).NotTo(HaveOccurred(), "failed to apply the test case resources")
		})

		It("checks the resources phase", func() {
			By(fmt.Sprintf("`VirtualImages` should be in the %q phase", virtv2.ImageReady), func() {
				WaitPhaseByLabel(kc.ResourceVI, PhaseReady, kc.WaitOptions{
					Labels:    testCaseLabel,
					Namespace: conf.Namespace,
					Timeout:   MaxWaitTimeout,
				})
			})
			By(fmt.Sprintf("`VirtualMachineClasses` should be in %s phases", virtv2.ClassPhaseReady), func() {
				WaitPhaseByLabel(kc.ResourceVMClass, PhaseReady, kc.WaitOptions{
					Labels:    testCaseLabel,
					Namespace: conf.Namespace,
					Timeout:   MaxWaitTimeout,
				})
			})
			By(fmt.Sprintf("`VirtualDisks` should be in the %q phase", virtv2.DiskReady), func() {
				WaitPhaseByLabel(kc.ResourceVD, PhaseReady, kc.WaitOptions{
					Labels:    testCaseLabel,
					Namespace: conf.Namespace,
					Timeout:   MaxWaitTimeout,
				})
			})
			By("`VirtualMachines` agents should be ready", func() {
				WaitVmAgentReady(kc.WaitOptions{
					Labels:    testCaseLabel,
					Namespace: conf.Namespace,
					Timeout:   MaxWaitTimeout,
				})
			})
		})
	})

	Context("When the virtual machines agents are ready", func() {
		It("checks the `status.nodeName` field of the `VirtualMachines`", func() {
			var (
				vmObjA = &virtv2.VirtualMachine{}
				vmObjB = &virtv2.VirtualMachine{}
				vmObjC = &virtv2.VirtualMachine{}
				vmObjD = &virtv2.VirtualMachine{}
				err    error
			)
			By("Obtain the `VirtualMachine` objects", func() {
				vmObjA, err = GetVirtualMachineObjByLabel(conf.Namespace, vmA)
				Expect(err).NotTo(HaveOccurred(), "failed to obtain the %q `VirtualMachine`", vmA)
				vmObjB, err = GetVirtualMachineObjByLabel(conf.Namespace, vmB)
				Expect(err).NotTo(HaveOccurred(), "failed to obtain the %q `VirtualMachine`", vmB)
				vmObjC, err = GetVirtualMachineObjByLabel(conf.Namespace, vmC)
				Expect(err).NotTo(HaveOccurred(), "failed to obtain the %q `VirtualMachine`", vmC)
				vmObjD, err = GetVirtualMachineObjByLabel(conf.Namespace, vmD)
				Expect(err).NotTo(HaveOccurred(), "failed to obtain the %q `VirtualMachine`", vmD)
			})
			By("Set affinity when creating the `VirtualMachines`.`: `vm-a` and `vm-c` should be running on the same node", func() {
				Expect(vmObjA.Status.Node).Should(Equal(vmObjC.Status.Node), "%q and %q `VirtualMachines` should be running on the same node", vmA, vmC)
			})
			By("Set anti-affinity when creating the `VirtualMachines`: `vm-a` and `vm-b` should be running on the different nodes", func() {
				Expect(vmObjA.Status.Node).ShouldNot(Equal(vmObjB.Status.Node), "%q and %q `VirtualMachines` should be running on the different nodes", vmA, vmB)
			})
			By("Set toleration when creating the `VirtualMachines`: `vm-d` should be running on a master node", func() {
				nodeObj := corev1.Node{}
				err := GetObject(kc.ResourceNode, vmObjD.Status.Node, &nodeObj, kc.GetOptions{})
				Expect(err).NotTo(HaveOccurred(), "failed to obtain the %q node", vmObjD.Status.Node)
				Expect(nodeObj.Labels).Should(HaveKeyWithValue(masterLabelKey, masterNodeLabel[masterLabelKey]), "%q `VirtualMachine` should be running on a master node", vmD)
			})
			By("Change affinity to anti-affinity when the `VirtualMachines` are runnning: `vm-a` and `vm-c` should be running on the different nodes", func() {
				wg := &sync.WaitGroup{}

				ExpectVirtualMachineIsMigratable(vmObjC)
				p, err := GenerateVirtualMachineAndPodAntiAffinityPatch(vmKey, nodeLabelKey, metav1.LabelSelectorOpIn, []string{vmA[vmKey]})
				Expect(err).NotTo(HaveOccurred(), "failed to generate the `VirtualMachineAndPodAntiAffinity` patch")
				jsonPatchAdd := &kc.JsonPatch{
					Op:    "add",
					Path:  "/spec/affinity/virtualMachineAndPodAntiAffinity",
					Value: string(p),
				}
				jsonPatchRemove := &kc.JsonPatch{
					Op:   "remove",
					Path: "/spec/affinity/virtualMachineAndPodAffinity",
				}
				wg.Add(1)
				go func() {
					defer GinkgoRecover()
					defer wg.Done()
					Eventually(func() error {
						updatedVmObjC := &virtv2.VirtualMachine{}
						err := GetObject(virtv2.VirtualMachineResource, vmObjC.Name, updatedVmObjC, kc.GetOptions{
							Namespace: conf.Namespace,
						})
						if err != nil {
							return err
						}
						if updatedVmObjC.Status.Phase != virtv2.MachineMigrating {
							return fmt.Errorf("the `VirtualMachine` should be %s", virtv2.MachineMigrating)
						}
						return nil
					}).WithTimeout(Timeout).WithPolling(migratingStatusPollingInterval).Should(Succeed())
				}()
				res := kubectl.PatchResource(kc.ResourceVM, vmObjC.Name, kc.PatchOptions{
					JsonPatch: []*kc.JsonPatch{
						jsonPatchAdd,
						jsonPatchRemove,
					},
					Namespace: conf.Namespace,
				})
				Expect(res.Error()).NotTo(HaveOccurred(), "failed to patch the %q `VirtualMachine`", vmC)
				wg.Wait()

				WaitVmAgentReady(kc.WaitOptions{
					Labels:    vmC,
					Namespace: conf.Namespace,
					Timeout:   MaxWaitTimeout,
				})
				updatedVmObjC := &virtv2.VirtualMachine{}
				err = GetObject(virtv2.VirtualMachineResource, vmObjC.Name, updatedVmObjC, kc.GetOptions{
					Namespace: conf.Namespace,
				})
				Expect(err).NotTo(HaveOccurred(), "failed to obtain the %q `VirtualMachine` object", vmC)
				Expect(updatedVmObjC.Status.MigrationState.Source.Node).Should(Equal(vmObjC.Status.Node))
				Expect(updatedVmObjC.Status.MigrationState.Target.Node).ShouldNot(Equal(vmObjA.Status.Node))
				Expect(updatedVmObjC.Status.Node).ShouldNot(Equal(vmObjA.Status.Node))
			})
			By("Change anti-affinity to affinity when the `VirtualMachines` are runnning: `vm-a` and `vm-c` should be running on the same node", func() {
				wg := &sync.WaitGroup{}

				updatedVmObjC := &virtv2.VirtualMachine{}
				err = GetObject(virtv2.VirtualMachineResource, vmObjC.Name, updatedVmObjC, kc.GetOptions{
					Namespace: conf.Namespace,
				})

				ExpectVirtualMachineIsMigratable(updatedVmObjC)
				p, err := GenerateVirtualMachineAndPodAffinityPatch(vmKey, nodeLabelKey, metav1.LabelSelectorOpIn, []string{vmA[vmKey]})
				Expect(err).NotTo(HaveOccurred(), "failed to generate the `VirtualMachineAndPodAffinity` patch")
				jsonPatchAdd := &kc.JsonPatch{
					Op:    "add",
					Path:  "/spec/affinity/virtualMachineAndPodAffinity",
					Value: string(p),
				}
				jsonPatchRemove := &kc.JsonPatch{
					Op:   "remove",
					Path: "/spec/affinity/virtualMachineAndPodAntiAffinity",
				}
				wg.Add(1)
				go func() {
					defer GinkgoRecover()
					defer wg.Done()
					Eventually(func() error {
						updatedVmObjC := &virtv2.VirtualMachine{}
						err := GetObject(virtv2.VirtualMachineResource, vmObjC.Name, updatedVmObjC, kc.GetOptions{
							Namespace: conf.Namespace,
						})
						if err != nil {
							return err
						}
						if updatedVmObjC.Status.Phase != virtv2.MachineMigrating {
							return fmt.Errorf("the `VirtualMachine` should be %s", virtv2.MachineMigrating)
						}
						return nil
					}).WithTimeout(Timeout).WithPolling(migratingStatusPollingInterval).Should(Succeed())
				}()
				res := kubectl.PatchResource(kc.ResourceVM, vmObjC.Name, kc.PatchOptions{
					JsonPatch: []*kc.JsonPatch{
						jsonPatchAdd,
						jsonPatchRemove,
					},
					Namespace: conf.Namespace,
				})
				Expect(res.Error()).NotTo(HaveOccurred(), "failed to patch the %q `VirtualMachine`", vmC)
				wg.Wait()

				WaitVmAgentReady(kc.WaitOptions{
					Labels:    vmC,
					Namespace: conf.Namespace,
					Timeout:   MaxWaitTimeout,
				})
				updatedVmObjC = &virtv2.VirtualMachine{}
				err = GetObject(virtv2.VirtualMachineResource, vmObjC.Name, updatedVmObjC, kc.GetOptions{
					Namespace: conf.Namespace,
				})
				Expect(err).NotTo(HaveOccurred(), "failed to obtain the %q `VirtualMachine` object", vmC)
				Expect(updatedVmObjC.Status.MigrationState.Source.Node).ShouldNot(Equal(vmObjA.Status.Node))
				Expect(updatedVmObjC.Status.MigrationState.Target.Node).Should(Equal(vmObjA.Status.Node))
				Expect(updatedVmObjC.Status.Node).Should(Equal(vmObjA.Status.Node))
			})
		})
	})

	Context("When the virtual machine `node-selector` agent is ready", func() {
		It("sets the `spec.nodeSelector` field", func() {
			var (
				sourceNode string
				targetNode string
				err        error
			)
			vmObj := &virtv2.VirtualMachine{}
			By("Sets the `spec.nodeSelector` with the `status.nodeSelector` value", func() {
				vmObj, err = GetVirtualMachineObjByLabel(conf.Namespace, vmNodeSelector)
				Expect(err).NotTo(HaveOccurred(), "failed to obtain the %q `VirtualMachine` object", vmNodeSelector)
				ExpectVirtualMachineIsMigratable(vmObj)
				sourceNode = vmObj.Status.Node
				Expect(sourceNode).ShouldNot(BeEmpty(), "the `vm.status.nodeName` should have a value")
				mergePatch := fmt.Sprintf(`{"spec":{"nodeSelector":{%q:%q}}}`, nodeLabelKey, sourceNode)
				err = MergePatchResource(kc.ResourceVM, vmObj.Name, mergePatch)
				Expect(err).NotTo(HaveOccurred(), "failed to patch the %q `VirtualMachine`", vmNodeSelector)
			})
			By("The `VirtualMachine` should not be migrated", func() {
				time.Sleep(20 * time.Second)
				updatedVmObj := &virtv2.VirtualMachine{}
				err := GetObject(virtv2.VirtualMachineResource, vmObj.Name, updatedVmObj, kc.GetOptions{
					Namespace: conf.Namespace,
				})
				Expect(err).NotTo(HaveOccurred(), "failed to obtain the %q `VirtualMachine` object", vmNodeSelector)
				for _, c := range updatedVmObj.Status.Conditions {
					if c.Type == string(vmcondition.TypeMigrating) {
						Expect(c.Status).Should(Equal(metav1.ConditionFalse))
					}
				}
				Expect(updatedVmObj.Status.MigrationState).Should(BeNil())
				Expect(updatedVmObj.Status.Node).Should(Equal(sourceNode))
			})
			By("Sets the `spec.nodeSelector` with `another node` value", func() {
				wg := &sync.WaitGroup{}

				updatedVmObj := &virtv2.VirtualMachine{}
				err := GetObject(virtv2.VirtualMachineResource, vmObj.Name, updatedVmObj, kc.GetOptions{
					Namespace: conf.Namespace,
				})
				Expect(err).NotTo(HaveOccurred(), "failed to obtain the %q `VirtualMachine` object", vmNodeSelector)
				Expect(updatedVmObj.Status.MigrationState).Should(BeNil())

				sourceNode := updatedVmObj.Status.Node
				Expect(sourceNode).ShouldNot(BeEmpty(), "the `vm.status.nodeName` should have a value")

				targetNode, err = DefineTargetNode(sourceNode, workerNodeLabel)
				Expect(err).NotTo(HaveOccurred())
				wg.Add(1)
				go func() {
					defer GinkgoRecover()
					defer wg.Done()
					Eventually(func() error {
						updatedVmObj := &virtv2.VirtualMachine{}
						err := GetObject(virtv2.VirtualMachineResource, vmObj.Name, updatedVmObj, kc.GetOptions{
							Namespace: conf.Namespace,
						})
						if err != nil {
							return err
						}
						if updatedVmObj.Status.Phase != virtv2.MachineMigrating {
							return fmt.Errorf("the `VirtualMachine` should be %s", virtv2.MachineMigrating)
						}
						return nil
					}).WithTimeout(Timeout).WithPolling(migratingStatusPollingInterval).Should(Succeed())
				}()
				mergePatch := fmt.Sprintf(`{"spec":{"nodeSelector":{%q:%q}}}`, nodeLabelKey, targetNode)
				err = MergePatchResource(kc.ResourceVM, vmObj.Name, mergePatch)
				Expect(err).NotTo(HaveOccurred(), "failed to patch the %q `VirtualMachine`", vmNodeSelector)
				wg.Wait()
			})
			By("The `VirtualMachine` should be migrated", func() {
				WaitVmAgentReady(kc.WaitOptions{
					Labels:    vmNodeSelector,
					Namespace: conf.Namespace,
					Timeout:   MaxWaitTimeout,
				})
				updatedVmObj := &virtv2.VirtualMachine{}
				err := GetObject(virtv2.VirtualMachineResource, vmObj.Name, updatedVmObj, kc.GetOptions{
					Namespace: conf.Namespace,
				})
				Expect(err).NotTo(HaveOccurred(), "failed to obtain the %q `VirtualMachine` object", vmNodeSelector)
				Expect(updatedVmObj.Status.MigrationState.Source.Node).Should(Equal(sourceNode))
				Expect(updatedVmObj.Status.MigrationState.Target.Node).Should(Equal(targetNode))
				Expect(updatedVmObj.Status.Node).Should(Equal(targetNode))
			})
		})
	})

	Context("When the virtual machine `node-affinity` agent is ready", func() {
		It("sets the `spec.affinity.nodeAffinity` field", func() {
			var (
				sourceNode string
				targetNode string
				err        error
			)
			vmObj := &virtv2.VirtualMachine{}
			By("Sets the `spec.affinity.nodeAffinity` with the `status.nodeSelector` value", func() {
				vmObj, err = GetVirtualMachineObjByLabel(conf.Namespace, vmNodeAffinity)
				Expect(err).NotTo(HaveOccurred())
				ExpectVirtualMachineIsMigratable(vmObj)
				sourceNode = vmObj.Status.Node
				Expect(sourceNode).ShouldNot(BeEmpty(), "the `vm.status.nodeName` should have a value")

				p, err := GenerateNodeAffinityPatch(nodeLabelKey, corev1.NodeSelectorOpIn, []string{sourceNode})
				Expect(err).NotTo(HaveOccurred())
				mergePatch := fmt.Sprintf(`{"spec":{"affinity":%s}}`, p)
				err = MergePatchResource(kc.ResourceVM, vmObj.Name, mergePatch)
				Expect(err).NotTo(HaveOccurred(), "failed to patch the %q `VirtualMachine`", vmNodeAffinity)
			})
			By("The `VirtualMachine` should not be migrated", func() {
				time.Sleep(20 * time.Second)
				updatedVmObj := &virtv2.VirtualMachine{}
				err := GetObject(virtv2.VirtualMachineResource, vmObj.Name, updatedVmObj, kc.GetOptions{
					Namespace: conf.Namespace,
				})
				Expect(err).NotTo(HaveOccurred(), "failed to obtain the %q `VirtualMachine` object", vmNodeAffinity)
				for _, c := range updatedVmObj.Status.Conditions {
					if c.Type == string(vmcondition.TypeMigrating) {
						Expect(c.Status).Should(Equal(metav1.ConditionFalse))
					}
				}
				Expect(updatedVmObj.Status.MigrationState).Should(BeNil())
				Expect(updatedVmObj.Status.Node).Should(Equal(sourceNode))
			})
			By("Sets the `spec.affinity.nodeAffinity` with `another node` value", func() {
				wg := &sync.WaitGroup{}

				updatedVmObj := &virtv2.VirtualMachine{}
				err := GetObject(virtv2.VirtualMachineResource, vmObj.Name, updatedVmObj, kc.GetOptions{
					Namespace: conf.Namespace,
				})
				Expect(err).NotTo(HaveOccurred(), "failed to obtain the %q `VirtualMachine` object", vmNodeAffinity)
				Expect(updatedVmObj.Status.MigrationState).Should(BeNil())

				sourceNode = updatedVmObj.Status.Node
				Expect(sourceNode).ShouldNot(BeEmpty(), "the `vm.status.nodeName` should have a value")

				targetNode, err = DefineTargetNode(sourceNode, workerNodeLabel)
				Expect(err).NotTo(HaveOccurred())

				p, err := GenerateNodeAffinityPatch(nodeLabelKey, corev1.NodeSelectorOpIn, []string{targetNode})
				Expect(err).NotTo(HaveOccurred(), "failed to generate the `NodeAffinity` patch")
				wg.Add(1)
				go func() {
					defer GinkgoRecover()
					defer wg.Done()
					Eventually(func() error {
						updatedVmObj := &virtv2.VirtualMachine{}
						err := GetObject(virtv2.VirtualMachineResource, vmObj.Name, updatedVmObj, kc.GetOptions{
							Namespace: conf.Namespace,
						})
						if err != nil {
							return err
						}
						if updatedVmObj.Status.Phase != virtv2.MachineMigrating {
							return fmt.Errorf("the `VirtualMachine` should be %s", virtv2.MachineMigrating)
						}
						return nil
					}).WithTimeout(Timeout).WithPolling(migratingStatusPollingInterval).Should(Succeed())
				}()
				mergePatch := fmt.Sprintf(`{"spec":{"affinity":%s}}`, p)
				err = MergePatchResource(kc.ResourceVM, vmObj.Name, mergePatch)
				Expect(err).NotTo(HaveOccurred(), "failed to patch the %q `VirtualMachine`", vmNodeAffinity)
				wg.Wait()
			})
			By("The `VirtualMachine` should be migrated", func() {
				WaitVmAgentReady(kc.WaitOptions{
					Labels:    vmNodeAffinity,
					Namespace: conf.Namespace,
					Timeout:   MaxWaitTimeout,
				})
				updatedVmObj := &virtv2.VirtualMachine{}
				err := GetObject(virtv2.VirtualMachineResource, vmObj.Name, updatedVmObj, kc.GetOptions{
					Namespace: conf.Namespace,
				})
				Expect(err).NotTo(HaveOccurred(), "failed to obtain the %q `VirtualMachine` object", vmNodeAffinity)
				Expect(updatedVmObj.Status.MigrationState.Source.Node).Should(Equal(sourceNode))
				Expect(updatedVmObj.Status.MigrationState.Target.Node).Should(Equal(targetNode))
				Expect(updatedVmObj.Status.Node).Should(Equal(targetNode))
			})
		})
	})

	Context("When test is completed", func() {
		It("deletes test case resources", func() {
			DeleteTestCaseResources(ResourcesToDelete{KustomizationDir: conf.TestData.AffinityToleration})
		})
	})
})
