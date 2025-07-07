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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmopcondition"
)

type Operation interface {
	Do(ctx context.Context) error
	Cancel(ctx context.Context) (bool, error)
	IsApplicableForVMPhase(phase virtv2.MachinePhase) bool
	IsApplicableForRunPolicy(runPolicy virtv2.RunPolicy) bool
	GetInProgressReason(ctx context.Context) (vmopcondition.ReasonCompleted, error)
	IsFinalState() bool
	IsComplete(ctx context.Context) (bool, string, error)
}

func NewOperationService(client client.Client, vmop *virtv2.VirtualMachineOperation) (Operation, error) {
	switch vmop.Spec.Type {
	case virtv2.VMOPTypeStart:
		return NewStartOperation(client, vmop), nil
	case virtv2.VMOPTypeStop:
		return NewStopOperation(client, vmop), nil
	case virtv2.VMOPTypeRestart:
		return NewRestartOperation(client, vmop), nil
	case virtv2.VMOPTypeEvict, virtv2.VMOPTypeMigrate:
		return NewMigrateOperation(client, vmop), nil
	default:
		return nil, fmt.Errorf("unknown virtual machine operation type: %v", vmop.Spec.Type)
	}
}

func isFinalState(vmop *virtv2.VirtualMachineOperation) bool {
	return vmop != nil && (vmop.Status.Phase == virtv2.VMOPPhaseCompleted ||
		vmop.Status.Phase == virtv2.VMOPPhaseFailed ||
		vmop.Status.Phase == virtv2.VMOPPhaseTerminating)
}

func isAfterSignalSentOrCreation(timestamp time.Time, vmop *virtv2.VirtualMachineOperation) bool {
	// Use vmop creation time or time from SignalSent condition.
	signalSentTime := vmop.GetCreationTimestamp().Time
	signalSendCond, found := conditions.GetCondition(vmopcondition.SignalSentType, vmop.Status.Conditions)
	if found && signalSendCond.Status == metav1.ConditionTrue && signalSendCond.LastTransitionTime.After(signalSentTime) {
		signalSentTime = signalSendCond.LastTransitionTime.Time
	}
	return timestamp.After(signalSentTime)
}

func virtualMachineKeyByVmop(vmop *virtv2.VirtualMachineOperation) types.NamespacedName {
	return types.NamespacedName{
		Name:      vmop.Spec.VirtualMachine,
		Namespace: vmop.Namespace,
	}
}
