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
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/common"
	kvvmutil "github.com/deckhouse/virtualization-controller/pkg/common/kvvm"
	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
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
	return object.FetchObject(ctx, types.NamespacedName{Namespace: vmNamespace, Name: vmName}, s.client, &virtv1.VirtualMachine{})
}

func (s VMOperationService) getKVVMI(ctx context.Context, vmNamespace, vmName string) (*virtv1.VirtualMachineInstance, error) {
	return object.FetchObject(ctx, types.NamespacedName{Namespace: vmNamespace, Name: vmName}, s.client, &virtv1.VirtualMachineInstance{})
}

func (s VMOperationService) Do(ctx context.Context, vmop *virtv2.VirtualMachineOperation) error {
	switch vmop.Spec.Type {
	case virtv2.VMOPTypeStart:
		return s.DoStart(ctx, vmop.GetNamespace(), vmop.Spec.VirtualMachine)
	case virtv2.VMOPTypeStop:
		return s.DoStop(ctx, vmop.GetNamespace(), vmop.Spec.VirtualMachine, vmop.Spec.Force)
	case virtv2.VMOPTypeRestart:
		return s.DoRestart(ctx, vmop.GetNamespace(), vmop.Spec.VirtualMachine, vmop.Spec.Force)
	case virtv2.VMOPTypeEvict, virtv2.VMOPTypeMigrate:
		return s.DoEvict(ctx, vmop)
	default:
		return fmt.Errorf("unexpected operation type %q: %w", vmop.Spec.Type, common.ErrUnknownValue)
	}
}

func (s VMOperationService) Cancel(ctx context.Context, vmop *virtv2.VirtualMachineOperation) error {
	switch vmop.Spec.Type {
	case virtv2.VMOPTypeStart, virtv2.VMOPTypeStop, virtv2.VMOPTypeRestart:
		return fmt.Errorf("unsupported operation type %q", vmop.Spec.Type)
	case virtv2.VMOPTypeEvict, virtv2.VMOPTypeMigrate:
		return s.deleteMigration(ctx, vmop)
	default:
		return fmt.Errorf("unexpected operation type %q: %w", vmop.Spec.Type, common.ErrUnknownValue)
	}
}

func (s VMOperationService) DoStart(ctx context.Context, vmNamespace, vmName string) error {
	kvvm, err := s.getKVVM(ctx, vmNamespace, vmName)
	if err != nil {
		return fmt.Errorf("get kvvm %q: %w", vmName, err)
	}
	return kvvmutil.AddStartAnnotation(ctx, s.client, kvvm)
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
	return kvvmutil.AddRestartAnnotation(ctx, s.client, kvvm)
}

func (s VMOperationService) DoEvict(ctx context.Context, vmop *virtv2.VirtualMachineOperation) error {
	if vmop == nil {
		return fmt.Errorf("vmop cannot be nil")
	}
	return s.createMigration(ctx, vmop)
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
		return vmop.Spec.Type == virtv2.VMOPTypeRestart || vmop.Spec.Type == virtv2.VMOPTypeEvict || vmop.Spec.Type == virtv2.VMOPTypeMigrate
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
	case virtv2.VMOPTypeEvict, virtv2.VMOPTypeMigrate:
		return phase == virtv2.MachineRunning
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
		if other.Status.Phase == virtv2.VMOPPhaseInProgress {
			return true, nil
		}
	}
	return false, nil
}

func (s VMOperationService) OtherMigrationsAreInProgress(ctx context.Context, vmop *virtv2.VirtualMachineOperation) (bool, error) {
	if !vmopIsMigrate(vmop) {
		return false, nil
	}
	migList := &virtv1.VirtualMachineInstanceMigrationList{}
	err := s.client.List(ctx, migList, client.InNamespace(vmop.GetNamespace()))
	if err != nil {
		return false, err
	}
	for _, mig := range migList.Items {
		if !mig.IsFinal() && mig.Spec.VMIName == vmop.Spec.VirtualMachine {
			return true, nil
		}
	}
	return false, nil
}

func (s VMOperationService) InProgressReasonForType(vmop *virtv2.VirtualMachineOperation) vmopcondition.ReasonCompleted {
	if vmop == nil || vmop.Spec.Type == "" {
		return vmopcondition.ReasonCompleted(conditions.ReasonUnknown)
	}
	switch vmop.Spec.Type {
	case virtv2.VMOPTypeStart:
		return vmopcondition.ReasonStartInProgress
	case virtv2.VMOPTypeStop:
		return vmopcondition.ReasonStopInProgress
	case virtv2.VMOPTypeRestart:
		return vmopcondition.ReasonRestartInProgress
	case virtv2.VMOPTypeEvict, virtv2.VMOPTypeMigrate:
		return vmopcondition.ReasonMigrationInProgress
	}
	return vmopcondition.ReasonCompleted(conditions.ReasonUnknown)
}

