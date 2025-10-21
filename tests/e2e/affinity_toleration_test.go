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
	"errors"
	"fmt"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
	"github.com/deckhouse/virtualization/tests/e2e/framework"
	kc "github.com/deckhouse/virtualization/tests/e2e/kubectl"
)

var _ = Describe("VirtualMachineAffinityAndToleration", framework.CommonE2ETestDecorators(), func() {
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
		ns                             string
	)

	BeforeAll(func() {
		kustomization := fmt.Sprintf("%s/%s", conf.TestData.AffinityToleration, "kustomization.yaml")
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

	Context("When the virtualization resources are applied:", func() {
		It("result should be succeeded", func() {
			res := kubectl.Apply(kc.ApplyOptions{
				Filename:       []string{conf.TestData.AffinityToleration},
				FilenameOption: kc.Kustomize,
			})
			Expect(res.Error()).NotTo(HaveOccurred(), "failed to apply the test case resources")
		})

		It("checks the resources phase", func() {
			By(fmt.Sprintf("`VirtualImages` should be in the %q phase", v1alpha2.ImageReady), func() {
				WaitPhaseByLabel(kc.ResourceVI, PhaseReady, kc.WaitOptions{
					Labels:    testCaseLabel,
					Namespace: ns,
					Timeout:   MaxWaitTimeout,
				})
			})
			By(fmt.Sprintf("`VirtualMachineClasses` should be in %s phases", v1alpha2.ClassPhaseReady), func() {
				WaitPhaseByLabel(kc.ResourceVMClass, PhaseReady, kc.WaitOptions{
					Labels:    testCaseLabel,
					Namespace: ns,
					Timeout:   MaxWaitTimeout,
				})
			})
			By(fmt.Sprintf("`VirtualDisks` should be in the %q phase", v1alpha2.DiskReady), func() {
				WaitPhaseByLabel(kc.ResourceVD, PhaseReady, kc.WaitOptions{
					Labels:    testCaseLabel,
					Namespace: ns,
					Timeout:   MaxWaitTimeout,
				})
			})
			By("`VirtualMachines` agents should be ready", func() {
				WaitVMAgentReady(kc.WaitOptions{
					Labels:    testCaseLabel,
					Namespace: ns,
					Timeout:   MaxWaitTimeout,
				})
			})
		})
	})

	Context("When the virtual machines agents are ready", func() {
		It("checks the `status.nodeName` field of the `VirtualMachines`", func() {
			var (
				vmObjA = &v1alpha2.VirtualMachine{}
				vmObjB = &v1alpha2.VirtualMachine{}
				vmObjC = &v1alpha2.VirtualMachine{}
				vmObjD = &v1alpha2.VirtualMachine{}
				err    error
			)
			By("Obtain the `VirtualMachine` objects", func() {
				vmObjA, err = GetVirtualMachineObjByLabel(ns, vmA)
				Expect(err).NotTo(HaveOccurred(), "failed to obtain the %q `VirtualMachine`", vmA)
				vmObjB, err = GetVirtualMachineObjByLabel(ns, vmB)
				Expect(err).NotTo(HaveOccurred(), "failed to obtain the %q `VirtualMachine`", vmB)
				vmObjC, err = GetVirtualMachineObjByLabel(ns, vmC)
				Expect(err).NotTo(HaveOccurred(), "failed to obtain the %q `VirtualMachine`", vmC)
				vmObjD, err = GetVirtualMachineObjByLabel(ns, vmD)
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
				jsonPatchAdd := &kc.JSONPatch{
					Op:    "add",
					Path:  "/spec/affinity/virtualMachineAndPodAntiAffinity",
					Value: string(p),
				}
				jsonPatchRemove := &kc.JSONPatch{
					Op:   "remove",
					Path: "/spec/affinity/virtualMachineAndPodAffinity",
				}
				wg.Add(1)
				go func() {
					defer GinkgoRecover()
					defer wg.Done()
					Eventually(func() error {
						updatedVMObjC := &v1alpha2.VirtualMachine{}
						err := GetObject(v1alpha2.VirtualMachineResource, vmObjC.Name, updatedVMObjC, kc.GetOptions{
							Namespace: ns,
						})
						if err != nil {
							return err
						}
						if updatedVMObjC.Status.Phase != v1alpha2.MachineMigrating {
							return fmt.Errorf("the `VirtualMachine` should be %s", v1alpha2.MachineMigrating)
						}
						return nil
					}).WithTimeout(LongWaitDuration).WithPolling(migratingStatusPollingInterval).Should(Succeed())
				}()
				res := kubectl.PatchResource(kc.ResourceVM, vmObjC.Name, kc.PatchOptions{
					JSONPatch: []*kc.JSONPatch{
						jsonPatchAdd,
						jsonPatchRemove,
					},
					Namespace: ns,
				})
				Expect(res.Error()).NotTo(HaveOccurred(), "failed to patch the %q `VirtualMachine`", vmC)
				wg.Wait()

				WaitVMAgentReady(kc.WaitOptions{
					Labels:    vmC,
					Namespace: ns,
					Timeout:   MaxWaitTimeout,
				})
				updatedVMObjC := &v1alpha2.VirtualMachine{}
				err = GetObject(v1alpha2.VirtualMachineResource, vmObjC.Name, updatedVMObjC, kc.GetOptions{
					Namespace: ns,
				})
				Expect(err).NotTo(HaveOccurred(), "failed to obtain the %q `VirtualMachine` object", vmC)
				Expect(updatedVMObjC.Status.MigrationState.Source.Node).Should(Equal(vmObjC.Status.Node))
				Expect(updatedVMObjC.Status.MigrationState.Target.Node).ShouldNot(Equal(vmObjA.Status.Node))
				Expect(updatedVMObjC.Status.Node).ShouldNot(Equal(vmObjA.Status.Node))
			})
			By("Change anti-affinity to affinity when the `VirtualMachines` are runnning: `vm-a` and `vm-c` should be running on the same node", func() {
				wg := &sync.WaitGroup{}

				updatedVMObjC := &v1alpha2.VirtualMachine{}
				err = GetObject(v1alpha2.VirtualMachineResource, vmObjC.Name, updatedVMObjC, kc.GetOptions{
					Namespace: ns,
				})

				ExpectVirtualMachineIsMigratable(updatedVMObjC)
				p, err := GenerateVirtualMachineAndPodAffinityPatch(vmKey, nodeLabelKey, metav1.LabelSelectorOpIn, []string{vmA[vmKey]})
				Expect(err).NotTo(HaveOccurred(), "failed to generate the `VirtualMachineAndPodAffinity` patch")
				jsonPatchAdd := &kc.JSONPatch{
					Op:    "add",
					Path:  "/spec/affinity/virtualMachineAndPodAffinity",
					Value: string(p),
				}
				jsonPatchRemove := &kc.JSONPatch{
					Op:   "remove",
					Path: "/spec/affinity/virtualMachineAndPodAntiAffinity",
				}

				migrationInitiatedAt := time.Now().UTC()
				res := kubectl.PatchResource(kc.ResourceVM, vmObjC.Name, kc.PatchOptions{
					JSONPatch: []*kc.JSONPatch{
						jsonPatchAdd,
						jsonPatchRemove,
					},
					Namespace: ns,
				})
				Expect(res.Error()).NotTo(HaveOccurred(), "failed to patch the %q `VirtualMachine`", vmC)

				wg.Add(1)
				go func() {
					defer GinkgoRecover()
					defer wg.Done()
					Eventually(func() error {
						updatedVMObjC = &v1alpha2.VirtualMachine{}
						err = GetObject(v1alpha2.VirtualMachineResource, vmObjC.Name, updatedVMObjC, kc.GetOptions{
							Namespace: ns,
						})
						if err != nil {
							return err
						}

						if updatedVMObjC.Status.MigrationState == nil || updatedVMObjC.Status.MigrationState.StartTimestamp == nil || updatedVMObjC.Status.MigrationState.StartTimestamp.UTC().Before(migrationInitiatedAt) {
							return errors.New("couldn't wait for the migration to start")
						}

						if updatedVMObjC.Status.MigrationState.Source.Node == vmObjA.Status.Node {
							return errors.New("migration should start from a different node")
						}

						if updatedVMObjC.Status.MigrationState.Target.Node != vmObjA.Status.Node {
							return errors.New("migration should end at the same node")
						}

						if updatedVMObjC.Status.Node != vmObjA.Status.Node {
							return errors.New("migration should end at the same node")
						}

						return nil
					}).WithTimeout(Timeout).WithPolling(migratingStatusPollingInterval).Should(Succeed())
				}()

				wg.Wait()

				WaitVMAgentReady(kc.WaitOptions{
					Labels:    vmC,
					Namespace: ns,
					Timeout:   MaxWaitTimeout,
				})
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
			vmObj := &v1alpha2.VirtualMachine{}
			By("Sets the `spec.nodeSelector` with the `status.nodeSelector` value", func() {
				vmObj, err = GetVirtualMachineObjByLabel(ns, vmNodeSelector)
				Expect(err).NotTo(HaveOccurred(), "failed to obtain the %q `VirtualMachine` object", vmNodeSelector)
				ExpectVirtualMachineIsMigratable(vmObj)
				sourceNode = vmObj.Status.Node
				Expect(sourceNode).ShouldNot(BeEmpty(), "the `vm.status.nodeName` should have a value")
				mergePatch := fmt.Sprintf(`{"spec":{"nodeSelector":{%q:%q}}}`, nodeLabelKey, sourceNode)
				err = MergePatchResource(kc.ResourceVM, ns, vmObj.Name, mergePatch)
				Expect(err).NotTo(HaveOccurred(), "failed to patch the %q `VirtualMachine`", vmNodeSelector)
			})
			By("The `VirtualMachine` should not be migrated", func() {
				time.Sleep(20 * time.Second)
				updatedVMObj := &v1alpha2.VirtualMachine{}
				err := GetObject(v1alpha2.VirtualMachineResource, vmObj.Name, updatedVMObj, kc.GetOptions{
					Namespace: ns,
				})
				Expect(err).NotTo(HaveOccurred(), "failed to obtain the %q `VirtualMachine` object", vmNodeSelector)
				for _, c := range updatedVMObj.Status.Conditions {
					if c.Type == string(vmcondition.TypeMigrating) {
						Expect(c.Status).Should(Equal(metav1.ConditionFalse))
					}
				}
				Expect(updatedVMObj.Status.MigrationState).Should(BeNil())
				Expect(updatedVMObj.Status.Node).Should(Equal(sourceNode))
			})
			By("Sets the `spec.nodeSelector` with `another node` value", func() {
				wg := &sync.WaitGroup{}

				updatedVMObj := &v1alpha2.VirtualMachine{}
				err := GetObject(v1alpha2.VirtualMachineResource, vmObj.Name, updatedVMObj, kc.GetOptions{
					Namespace: ns,
				})
				Expect(err).NotTo(HaveOccurred(), "failed to obtain the %q `VirtualMachine` object", vmNodeSelector)
				Expect(updatedVMObj.Status.MigrationState).Should(BeNil())

				sourceNode := updatedVMObj.Status.Node
				Expect(sourceNode).ShouldNot(BeEmpty(), "the `vm.status.nodeName` should have a value")

				targetNode, err = DefineTargetNode(sourceNode, workerNodeLabel)
				Expect(err).NotTo(HaveOccurred())
				wg.Add(1)
				go func() {
					defer GinkgoRecover()
					defer wg.Done()
					Eventually(func() error {
						updatedVMObj := &v1alpha2.VirtualMachine{}
						err := GetObject(v1alpha2.VirtualMachineResource, vmObj.Name, updatedVMObj, kc.GetOptions{
							Namespace: ns,
						})
						if err != nil {
							return err
						}
						if updatedVMObj.Status.Phase != v1alpha2.MachineMigrating {
							return fmt.Errorf("the `VirtualMachine` should be %s", v1alpha2.MachineMigrating)
						}
						return nil
					}).WithTimeout(Timeout).WithPolling(migratingStatusPollingInterval).Should(Succeed())
				}()
				mergePatch := fmt.Sprintf(`{"spec":{"nodeSelector":{%q:%q}}}`, nodeLabelKey, targetNode)
				err = MergePatchResource(kc.ResourceVM, ns, vmObj.Name, mergePatch)
				Expect(err).NotTo(HaveOccurred(), "failed to patch the %q `VirtualMachine`", vmNodeSelector)
				wg.Wait()
			})
			By("The `VirtualMachine` should be migrated", func() {
				WaitVMAgentReady(kc.WaitOptions{
					Labels:    vmNodeSelector,
					Namespace: ns,
					Timeout:   MaxWaitTimeout,
				})
				updatedVMObj := &v1alpha2.VirtualMachine{}
				err := GetObject(v1alpha2.VirtualMachineResource, vmObj.Name, updatedVMObj, kc.GetOptions{
					Namespace: ns,
				})
				Expect(err).NotTo(HaveOccurred(), "failed to obtain the %q `VirtualMachine` object", vmNodeSelector)
				Expect(updatedVMObj.Status.MigrationState.Source.Node).Should(Equal(sourceNode))
				Expect(updatedVMObj.Status.MigrationState.Target.Node).Should(Equal(targetNode))
				Expect(updatedVMObj.Status.Node).Should(Equal(targetNode))
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
			vmObj := &v1alpha2.VirtualMachine{}
			By("Sets the `spec.affinity.nodeAffinity` with the `status.nodeSelector` value", func() {
				vmObj, err = GetVirtualMachineObjByLabel(ns, vmNodeAffinity)
				Expect(err).NotTo(HaveOccurred())
				ExpectVirtualMachineIsMigratable(vmObj)
				sourceNode = vmObj.Status.Node
				Expect(sourceNode).ShouldNot(BeEmpty(), "the `vm.status.nodeName` should have a value")

				p, err := GenerateNodeAffinityPatch(nodeLabelKey, corev1.NodeSelectorOpIn, []string{sourceNode})
				Expect(err).NotTo(HaveOccurred())
				mergePatch := fmt.Sprintf(`{"spec":{"affinity":%s}}`, p)
				err = MergePatchResource(kc.ResourceVM, ns, vmObj.Name, mergePatch)
				Expect(err).NotTo(HaveOccurred(), "failed to patch the %q `VirtualMachine`", vmNodeAffinity)
			})
			By("The `VirtualMachine` should not be migrated", func() {
				time.Sleep(20 * time.Second)
				updatedVMObj := &v1alpha2.VirtualMachine{}
				err := GetObject(v1alpha2.VirtualMachineResource, vmObj.Name, updatedVMObj, kc.GetOptions{
					Namespace: ns,
				})
				Expect(err).NotTo(HaveOccurred(), "failed to obtain the %q `VirtualMachine` object", vmNodeAffinity)
				for _, c := range updatedVMObj.Status.Conditions {
					if c.Type == string(vmcondition.TypeMigrating) {
						Expect(c.Status).Should(Equal(metav1.ConditionFalse))
					}
				}
				Expect(updatedVMObj.Status.MigrationState).Should(BeNil())
				Expect(updatedVMObj.Status.Node).Should(Equal(sourceNode))
			})
			By("Sets the `spec.affinity.nodeAffinity` with `another node` value", func() {
				wg := &sync.WaitGroup{}

				updatedVMObj := &v1alpha2.VirtualMachine{}
				err := GetObject(v1alpha2.VirtualMachineResource, vmObj.Name, updatedVMObj, kc.GetOptions{
					Namespace: ns,
				})
				Expect(err).NotTo(HaveOccurred(), "failed to obtain the %q `VirtualMachine` object", vmNodeAffinity)
				Expect(updatedVMObj.Status.MigrationState).Should(BeNil())

				sourceNode = updatedVMObj.Status.Node
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
						updatedVMObj := &v1alpha2.VirtualMachine{}
						err := GetObject(v1alpha2.VirtualMachineResource, vmObj.Name, updatedVMObj, kc.GetOptions{
							Namespace: ns,
						})
						if err != nil {
							return err
						}
						if updatedVMObj.Status.Phase != v1alpha2.MachineMigrating {
							return fmt.Errorf("the `VirtualMachine` should be %s", v1alpha2.MachineMigrating)
						}
						return nil
					}).WithTimeout(Timeout).WithPolling(migratingStatusPollingInterval).Should(Succeed())
				}()
				mergePatch := fmt.Sprintf(`{"spec":{"affinity":%s}}`, p)
				err = MergePatchResource(kc.ResourceVM, ns, vmObj.Name, mergePatch)
				Expect(err).NotTo(HaveOccurred(), "failed to patch the %q `VirtualMachine`", vmNodeAffinity)
				wg.Wait()
			})
			By("The `VirtualMachine` should be migrated", func() {
				WaitVMAgentReady(kc.WaitOptions{
					Labels:    vmNodeAffinity,
					Namespace: ns,
					Timeout:   MaxWaitTimeout,
				})
				updatedVMObj := &v1alpha2.VirtualMachine{}
				err := GetObject(v1alpha2.VirtualMachineResource, vmObj.Name, updatedVMObj, kc.GetOptions{
					Namespace: ns,
				})
				Expect(err).NotTo(HaveOccurred(), "failed to obtain the %q `VirtualMachine` object", vmNodeAffinity)
				Expect(updatedVMObj.Status.MigrationState.Source.Node).Should(Equal(sourceNode))
				Expect(updatedVMObj.Status.MigrationState.Target.Node).Should(Equal(targetNode))
				Expect(updatedVMObj.Status.Node).Should(Equal(targetNode))
			})
		})
	})

	Context("When test is completed", func() {
		It("deletes test case resources", func() {
			DeleteTestCaseResources(ns, ResourcesToDelete{KustomizationDir: conf.TestData.AffinityToleration})
		})
	})
})

func ExpectVirtualMachineIsMigratable(vmObj *v1alpha2.VirtualMachine) {
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

func GetVirtualMachineObjByLabel(namespace string, label map[string]string) (*v1alpha2.VirtualMachine, error) {
	vmObjects := v1alpha2.VirtualMachineList{}
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
	vmAffinity := &v1alpha2.VMAffinity{
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
	vmAndPodAntiAffinity := &v1alpha2.VirtualMachineAndPodAntiAffinity{
		RequiredDuringSchedulingIgnoredDuringExecution: []v1alpha2.VirtualMachineAndPodAffinityTerm{
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
	vmAndPodAffinity := &v1alpha2.VirtualMachineAndPodAffinity{
		RequiredDuringSchedulingIgnoredDuringExecution: []v1alpha2.VirtualMachineAndPodAffinityTerm{
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
