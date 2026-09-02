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

package internal

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	commonvmop "github.com/deckhouse/virtualization-controller/pkg/common/vmop"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/indexer"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vm/internal/state"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmopcondition"
)

const nameOperationHandler = "OperationHandler"

// OperationHandler reports what is being done to the virtual machine right now, and what the last
// thing done to it was.
//
// It is an aggregate. Every operation it reports is described by a resource of its own —
// VirtualMachineOperation, VirtualMachineSnapshot, VirtualMachineBlockDeviceAttachment — and the
// detailed conditions of the machine (Migrating, Snapshotting) stay as they are. This one answers a
// single question, "is anything happening to this machine", so that a consumer does not have to
// know which resource to look at.
//
// The condition has two states and no third one: True while an operation is being performed, False
// when the last operation has failed. Nothing is remembered — it is recalculated from the resources
// that exist at the moment, and a failure is reported for as long as the failed operation is kept.
type OperationHandler struct {
	client client.Client
}

func NewOperationHandler(client client.Client) *OperationHandler {
	return &OperationHandler{client: client}
}

func (h *OperationHandler) Handle(ctx context.Context, s state.VirtualMachineState) (reconcile.Result, error) {
	if s.VirtualMachine().IsEmpty() {
		return reconcile.Result{}, nil
	}

	vm := s.VirtualMachine().Changed()

	if isDeletion(vm) {
		return reconcile.Result{}, nil
	}

	cb, err := h.observe(ctx, s, vm)
	if err != nil {
		return reconcile.Result{}, err
	}

	if cb == nil {
		conditions.RemoveCondition(vmcondition.TypeOperationInProgress, &vm.Status.Conditions)
		return reconcile.Result{}, nil
	}

	conditions.SetCondition(cb.Generation(vm.GetGeneration()), &vm.Status.Conditions)

	return reconcile.Result{}, nil
}

func (h *OperationHandler) Name() string {
	return nameOperationHandler
}

// observe returns the condition to report, or nil when nothing is known about the operations of the
// virtual machine. The order of the checks is the order of importance: a running operation matters
// more than a finished one, and an operation applied to the machine as a whole matters more than an
// attachment of a single block device.
func (h *OperationHandler) observe(ctx context.Context, s state.VirtualMachineState, vm *v1alpha2.VirtualMachine) (*conditions.ConditionBuilder, error) {
	vmops, err := s.VMOPs(ctx)
	if err != nil {
		return nil, fmt.Errorf("list VirtualMachineOperations: %w", err)
	}

	slices.SortFunc(vmops, func(a, b *v1alpha2.VirtualMachineOperation) int {
		return cmp.Compare(a.GetCreationTimestamp().UnixNano(), b.GetCreationTimestamp().UnixNano())
	})

	for _, vmop := range vmops {
		if !commonvmop.IsInProgressOrPending(vmop) {
			continue
		}
		if cb := operationRunning(vmop); cb != nil {
			return cb, nil
		}
	}

	cb, err := h.snapshotRunning(ctx, vm)
	if err != nil || cb != nil {
		return cb, err
	}

	cb, err = h.attachmentRunning(ctx, vm)
	if err != nil || cb != nil {
		return cb, err
	}

	kvvm, err := s.KVVM(ctx)
	if err != nil {
		return nil, fmt.Errorf("get internal virtual machine: %w", err)
	}
	if cb = powerStateRequested(kvvm); cb != nil {
		return cb, nil
	}

	// Only a failure outlives the operation that caused it. An operation that has completed, or
	// that was superseded or is being deleted, leaves nothing behind: a machine nobody touches
	// reports no condition at all, so its presence always means either work in progress or a
	// failure to look at. A later operation that finishes cleanly clears the failure of an
	// earlier one, because only the newest finished operation is reported.
	if last := lastFinished(vmops); last != nil {
		if cb = operationFailed(last); cb != nil {
			return cb, nil
		}
	}

	return nil, nil
}

// lastFinished returns the newest operation that has reached a terminal phase.
func lastFinished(vmops []*v1alpha2.VirtualMachineOperation) *v1alpha2.VirtualMachineOperation {
	for i := len(vmops) - 1; i >= 0; i-- {
		if commonvmop.IsFinished(vmops[i]) {
			return vmops[i]
		}
	}

	return nil
}

