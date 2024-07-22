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
	"fmt"
	"log/slog"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vm/internal/state"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
)

var lifeCycleConditions = []string{
	string(vmcondition.TypeRunning),
	string(vmcondition.TypeMigrating),
	string(vmcondition.TypePodStarted),
}

const nameLifeCycleHandler = "LifeCycleHandler"

func NewLifeCycleHandler(client client.Client, recorder record.EventRecorder, logger *slog.Logger) *LifeCycleHandler {
	return &LifeCycleHandler{client: client, recorder: recorder, logger: logger.With("handler", nameLifeCycleHandler)}
}

type LifeCycleHandler struct {
	client   client.Client
	recorder record.EventRecorder
	logger   *slog.Logger
}

func (h *LifeCycleHandler) Handle(ctx context.Context, s state.VirtualMachineState) (reconcile.Result, error) {
	current := s.VirtualMachine().Current()
	changed := s.VirtualMachine().Changed()

	defer func() {
		if changed == nil {
			return
		}
		if len(changed.Status.Conditions) == 0 {
			changed.Status.ObservedGeneration = changed.GetGeneration()
			return
		}
		gen := changed.Status.Conditions[0].ObservedGeneration
		for _, c := range changed.Status.Conditions {
			if gen != c.ObservedGeneration {
				return
			}
		}
		changed.Status.ObservedGeneration = gen
	}()
	if isDeletion(current) {
		changed.Status.Phase = virtv2.MachineTerminating
		return reconcile.Result{}, nil
	}

	if updated := addAllUnknown(changed, lifeCycleConditions...); updated {
		changed.Status.Phase = virtv2.MachinePending
		return reconcile.Result{Requeue: true}, nil
	}
	kvvm, err := s.KVVM(ctx)
	if err != nil {
		return reconcile.Result{}, err
	}
	phase := getPhase(kvvm)
	switch phase {
	case "":
		phase = virtv2.MachinePending
		h.logger.Error(fmt.Sprintf("unexpected KVVM state: status %q, fallback VM phase to %q", kvvm.Status.PrintableStatus, phase))
	case virtv2.MachineDegraded:
		h.recorder.Event(changed, corev1.EventTypeWarning, virtv2.ReasonVMDegraded, "KVVM failure.")
		h.logger.Error("KVVM failure", "status", kvvm.Status.PrintableStatus, "kvvm", kvvm.GetName())
	}
	changed.Status.Phase = phase
	kvvmi, err := s.KVVMI(ctx)
	if err != nil {
		return reconcile.Result{}, err
	}

	pod, err := s.Pod(ctx)
	if err != nil {
		return reconcile.Result{}, err
	}

	h.syncMigrationState(changed, kvvmi)
	h.syncPodStarted(changed, pod)
	h.syncRunning(changed, kvvmi)
	return reconcile.Result{}, nil
}

func (h *LifeCycleHandler) Name() string {
	return nameLifeCycleHandler
}

func (h *LifeCycleHandler) syncMigrationState(vm *virtv2.VirtualMachine, kvvmi *virtv1.VirtualMachineInstance) {
	if vm == nil {
		return
	}
	if kvvmi == nil || kvvmi.Status.MigrationState == nil {
		vm.Status.MigrationState = nil
	} else {
		vm.Status.MigrationState = h.wrapMigrationState(kvvmi.Status.MigrationState)
	}

	mgr := conditions.NewManager(vm.Status.Conditions)
	cb := conditions.NewConditionBuilder2(vmcondition.TypeMigrating).Generation(vm.GetGeneration())
	if vm.Status.MigrationState != nil &&
		vm.Status.MigrationState.StartTimestamp != nil &&
		vm.Status.MigrationState.EndTimestamp == nil {
		mgr.Update(cb.
			Reason2(vmcondition.ReasonVmIsMigrating).
			Status(metav1.ConditionTrue).
			Condition())
		vm.Status.Conditions = mgr.Generate()
		return
	}
	mgr.Update(cb.
		Status(metav1.ConditionFalse).
		Reason2(vmcondition.ReasonVmIsNotMigrating).
		Condition())
	vm.Status.Conditions = mgr.Generate()
}

func (h *LifeCycleHandler) syncPodStarted(vm *virtv2.VirtualMachine, pod *corev1.Pod) {
	if vm == nil {
		return
	}

	mgr := conditions.NewManager(vm.Status.Conditions)
	cb := conditions.NewConditionBuilder2(vmcondition.TypePodStarted).Generation(vm.GetGeneration())

	if pod != nil && pod.Status.StartTime != nil {
		mgr.Update(cb.
			Status(metav1.ConditionTrue).
			Reason2(vmcondition.ReasonPodStarted).
			Condition())
		vm.Status.Conditions = mgr.Generate()
		return
	}
	cb.Status(metav1.ConditionFalse)
	// TODO: wrap pod reasons
	if pod != nil {
		mgr.Update(cb.
			Reason(pod.Status.Reason).
			Message(pod.Status.Message).
			Condition())
		vm.Status.Conditions = mgr.Generate()
		return
	}
	mgr.Update(cb.
		Reason2(vmcondition.ReasonPodNodFound).
		Message("Pod of the virtual machine was not found").
		Condition())
	vm.Status.Conditions = mgr.Generate()
}

func (h *LifeCycleHandler) syncRunning(vm *virtv2.VirtualMachine, kvvmi *virtv1.VirtualMachineInstance) {
	if vm == nil {
		return
	}

	mgr := conditions.NewManager(vm.Status.Conditions)
	cb := conditions.NewConditionBuilder2(vmcondition.TypeRunning).Generation(vm.GetGeneration())

	if kvvmi != nil {
		vm.Status.Node = kvvmi.Status.NodeName

		if vm.Status.Phase == virtv2.MachineRunning {
			mgr.Update(cb.
				Reason2(vmcondition.ReasonVmIsRunning).
				Status(metav1.ConditionTrue).
				Condition())
			vm.Status.Conditions = mgr.Generate()
			return
		}
		for _, c := range kvvmi.Status.Conditions {
			if c.Type == virtv1.VirtualMachineInstanceReady {
				mgr.Update(cb.
					Status(conditionStatus(string(c.Status))).
					Reason(c.Reason).
					Message(c.Message).
					Condition())
				vm.Status.Conditions = mgr.Generate()
				return
			}
		}
	}
	mgr.Update(cb.
		Reason2(vmcondition.ReasonVmIsNotRunning).
		Status(metav1.ConditionFalse).
		Condition())
	vm.Status.Conditions = mgr.Generate()
}

func (h *LifeCycleHandler) wrapMigrationState(state *virtv1.VirtualMachineInstanceMigrationState) *virtv2.VirtualMachineMigrationState {
	if state == nil {
		return nil
	}
	return &virtv2.VirtualMachineMigrationState{
		StartTimestamp: state.StartTimestamp,
		EndTimestamp:   state.EndTimestamp,
		Target: virtv2.VirtualMachineLocation{
			Node: state.TargetNode,
			Pod:  state.TargetPod,
		},
		Source: virtv2.VirtualMachineLocation{
			Node: state.SourceNode,
		},
		Result: h.getResult(state),
	}
}

func (h *LifeCycleHandler) getResult(state *virtv1.VirtualMachineInstanceMigrationState) virtv2.MigrationResult {
	if state == nil {
		return ""
	}
	switch {
	case state.Completed && !state.Failed:
		return virtv2.MigrationResultSucceeded
	case state.Failed:
		return virtv2.MigrationResultFailed
	default:
		return ""
	}
}
