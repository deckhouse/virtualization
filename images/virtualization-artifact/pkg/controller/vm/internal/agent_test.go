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
	"github.com/deckhouse/virtualization-controller/pkg/common/testutil"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/reconciler"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vm/internal/state"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
)

var _ = Describe("AgentHandler Tests", func() {
	const (
		name      = "vm-agent"
		namespace = "default"
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

	newVM := func(phase v1alpha2.MachinePhase) *v1alpha2.VirtualMachine {
		vm := vmbuilder.NewEmpty(name, namespace)
		vm.Status.Phase = phase
		return vm
	}

	newKVVMI := func(agentConnected, agentUnsupported bool) *virtv1.VirtualMachineInstance {
		kvvmi := newEmptyKVVMI(name, namespace)
		conditions := make([]virtv1.VirtualMachineInstanceCondition, 0)
		if agentConnected {
			conditions = append(conditions, virtv1.VirtualMachineInstanceCondition{
				Type:   virtv1.VirtualMachineInstanceAgentConnected,
				Status: corev1.ConditionTrue,
			})
		}
		if agentUnsupported {
			conditions = append(conditions, virtv1.VirtualMachineInstanceCondition{
				Type:   virtv1.VirtualMachineInstanceUnsupportedAgent,
				Status: corev1.ConditionTrue,
			})
		} else {
			conditions = append(conditions, virtv1.VirtualMachineInstanceCondition{
				Type:   virtv1.VirtualMachineInstanceUnsupportedAgent,
				Status: corev1.ConditionFalse,
			})
		}

		kvvmi.Status.Conditions = conditions
		return kvvmi
	}

	reconcile := func() {
		h := NewAgentHandler()
		_, err := h.Handle(ctx, vmState)
		Expect(err).NotTo(HaveOccurred())
		err = resource.Update(context.Background())
		Expect(err).NotTo(HaveOccurred())
	}

	DescribeTable("AgentReady Condition Tests",
		func(phase v1alpha2.MachinePhase, agentConnected bool, expectedStatus metav1.ConditionStatus, expectedExistence bool) {
			vm := newVM(phase)
			kvvmi := newKVVMI(agentConnected, false)
			fakeClient, resource, vmState = setupEnvironment(vm, kvvmi)

			reconcile()

			newVM := &v1alpha2.VirtualMachine{}
			err := fakeClient.Get(ctx, client.ObjectKeyFromObject(vm), newVM)
			Expect(err).NotTo(HaveOccurred())

			cond, exists := conditions.GetCondition(vmcondition.TypeAgentReady, newVM.Status.Conditions)
			Expect(exists).To(Equal(expectedExistence))
			if exists {
				Expect(cond.Status).To(Equal(expectedStatus))
			}
		},
		Entry("Should add AgentReady as True if agent is connected", v1alpha2.MachineRunning, true, metav1.ConditionTrue, true),
		Entry("Should add AgentReady as False if agent is not connected", v1alpha2.MachineRunning, false, metav1.ConditionFalse, true),

		Entry("Should add AgentReady as True if agent is connected", v1alpha2.MachineStopping, true, metav1.ConditionTrue, true),
		Entry("Should add AgentReady as False if agent is not connected", v1alpha2.MachineStopping, false, metav1.ConditionFalse, true),

		Entry("Should add AgentReady as True if agent is connected", v1alpha2.MachineMigrating, true, metav1.ConditionTrue, true),
		Entry("Should add AgentReady as False if agent is not connected", v1alpha2.MachineMigrating, false, metav1.ConditionFalse, true),

		Entry("Should not add AgentReady if VM is in Pending phase and the agent is connected", v1alpha2.MachinePending, true, metav1.ConditionUnknown, false),
		Entry("Should not add AgentReady if VM is in Pending phase and the agent is not connected", v1alpha2.MachinePending, false, metav1.ConditionUnknown, false),

		Entry("Should not add AgentReady if VM is in Starting phase and the agent is connected", v1alpha2.MachineStarting, true, metav1.ConditionUnknown, false),
		Entry("Should not add AgentReady if VM is in Starting phase and the agent is not connected", v1alpha2.MachineStarting, false, metav1.ConditionUnknown, false),

		Entry("Should not add AgentReady if VM is in Stopped phase and the agent is connected", v1alpha2.MachineStopped, true, metav1.ConditionUnknown, false),
		Entry("Should not add AgentReady if VM is in Stopped phase and the agent is not connected", v1alpha2.MachineStopped, false, metav1.ConditionUnknown, false),
	)

	DescribeTable("AgentVersionNotSupported Condition Tests",
		func(phase v1alpha2.MachinePhase, agentUnsupported bool, expectedStatus metav1.ConditionStatus, expectedExistence bool) {
			vm := newVM(phase)
			vmi := newKVVMI(true, agentUnsupported)
			fakeClient, resource, vmState = setupEnvironment(vm, vmi)

			reconcile()

			newVM := &v1alpha2.VirtualMachine{}
			err := fakeClient.Get(ctx, client.ObjectKeyFromObject(vm), newVM)
			Expect(err).NotTo(HaveOccurred())

			cond, exists := conditions.GetCondition(vmcondition.TypeAgentVersionNotSupported, newVM.Status.Conditions)
			Expect(exists).To(Equal(expectedExistence))
			if exists {
				Expect(cond.Status).To(Equal(expectedStatus))
			}
		},
		Entry("Should set unsupported version condition as True in Running phase", v1alpha2.MachineRunning, true, metav1.ConditionTrue, true),
		Entry("Should not set unsupported version condition as False in Running phase", v1alpha2.MachineRunning, false, metav1.ConditionUnknown, false),

		Entry("Should set unsupported version condition as True in Stopping phase", v1alpha2.MachineStopping, true, metav1.ConditionTrue, true),
		Entry("Should set unsupported version condition as False in Stopping phase", v1alpha2.MachineStopping, false, metav1.ConditionUnknown, false),

		Entry("Should set unsupported version condition as True in Migrating phase", v1alpha2.MachineMigrating, true, metav1.ConditionTrue, true),
		Entry("Should set unsupported version condition as False in Migrating phase", v1alpha2.MachineMigrating, false, metav1.ConditionUnknown, false),

		Entry("Should not set unsupported version condition as True in Pending phase", v1alpha2.MachinePending, true, metav1.ConditionUnknown, false),
		Entry("Should not set unsupported version condition as False in Pending phase", v1alpha2.MachinePending, false, metav1.ConditionUnknown, false),

		Entry("Should not set unsupported version condition as True in Starting phase", v1alpha2.MachineStarting, true, metav1.ConditionUnknown, false),
		Entry("Should not set unsupported version condition as False in Starting phase", v1alpha2.MachineStarting, false, metav1.ConditionUnknown, false),

		Entry("Should not set unsupported version condition as True in Stopped phase", v1alpha2.MachineStopped, true, metav1.ConditionUnknown, false),
		Entry("Should not set unsupported version condition as False in Stopped phase", v1alpha2.MachineStopped, false, metav1.ConditionUnknown, false),
	)
})
