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
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vm/internal/state"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdcondition"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
)

type virtualDisksState struct {
	// Counters for disks in different states
	counts struct {
		readyToUse    int // Available for use by the current VM
		creatingImage int // Currently used for image creation
		onOtherVMs    int // Attached to other virtual machines
	}

	// Names of specific disks (if detailed tracking is needed)
	diskNames struct {
		imageCreation string // Disk being used for image creation
		usedByOtherVM string // Disk attached to another VM
		notReady      string // Disk not yet ready for use
	}
}

func (h *BlockDeviceHandler) checkVirtualDisksToBeWFFC(ctx context.Context, s state.VirtualMachineState) (bool, error) {
	vds, err := s.VirtualDisksByName(ctx)
	if err != nil {
		return false, err
	}

	for _, vd := range vds {
		if vd.Status.Phase == v1alpha2.DiskWaitForFirstConsumer {
			return true, nil
		}
	}

	return false, nil
}

func (h *BlockDeviceHandler) handleVirtualDisksReadyForUse(ctx context.Context, s state.VirtualMachineState) (bool, error) {
	vm := s.VirtualMachine().Changed()

	vds, err := s.VirtualDisksByName(ctx)
	if err != nil {
		return false, err
	}
	diskState := h.getVirtualDisksState(vm, vds)

	ready := len(vds) == diskState.counts.readyToUse
	if !ready {
		message := h.getStatusMessage(diskState, vds)
		h.setConditionNotReady(vm, message)
		return true, nil
	}

	return false, nil
}

const (
	UsageTypeImageCreation string = "for image creation"
	UsageTypeAnotherVM     string = "by another VM"
)

func (h *BlockDeviceHandler) getStatusMessage(diskState virtualDisksState, vds map[string]*v1alpha2.VirtualDisk) string {
	summaryCount := len(vds)

	var messages []string

	messages = append(messages, fmt.Sprintf(
		"Waiting for block devices to be ready to use: %d/%d",
		diskState.counts.readyToUse, summaryCount))

	addUsageMessage := func(count int, name, usageType string) {
		if count == 0 {
			return
		}

		var msg string
		if count == 1 {
			msg = fmt.Sprintf("Virtual disk %q is in use %s", name, usageType)
		} else {
			msg = fmt.Sprintf("Virtual disks %d/%d are in use %s", count, summaryCount, usageType)
		}

		messages = append(messages, msg)
	}

	addUsageMessage(diskState.counts.creatingImage, diskState.diskNames.imageCreation, UsageTypeImageCreation)
	addUsageMessage(diskState.counts.onOtherVMs, diskState.diskNames.usedByOtherVM, UsageTypeAnotherVM)

	if summaryCount == 1 {
		if diskState.counts.creatingImage == 1 || diskState.counts.onOtherVMs == 1 {
			return messages[len(messages)-1] + "."
		}

		var diskName string
		for _, vd := range vds {
			diskName = vd.Name
		}

		return fmt.Sprintf("Waiting for block device %q to be ready to use.", diskName)
	}

	return strings.Join(messages, "; ") + "."
}

func (h *BlockDeviceHandler) setConditionReady(vm *v1alpha2.VirtualMachine) {
	conditions.SetCondition(
		conditions.NewConditionBuilder(vmcondition.TypeBlockDevicesReady).
			Generation(vm.Generation).
			Status(metav1.ConditionTrue).
			Reason(vmcondition.ReasonBlockDevicesReady).
			Message(""),
		&vm.Status.Conditions,
	)
}

func (h *BlockDeviceHandler) setConditionNotReady(vm *v1alpha2.VirtualMachine, message string) {
	conditions.SetCondition(
		conditions.NewConditionBuilder(vmcondition.TypeBlockDevicesReady).
			Generation(vm.Generation).
			Status(metav1.ConditionFalse).
			Reason(vmcondition.ReasonBlockDevicesNotReady).
			Message(message),
		&vm.Status.Conditions,
	)
}

func (h *BlockDeviceHandler) getVirtualDisksState(vm *v1alpha2.VirtualMachine, vds map[string]*v1alpha2.VirtualDisk) virtualDisksState {
	vdsState := virtualDisksState{}

	for _, vd := range vds {
		inUseCondition, _ := conditions.GetCondition(vdcondition.InUseType, vd.Status.Conditions)
		if !conditions.IsLastUpdated(inUseCondition, vd) {
			continue
		}

		h.handleImageCreationDisk(vd, inUseCondition, &vdsState)
		h.handleAttachedDisk(vd, vm, inUseCondition, &vdsState)
		h.handleReadyForUseDisk(vd, vm, inUseCondition, &vdsState)
	}

	return vdsState
}

func (h *BlockDeviceHandler) handleImageCreationDisk(
	vd *v1alpha2.VirtualDisk,
	condition metav1.Condition,
	state *virtualDisksState,
) {
	if condition.Status == metav1.ConditionTrue && condition.Reason == vdcondition.UsedForImageCreation.String() {
		state.counts.creatingImage++
		state.diskNames.imageCreation = vd.Name
	}
}

func (h *BlockDeviceHandler) handleAttachedDisk(
	vd *v1alpha2.VirtualDisk,
	vm *v1alpha2.VirtualMachine,
	condition metav1.Condition,
	state *virtualDisksState,
) {
	if condition.Status == metav1.ConditionTrue && condition.Reason == vdcondition.AttachedToVirtualMachine.String() {
		if !h.checkVMToMountVD(vd, vm) {
			state.counts.onOtherVMs++
			state.diskNames.usedByOtherVM = vd.Name
		} else {
			state.counts.readyToUse++
		}
	}
}

