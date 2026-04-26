/*
Copyright 2026 Flant JSC

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

package vm

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"

	vdbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vd"
	vmbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vm"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	"github.com/deckhouse/virtualization/test/e2e/internal/object"
	"github.com/deckhouse/virtualization/test/e2e/internal/precheck"
	"github.com/deckhouse/virtualization/test/e2e/internal/util"
)

const (
	affinityHostnameLabelKey    = "kubernetes.io/hostname"
	masterLabelKey              = "node.deckhouse.io/group"
	kvmEnabledLabelKey          = "virtualization.deckhouse.io/kvm-enabled"
	vmLabelKey                  = "vm"
	placementNoMigrationWait    = 20 * time.Second
	placementNoMigrationPolling = time.Second
)

type migrationTargetExpectation int

const (
	migrationTargetMustMatch migrationTargetExpectation = iota
	migrationTargetMustDiffer
)

var _ = Describe("VirtualMachineAffinityAndToleration", Ordered, Label(precheck.NoPrecheck), func() {
	var (
		f   *framework.Framework
		ctx context.Context

		vmA *v1alpha2.VirtualMachine
		vmB *v1alpha2.VirtualMachine
		vmC *v1alpha2.VirtualMachine
		vmD *v1alpha2.VirtualMachine
	)

	BeforeAll(func() {
		f = framework.NewFramework("vm-affinity-toleration")
	})

	BeforeEach(func() {
		DeferCleanup(f.After)
		f.Before()
		ctx = context.Background()
	})

	It("checks placement via status.nodeName and migrations after affinity changes", func() {
		By("Checking test prerequisites", func() {
			readyNodes, err := listReadyNodes(ctx, f, map[string]string{kvmEnabledLabelKey: "true"})
			Expect(err).NotTo(HaveOccurred())
			Expect(len(readyNodes)).To(BeNumerically(">=", 2), "at least two ready KVM-enabled nodes are required, got %d", len(readyNodes))

			masterNodes, err := listReadyNodes(ctx, f, map[string]string{kvmEnabledLabelKey: "true", masterLabelKey: "master"})
			Expect(err).NotTo(HaveOccurred())
			Expect(len(masterNodes)).To(BeNumerically(">", 0), "at least one ready KVM-enabled master node is required, got %d", len(masterNodes))
		})

		By("Creating vm-a", func() {
			vmA = newPlacementVM("vm-a", f.Namespace().Name, nil)
			err := f.CreateWithDeferredDeletion(
				ctx,
				object.NewVDFromCVI(
					rootVDNameForVM(vmA.Name),
					f.Namespace().Name,
					object.PrecreatedCVIAlpineBIOS,
					vdbuilder.WithSize(ptr.To(resource.MustParse("350Mi"))),
				),
				vmA,
			)
			Expect(err).NotTo(HaveOccurred())

			util.UntilObjectPhase(string(v1alpha2.MachineRunning), framework.LongTimeout, vmA)
		})

		By("Creating vm-b, vm-c and vm-d", func() {
			vmB = newPlacementVM("vm-b", f.Namespace().Name, antiAffinityToVM("vm-a"))
			vmC = newPlacementVM("vm-c", f.Namespace().Name, affinityToVM("vm-a"))
			vmD = newPlacementVM("vm-d", f.Namespace().Name, masterNodeAffinity())

			objs := []crclient.Object{
				object.NewVDFromCVI(
					rootVDNameForVM(vmB.Name),
					f.Namespace().Name,
					object.PrecreatedCVIAlpineBIOS,
					vdbuilder.WithSize(ptr.To(resource.MustParse("350Mi"))),
				),
				vmB,
				object.NewVDFromCVI(
					rootVDNameForVM(vmC.Name),
					f.Namespace().Name,
					object.PrecreatedCVIAlpineBIOS,
					vdbuilder.WithSize(ptr.To(resource.MustParse("350Mi"))),
				),
				vmC,
				object.NewVDFromCVI(
					rootVDNameForVM(vmD.Name),
					f.Namespace().Name,
					object.PrecreatedCVIAlpineBIOS,
					vdbuilder.WithSize(ptr.To(resource.MustParse("350Mi"))),
				),
				vmD,
			}
			err := f.CreateWithDeferredDeletion(ctx, objs...)
			Expect(err).NotTo(HaveOccurred())

			util.UntilObjectPhase(string(v1alpha2.MachineRunning), framework.LongTimeout, vmB, vmC, vmD)
		})

		var nodeA string
		By("Verifying initial placement via status.nodeName", func() {
			vmA = getVirtualMachine(ctx, f, vmA.Name)
			vmB = getVirtualMachine(ctx, f, vmB.Name)
			vmC = getVirtualMachine(ctx, f, vmC.Name)
			vmD = getVirtualMachine(ctx, f, vmD.Name)

			nodeA = vmA.Status.Node
			Expect(nodeA).NotTo(BeEmpty())
			Expect(vmB.Status.Node).NotTo(BeEmpty())
			Expect(vmC.Status.Node).NotTo(BeEmpty())
			Expect(vmD.Status.Node).NotTo(BeEmpty())

			Expect(vmC.Status.Node).To(Equal(nodeA), "vm-a and vm-c should run on the same node, got vm-a=%q vm-c=%q", nodeA, vmC.Status.Node)
			Expect(vmB.Status.Node).NotTo(Equal(nodeA), "vm-a and vm-b should run on different nodes, got vm-a=%q vm-b=%q", nodeA, vmB.Status.Node)

			nodeD := getNode(ctx, f, vmD.Status.Node)
			Expect(nodeD.Labels).To(HaveKeyWithValue(masterLabelKey, "master"), "vm-d should run on a master node, got vm-d node=%q", vmD.Status.Node)
		})

		By("Changing vm-c affinity to anti-affinity and verifying migration to another node", func() {
			util.UntilConditionStatus(vmcondition.TypeMigratable.String(), string(metav1.ConditionTrue), framework.LongTimeout, vmC)

			vmC = getVirtualMachine(ctx, f, vmC.Name)
			sourceNode := vmC.Status.Node
			startedAt := time.Now().UTC()

			vmC.Spec.Affinity = antiAffinityToVM("vm-a")
			err := f.GenericClient().Update(ctx, vmC)
			Expect(err).NotTo(HaveOccurred())

			waitForStabilizedVMMigration(ctx, f, crclient.ObjectKeyFromObject(vmC), startedAt, sourceNode, nodeA, migrationTargetMustDiffer, framework.MaxTimeout)
			util.UntilConditionStatus(vmcondition.TypeMigratable.String(), string(metav1.ConditionTrue), framework.LongTimeout, vmC)
		})

		var migratedNodeC string
		By("Verifying vm-c moved away from vm-a node", func() {
			vmC = getVirtualMachine(ctx, f, vmC.Name)
			migratedNodeC = vmC.Status.Node
			Expect(migratedNodeC).NotTo(Equal(nodeA), "vm-c should run on a different node after anti-affinity update, got old=%q new=%q", nodeA, migratedNodeC)
		})

		By("Changing vm-c anti-affinity back to affinity and verifying migration back to vm-a node", func() {
			util.UntilConditionStatus(vmcondition.TypeMigratable.String(), string(metav1.ConditionTrue), framework.LongTimeout, vmC)

			vmC = getVirtualMachine(ctx, f, vmC.Name)
			startedAt := time.Now().UTC()

			vmC.Spec.Affinity = affinityToVM("vm-a")
			err := f.GenericClient().Update(ctx, vmC)
			Expect(err).NotTo(HaveOccurred())

			waitForStabilizedVMMigration(ctx, f, crclient.ObjectKeyFromObject(vmC), startedAt, migratedNodeC, nodeA, migrationTargetMustMatch, framework.MaxTimeout)
			util.UntilConditionStatus(vmcondition.TypeMigratable.String(), string(metav1.ConditionTrue), framework.LongTimeout, vmC)
		})

		By("Verifying vm-c returned to vm-a node via status.nodeName", func() {
			vmC = getVirtualMachine(ctx, f, vmC.Name)
			Expect(vmC.Status.Node).To(Equal(nodeA), "vm-c should return to the same node as vm-a, got vm-a=%q vm-c=%q", nodeA, vmC.Status.Node)
		})
	})

	It("keeps placement when nodeSelector points to the current node and migrates after switching it to another node", func() {
		By("Checking test prerequisites", func() {
			workerNodes, err := listReadyNodes(ctx, f, map[string]string{kvmEnabledLabelKey: "true", masterLabelKey: "worker"})
			Expect(err).NotTo(HaveOccurred())
			Expect(len(workerNodes)).To(BeNumerically(">=", 2), "at least two ready KVM-enabled worker nodes are required, got %d", len(workerNodes))
		})

		vmNodeSelector := newPlacementVM("vm-node-selector", f.Namespace().Name, nil)
		By("Creating the virtual machine", func() {
			err := f.CreateWithDeferredDeletion(
				ctx,
				object.NewVDFromCVI(
					rootVDNameForVM(vmNodeSelector.Name),
					f.Namespace().Name,
					object.PrecreatedCVIAlpineBIOS,
					vdbuilder.WithSize(ptr.To(resource.MustParse("350Mi"))),
				),
				vmNodeSelector,
			)
			Expect(err).NotTo(HaveOccurred())

			util.UntilObjectPhase(string(v1alpha2.MachineRunning), framework.LongTimeout, vmNodeSelector)
			util.UntilConditionStatus(vmcondition.TypeMigratable.String(), string(metav1.ConditionTrue), framework.LongTimeout, vmNodeSelector)
		})

		var sourceNode string
		By("Setting spec.nodeSelector to the current node and verifying that no migration happens", func() {
			vmNodeSelector = getVirtualMachine(ctx, f, vmNodeSelector.Name)
			sourceNode = vmNodeSelector.Status.Node
			Expect(sourceNode).NotTo(BeEmpty())

			vmNodeSelector.Spec.NodeSelector = map[string]string{affinityHostnameLabelKey: sourceNode}
			err := f.GenericClient().Update(ctx, vmNodeSelector)
			Expect(err).NotTo(HaveOccurred())

			assertNoVMMigration(ctx, f, crclient.ObjectKeyFromObject(vmNodeSelector), sourceNode, placementNoMigrationWait)
		})

		var targetNode string
		By("Setting spec.nodeSelector to another worker node and verifying migration", func() {
			var err error
			targetNode, err = defineReadyTargetNode(ctx, f, sourceNode, map[string]string{kvmEnabledLabelKey: "true", masterLabelKey: "worker"})
			Expect(err).NotTo(HaveOccurred())

			vmNodeSelector = getVirtualMachine(ctx, f, vmNodeSelector.Name)
			startedAt := time.Now().UTC()
			vmNodeSelector.Spec.NodeSelector = map[string]string{affinityHostnameLabelKey: targetNode}
			err = f.GenericClient().Update(ctx, vmNodeSelector)
			Expect(err).NotTo(HaveOccurred())

			waitForStabilizedVMMigration(ctx, f, crclient.ObjectKeyFromObject(vmNodeSelector), startedAt, sourceNode, targetNode, migrationTargetMustMatch, framework.MaxTimeout)
			util.UntilConditionStatus(vmcondition.TypeMigratable.String(), string(metav1.ConditionTrue), framework.LongTimeout, vmNodeSelector)
		})

		By("Verifying the nodeSelector migration result via status.nodeName", func() {
			vmNodeSelector = getVirtualMachine(ctx, f, vmNodeSelector.Name)
			Expect(vmNodeSelector.Status.MigrationState).NotTo(BeNil())
			Expect(vmNodeSelector.Status.MigrationState.Source.Node).To(Equal(sourceNode))
			Expect(vmNodeSelector.Status.MigrationState.Target.Node).To(Equal(targetNode))
			Expect(vmNodeSelector.Status.Node).To(Equal(targetNode))
		})
	})

	It("keeps placement when nodeAffinity points to the current node and migrates after switching it to another node", func() {
		By("Checking test prerequisites", func() {
			workerNodes, err := listReadyNodes(ctx, f, map[string]string{kvmEnabledLabelKey: "true", masterLabelKey: "worker"})
			Expect(err).NotTo(HaveOccurred())
			Expect(len(workerNodes)).To(BeNumerically(">=", 2), "at least two ready KVM-enabled worker nodes are required, got %d", len(workerNodes))
		})

		vmNodeAffinity := newPlacementVM("vm-node-affinity", f.Namespace().Name, nil)
		By("Creating the virtual machine", func() {
			err := f.CreateWithDeferredDeletion(
				ctx,
				object.NewVDFromCVI(
					rootVDNameForVM(vmNodeAffinity.Name),
					f.Namespace().Name,
					object.PrecreatedCVIAlpineBIOS,
					vdbuilder.WithSize(ptr.To(resource.MustParse("350Mi"))),
				),
				vmNodeAffinity,
			)
			Expect(err).NotTo(HaveOccurred())

			util.UntilObjectPhase(string(v1alpha2.MachineRunning), framework.LongTimeout, vmNodeAffinity)
			util.UntilConditionStatus(vmcondition.TypeMigratable.String(), string(metav1.ConditionTrue), framework.LongTimeout, vmNodeAffinity)
		})

		var sourceNode string
		By("Setting spec.affinity.nodeAffinity to the current node and verifying that no migration happens", func() {
			vmNodeAffinity = getVirtualMachine(ctx, f, vmNodeAffinity.Name)
			sourceNode = vmNodeAffinity.Status.Node
			Expect(sourceNode).NotTo(BeEmpty())

			vmNodeAffinity.Spec.Affinity = nodeAffinityForNode(sourceNode)
			err := f.GenericClient().Update(ctx, vmNodeAffinity)
			Expect(err).NotTo(HaveOccurred())

			assertNoVMMigration(ctx, f, crclient.ObjectKeyFromObject(vmNodeAffinity), sourceNode, placementNoMigrationWait)
		})

		var targetNode string
		By("Setting spec.affinity.nodeAffinity to another worker node and verifying migration", func() {
			var err error
			targetNode, err = defineReadyTargetNode(ctx, f, sourceNode, map[string]string{kvmEnabledLabelKey: "true", masterLabelKey: "worker"})
			Expect(err).NotTo(HaveOccurred())

			vmNodeAffinity = getVirtualMachine(ctx, f, vmNodeAffinity.Name)
			startedAt := time.Now().UTC()
			vmNodeAffinity.Spec.Affinity = nodeAffinityForNode(targetNode)
			err = f.GenericClient().Update(ctx, vmNodeAffinity)
			Expect(err).NotTo(HaveOccurred())

			waitForStabilizedVMMigration(ctx, f, crclient.ObjectKeyFromObject(vmNodeAffinity), startedAt, sourceNode, targetNode, migrationTargetMustMatch, framework.MaxTimeout)
			util.UntilConditionStatus(vmcondition.TypeMigratable.String(), string(metav1.ConditionTrue), framework.LongTimeout, vmNodeAffinity)
		})

		By("Verifying the nodeAffinity migration result via status.nodeName", func() {
			vmNodeAffinity = getVirtualMachine(ctx, f, vmNodeAffinity.Name)
			Expect(vmNodeAffinity.Status.MigrationState).NotTo(BeNil())
			Expect(vmNodeAffinity.Status.MigrationState.Source.Node).To(Equal(sourceNode))
			Expect(vmNodeAffinity.Status.MigrationState.Target.Node).To(Equal(targetNode))
			Expect(vmNodeAffinity.Status.Node).To(Equal(targetNode))
		})
	})
})

func newPlacementVM(name, namespace string, affinity *v1alpha2.VMAffinity) *v1alpha2.VirtualMachine {
	vm := object.NewMinimalVM(
		"",
		namespace,
		vmbuilder.WithName(name),
		vmbuilder.WithBootloader(v1alpha2.BIOS),
		vmbuilder.WithBlockDeviceRefs(v1alpha2.BlockDeviceSpecRef{
			Kind: v1alpha2.VirtualDiskKind,
			Name: rootVDNameForVM(name),
		}),
		vmbuilder.WithLabel(vmLabelKey, name),
		vmbuilder.WithTolerations([]corev1.Toleration{{
			Operator: corev1.TolerationOpExists,
			Effect:   corev1.TaintEffectNoSchedule,
		}}),
	)
	vm.Spec.Affinity = affinity
	return vm
}

func rootVDNameForVM(vmName string) string {
	return fmt.Sprintf("%s-root", vmName)
}

func affinityToVM(vmName string) *v1alpha2.VMAffinity {
	return &v1alpha2.VMAffinity{
		VirtualMachineAndPodAffinity: &v1alpha2.VirtualMachineAndPodAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: []v1alpha2.VirtualMachineAndPodAffinityTerm{vmAffinityTerm(vmName)},
		},
	}
}

func antiAffinityToVM(vmName string) *v1alpha2.VMAffinity {
	return &v1alpha2.VMAffinity{
		VirtualMachineAndPodAntiAffinity: &v1alpha2.VirtualMachineAndPodAntiAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: []v1alpha2.VirtualMachineAndPodAffinityTerm{vmAffinityTerm(vmName)},
		},
	}
}

func nodeAffinityForNode(nodeName string) *v1alpha2.VMAffinity {
	return &v1alpha2.VMAffinity{
		NodeAffinity: &corev1.NodeAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
				NodeSelectorTerms: []corev1.NodeSelectorTerm{{
					MatchExpressions: []corev1.NodeSelectorRequirement{{
						Key:      affinityHostnameLabelKey,
						Operator: corev1.NodeSelectorOpIn,
						Values:   []string{nodeName},
					}},
				}},
			},
		},
	}
}

func masterNodeAffinity() *v1alpha2.VMAffinity {
	return &v1alpha2.VMAffinity{
		NodeAffinity: &corev1.NodeAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
				NodeSelectorTerms: []corev1.NodeSelectorTerm{{
					MatchExpressions: []corev1.NodeSelectorRequirement{{
						Key:      masterLabelKey,
						Operator: corev1.NodeSelectorOpIn,
						Values:   []string{"master"},
					}},
				}},
			},
		},
	}
}

func vmAffinityTerm(vmName string) v1alpha2.VirtualMachineAndPodAffinityTerm {
	return v1alpha2.VirtualMachineAndPodAffinityTerm{
		LabelSelector: &metav1.LabelSelector{
			MatchExpressions: []metav1.LabelSelectorRequirement{{
				Key:      vmLabelKey,
				Operator: metav1.LabelSelectorOpIn,
				Values:   []string{vmName},
			}},
		},
		TopologyKey: affinityHostnameLabelKey,
	}
}

func waitForStabilizedVMMigration(
	ctx context.Context,
	f *framework.Framework,
	key crclient.ObjectKey,
	notBefore time.Time,
	sourceNode string,
	targetNode string,
	targetExpectation migrationTargetExpectation,
	timeout time.Duration,
) {
	GinkgoHelper()

	Eventually(func(g Gomega) {
		vm := getVirtualMachine(ctx, f, key.Name)
		util.SkipIfKnownMigrationFailureWithContext(ctx, vm)

		state := vm.Status.MigrationState
		g.Expect(state).NotTo(BeNil())
		g.Expect(state.StartTimestamp).NotTo(BeNil())
		g.Expect(state.StartTimestamp.UTC().Before(notBefore)).To(BeFalse(), "expected a fresh migration")
		g.Expect(state.EndTimestamp.IsZero()).To(BeFalse(), "migration is not completed")

		if state.Result == v1alpha2.MigrationResultFailed {
			Fail(fmt.Sprintf("migration failed for vm %s/%s: %s", vm.Namespace, vm.Name, migrationFailureDetails(vm)))
		}

		g.Expect(state.Result).To(Equal(v1alpha2.MigrationResultSucceeded))

		g.Expect(state.Source.Node).To(Equal(sourceNode))
		g.Expect(vm.Status.Node).To(Equal(state.Target.Node))

		switch targetExpectation {
		case migrationTargetMustMatch:
			g.Expect(state.Target.Node).To(Equal(targetNode))
		case migrationTargetMustDiffer:
			g.Expect(state.Target.Node).NotTo(Equal(targetNode))
		default:
			Fail(fmt.Sprintf("unknown migration target expectation: %d", targetExpectation))
		}
	}).WithTimeout(timeout).WithPolling(time.Second).Should(Succeed())
}

func migrationFailureDetails(vm *v1alpha2.VirtualMachine) string {
	GinkgoHelper()

	for _, condition := range vm.Status.Conditions {
		if condition.Type == vmcondition.TypeMigrating.String() {
			return fmt.Sprintf("result=%s reason=%s message=%s source=%s target=%s current=%s", vm.Status.MigrationState.Result, condition.Reason, condition.Message, vm.Status.MigrationState.Source.Node, vm.Status.MigrationState.Target.Node, vm.Status.Node)
		}
	}

	return fmt.Sprintf("result=%s source=%s target=%s current=%s", vm.Status.MigrationState.Result, vm.Status.MigrationState.Source.Node, vm.Status.MigrationState.Target.Node, vm.Status.Node)
}

func assertNoVMMigration(
	ctx context.Context,
	f *framework.Framework,
	key crclient.ObjectKey,
	expectedNode string,
	duration time.Duration,
) {
	GinkgoHelper()

	Consistently(func(g Gomega) {
		vm := getVirtualMachine(ctx, f, key.Name)
		g.Expect(vm.Status.Node).To(Equal(expectedNode))
		g.Expect(vm.Status.MigrationState).To(BeNil())

		for _, condition := range vm.Status.Conditions {
			if condition.Type == vmcondition.TypeMigrating.String() {
				g.Expect(condition.Status).To(Equal(metav1.ConditionFalse))
			}
		}
	}).WithTimeout(duration).WithPolling(placementNoMigrationPolling).Should(Succeed())
}

func getVirtualMachine(ctx context.Context, f *framework.Framework, name string) *v1alpha2.VirtualMachine {
	GinkgoHelper()

	vm, err := f.VirtClient().VirtualMachines(f.Namespace().Name).Get(ctx, name, metav1.GetOptions{})
	Expect(err).NotTo(HaveOccurred())
	return vm
}

func getNode(ctx context.Context, f *framework.Framework, name string) *corev1.Node {
	GinkgoHelper()

	node := &corev1.Node{}
	err := f.GenericClient().Get(ctx, crclient.ObjectKey{Name: name}, node)
	Expect(err).NotTo(HaveOccurred())
	return node
}

func listReadyNodes(ctx context.Context, f *framework.Framework, labels map[string]string) ([]corev1.Node, error) {
	nodes := &corev1.NodeList{}
	err := f.GenericClient().List(ctx, nodes, crclient.MatchingLabels(labels))
	if err != nil {
		return nil, err
	}

	readyNodes := make([]corev1.Node, 0, len(nodes.Items))
	for _, node := range nodes.Items {
		for _, condition := range node.Status.Conditions {
			if condition.Type == corev1.NodeReady && condition.Status == corev1.ConditionTrue {
				readyNodes = append(readyNodes, node)
				break
			}
		}
	}

	return readyNodes, nil
}

func defineReadyTargetNode(ctx context.Context, f *framework.Framework, currentNode string, labels map[string]string) (string, error) {
	readyNodes, err := listReadyNodes(ctx, f, labels)
	if err != nil {
		return "", err
	}

	for _, node := range readyNodes {
		if node.Name != currentNode {
			return node.Name, nil
		}
	}

	return "", fmt.Errorf("no alternative ready node found for current node %q", currentNode)
}
