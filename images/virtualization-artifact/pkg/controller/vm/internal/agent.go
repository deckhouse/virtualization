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
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
)

const nameAgentHandler = "AgentHandler"

var agentConditions = []vmcondition.Type{
	vmcondition.TypeAgentReady,
	vmcondition.TypeAgentVersionNotSupported,
}

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

	if update := addAllUnknown(changed, agentConditions...); update {
		return reconcile.Result{Requeue: true}, nil
	}

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

func (h *AgentHandler) syncAgentReady(vm *virtv2.VirtualMachine, kvvmi *virtv1.VirtualMachineInstance) {
	if vm == nil {
		return
	}

	//nolint:staticcheck
	mgr := conditions.NewManager(vm.Status.Conditions)

	cb := conditions.NewConditionBuilder(vmcondition.TypeAgentReady).Generation(vm.GetGeneration())

	if kvvmi == nil {
		mgr.Update(cb.Status(metav1.ConditionFalse).
			Reason(vmcondition.ReasonAgentNotReady).
			Message("VirtualMachine is not running.").
			Condition())
		vm.Status.Conditions = mgr.Generate()
		return
	}
	for _, c := range kvvmi.Status.Conditions {
		// TODO: wrap kvvmi reasons
		if c.Type == virtv1.VirtualMachineInstanceAgentConnected {
			status := conditionStatus(string(c.Status))
			//nolint:staticcheck
			cb.Status(status).Reason(conditions.DeprecatedWrappedString(c.Reason))
			if status != metav1.ConditionTrue {
				cb.Message(c.Message)
			}
			mgr.Update(cb.Condition())
			vm.Status.Conditions = mgr.Generate()
			return
		}
	}
	mgr.Update(cb.Status(metav1.ConditionFalse).
		Reason(vmcondition.ReasonAgentNotReady).
		Message("Failed to connect to VM Agent.").
		Condition())
	vm.Status.Conditions = mgr.Generate()
}

func (h *AgentHandler) syncAgentVersionNotSupport(vm *virtv2.VirtualMachine, kvvmi *virtv1.VirtualMachineInstance) {
	if vm == nil {
		return
	}

	//nolint:staticcheck
	mgr := conditions.NewManager(vm.Status.Conditions)
	cb := conditions.NewConditionBuilder(vmcondition.TypeAgentVersionNotSupported).Generation(vm.GetGeneration())

	if kvvmi == nil {
		mgr.Update(cb.Status(metav1.ConditionFalse).
			Reason(vmcondition.ReasonAgentNotReady).
			Message("Failed to check version, because Vm is not running.").
			Condition())
		vm.Status.Conditions = mgr.Generate()
		return
	}
	for _, c := range kvvmi.Status.Conditions {
		if c.Type == virtv1.VirtualMachineInstanceUnsupportedAgent {
			status := conditionStatus(string(c.Status))
			//nolint:staticcheck
			cb.Status(status).Reason(conditions.DeprecatedWrappedString(c.Reason))
			if status != metav1.ConditionTrue {
				cb.Message(c.Message)
			}
			mgr.Update(cb.Condition())
			vm.Status.Conditions = mgr.Generate()
			return
		}
	}
	mgr.Update(cb.Status(metav1.ConditionFalse).
		Reason(vmcondition.ReasonAgentNotReady).
		Message("Failed to connect to VM Agent.").
		Condition())
	vm.Status.Conditions = mgr.Generate()
}
