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

package internal

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vm/internal/state"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
)

const nameAgentHandler = "AgentHandler"

func NewAgentHandler() *AgentHandler {
	return &AgentHandler{}
}

type AgentHandler struct{}

func (h *AgentHandler) Handle(ctx context.Context, s state.VirtualMachineState) (reconcile.Result, error) {
	if s.VirtualMachine().IsEmpty() {
		return reconcile.Result{}, nil
	}
	current := s.VirtualMachine().Current()
	changed := s.VirtualMachine().Changed()

	if isDeletion(current) {
		return reconcile.Result{}, nil
	}

	kvvmi, err := s.KVVMI(ctx)
	if err != nil {
		return reconcile.Result{}, err
	}
	h.syncAgentReady(changed, kvvmi)
	h.syncAgentVersionNotSupport(changed, kvvmi)
	if kvvmi != nil {
		changed.Status.GuestOSInfo = kvvmi.Status.GuestOSInfo
	}
	return reconcile.Result{}, nil
}

func (h *AgentHandler) Name() string {
	return nameAgentHandler
}

func (h *AgentHandler) syncAgentReady(vm *v1alpha2.VirtualMachine, kvvmi *virtv1.VirtualMachineInstance) {
	if vm == nil {
		return
	}

	cb := conditions.NewConditionBuilder(vmcondition.TypeAgentReady).Generation(vm.GetGeneration())

	defer func() {
		phase := vm.Status.Phase
		if phase == v1alpha2.MachinePending || phase == v1alpha2.MachineStarting || phase == v1alpha2.MachineStopped {
			conditions.RemoveCondition(vmcondition.TypeAgentReady, &vm.Status.Conditions)
		} else {
			conditions.SetCondition(cb, &vm.Status.Conditions)
		}
	}()

	if kvvmi == nil {
		cb.Status(metav1.ConditionFalse).
			Reason(vmcondition.ReasonAgentNotReady).
			Message("VirtualMachine is not running.")
		return
	}

	for _, c := range kvvmi.Status.Conditions {
		if c.Type == virtv1.VirtualMachineInstanceAgentConnected {
			status := conditionStatus(string(c.Status))

			switch status {
			case metav1.ConditionTrue:
				cb.Status(status).Reason(vmcondition.ReasonAgentReady).Message(c.Message)
			case metav1.ConditionFalse:
				cb.Status(status).Reason(vmcondition.ReasonAgentNotReady).Message(c.Message)
			}

			return
		}
	}

	cb.Status(metav1.ConditionFalse).
		Reason(vmcondition.ReasonAgentNotReady).
		Message("Failed to connect to VM Agent.")
}

func (h *AgentHandler) syncAgentVersionNotSupport(vm *v1alpha2.VirtualMachine, kvvmi *virtv1.VirtualMachineInstance) {
	if vm == nil {
		return
	}

	cb := conditions.NewConditionBuilder(vmcondition.TypeAgentVersionNotSupported).Generation(vm.GetGeneration())

	defer func() {
		switch vm.Status.Phase {
		case v1alpha2.MachinePending, v1alpha2.MachineStarting, v1alpha2.MachineStopped:
			conditions.RemoveCondition(vmcondition.TypeAgentVersionNotSupported, &vm.Status.Conditions)

		default:
			if cb.Condition().Status == metav1.ConditionTrue {
				conditions.SetCondition(cb, &vm.Status.Conditions)
			} else {
				conditions.RemoveCondition(vmcondition.TypeAgentVersionNotSupported, &vm.Status.Conditions)
			}
		}
	}()

	if kvvmi == nil {
		cb.Status(metav1.ConditionFalse).
			Reason(vmcondition.ReasonAgentNotReady).
			Message("Failed to check version, because Vm is not running.")
		return
	}

	for _, c := range kvvmi.Status.Conditions {
		status := conditionStatus(string(c.Status))
		if c.Type == virtv1.VirtualMachineInstanceUnsupportedAgent && status == metav1.ConditionTrue {
			cb.Status(status).Reason(vmcondition.ReasonAgentNotSupported).Message(c.Reason)
			return
		}
	}

	cb.Status(metav1.ConditionFalse).Reason(vmcondition.ReasonAgentSupported)
}