func (h *BlockDeviceHandler) handleReadyForUseDisk(
	vd *v1alpha2.VirtualDisk,
	vm *v1alpha2.VirtualMachine,
	condition metav1.Condition,
	state *virtualDisksState,
) {
	if condition.Status != metav1.ConditionTrue &&
		vm.Status.Phase == v1alpha2.MachineStopped &&
		h.checkVDToUseVM(vd, vm) {
		state.counts.readyToUse++
	}
}

func (h *BlockDeviceHandler) checkVDToUseVM(vd *v1alpha2.VirtualDisk, vm *v1alpha2.VirtualMachine) bool {
	attachedVMs := vd.Status.AttachedToVirtualMachines

	for _, attachedVM := range attachedVMs {
		if attachedVM.Name == vm.Name {
			return true
		}
	}

	return false
}

func (h *BlockDeviceHandler) checkVMToMountVD(vd *v1alpha2.VirtualDisk, vm *v1alpha2.VirtualMachine) bool {
	attachedVMs := vd.Status.AttachedToVirtualMachines

	for _, attachedVM := range attachedVMs {
		if attachedVM.Name == vm.Name {
			return attachedVM.Mounted
		}
	}

	return false
}

func (h *BlockDeviceHandler) handleBlockDevicesReady(ctx context.Context, s state.VirtualMachineState, bdState BlockDevicesState) error {
	log := logger.FromContext(ctx).With(logger.SlogHandler(nameBlockDeviceHandler))

	current := s.VirtualMachine().Current()
	changed := s.VirtualMachine().Changed()

	isWFFC, err := h.checkVirtualDisksToBeWFFC(ctx, s)
	if err != nil {
		return err
	}

	if readyCount, canStartVM, warnings := h.countReadyBlockDevices(current, bdState, isWFFC); len(current.Spec.BlockDeviceRefs) != readyCount {
		var msg string
		if len(current.Spec.BlockDeviceRefs) == 1 {
			msg = fmt.Sprintf("Waiting for block device %q to be ready", current.Spec.BlockDeviceRefs[0].Name)
		} else {
			msg = fmt.Sprintf("Waiting for block devices to be ready: %d/%d", readyCount, len(current.Spec.BlockDeviceRefs))
		}
		if len(warnings) > 0 {
			msg = msg + "; " + strings.Join(warnings, "; ")
		}

		msg += "."

		log.Info(msg, "actualReady", readyCount, "expectedReady", len(current.Spec.BlockDeviceRefs))

		cb := conditions.NewConditionBuilder(vmcondition.TypeBlockDevicesReady).
			Generation(changed.Generation).
			Status(metav1.ConditionFalse).
			Message(msg)

		if canStartVM && isWFFC {
			cb.Reason(vmcondition.ReasonWaitingForWaitForFirstConsumerBlockDevicesToBeReady)
			conditions.SetCondition(cb, &changed.Status.Conditions)
			return errBlockDeviceWaitForProvisioning
		}

		cb.Reason(vmcondition.ReasonBlockDevicesNotReady)
		conditions.SetCondition(cb, &changed.Status.Conditions)
		return errBlockDeviceNotReady
	}

	return nil
}

// countReadyBlockDevices check if all attached images and disks are ready to use by the VM.
func (h *BlockDeviceHandler) countReadyBlockDevices(vm *v1alpha2.VirtualMachine, s BlockDevicesState, wffc bool) (int, bool, []string) {
	if vm == nil {
		return 0, false, nil
	}

	var warnings []string
	ready := 0
	canStartKVVM := true
	for _, bd := range vm.Spec.BlockDeviceRefs {
		switch bd.Kind {
		case v1alpha2.ImageDevice:
			if vi, hasKey := s.VIByName[bd.Name]; hasKey && vi.Status.Phase == v1alpha2.ImageReady {
				ready++
				continue
			}
			canStartKVVM = false
		case v1alpha2.ClusterImageDevice:
			if cvi, hasKey := s.CVIByName[bd.Name]; hasKey && cvi.Status.Phase == v1alpha2.ImageReady {
				ready++
				continue
			}
			canStartKVVM = false
		case v1alpha2.DiskDevice:
			vd, hasKey := s.VDByName[bd.Name]
			if !hasKey {
				canStartKVVM = false
				continue
			}

			if vd.Status.Target.PersistentVolumeClaim == "" {
				canStartKVVM = false
				continue
			}

			if vd.DeletionTimestamp != nil {
				canStartKVVM = false
				warnings = append(warnings, fmt.Sprintf("Virtual disk %s is terminating", vd.Name))
				continue
			}

			readyCondition, _ := conditions.GetCondition(vdcondition.ReadyType, vd.Status.Conditions)
			if readyCondition.Status == metav1.ConditionTrue {
				ready++
			} else {
				var msg string
				if wffc && vm.Status.Phase == v1alpha2.MachineStopped {
					msg = fmt.Sprintf("Virtual disk %s is waiting for the virtual machine to be starting", vd.Name)
				} else {
					msg = fmt.Sprintf("Virtual disk %s is waiting for the underlying PVC to be bound", vd.Name)
				}

				warnings = append(warnings, msg)
			}
		}
	}

	return ready, canStartKVVM, warnings
}
