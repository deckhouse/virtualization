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

package service

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/types"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/controller/common"
	"github.com/deckhouse/virtualization-controller/pkg/controller/powerstate"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmopcondition"
)

type VMOperationService struct {
	client client.Client
}

func NewVMOperationService(client client.Client) VMOperationService {
	return VMOperationService{
		client: client,
	}
}

func (s VMOperationService) getKVVM(ctx context.Context, vmNamespace, vmName string) (*virtv1.VirtualMachine, error) {
	kvvm := virtv1.VirtualMachine{}
	key := types.NamespacedName{Name: vmName, Namespace: vmNamespace}
	err := s.client.Get(ctx, key, &kvvm)
	if err != nil {
		return nil, err
	}
	return &kvvm, nil
}

func (s VMOperationService) getKVVMI(ctx context.Context, vmNamespace, vmName string) (*virtv1.VirtualMachineInstance, error) {
	kvvmi := virtv1.VirtualMachineInstance{}
	key := types.NamespacedName{Name: vmName, Namespace: vmNamespace}
	err := s.client.Get(ctx, key, &kvvmi)
	if err != nil {
		return nil, err
	}
	return &kvvmi, nil
}

func (s VMOperationService) Do(ctx context.Context, vmop *virtv2.VirtualMachineOperation) error {
	switch vmop.Spec.Type {
	case virtv2.VMOPTypeStart:
		return s.DoStart(ctx, vmop.GetNamespace(), vmop.Spec.VirtualMachine)
	case virtv2.VMOPTypeStop:
		return s.DoStop(ctx, vmop.GetNamespace(), vmop.Spec.VirtualMachine, vmop.Spec.Force)
	case virtv2.VMOPTypeRestart:
		return s.DoRestart(ctx, vmop.GetNamespace(), vmop.Spec.VirtualMachine, vmop.Spec.Force)
	default:
		return fmt.Errorf("unexpected operation type %q: %w", vmop.Spec.Type, common.ErrUnknownValue)
	}
}

func (s VMOperationService) DoStart(ctx context.Context, vmNamespace, vmName string) error {
	kvvm, err := s.getKVVM(ctx, vmNamespace, vmName)
	if err != nil {
		return fmt.Errorf("get kvvm %q: %w", vmName, err)
	}
	return powerstate.StartVM(ctx, s.client, kvvm)
}

func (s VMOperationService) DoStop(ctx context.Context, vmNamespace, vmName string, force bool) error {
	kvvmi, err := s.getKVVMI(ctx, vmNamespace, vmName)
	if err != nil {
		return fmt.Errorf("get kvvmi %q: %w", vmName, err)
	}
	return powerstate.StopVM(ctx, s.client, kvvmi, force)
}

func (s VMOperationService) DoRestart(ctx context.Context, vmNamespace, vmName string, force bool) error {
	kvvm, err := s.getKVVM(ctx, vmNamespace, vmName)
	if err != nil {
		return fmt.Errorf("get kvvm %q: %w", vmName, err)
	}
	kvvmi, err := s.getKVVMI(ctx, vmNamespace, vmName)
	if err != nil {
		return fmt.Errorf("get kvvmi %q: %w", vmName, err)
	}
	return powerstate.RestartVM(ctx, s.client, kvvm, kvvmi, force)
}

func (s VMOperationService) IsAllowedForVM(vmop *virtv2.VirtualMachineOperation, vm *virtv2.VirtualMachine) bool {
	if vm == nil {
		return false
	}
	return s.IsApplicableForRunPolicy(vmop, vm.Spec.RunPolicy) && s.IsApplicableForVMPhase(vmop, vm.Status.Phase)
}

func (s VMOperationService) IsApplicableForRunPolicy(vmop *virtv2.VirtualMachineOperation, runPolicy virtv2.RunPolicy) bool {
	switch runPolicy {
	case virtv2.AlwaysOnPolicy:
		return vmop.Spec.Type == virtv2.VMOPTypeRestart
	case virtv2.AlwaysOffPolicy:
		return false
	case virtv2.ManualPolicy, virtv2.AlwaysOnUnlessStoppedManually:
		return true
	default:
		return false
	}
}

func (s VMOperationService) IsApplicableForVMPhase(vmop *virtv2.VirtualMachineOperation, phase virtv2.MachinePhase) bool {
	if phase == virtv2.MachineTerminating ||
		phase == virtv2.MachinePending ||
		phase == virtv2.MachineMigrating {
		return false
	}
	switch vmop.Spec.Type {
	case virtv2.VMOPTypeStart:
		return phase == virtv2.MachineStopped || phase == virtv2.MachineStopping
	case virtv2.VMOPTypeStop, virtv2.VMOPTypeRestart:
		return phase == virtv2.MachineRunning ||
			phase == virtv2.MachineDegraded ||
			phase == virtv2.MachineStarting ||
			phase == virtv2.MachinePause
	default:
		return false
	}
}

// OtherVMOPIsInProgress check if there is at least one VMOP for the same VM in progress phase.
func (s VMOperationService) OtherVMOPIsInProgress(ctx context.Context, vmop *virtv2.VirtualMachineOperation) (bool, error) {
	var vmopList virtv2.VirtualMachineOperationList
	err := s.client.List(ctx, &vmopList, client.InNamespace(vmop.GetNamespace()))
	if err != nil {
		return false, err
	}

	for _, other := range vmopList.Items {
		// Ignore ourself.
		if other.GetName() == vmop.GetName() {
			continue
		}
		// Ignore VMOPs for different VMs.
		if other.Spec.VirtualMachine != vmop.Spec.VirtualMachine {
			continue
		}
		// Return true if other VMOP is in progress.
		if vmop.Status.Phase == virtv2.VMOPPhaseInProgress {
			return true, nil
		}
	}
	return false, nil
}

func (s VMOperationService) InProgressReasonForType(vmop *virtv2.VirtualMachineOperation) vmopcondition.Reason {
	if vmop == nil || vmop.Spec.Type == "" {
		return vmopcondition.ReasonUnknown
	}
	switch vmop.Spec.Type {
	case virtv2.VMOPTypeStart:
		return vmopcondition.ReasonStartInProgress
	case virtv2.VMOPTypeStop:
		return vmopcondition.ReasonStopInProgress
	case virtv2.VMOPTypeRestart:
		return vmopcondition.ReasonRestartInProgress
	}
	return vmopcondition.ReasonUnknown
}

func (s VMOperationService) IsComplete(vmop *virtv2.VirtualMachineOperation, vm *virtv2.VirtualMachine) bool {
	if vmop == nil || vm == nil {
		return false
	}

	vmopType := vmop.Spec.Type
	vmPhase := vm.Status.Phase

	switch vmopType {
	case virtv2.VMOPTypeRestart, virtv2.VMOPTypeStart:
		return vmPhase == virtv2.MachineRunning
	case virtv2.VMOPTypeStop:
		return vmPhase == virtv2.MachineStopped
	default:
		return false
	}
}

func (s VMOperationService) IsFinalState(vmop *virtv2.VirtualMachineOperation) bool {
	if vmop == nil {
		return false
	}

	return vmop.Status.Phase == virtv2.VMOPPhaseCompleted ||
		vmop.Status.Phase == virtv2.VMOPPhaseFailed ||
		vmop.Status.Phase == virtv2.VMOPPhaseTerminating
}