// snapshotRunning reports a snapshot of the whole machine. A snapshot of a single disk is not
// reported: it belongs to the disk, and the machine only lends it a frozen filesystem, which the
// FilesystemFrozen condition already describes.
func (h *OperationHandler) snapshotRunning(ctx context.Context, vm *v1alpha2.VirtualMachine) (*conditions.ConditionBuilder, error) {
	var snapshots v1alpha2.VirtualMachineSnapshotList
	err := h.client.List(ctx, &snapshots,
		client.InNamespace(vm.GetNamespace()),
		client.MatchingFields{indexer.IndexFieldVMSnapshotByVM: vm.GetName()},
	)
	if err != nil {
		return nil, fmt.Errorf("list VirtualMachineSnapshots: %w", err)
	}

	oldest := oldestBy(snapshots.Items, func(snapshot *v1alpha2.VirtualMachineSnapshot) bool {
		switch snapshot.Status.Phase {
		case v1alpha2.VirtualMachineSnapshotPhasePending, v1alpha2.VirtualMachineSnapshotPhaseInProgress:
			return snapshot.GetDeletionTimestamp().IsZero()
		default:
			return false
		}
	})
	if oldest == nil {
		return nil, nil
	}

	message := "A snapshot of the virtual machine is being taken"
	if oldest.Status.Phase == v1alpha2.VirtualMachineSnapshotPhasePending {
		message = "The virtual machine is selected for taking a snapshot"
	}

	return conditions.NewConditionBuilder(vmcondition.TypeOperationInProgress).
		Status(metav1.ConditionTrue).
		Reason(vmcondition.ReasonVirtualMachineSnapshotting).
		Message(fmt.Sprintf("%s; VirtualMachineSnapshot: %s.", message, oldest.GetName())), nil
}

// attachmentRunning reports a block device being hot-plugged into the machine or unplugged from it.
func (h *OperationHandler) attachmentRunning(ctx context.Context, vm *v1alpha2.VirtualMachine) (*conditions.ConditionBuilder, error) {
	var vmbdas v1alpha2.VirtualMachineBlockDeviceAttachmentList
	err := h.client.List(ctx, &vmbdas,
		client.InNamespace(vm.GetNamespace()),
		client.MatchingFields{indexer.IndexFieldVMBDAByVM: vm.GetName()},
	)
	if err != nil {
		return nil, fmt.Errorf("list VirtualMachineBlockDeviceAttachments: %w", err)
	}

	detaching := oldestBy(vmbdas.Items, func(vmbda *v1alpha2.VirtualMachineBlockDeviceAttachment) bool {
		return !vmbda.GetDeletionTimestamp().IsZero() ||
			vmbda.Status.Phase == v1alpha2.BlockDeviceAttachmentPhaseTerminating
	})
	if detaching != nil {
		return conditions.NewConditionBuilder(vmcondition.TypeOperationInProgress).
			Status(metav1.ConditionTrue).
			Reason(vmcondition.ReasonBlockDeviceDetaching).
			Message(fmt.Sprintf(
				"The %s %q is being detached from the virtual machine; VirtualMachineBlockDeviceAttachment: %s.",
				detaching.Spec.BlockDeviceRef.Kind, detaching.Spec.BlockDeviceRef.Name, detaching.GetName(),
			)), nil
	}

	attaching := oldestBy(vmbdas.Items, func(vmbda *v1alpha2.VirtualMachineBlockDeviceAttachment) bool {
		switch vmbda.Status.Phase {
		case v1alpha2.BlockDeviceAttachmentPhasePending, v1alpha2.BlockDeviceAttachmentPhaseInProgress:
			return true
		default:
			return false
		}
	})
	if attaching == nil {
		return nil, nil
	}

	return conditions.NewConditionBuilder(vmcondition.TypeOperationInProgress).
		Status(metav1.ConditionTrue).
		Reason(vmcondition.ReasonBlockDeviceAttaching).
		Message(fmt.Sprintf(
			"The %s %q is being attached to the virtual machine; VirtualMachineBlockDeviceAttachment: %s.",
			attaching.Spec.BlockDeviceRef.Kind, attaching.Spec.BlockDeviceRef.Name, attaching.GetName(),
		)), nil
}

