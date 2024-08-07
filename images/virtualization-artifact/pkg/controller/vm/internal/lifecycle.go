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
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vm/internal/state"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
)

var lifeCycleConditions = []string{
	string(vmcondition.TypeRunning),
	string(vmcondition.TypeMigrating),
	string(vmcondition.TypeMigratable),
	string(vmcondition.TypePodStarted),
}

const nameLifeCycleHandler = "LifeCycleHandler"

func NewLifeCycleHandler(client client.Client, recorder record.EventRecorder) *LifeCycleHandler {
	return &LifeCycleHandler{client: client, recorder: recorder}
}

type LifeCycleHandler struct {
	client   client.Client
	recorder record.EventRecorder
}

func (h *LifeCycleHandler) Handle(ctx context.Context, s state.VirtualMachineState) (reconcile.Result, error) {
	log := logger.FromContext(ctx).With(logger.SlogHandler(nameLifeCycleHandler))

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

	changed.Status.Phase = getPhase(kvvm)

	kvvmi, err := s.KVVMI(ctx)
	if err != nil {
		return reconcile.Result{}, err
	}

	pod, err := s.Pod(ctx)
	if err != nil {
		return reconcile.Result{}, err
	}

	h.syncMigrationState(changed, kvvm, kvvmi)
	h.syncPodStarted(changed, kvvm, kvvmi, pod)
	h.syncRunning(changed, kvvm, kvvmi, log)
	return reconcile.Result{}, nil
}

func (h *LifeCycleHandler) Name() string {
	return nameLifeCycleHandler
}

func (h *LifeCycleHandler) syncMigrationState(vm *virtv2.VirtualMachine, kvvm *virtv1.VirtualMachine, kvvmi *virtv1.VirtualMachineInstance) {
	if vm == nil {
		return
	}
	if kvvmi == nil || kvvmi.Status.MigrationState == nil {
		vm.Status.MigrationState = nil
	} else {
		vm.Status.MigrationState = h.wrapMigrationState(kvvmi.Status.MigrationState)
	}

	mgr := conditions.NewManager(vm.Status.Conditions)
	cbMigrating := conditions.NewConditionBuilder2(vmcondition.TypeMigrating).Generation(vm.GetGeneration())
	if vm.Status.MigrationState != nil &&
		vm.Status.MigrationState.StartTimestamp != nil &&
		vm.Status.MigrationState.EndTimestamp == nil {
		mgr.Update(cbMigrating.
			Status(metav1.ConditionTrue).
			Reason2(vmcondition.ReasonVmIsMigrating).
			Condition())
	} else {
		mgr.Update(cbMigrating.
			Status(metav1.ConditionFalse).
			Reason2(vmcondition.ReasonVmIsNotMigrating).
			Condition())
	}

	cbMigratable := conditions.NewConditionBuilder2(vmcondition.TypeMigratable).Generation(vm.GetGeneration())

	if kvvm != nil {
		liveMigratable := service.GetKVVMCondition(string(virtv1.VirtualMachineInstanceIsMigratable), kvvm.Status.Conditions)
		if liveMigratable != nil && liveMigratable.Status == corev1.ConditionFalse && liveMigratable.Reason == virtv1.VirtualMachineInstanceReasonDisksNotMigratable {
			mgr.Update(cbMigratable.
				Status(metav1.ConditionFalse).
				Reason2(vmcondition.ReasonNotMigratable).
				Message("Live migration requires that all PVCs must be shared (using ReadWriteMany access mode)").
				Condition())
			vm.Status.Conditions = mgr.Generate()
			return
		}
	}

	mgr.Update(cbMigratable.
		Status(metav1.ConditionTrue).
		Reason2(vmcondition.ReasonMigratable).
		Condition())
	vm.Status.Conditions = mgr.Generate()
}

