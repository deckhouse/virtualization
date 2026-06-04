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
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	kvvmutil "github.com/deckhouse/virtualization-controller/pkg/common/kvvm"
	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	commonvmop "github.com/deckhouse/virtualization-controller/pkg/common/vmop"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmop/supersede"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmopcondition"
)

type BaseVMOPService struct {
	client   client.Client
	recorder eventrecord.EventRecorderLogger
}

func NewBaseVMOPService(client client.Client, recorder eventrecord.EventRecorderLogger) *BaseVMOPService {
	return &BaseVMOPService{
		client:   client,
		recorder: recorder,
	}
}

func (s *BaseVMOPService) ShouldExecuteOrSetFailedPhase(ctx context.Context, vmop *v1alpha2.VirtualMachineOperation) (bool, error) {
	return s.ShouldExecuteOrSupersedeOrSetFailedPhase(ctx, vmop)
}

func (s *BaseVMOPService) ShouldExecuteOrSupersedeOrSetFailedPhase(ctx context.Context, vmop *v1alpha2.VirtualMachineOperation) (bool, error) {
	blockers, err := s.findOlderActiveVMOPs(ctx, vmop)
	if err != nil {
		return false, err
	}
	if len(blockers) == 0 {
		return true, nil
	}

	for i := range blockers {
		oldVMOP := &blockers[i]
		if !supersede.CanSupersede(oldVMOP, vmop) {
			s.setNotReadyToBeExecuted(vmop)
			return false, nil
		}
	}

	for i := range blockers {
		oldVMOP := &blockers[i]
		if err := s.cleanupSupersededOperation(ctx, oldVMOP); err != nil {
			return false, err
		}
		if err := s.markSuperseded(ctx, oldVMOP, vmop); err != nil {
			return false, err
		}
	}

	return true, nil
}

func (s *BaseVMOPService) ShouldExecute(ctx context.Context, vmop *v1alpha2.VirtualMachineOperation) (bool, error) {
	blockers, err := s.findOlderActiveVMOPs(ctx, vmop)
	if err != nil {
		return false, err
	}
	return len(blockers) == 0, nil
}

func (s *BaseVMOPService) findOlderActiveVMOPs(ctx context.Context, vmop *v1alpha2.VirtualMachineOperation) ([]v1alpha2.VirtualMachineOperation, error) {
	var vmopList v1alpha2.VirtualMachineOperationList
	if err := s.client.List(ctx, &vmopList, client.InNamespace(vmop.GetNamespace())); err != nil {
		return nil, err
	}

	blockers := make([]v1alpha2.VirtualMachineOperation, 0)
	for _, other := range vmopList.Items {
		if other.Spec.VirtualMachine != vmop.Spec.VirtualMachine {
			continue
		}
		if commonvmop.IsFinished(&other) {
			continue
		}
		if other.GetUID() == vmop.GetUID() {
			continue
		}
		if isOlderVMOP(&other, vmop) {
			blockers = append(blockers, other)
		}
	}

	sort.SliceStable(blockers, func(i, j int) bool {
		return isOlderVMOP(&blockers[i], &blockers[j])
	})

	return blockers, nil
}

func isOlderVMOP(a, b *v1alpha2.VirtualMachineOperation) bool {
	if !a.CreationTimestamp.Equal(&b.CreationTimestamp) {
		return a.CreationTimestamp.Before(&b.CreationTimestamp)
	}
	return a.GetName() < b.GetName()
}

func (s *BaseVMOPService) setNotReadyToBeExecuted(vmop *v1alpha2.VirtualMachineOperation) {
	vmop.Status.Phase = v1alpha2.VMOPPhaseFailed
	conditions.SetCondition(
		conditions.NewConditionBuilder(vmopcondition.TypeCompleted).
			Generation(vmop.GetGeneration()).
			Reason(vmopcondition.ReasonNotReadyToBeExecuted).
			Message("VMOP cannot be executed now. Previously created operation should finish first.").
			Status(metav1.ConditionFalse),
		&vmop.Status.Conditions)
}

func (s *BaseVMOPService) cleanupSupersededOperation(ctx context.Context, oldVMOP *v1alpha2.VirtualMachineOperation) error {
	key := types.NamespacedName{Name: oldVMOP.Spec.VirtualMachine, Namespace: oldVMOP.Namespace}

	switch oldVMOP.Spec.Type {
	case v1alpha2.VMOPTypeStart:
		kvvm := &virtv1.VirtualMachine{}
		if err := s.client.Get(ctx, key, kvvm); err != nil {
			return client.IgnoreNotFound(err)
		}
		return kvvmutil.RemoveStartAnnotation(ctx, s.client, kvvm)
	case v1alpha2.VMOPTypeRestart:
		kvvm := &virtv1.VirtualMachine{}
		if err := s.client.Get(ctx, key, kvvm); err != nil {
			return client.IgnoreNotFound(err)
		}
		if err := kvvmutil.RemoveRestartAnnotation(ctx, s.client, kvvm); err != nil {
			return err
		}
		return kvvmutil.RemoveStartAnnotation(ctx, s.client, kvvm)
	case v1alpha2.VMOPTypeMigrate, v1alpha2.VMOPTypeEvict:
		mig := &virtv1.VirtualMachineInstanceMigration{}
		if err := s.client.Get(ctx, types.NamespacedName{Name: fmt.Sprintf("vmop-%s", oldVMOP.Name), Namespace: oldVMOP.Namespace}, mig); err != nil {
			return client.IgnoreNotFound(err)
		}
		return client.IgnoreNotFound(s.client.Delete(ctx, mig))
	case v1alpha2.VMOPTypeStop:
		return nil
	default:
		return nil
	}
}

