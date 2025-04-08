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
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	podutil "github.com/deckhouse/virtualization-controller/pkg/common/pod"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vm/internal/state"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
)

var lifeCycleConditions = []vmcondition.Type{
	vmcondition.TypeRunning,
	vmcondition.TypeMigrating,
	vmcondition.TypeMigratable,
	vmcondition.TypePodStarted,
}

const nameLifeCycleHandler = "LifeCycleHandler"

func NewLifeCycleHandler(client client.Client, recorder eventrecord.EventRecorderLogger) *LifeCycleHandler {
	return &LifeCycleHandler{client: client, recorder: recorder}
}

type LifeCycleHandler struct {
	client   client.Client
	recorder eventrecord.EventRecorderLogger
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

	if updated := addAllUnknown(changed, lifeCycleConditions...); updated || changed.Status.Phase == "" {
		changed.Status.Phase = virtv2.MachinePending
		return reconcile.Result{Requeue: true}, nil
	}

	kvvm, err := s.KVVM(ctx)
	if err != nil {
		return reconcile.Result{}, err
	}

	changed.Status.Phase = getPhase(changed, kvvm)

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

	cbMigrating := conditions.NewConditionBuilder(vmcondition.TypeMigrating).Generation(vm.GetGeneration())

	switch {
	case vm.Status.MigrationState != nil &&
		vm.Status.MigrationState.StartTimestamp != nil &&
		vm.Status.MigrationState.EndTimestamp == nil:

		cbMigrating.Status(metav1.ConditionTrue).Reason(vmcondition.ReasonVmIsMigrating)
		conditions.SetCondition(cbMigrating, &vm.Status.Conditions)

	case kvvmi != nil && kvvmi.Status.MigrationState != nil &&
		kvvmi.Status.MigrationState.EndTimestamp != nil &&
		kvvmi.Status.MigrationState.Failed:

		msg := kvvmi.Status.MigrationState.FailureReason
		cbMigrating.Status(metav1.ConditionFalse).
			Reason(vmcondition.ReasonLastMigrationFinishedWithError).
			Message(msg)
		conditions.SetCondition(cbMigrating, &vm.Status.Conditions)

	default:
		cbMigrating.Status(metav1.ConditionFalse).Reason(vmcondition.ReasonVmIsNotMigrating)
		conditions.SetCondition(cbMigrating, &vm.Status.Conditions)
	}

	cbMigratable := conditions.NewConditionBuilder(vmcondition.TypeMigratable).Generation(vm.GetGeneration())

	if kvvm != nil {
		liveMigratable := service.GetKVVMCondition(string(virtv1.VirtualMachineInstanceIsMigratable), kvvm.Status.Conditions)
		if liveMigratable != nil && liveMigratable.Status == corev1.ConditionFalse && liveMigratable.Reason == virtv1.VirtualMachineInstanceReasonDisksNotMigratable {
			cbMigratable.Status(metav1.ConditionFalse).
				Reason(vmcondition.ReasonNotMigratable).
				Message("Live migration requires that all PVCs must be shared (using ReadWriteMany access mode)")
			conditions.SetCondition(cbMigratable, &vm.Status.Conditions)
			return
		}
	}
	cbMigratable.Status(metav1.ConditionTrue).Reason(vmcondition.ReasonMigratable)
	conditions.SetCondition(cbMigratable, &vm.Status.Conditions)
}