func (s VMOperationService) IsComplete(ctx context.Context, vmop *virtv2.VirtualMachineOperation, vm *virtv2.VirtualMachine) (bool, string, error) {
	if vmop == nil || vm == nil {
		return false, "", nil
	}

	vmopType := vmop.Spec.Type
	vmPhase := vm.Status.Phase

	switch vmopType {
	case virtv2.VMOPTypeStart:
		return vmPhase == virtv2.MachineRunning, "", nil
	case virtv2.VMOPTypeStop:
		return vmPhase == virtv2.MachineStopped, "", nil
	case virtv2.VMOPTypeRestart:
		kvvmi, err := s.getKVVMI(ctx, vmop.GetNamespace(), vmop.Spec.VirtualMachine)
		if err != nil {
			return false, "", err
		}

		return kvvmi != nil && vmPhase == virtv2.MachineRunning &&
			s.isAfterSignalSentOrCreation(kvvmi.GetCreationTimestamp().Time, vmop), "", nil
	case virtv2.VMOPTypeEvict, virtv2.VMOPTypeMigrate:
		mig, err := s.GetMigration(ctx, vmop)
		if err != nil {
			return false, "", err
		}
		if mig != nil {
			switch mig.Status.Phase {
			case virtv1.MigrationSucceeded:
				return true, "", nil
			case virtv1.MigrationFailed:
				return true, fmt.Sprintf("Migration failed: %s", mig.Status.MigrationState.FailureReason), nil
			default:
				return false, "", nil
			}
		}
		kvvmi, err := s.getKVVMI(ctx, vmop.GetNamespace(), vmop.Spec.VirtualMachine)
		if err != nil {
			return false, "", err
		}
		if kvvmi == nil {
			return false, "Migration failed because the virtual machine is currently not running.", nil
		}
		migrationState := kvvmi.Status.MigrationState
		if migrationState != nil && migrationState.EndTimestamp != nil && s.isAfterSignalSentOrCreation(migrationState.EndTimestamp.Time, vmop) {
			reason := ""
			if migrationState.Failed {
				reason = fmt.Sprintf("Migration failed: %s", migrationState.FailureReason)
			}
			return true, reason, nil
		}
		return true, "Migration was canceled", nil

	default:
		return false, "", nil
	}
}

func (s VMOperationService) isAfterSignalSentOrCreation(timestamp time.Time, vmop *virtv2.VirtualMachineOperation) bool {
	// Use vmop creation time or time from SignalSent condition.
	signalSentTime := vmop.GetCreationTimestamp().Time
	signalSendCond, found := conditions.GetCondition(vmopcondition.SignalSentType, vmop.Status.Conditions)
	if found && signalSendCond.Status == metav1.ConditionTrue && signalSendCond.LastTransitionTime.After(signalSentTime) {
		signalSentTime = signalSendCond.LastTransitionTime.Time
	}
	return timestamp.After(signalSentTime)
}

func (s VMOperationService) IsFinalState(vmop *virtv2.VirtualMachineOperation) bool {
	return vmop != nil && (vmop.Status.Phase == virtv2.VMOPPhaseCompleted ||
		vmop.Status.Phase == virtv2.VMOPPhaseFailed ||
		vmop.Status.Phase == virtv2.VMOPPhaseTerminating)
}

func (s VMOperationService) GetMigration(ctx context.Context, vmop *virtv2.VirtualMachineOperation) (*virtv1.VirtualMachineInstanceMigration, error) {
	return object.FetchObject(ctx, types.NamespacedName{
		Name:      migrationName(vmop),
		Namespace: vmop.GetNamespace(),
	}, s.client, &virtv1.VirtualMachineInstanceMigration{})
}

func (s VMOperationService) deleteMigration(ctx context.Context, vmop *virtv2.VirtualMachineOperation) error {
	mig, err := s.GetMigration(ctx, vmop)
	if err != nil {
		return err
	}
	if mig == nil {
		return nil
	}
	return s.client.Delete(ctx, mig)
}

func (s VMOperationService) createMigration(ctx context.Context, vmop *virtv2.VirtualMachineOperation) error {
	return s.client.Create(ctx, &virtv1.VirtualMachineInstanceMigration{
		TypeMeta: metav1.TypeMeta{
			APIVersion: virtv1.SchemeGroupVersion.String(),
			Kind:       "VirtualMachineInstanceMigration",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: vmop.GetNamespace(),
			Name:      migrationName(vmop),
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion:         virtv2.SchemeGroupVersion.String(),
					Kind:               virtv2.VirtualMachineOperationKind,
					Name:               vmop.GetName(),
					UID:                vmop.GetUID(),
					BlockOwnerDeletion: ptr.To(true),
					Controller:         ptr.To(true),
				},
			},
		},
		Spec: virtv1.VirtualMachineInstanceMigrationSpec{
			VMIName: vmop.Spec.VirtualMachine,
		},
	})
}

const vmopPrefix = "vmop-"

func migrationName(vmop *virtv2.VirtualMachineOperation) string {
	return fmt.Sprintf("%s%s", vmopPrefix, vmop.GetName())
}

func vmopIsMigrate(vmop *virtv2.VirtualMachineOperation) bool {
	return vmop != nil && (vmop.Spec.Type == virtv2.VMOPTypeMigrate || vmop.Spec.Type == virtv2.VMOPTypeEvict)
}