// powerStateRequested reports a power state change that no operation has asked for. The controller
// restarts a machine on its own to apply the changes that need a restart when they are approved
// automatically, and nothing but the request on the internal virtual machine describes such a
// restart.
func powerStateRequested(kvvm *virtv1.VirtualMachine) *conditions.ConditionBuilder {
	if kvvm == nil {
		return nil
	}

	var start, stop bool
	for _, request := range kvvm.Status.StateChangeRequests {
		switch request.Action {
		case virtv1.StartRequest:
			start = true
		case virtv1.StopRequest:
			stop = true
		}
	}

	var (
		reason  vmcondition.OperationInProgressReason
		message string
	)

	switch {
	case start && stop:
		reason, message = vmcondition.ReasonVirtualMachineRestarting, "The virtual machine is restarting."
	case start:
		reason, message = vmcondition.ReasonVirtualMachineStarting, "The virtual machine is starting."
	case stop:
		reason, message = vmcondition.ReasonVirtualMachineStopping, "The virtual machine is stopping."
	default:
		return nil
	}

	return conditions.NewConditionBuilder(vmcondition.TypeOperationInProgress).
		Status(metav1.ConditionTrue).
		Reason(reason).
		Message(message)
}

func operationRunning(vmop *v1alpha2.VirtualMachineOperation) *conditions.ConditionBuilder {
	description := describeOperation(vmop)
	if description.reason == "" {
		return nil
	}

	message := description.running
	if detail := operationDetail(vmop); detail != "" {
		message = fmt.Sprintf("%s: %s", message, detail)
	}

	return conditions.NewConditionBuilder(vmcondition.TypeOperationInProgress).
		Status(metav1.ConditionTrue).
		Reason(description.reason).
		Message(fmt.Sprintf("%s; VirtualMachineOperation: %s.", message, vmop.GetName()))
}

// operationFailed reports an operation that has failed. Any other outcome is not reported at all:
// a machine that has been restarted successfully is simply a machine nobody is doing anything to.
func operationFailed(vmop *v1alpha2.VirtualMachineOperation) *conditions.ConditionBuilder {
	if vmop.Status.Phase != v1alpha2.VMOPPhaseFailed {
		return nil
	}

	description := describeOperation(vmop)
	if description.reason == "" {
		return nil
	}

	message := fmt.Sprintf("%s has failed", description.noun)
	if detail := operationDetail(vmop); detail != "" {
		message = fmt.Sprintf("%s: %s", message, detail)
	}

	return conditions.NewConditionBuilder(vmcondition.TypeOperationInProgress).
		Status(metav1.ConditionFalse).
		Reason(vmcondition.ReasonOperationFailed).
		Message(fmt.Sprintf("%s; VirtualMachineOperation: %s.", message, vmop.GetName()))
}

// operationDetail returns what the operation reports about itself: the migration controller keeps
// the description of the current step, or of the failure, in the Completed condition.
func operationDetail(vmop *v1alpha2.VirtualMachineOperation) string {
	completed, _ := conditions.GetCondition(vmopcondition.TypeCompleted, vmop.Status.Conditions)
	return strings.TrimSuffix(strings.TrimSpace(completed.Message), ".")
}

// operationDescription tells what an operation does to the virtual machine.
//
// The type of an operation is not enough on its own: the controllers of the module create evictions
// of their own to update the firmware, to apply a new node placement, to hot-plug CPU and memory,
// and to move the disks to another storage. All of them are evictions, and only an annotation and
// the generated name tell them apart from an eviction of a node being drained. Reporting all of
// them as an eviction would leave the user wondering why a machine nobody touched is leaving its
// node.
type operationDescription struct {
	reason vmcondition.OperationInProgressReason
	// running describes the operation while it is being performed.
	running string
	// noun names the operation when its outcome is reported.
	noun string
}

