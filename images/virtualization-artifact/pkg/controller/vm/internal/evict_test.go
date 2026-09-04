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

package internal

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	vmbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vm"
	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/testutil"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/reconciler"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vm/internal/state"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
)

var _ = Describe("TestEvictHandler", func() {
	const (
		name      = "vm-evict"
		namespace = "default"
		nodeName  = "node1"
	)

	var (
		ctx        = testutil.ContextBackgroundWithNoOpLogger()
		fakeClient client.WithWatch
		resource   *reconciler.Resource[*v1alpha2.VirtualMachine, v1alpha2.VirtualMachineStatus]
		vmState    state.VirtualMachineState
	)

	AfterEach(func() {
		fakeClient = nil
		resource = nil
		vmState = nil
	})

	// A machine whose placement rules match no node: the reason does not change while it runs.
	newVMNoTarget := func() *v1alpha2.VirtualMachine {
		vm := vmbuilder.NewEmpty(name, namespace)
		vm.Status.Conditions = append(vm.Status.Conditions, metav1.Condition{
			Type:   vmcondition.TypeMigratable.String(),
			Status: metav1.ConditionFalse,
			Reason: vmcondition.ReasonNoMigrationTarget.String(),
		})
		return vm
	}

	// A machine that only lacks a node to accept it right now: it is reported as migratable, and
	// only the reason tells it apart from a machine that is already on its way.
	newVMWaitingForTarget := func() *v1alpha2.VirtualMachine {
		vm := vmbuilder.NewEmpty(name, namespace)
		vm.Status.Conditions = append(vm.Status.Conditions, metav1.Condition{
			Type:   vmcondition.TypeMigratable.String(),
			Status: metav1.ConditionTrue,
			Reason: vmcondition.ReasonWaitingForMigrationTarget.String(),
		})
		return vm
	}

	newVM := func(withCond bool, migratable metav1.ConditionStatus) *v1alpha2.VirtualMachine {
		vm := vmbuilder.NewEmpty(name, namespace)
		if withCond {
			vm.Status.Conditions = append(vm.Status.Conditions, metav1.Condition{
				Type:    vmcondition.TypeEvictionRequired.String(),
				Status:  metav1.ConditionTrue,
				Reason:  vmcondition.ReasonEvictionRequired.String(),
				Message: "Some message",
			})
		}
		if migratable != "" {
			vm.Status.Conditions = append(vm.Status.Conditions, metav1.Condition{
				Type:   vmcondition.TypeMigratable.String(),
				Status: migratable,
				Reason: vmcondition.ReasonMigratable.String(),
			})
		}
		return vm
	}

	newKVVMI := func(evacuationNodeName string, phase virtv1.VirtualMachineInstancePhase) *virtv1.VirtualMachineInstance {
		kvvmi := newEmptyKVVMI(name, namespace)
		kvvmi.Status.EvacuationNodeName = evacuationNodeName
		kvvmi.Status.Phase = phase
		kvvmi.Status.NodeName = nodeName
		return kvvmi
	}

	approvedAnnotations := func(extra map[string]string) map[string]string {
		out := map[string]string{annotations.AnnNodeVMRestartApproved: ""}
		for k, v := range extra {
			out[k] = v
		}
		return out
	}

	newNode := func(unschedulable bool, annotations map[string]string) *corev1.Node {
		return &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name:        nodeName,
				Annotations: annotations,
			},
			Spec: corev1.NodeSpec{Unschedulable: unschedulable},
		}
	}

	reconcile := func() {
		h := NewEvictHandler()
		_, err := h.Handle(testutil.ContextBackgroundWithNoOpLogger(), vmState)
		Expect(err).NotTo(HaveOccurred())
		err = resource.Update(context.Background())
		Expect(err).NotTo(HaveOccurred())
	}

	DescribeTable("Condition EvictionRequired should be in expected state",
		func(vm *v1alpha2.VirtualMachine, kvvmi *virtv1.VirtualMachineInstance, node *corev1.Node, condShouldExists bool, expectedReason vmcondition.EvictionRequiredReason) {
			fakeClient, resource, vmState = setupEnvironment(vm, kvvmi, node)
			reconcile()

			newVM := &v1alpha2.VirtualMachine{}
			err := fakeClient.Get(ctx, client.ObjectKeyFromObject(vm), newVM)
			Expect(err).NotTo(HaveOccurred())

			evictionRequired, exists := conditions.GetCondition(vmcondition.TypeEvictionRequired, newVM.Status.Conditions)
			if condShouldExists {
				Expect(exists).To(BeTrue())
				Expect(evictionRequired.Status).To(Equal(metav1.ConditionTrue))
				Expect(evictionRequired.Reason).To(Equal(expectedReason.String()))
				Expect(evictionRequired.Message).ToNot(BeEmpty())
			} else {
				Expect(exists).To(BeFalse())
			}
		},
		Entry("adds the condition when the instance has an evacuation node",
			newVM(false, metav1.ConditionTrue), newKVVMI(nodeName, virtv1.Running), newNode(false, nil),
			true, vmcondition.ReasonEvictionRequired),
		// A closed node is a warning, not a promise: a drain may never follow.
		Entry("warns while the node is being drained by node-manager",
			newVM(false, metav1.ConditionTrue), newKVVMI("", virtv1.Running), newNode(true, map[string]string{annotations.AnnNodeDraining: "user"}),
			true, vmcondition.ReasonNodeUnderMaintenance),
		Entry("keeps warning after the drain has given up",
			newVM(false, metav1.ConditionTrue), newKVVMI("", virtv1.Running), newNode(true, map[string]string{annotations.AnnNodeDrained: "user"}),
			true, vmcondition.ReasonNodeUnderMaintenance),
		Entry("warns when the shutdown inhibitor closed the node",
			newVM(false, metav1.ConditionTrue), newKVVMI("", virtv1.Running), newNode(true, map[string]string{annotations.AnnNodeCordonedBy: "shutdown-inhibitor"}),
			true, vmcondition.ReasonNodeUnderMaintenance),
		Entry("warns a machine that cannot be live migrated while no restart is approved",
			newVM(false, metav1.ConditionFalse), newKVVMI("", virtv1.Running), newNode(true, map[string]string{annotations.AnnNodeCordonedBy: "shutdown-inhibitor"}),
			true, vmcondition.ReasonNodeUnderMaintenance),
		// Without a permission on the node the platform restarts nothing: the node waits for a person.
		Entry("blocks the eviction while no restart is approved",
			newVM(false, metav1.ConditionFalse), newKVVMI(nodeName, virtv1.Running), newNode(true, map[string]string{annotations.AnnNodeDraining: "user"}),
			true, vmcondition.ReasonEvictionBlocked),
		Entry("blocks the eviction of a machine whose placement rules match no node",
			newVMNoTarget(), newKVVMI(nodeName, virtv1.Running), newNode(true, map[string]string{annotations.AnnNodeDraining: "user"}),
			true, vmcondition.ReasonEvictionBlocked),
		// A machine that only lacks a free target keeps its migratability, so it leaves the node
		// alive as soon as a target appears and is never counted as blocking the maintenance.
		Entry("keeps promising a migration while only a target is missing",
			newVMWaitingForTarget(), newKVVMI(nodeName, virtv1.Running), newNode(true, map[string]string{annotations.AnnNodeDraining: "user"}),
			true, vmcondition.ReasonEvictionRequired),
		// The administrator allowed the restart on this node, so the machine is restarted.
		Entry("restarts by the approval on the node",
			newVM(false, metav1.ConditionFalse), newKVVMI(nodeName, virtv1.Running),
			newNode(true, approvedAnnotations(map[string]string{annotations.AnnNodeDraining: "user"})),
			true, vmcondition.ReasonRestartRequired),
		// The permission never reaches a machine that can be live migrated, even when it has nowhere
		// to go at this moment: restarting a guest that would have left alive is not an option.
		Entry("does not restart a machine waiting for a target even when the restart is approved",
			newVMWaitingForTarget(), newKVVMI(nodeName, virtv1.Running),
			newNode(true, approvedAnnotations(map[string]string{annotations.AnnNodeDraining: "user"})),
			true, vmcondition.ReasonEvictionRequired),
		// The platform reacts to an eviction and to nothing else: an approval on a closed node asks
		// for no restart until somebody actually drains the node.
		Entry("waits for an eviction even when the restart is approved",
			newVM(false, metav1.ConditionFalse), newKVVMI("", virtv1.Running),
			newNode(true, approvedAnnotations(map[string]string{annotations.AnnNodeDraining: "user"})),
			true, vmcondition.ReasonNodeUnderMaintenance),
		// A machine that can be live migrated still only gets a warning: it leaves the node alive
		// once the eviction arrives, and the permission changes nothing for it.
		Entry("warns a migratable machine even when the restart is approved",
			newVM(false, metav1.ConditionTrue), newKVVMI("", virtv1.Running),
			newNode(true, approvedAnnotations(map[string]string{annotations.AnnNodeDraining: "user"})),
			true, vmcondition.ReasonNodeUnderMaintenance),
		// A bare cordon is not maintenance: an administrator closes a node for other reasons too.
		Entry("ignores a node closed without a maintenance marker",
			newVM(true, metav1.ConditionFalse), newKVVMI("", virtv1.Running), newNode(true, nil),
			false, vmcondition.EvictionRequiredReason("")),
		Entry("removes the condition when the node works as usual",
			newVM(true, metav1.ConditionTrue), newKVVMI("", virtv1.Running), newNode(false, map[string]string{annotations.AnnNodeDraining: "user"}),
			false, vmcondition.EvictionRequiredReason("")),
		Entry("removes the condition when the instance is not running",
			newVM(true, metav1.ConditionTrue), newKVVMI("", virtv1.Failed), newNode(true, map[string]string{annotations.AnnNodeDraining: "user"}),
			false, vmcondition.EvictionRequiredReason("")),
	)
})