func (h *LifeCycleHandler) syncPodStarted(vm *virtv2.VirtualMachine, kvvm *virtv1.VirtualMachine, kvvmi *virtv1.VirtualMachineInstance, pod *corev1.Pod) {
	if vm == nil {
		return
	}

	cb := conditions.NewConditionBuilder(vmcondition.TypePodStarted).Generation(vm.GetGeneration())

	if podutil.IsPodStarted(pod) {
		cb.Status(metav1.ConditionTrue).Reason(vmcondition.ReasonPodStarted)
		conditions.SetCondition(cb, &vm.Status.Conditions)
		return
	}

	// Try to extract error from pod.
	if pod != nil && pod.Status.Message != "" {
		cb.Status(metav1.ConditionFalse).
			Reason(vmcondition.ReasonPodNotStarted).
			Message(fmt.Sprintf("%s: %s", pod.Status.Reason, pod.Status.Message))
		conditions.SetCondition(cb, &vm.Status.Conditions)
		return
	}

	if kvvm != nil {
		// Try to extract error from kvvm PodScheduled condition.
		cond := service.GetKVVMCondition(string(corev1.PodScheduled), kvvm.Status.Conditions)
		if cond != nil && cond.Status == corev1.ConditionFalse && cond.Message != "" {
			cb.Status(metav1.ConditionFalse).
				Reason(vmcondition.ReasonPodNotStarted).
				Message(fmt.Sprintf("%s: %s", cond.Reason, cond.Message))
			conditions.SetCondition(cb, &vm.Status.Conditions)
			return
		}

		// Try to extract error from kvvm Synchronized condition.
		if isPodStartedError(kvvm) {
			msg := fmt.Sprintf("Failed to start pod: %s", kvvm.Status.PrintableStatus)
			if kvvmi != nil {
				msg = fmt.Sprintf("%s, %s", msg, kvvmi.Status.Phase)
			}
			synchronized := service.GetKVVMCondition(string(virtv1.VirtualMachineInstanceSynchronized), kvvm.Status.Conditions)
			if synchronized != nil && synchronized.Status == corev1.ConditionFalse && synchronized.Message != "" {
				msg = fmt.Sprintf("%s; %s: %s", msg, synchronized.Reason, synchronized.Message)
			}
			cb.Status(metav1.ConditionFalse).
				Reason(vmcondition.ReasonPodNotStarted).
				Message(msg)
			conditions.SetCondition(cb, &vm.Status.Conditions)
			return
		}
	}

	cb.Status(metav1.ConditionFalse).
		Reason(vmcondition.ReasonPodNotFound).
		Message("Pod of the virtual machine was not found")
	conditions.SetCondition(cb, &vm.Status.Conditions)
}

func (h *LifeCycleHandler) syncRunning(vm *virtv2.VirtualMachine, kvvm *virtv1.VirtualMachine, kvvmi *virtv1.VirtualMachineInstance, log *slog.Logger) {
	if vm == nil {
		return
	}

	cb := conditions.NewConditionBuilder(vmcondition.TypeRunning).Generation(vm.GetGeneration())

	if kvvm != nil {
		podScheduled := service.GetKVVMCondition(string(corev1.PodScheduled), kvvm.Status.Conditions)
		if podScheduled != nil && podScheduled.Status == corev1.ConditionFalse {
			vm.Status.Phase = virtv2.MachinePending
			return
		}

		if isInternalVirtualMachineError(kvvm.Status.PrintableStatus) {
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

			cb.
				Status(metav1.ConditionFalse).
				Reason(vmcondition.ReasonInternalVirtualMachineError).
				Message(msg)
			conditions.SetCondition(cb, &vm.Status.Conditions)
			return
		}
	}

	if kvvmi != nil && vm.Status.Phase == virtv2.MachineRunning {
		vm.Status.Versions.Libvirt = kvvmi.Annotations[annotations.AnnLibvirtVersion]
		vm.Status.Versions.Qemu = kvvmi.Annotations[annotations.AnnQemuVersion]
	}

	if kvvmi != nil {
		vm.Status.Node = kvvmi.Status.NodeName

		if vm.Status.Phase == virtv2.MachineRunning {
			cb.Reason(vmcondition.ReasonVmIsRunning).Status(metav1.ConditionTrue)
			conditions.SetCondition(cb, &vm.Status.Conditions)
			return
		}
		for _, c := range kvvmi.Status.Conditions {
			if c.Type == virtv1.VirtualMachineInstanceReady {
				cb.Status(conditionStatus(string(c.Status))).
					Reason(getKVMIReadyReason(c.Reason)).
					Message(c.Message)
				conditions.SetCondition(cb, &vm.Status.Conditions)
				return
			}
		}
	} else {
		vm.Status.Node = ""
	}
	cb.Reason(vmcondition.ReasonVmIsNotRunning).Status(metav1.ConditionFalse)
	conditions.SetCondition(cb, &vm.Status.Conditions)
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