func (s *BaseVMOPService) markSuperseded(ctx context.Context, oldVMOP, newVMOP *v1alpha2.VirtualMachineOperation) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		current := &v1alpha2.VirtualMachineOperation{}
		if err := s.client.Get(ctx, types.NamespacedName{Name: oldVMOP.Name, Namespace: oldVMOP.Namespace}, current); err != nil {
			return client.IgnoreNotFound(err)
		}
		if commonvmop.IsFinished(current) {
			return nil
		}

		base := current.DeepCopy()
		current.Status.Phase = v1alpha2.VMOPPhaseCompleted
		conditions.SetCondition(
			conditions.NewConditionBuilder(vmopcondition.TypeCompleted).
				Generation(current.GetGeneration()).
				Reason(vmopcondition.ReasonSuperseded).
				Message(fmt.Sprintf("Superseded by %s with type %s", newVMOP.Name, newVMOP.Spec.Type)).
				Status(metav1.ConditionTrue),
			&current.Status.Conditions)

		return s.client.Status().Patch(ctx, current, client.MergeFrom(base))
	})
}

func (s *BaseVMOPService) Init(vmop *v1alpha2.VirtualMachineOperation) {
	if vmop.Status.Phase == "" {
		s.recorder.Event(vmop, corev1.EventTypeNormal, v1alpha2.ReasonVMOPStarted, "VirtualMachineOperation started")
		vmop.Status.Phase = v1alpha2.VMOPPhasePending
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

func (s *BaseVMOPService) FetchVirtualMachineOrSetFailedPhase(ctx context.Context, vmop *v1alpha2.VirtualMachineOperation) (*v1alpha2.VirtualMachine, error) {
	// 1. Get VirtualMachine for validation vmop.
	vm, err := object.FetchObject(ctx, types.NamespacedName{Name: vmop.Spec.VirtualMachine, Namespace: vmop.Namespace}, s.client, &v1alpha2.VirtualMachine{})
	if err != nil {
		return nil, fmt.Errorf("get VirtualMachine for VMOP: %w", err)
	}

	// 2. If VirtualMachine is not found, set vmop to failed
	if vm == nil {
		s.recorder.Event(vmop, corev1.EventTypeWarning, v1alpha2.ReasonErrVMOPFailed, "VirtualMachine not found")
		vmop.Status.Phase = v1alpha2.VMOPPhaseFailed
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

func (s *BaseVMOPService) IsApplicableOrSetFailedPhase(checker ApplicableChecker, vmop *v1alpha2.VirtualMachineOperation, vm *v1alpha2.VirtualMachine) bool {
	// 1. Fail if VirtualMachineOperation is not applicable for run policy.
	if !checker.IsApplicableForRunPolicy(vm.Spec.RunPolicy) {
		vmop.Status.Phase = v1alpha2.VMOPPhaseFailed

		failMsg := fmt.Sprintf("Operation type %s is not applicable for VirtualMachine with runPolicy %s", vmop.Spec.Type, vm.Spec.RunPolicy)
		s.recorder.Event(vmop, corev1.EventTypeWarning, v1alpha2.ReasonErrVMOPFailed, failMsg)
		conditions.SetCondition(
			conditions.NewConditionBuilder(vmopcondition.TypeCompleted).
				Generation(vmop.GetGeneration()).
				Reason(vmopcondition.ReasonNotApplicableForRunPolicy).
				Status(metav1.ConditionFalse).
				Message(failMsg),
			&vmop.Status.Conditions)
		return false
	}

	// 2. Fail if VirtualMachineOperation is not applicable for VM phase.
	if !checker.IsApplicableForVMPhase(vm.Status.Phase) {
		vmop.Status.Phase = v1alpha2.VMOPPhaseFailed

		failMsg := fmt.Sprintf("Operation type %s is not applicable for VirtualMachine in phase %s", vmop.Spec.Type, vm.Status.Phase)
		s.recorder.Event(vmop, corev1.EventTypeWarning, v1alpha2.ReasonErrVMOPFailed, failMsg)
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

func IsAfterSignalSentOrCreation(timestamp time.Time, vmop *v1alpha2.VirtualMachineOperation) bool {
	// Use vmop creation time or time from SignalSent condition.
	signalSentTime := vmop.GetCreationTimestamp().Time
	signalSendCond, found := conditions.GetCondition(vmopcondition.TypeSignalSent, vmop.Status.Conditions)
	if found && signalSendCond.Status == metav1.ConditionTrue && signalSendCond.LastTransitionTime.After(signalSentTime) {
		signalSentTime = signalSendCond.LastTransitionTime.Time
	}
	return timestamp.After(signalSentTime)
}
