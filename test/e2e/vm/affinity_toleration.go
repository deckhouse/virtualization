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
	"github.com/deckhouse/virtualization/test/e2e/internal/util"
)

const (
	affinityHostnameLabelKey = "kubernetes.io/hostname"
	masterLabelKey           = "node.deckhouse.io/group"
	kvmEnabledLabelKey       = "virtualization.deckhouse.io/kvm-enabled"
	vmLabelKey               = "vm"
)

var _ = Describe("VirtualMachineAffinityAndToleration", func() {
	var (
		f   = framework.NewFramework("vm-affinity-toleration")
		ctx context.Context

		vmA *v1alpha2.VirtualMachine
		vmB *v1alpha2.VirtualMachine
		vmC *v1alpha2.VirtualMachine
		vmD *v1alpha2.VirtualMachine
	)

	BeforeEach(func() {
		DeferCleanup(f.After)
		f.Before()
		ctx = context.Background()
	})

	It("checks placement via status.nodeName and migrations after affinity changes", func() {
		By("Checking test prerequisites", func() {
			readyNodes, err := listReadyNodes(ctx, f, map[string]string{kvmEnabledLabelKey: "true"})
			Expect(err).NotTo(HaveOccurred())
			if len(readyNodes) < 2 {
				Skip("at least two ready KVM-enabled nodes are required")
			}

			masterNodes, err := listReadyNodes(ctx, f, map[string]string{kvmEnabledLabelKey: "true", masterLabelKey: "master"})
			Expect(err).NotTo(HaveOccurred())
			if len(masterNodes) == 0 {
				Skip("at least one ready KVM-enabled master node is required")
			}
		})

		By("Creating vm-a", func() {
			vmA = newPlacementVM("vm-a", f.Namespace().Name, nil)
			err := f.CreateWithDeferredDeletion(ctx, newRootVDForPlacement(vmA.Name, f.Namespace().Name), vmA)
			Expect(err).NotTo(HaveOccurred())

			util.UntilObjectPhase(string(v1alpha2.MachineRunning), framework.LongTimeout, vmA)
			util.UntilVMAgentReady(crclient.ObjectKeyFromObject(vmA), framework.LongTimeout)
		})

		By("Creating vm-b, vm-c and vm-d", func() {
			vmB = newPlacementVM("vm-b", f.Namespace().Name, antiAffinityToVM("vm-a"))
			vmC = newPlacementVM("vm-c", f.Namespace().Name, affinityToVM("vm-a"))
			vmD = newPlacementVM("vm-d", f.Namespace().Name, masterNodeAffinity())

			objs := []crclient.Object{
				newRootVDForPlacement(vmB.Name, f.Namespace().Name), vmB,
				newRootVDForPlacement(vmC.Name, f.Namespace().Name), vmC,
				newRootVDForPlacement(vmD.Name, f.Namespace().Name), vmD,
			}
			err := f.CreateWithDeferredDeletion(ctx, objs...)
			Expect(err).NotTo(HaveOccurred())

			util.UntilObjectPhase(string(v1alpha2.MachineRunning), framework.LongTimeout, vmB, vmC, vmD)
			util.UntilVMAgentReady(crclient.ObjectKeyFromObject(vmB), framework.LongTimeout)
			util.UntilVMAgentReady(crclient.ObjectKeyFromObject(vmC), framework.LongTimeout)
			util.UntilVMAgentReady(crclient.ObjectKeyFromObject(vmD), framework.LongTimeout)
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

			Expect(vmC.Status.Node).To(Equal(nodeA), "vm-a and vm-c should run on the same node")
			Expect(vmB.Status.Node).NotTo(Equal(nodeA), "vm-a and vm-b should run on different nodes")

			nodeD := getNode(ctx, f, vmD.Status.Node)
			Expect(nodeD.Labels).To(HaveKeyWithValue(masterLabelKey, "master"), "vm-d should run on a master node")
		})

		By("Changing vm-c affinity to anti-affinity and verifying migration to another node", func() {
			util.UntilConditionStatus(vmcondition.TypeMigratable.String(), string(metav1.ConditionTrue), framework.LongTimeout, vmC)

			vmC = getVirtualMachine(ctx, f, vmC.Name)
			sourceNode := vmC.Status.Node
			startedAt := time.Now().UTC()

			vmC.Spec.Affinity = antiAffinityToVM("vm-a")
			err := f.GenericClient().Update(ctx, vmC)
			Expect(err).NotTo(HaveOccurred())

			waitForFreshVMMigration(ctx, f, crclient.ObjectKeyFromObject(vmC), startedAt, sourceNode, nodeA, false, framework.MaxTimeout)
			util.UntilVMAgentReady(crclient.ObjectKeyFromObject(vmC), framework.LongTimeout)
		})

		var migratedNodeC string
		By("Verifying vm-c moved away from vm-a node", func() {
			vmC = getVirtualMachine(ctx, f, vmC.Name)
			migratedNodeC = vmC.Status.Node
			Expect(migratedNodeC).NotTo(Equal(nodeA), "vm-c should run on a different node after anti-affinity update")
		})

		By("Changing vm-c anti-affinity back to affinity and verifying migration back to vm-a node", func() {
			util.UntilConditionStatus(vmcondition.TypeMigratable.String(), string(metav1.ConditionTrue), framework.LongTimeout, vmC)

			vmC = getVirtualMachine(ctx, f, vmC.Name)
			startedAt := time.Now().UTC()

			vmC.Spec.Affinity = affinityToVM("vm-a")
			err := f.GenericClient().Update(ctx, vmC)
			Expect(err).NotTo(HaveOccurred())

			waitForFreshVMMigration(ctx, f, crclient.ObjectKeyFromObject(vmC), startedAt, migratedNodeC, nodeA, true, framework.MaxTimeout)
			util.UntilVMAgentReady(crclient.ObjectKeyFromObject(vmC), framework.LongTimeout)
		})

		By("Verifying vm-c returned to vm-a node via status.nodeName", func() {
			vmC = getVirtualMachine(ctx, f, vmC.Name)
			Expect(vmC.Status.Node).To(Equal(nodeA), "vm-c should return to the same node as vm-a")
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

func newRootVDForPlacement(vmName, namespace string) *v1alpha2.VirtualDisk {
	return object.NewVDFromCVI(
		rootVDNameForVM(vmName),
		namespace,
		object.PrecreatedCVIAlpineBIOS,
		vdbuilder.WithSize(ptr.To(resource.MustParse("350Mi"))),
	)
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

func waitForFreshVMMigration(
	ctx context.Context,
	f *framework.Framework,
	key crclient.ObjectKey,
	notBefore time.Time,
	sourceNode string,
	targetNode string,
	expectSameTarget bool,
	timeout time.Duration,
) {
	GinkgoHelper()

	Eventually(func(g Gomega) {
		vm := getVirtualMachine(ctx, f, key.Name)
		util.SkipIfKnownMigrationFailure(vm)

		state := vm.Status.MigrationState
		g.Expect(state).NotTo(BeNil())
		g.Expect(state.StartTimestamp).NotTo(BeNil())
		g.Expect(state.StartTimestamp.UTC().Before(notBefore)).To(BeFalse(), "expected a fresh migration")
		g.Expect(state.EndTimestamp.IsZero()).To(BeFalse(), "migration is not completed")
		g.Expect(state.Result).To(Equal(v1alpha2.MigrationResultSucceeded))
		g.Expect(state.Source.Node).To(Equal(sourceNode))
		g.Expect(vm.Status.Node).To(Equal(state.Target.Node))
		if expectSameTarget {
			g.Expect(state.Target.Node).To(Equal(targetNode))
		} else {
			g.Expect(state.Target.Node).NotTo(Equal(targetNode))
		}
	}).WithTimeout(timeout).WithPolling(time.Second).Should(Succeed())
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
