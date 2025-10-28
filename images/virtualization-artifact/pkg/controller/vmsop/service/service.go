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

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	commonvmsop "github.com/deckhouse/virtualization-controller/pkg/common/vmsop"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmsopcondition"
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

func (s *BaseVMSOPService) ShouldExecuteOrSetFailedPhase(ctx context.Context, vmsop *v1alpha2.VirtualMachineSnapshotOperation) (bool, error) {
	should, err := s.ShouldExecute(ctx, vmsop)
	if err != nil {
		return false, err
	}
	if should {
		return true, nil
	}

	vmsop.Status.Phase = v1alpha2.VMSOPPhaseFailed
	conditions.SetCondition(
		conditions.NewConditionBuilder(vmsopcondition.TypeCompleted).
			Generation(vmsop.GetGeneration()).
			Reason(vmsopcondition.ReasonNotReadyToBeExecuted).
			Message("VMSOP cannot be executed now. Previously created operation should finish first.").
			Status(metav1.ConditionFalse),
		&vmsop.Status.Conditions)
	return false, nil
}

func (s *BaseVMSOPService) ShouldExecute(ctx context.Context, vmsop *v1alpha2.VirtualMachineSnapshotOperation) (bool, error) {
	var vmsopList v1alpha2.VirtualMachineSnapshotOperationList
	err := s.client.List(ctx, &vmsopList, client.InNamespace(vmsop.GetNamespace()))
	if err != nil {
		return false, err
	}

	for _, other := range vmsopList.Items {
		if other.Spec.VirtualMachineSnapshotName != vmsop.Spec.VirtualMachineSnapshotName {
			continue
		}
		if commonvmsop.IsFinished(&other) {
			continue
		}
		if other.GetUID() == vmsop.GetUID() {
			continue
		}
		if other.CreationTimestamp.Before(ptr.To(vmsop.CreationTimestamp)) {
			return false, nil
		}
	}

	return true, nil
}

func (s *BaseVMSOPService) Init(vmsop *v1alpha2.VirtualMachineSnapshotOperation) {
	if vmsop.Status.Phase == "" {
		s.recorder.Event(vmsop, corev1.EventTypeNormal, v1alpha2.ReasonVMSOPStarted, "VirtualMachineSnapshotOperation started")
		vmsop.Status.Phase = v1alpha2.VMSOPPhasePending
		// Add all conditions in unknown state.
		conditions.SetCondition(
			conditions.NewConditionBuilder(vmsopcondition.TypeCompleted).
				Generation(vmsop.GetGeneration()).
				Reason(conditions.ReasonUnknown).
				Status(metav1.ConditionUnknown).
				Message(""),
			&vmsop.Status.Conditions,
		)
	}
}

func (s *BaseVMSOPService) FetchVirtualMachineSnapshotOrSetFailedPhase(ctx context.Context, vmsop *v1alpha2.VirtualMachineSnapshotOperation) (*v1alpha2.VirtualMachineSnapshot, error) {
	// 1. Get VirtualMachineSnapshot for validation vmsop.
	vms, err := object.FetchObject(ctx, types.NamespacedName{Name: vmsop.Spec.VirtualMachineSnapshotName, Namespace: vmsop.Namespace}, s.client, &v1alpha2.VirtualMachineSnapshot{})
	if err != nil {
		return nil, fmt.Errorf("get VirtualMachine for VMSOP: %w", err)
	}

	// 2. If VirtualMachine is not found, set vmsop to failed
	if vms == nil {
		s.recorder.Event(vmsop, corev1.EventTypeWarning, v1alpha2.ReasonErrVMSOPFailed, "VirtualMachineSnapshot not found")
		vmsop.Status.Phase = v1alpha2.VMSOPPhaseFailed
		conditions.SetCondition(
			conditions.NewConditionBuilder(vmsopcondition.TypeCompleted).
				Generation(vmsop.GetGeneration()).
				Reason(vmsopcondition.ReasonVirtualMachineSnapshotNotFound).
				Status(metav1.ConditionFalse).
				Message("VirtualMachine not found"),
			&vmsop.Status.Conditions)
		return nil, nil
	}

	return vms, nil
}

type ApplicableChecker interface {
	IsApplicableForVMSPhase(phase v1alpha2.VirtualMachineSnapshotPhase) bool
}

func (s *BaseVMSOPService) IsApplicableOrSetFailedPhase(checker ApplicableChecker, vmsop *v1alpha2.VirtualMachineSnapshotOperation, vms *v1alpha2.VirtualMachineSnapshot) bool {
	if !checker.IsApplicableForVMSPhase(vms.Status.Phase) {
		vmsop.Status.Phase = v1alpha2.VMSOPPhaseFailed

		failMsg := fmt.Sprintf("Operation type %s is not applicable for VirtualMachineSnapshot in phase %s", vmsop.Spec.Type, vms.Status.Phase)
		s.recorder.Event(vmsop, corev1.EventTypeWarning, v1alpha2.ReasonErrVMSOPFailed, failMsg)
		conditions.SetCondition(
			conditions.NewConditionBuilder(vmsopcondition.TypeCompleted).
				Generation(vmsop.GetGeneration()).
				Reason(vmsopcondition.ReasonNotApplicableForVMSPhase).
				Status(metav1.ConditionFalse).
				Message(failMsg),
			&vmsop.Status.Conditions)
		return false
	}

	return true
}