func describeOperation(vmop *v1alpha2.VirtualMachineOperation) operationDescription {
	switch vmop.Spec.Type {
	case v1alpha2.VMOPTypeStart:
		return operationDescription{
			reason:  vmcondition.ReasonVirtualMachineStarting,
			running: "The virtual machine is starting",
			noun:    "Start of the virtual machine",
		}
	case v1alpha2.VMOPTypeStop:
		return operationDescription{
			reason:  vmcondition.ReasonVirtualMachineStopping,
			running: "The virtual machine is stopping",
			noun:    "Stop of the virtual machine",
		}
	case v1alpha2.VMOPTypeRestart:
		return operationDescription{
			reason:  vmcondition.ReasonVirtualMachineRestarting,
			running: "The virtual machine is restarting",
			noun:    "Restart of the virtual machine",
		}
	case v1alpha2.VMOPTypeMigrate:
		return operationDescription{
			reason:  vmcondition.ReasonVirtualMachineMigrating,
			running: "The virtual machine is being migrated to another node",
			noun:    "Migration of the virtual machine",
		}
	case v1alpha2.VMOPTypeEvict:
		return describeEviction(vmop)
	case v1alpha2.VMOPTypeRestore:
		return operationDescription{
			reason:  vmcondition.ReasonVirtualMachineRestoring,
			running: "The virtual machine is being restored from a snapshot",
			noun:    "Restore of the virtual machine",
		}
	case v1alpha2.VMOPTypeClone:
		return operationDescription{
			reason:  vmcondition.ReasonVirtualMachineCloning,
			running: "The virtual machine is being cloned",
			noun:    "Clone of the virtual machine",
		}
	default:
		return operationDescription{}
	}
}

func describeEviction(vmop *v1alpha2.VirtualMachineOperation) operationDescription {
	annotationsOfVMOP := vmop.GetAnnotations()
	name := vmop.GetName()

	if _, ok := annotationsOfVMOP[annotations.AnnVMOPVolumeMigration]; ok || strings.HasPrefix(name, commonvmop.VolumeMigrationPrefix) {
		return operationDescription{
			reason:  vmcondition.ReasonVolumeMigrating,
			running: "The disks of the virtual machine are being moved to another storage",
			noun:    "Migration of the disks of the virtual machine",
		}
	}

	if _, ok := annotationsOfVMOP[annotations.AnnVMOPWorkloadUpdate]; ok {
		switch {
		case strings.HasPrefix(name, commonvmop.FirmwareUpdatePrefix):
			return operationDescription{
				reason:  vmcondition.ReasonFirmwareUpdating,
				running: "The virtual machine is being migrated to update its firmware",
				noun:    "Firmware update of the virtual machine",
			}
		case strings.HasPrefix(name, commonvmop.NodePlacementUpdatePrefix):
			return operationDescription{
				reason:  vmcondition.ReasonNodePlacementUpdating,
				running: "The virtual machine is being migrated to apply its new node placement",
				noun:    "Node placement update of the virtual machine",
			}
		case strings.HasPrefix(name, commonvmop.HotplugResourcesPrefix):
			return operationDescription{
				reason:  vmcondition.ReasonResourcesHotplugging,
				running: "The virtual machine is being migrated to apply the hot-plugged CPU and memory",
				noun:    "Hot-plug of CPU and memory of the virtual machine",
			}
		default:
			return operationDescription{
				reason:  vmcondition.ReasonWorkloadUpdating,
				running: "The virtual machine is being migrated to update its workload",
				noun:    "Workload update of the virtual machine",
			}
		}
	}

	return operationDescription{
		reason:  vmcondition.ReasonVirtualMachineEvacuating,
		running: "The virtual machine is being evicted from its node",
		noun:    "Eviction of the virtual machine",
	}
}

// oldestBy returns the oldest of the items the predicate accepts, so that a machine with several
// operations of the same kind reports the one that started first.
func oldestBy[T any, PT interface {
	*T
	metav1.Object
}](items []T, accept func(PT) bool) PT {
	var oldest PT

	for i := range items {
		item := PT(&items[i])
		if !accept(item) {
			continue
		}
		if oldest == nil || item.GetCreationTimestamp().Time.Before(oldest.GetCreationTimestamp().Time) {
			oldest = item
		}
	}

	return oldest
}
