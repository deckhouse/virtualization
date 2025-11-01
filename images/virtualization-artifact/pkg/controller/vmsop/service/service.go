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

package service

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	commonvmsop "github.com/deckhouse/virtualization-controller/pkg/common/vmsop"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmopcondition"
)

type BaseVMSOPService struct {
	client   client.Client
	recorder eventrecord.EventRecorderLogger
}

func NewBaseVMSOPService(client client.Client, recorder eventrecord.EventRecorderLogger) *BaseVMSOPService {
	return &BaseVMSOPService{
		client:   client,
		recorder: recorder,
	}
}

func (s *BaseVMSOPService) ShouldExecuteOrSetFailedPhase(ctx context.Context, vmop *v1alpha2.VirtualMachineSnapshotOperation) (bool, error) {
	should, err := s.ShouldExecute(ctx, vmop)
	if err != nil {
		return false, err
	}
	if should {
		return true, nil
	}

	vmop.Status.Phase = v1alpha2.VMSOPPhaseFailed
	conditions.SetCondition(
		conditions.NewConditionBuilder(vmopcondition.TypeCompleted).
			Generation(vmop.GetGeneration()).
			Reason(vmopcondition.ReasonNotReadyToBeExecuted).
			Message("VMSOP cannot be executed now. Previously created operation should finish first.").
			Status(metav1.ConditionFalse),
		&vmop.Status.Conditions)
	return false, nil
}

func (s *BaseVMSOPService) ShouldExecute(ctx context.Context, vmop *v1alpha2.VirtualMachineSnapshotOperation) (bool, error) {
	var vmopList v1alpha2.VirtualMachineSnapshotOperationList
	err := s.client.List(ctx, &vmopList, client.InNamespace(vmop.GetNamespace()))
	if err != nil {
		return false, err
	}

	for _, other := range vmopList.Items {
		if other.Spec.VirtualMachineSnapshot != vmop.Spec.VirtualMachineSnapshot {
			continue
		}
		if commonvmsop.IsFinished(&other) {
			continue
		}
		if other.GetUID() == vmop.GetUID() {
			continue
		}
		if other.CreationTimestamp.Before(ptr.To(vmop.CreationTimestamp)) {
			return false, nil
		}
	}

	return true, nil
}

func (s *BaseVMSOPService) Init(vmop *v1alpha2.VirtualMachineSnapshotOperation) {
	if vmop.Status.Phase == "" {
		s.recorder.Event(vmop, corev1.EventTypeNormal, v1alpha2.ReasonVMSOPStarted, "VirtualMachineSnapshotOperation started")
		vmop.Status.Phase = v1alpha2.VMSOPPhasePending
		// Add all conditions in unknown state.
		conditions.SetCondition(
			conditions.NewConditionBuilder(vmopcondition.TypeCompleted).
				Generation(vmop.GetGeneration()).
				Reason(conditions.ReasonUnknown).
				Status(metav1.ConditionUnknown).
				Message(""),
			&vmop.Status.Conditions,
		)
	}
}

func (s *BaseVMSOPService) FetchVirtualMachineOrSetFailedPhase(ctx context.Context, vmop *v1alpha2.VirtualMachineSnapshotOperation) (*v1alpha2.VirtualMachine, error) {
	// 1. Get VirtualMachine for validation vmop.
	vm, err := object.FetchObject(ctx, types.NamespacedName{Name: vmop.Spec.VirtualMachineSnapshot, Namespace: vmop.Namespace}, s.client, &v1alpha2.VirtualMachine{})
	if err != nil {
		return nil, fmt.Errorf("get VirtualMachine for VMSOP: %w", err)
	}

	// 2. If VirtualMachine is not found, set vmop to failed
	if vm == nil {
		s.recorder.Event(vmop, corev1.EventTypeWarning, v1alpha2.ReasonErrVMSOPFailed, "VirtualMachine not found")
		vmop.Status.Phase = v1alpha2.VMSOPPhaseFailed
		conditions.SetCondition(
			conditions.NewConditionBuilder(vmopcondition.TypeCompleted).
				Generation(vmop.GetGeneration()).
				Reason(vmopcondition.ReasonVirtualMachineNotFound).
				Status(metav1.ConditionFalse).
				Message("VirtualMachine not found"),
			&vmop.Status.Conditions)
		return nil, nil
	}
	annotations.AddLabel(vmop, annotations.LabelVirtualMachineUID, string(vm.GetUID()))

	return vm, nil
}

type ApplicableChecker interface {
	IsApplicableForVMPhase(phase v1alpha2.MachinePhase) bool
	IsApplicableForRunPolicy(runPolicy v1alpha2.RunPolicy) bool
}

func (s *BaseVMSOPService) IsApplicableOrSetFailedPhase(checker ApplicableChecker, vmop *v1alpha2.VirtualMachineSnapshotOperation, vm *v1alpha2.VirtualMachine) bool {
	// 1. Fail if VirtualMachineSnapshotOperation is not applicable for run policy.
	if !checker.IsApplicableForRunPolicy(vm.Spec.RunPolicy) {
		vmop.Status.Phase = v1alpha2.VMSOPPhaseFailed

		failMsg := fmt.Sprintf("Operation type %s is not applicable for VirtualMachine with runPolicy %s", vmop.Spec.Type, vm.Spec.RunPolicy)
		s.recorder.Event(vmop, corev1.EventTypeWarning, v1alpha2.ReasonErrVMSOPFailed, failMsg)
		conditions.SetCondition(
			conditions.NewConditionBuilder(vmopcondition.TypeCompleted).
				Generation(vmop.GetGeneration()).
				Reason(vmopcondition.ReasonNotApplicableForRunPolicy).
				Status(metav1.ConditionFalse).
				Message(failMsg),
			&vmop.Status.Conditions)
		return false
	}

	// 2. Fail if VirtualMachineSnapshotOperation is not applicable for VM phase.
	if !checker.IsApplicableForVMPhase(vm.Status.Phase) {
		vmop.Status.Phase = v1alpha2.VMSOPPhaseFailed

		failMsg := fmt.Sprintf("Operation type %s is not applicable for VirtualMachine in phase %s", vmop.Spec.Type, vm.Status.Phase)
		s.recorder.Event(vmop, corev1.EventTypeWarning, v1alpha2.ReasonErrVMSOPFailed, failMsg)
		conditions.SetCondition(
			conditions.NewConditionBuilder(vmopcondition.TypeCompleted).
				Generation(vmop.GetGeneration()).
				Reason(vmopcondition.ReasonNotApplicableForVMPhase).
				Status(metav1.ConditionFalse).
				Message(failMsg),
			&vmop.Status.Conditions)
		return false
	}

	return true
}

func IsAfterSignalSentOrCreation(timestamp time.Time, vmop *v1alpha2.VirtualMachineSnapshotOperation) bool {
	// Use vmop creation time or time from SignalSent condition.
	signalSentTime := vmop.GetCreationTimestamp().Time
	signalSendCond, found := conditions.GetCondition(vmopcondition.TypeSignalSent, vmop.Status.Conditions)
	if found && signalSendCond.Status == metav1.ConditionTrue && signalSendCond.LastTransitionTime.After(signalSentTime) {
		signalSentTime = signalSendCond.LastTransitionTime.Time
	}
	return timestamp.After(signalSentTime)
}