func (h *LifeCycleHandler) syncPodStarted(vm *virtv2.VirtualMachine, kvvm *virtv1.VirtualMachine, kvvmi *virtv1.VirtualMachineInstance, pod *corev1.Pod) {
	if vm == nil {
		return
	}

	mgr := conditions.NewManager(vm.Status.Conditions)
	cb := conditions.NewConditionBuilder2(vmcondition.TypePodStarted).Generation(vm.GetGeneration())

	if isPodStarted(pod) {
		mgr.Update(cb.Status(metav1.ConditionTrue).
			Reason2(vmcondition.ReasonPodStarted).
			Condition())
		vm.Status.Conditions = mgr.Generate()
		return
	}

	// Try to extract error from pod.
	if pod != nil && pod.Status.Message != "" {
		mgr.Update(cb.Status(metav1.ConditionFalse).
			Reason2(vmcondition.ReasonPodNotStarted).
			Message(fmt.Sprintf("%s: %s", pod.Status.Reason, pod.Status.Message)).
			Condition())
		vm.Status.Conditions = mgr.Generate()
		return
	}

	if kvvm != nil {
		// Try to extract error from kvvm PodScheduled condition.
		cond := service.GetKVVMCondition(string(corev1.PodScheduled), kvvm.Status.Conditions)
		if cond != nil && cond.Status == corev1.ConditionFalse && cond.Message != "" {
			mgr.Update(cb.
				Status(metav1.ConditionFalse).
				Reason2(vmcondition.ReasonPodNotStarted).
				Message(fmt.Sprintf("%s: %s", cond.Reason, cond.Message)).
				Condition())
			vm.Status.Conditions = mgr.Generate()
			return
		}

		// Try to extract error from kvvm Synchronized condition.
		if isPodStartedError(kvvm.Status.PrintableStatus) {
			msg := fmt.Sprintf("Failed to start pod: %s", kvvm.Status.PrintableStatus)
			if kvvmi != nil {
				msg = fmt.Sprintf("%s, %s", msg, kvvmi.Status.Phase)
			}
			synchronized := service.GetKVVMCondition(string(virtv1.VirtualMachineInstanceSynchronized), kvvm.Status.Conditions)
			if synchronized != nil && synchronized.Status == corev1.ConditionFalse && synchronized.Message != "" {
				msg = fmt.Sprintf("%s; %s: %s", msg, synchronized.Reason, synchronized.Message)
			}
			mgr.Update(cb.Status(metav1.ConditionFalse).
				Reason2(vmcondition.ReasonPodNotStarted).
				Message(msg).
				Condition())
			vm.Status.Conditions = mgr.Generate()
			return
		}
	}

	mgr.Update(cb.Status(metav1.ConditionFalse).
		Reason2(vmcondition.ReasonPodNotFound).
		Message("Pod of the virtual machine was not found").
		Condition())
}

func (h *LifeCycleHandler) syncRunning(vm *virtv2.VirtualMachine, kvvm *virtv1.VirtualMachine, kvvmi *virtv1.VirtualMachineInstance, log *slog.Logger) {
	if vm == nil {
		return
	}

	mgr := conditions.NewManager(vm.Status.Conditions)
	cb := conditions.NewConditionBuilder2(vmcondition.TypeRunning).Generation(vm.GetGeneration())

	if kvvm != nil && isInternalVirtualMachineError(kvvm.Status.PrintableStatus) {
		msg := fmt.Sprintf("Internal virtual machine error: %s", kvvm.Status.PrintableStatus)
		if kvvmi != nil {
			msg = fmt.Sprintf("%s, %s", msg, kvvmi.Status.Phase)
		}

		synchronized := service.GetKVVMCondition(string(virtv1.VirtualMachineInstanceSynchronized), kvvm.Status.Conditions)
		if synchronized != nil && synchronized.Status == corev1.ConditionFalse && synchronized.Message != "" {
			msg = fmt.Sprintf("%s; %s: %s", msg, synchronized.Reason, synchronized.Message)
		}

		log.Error(msg)
		h.recorder.Event(vm, corev1.EventTypeWarning, vmcondition.ReasonInternalVirtualMachineError.String(), msg)

		mgr.Update(cb.
			Status(metav1.ConditionFalse).
			Reason(vmcondition.ReasonInternalVirtualMachineError.String()).
			Message(msg).
			Condition())
		vm.Status.Conditions = mgr.Generate()
		return
	}

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
