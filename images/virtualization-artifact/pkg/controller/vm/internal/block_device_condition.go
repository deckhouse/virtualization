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

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vm/internal/state"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdcondition"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
)

type virtualDisksState struct {
	// Counters for disks in different states
	counts struct {
		ready         int // Available for use by the current VM
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
		if vd.Status.Phase == virtv2.DiskWaitForFirstConsumer {
			return true, nil
		}
	}

	return false, nil
}

func (h *BlockDeviceHandler) validateVirtualDisksToBeReadyForUse(ctx context.Context, s state.VirtualMachineState) error {
	vm := s.VirtualMachine().Changed()
	cb := conditions.NewConditionBuilder(vmcondition.TypeBlockDevicesReady).
		Generation(vm.Generation)
	vds, err := s.VirtualDisksByName(ctx)
	if err != nil {
		return err
	}
	diskState := h.getVirtualDisksState(vm, vds)

	ready := len(vds) == diskState.counts.ready
	if !ready {
		message := h.getStatusMessage(diskState, vds)
		h.setConditionNotReady(cb, vm, message)
		return ErrBlockDeviceNotReadyForUse
	}

	return nil
}

type DiskUsageType string

const (
	UsageTypeImageCreation DiskUsageType = "for image creation"
	UsageTypeAnotherVM     DiskUsageType = "by another VM"
)

func (h *BlockDeviceHandler) getStatusMessage(diskState virtualDisksState, vds map[string]*virtv2.VirtualDisk) string {
	summaryCount := len(vds)

	if summaryCount == 1 {
		return h.getSingleDiskMessage(diskState, vds)
	}
	return h.getMultipleDisksMessage(diskState, summaryCount)
}

func (h *BlockDeviceHandler) getSingleDiskMessage(diskState virtualDisksState, vds map[string]*virtv2.VirtualDisk) string {
	var diskName string
	for _, vd := range vds {
		diskName = vd.Name
	}

	switch {
	case diskState.counts.creatingImage == 1:
		return h.getDiskUsageMessage(1, 1, diskState.diskNames.imageCreation, UsageTypeImageCreation) + "."
	case diskState.counts.onOtherVMs == 1:
		return h.getDiskUsageMessage(1, 1, diskState.diskNames.usedByOtherVM, UsageTypeAnotherVM) + "."
	default:
		return fmt.Sprintf("Waiting for block device %q to be ready to use.", diskName)
	}
}

func (h *BlockDeviceHandler) getMultipleDisksMessage(diskState virtualDisksState, summaryCount int) string {
	var messages []string

	messages = append(messages, fmt.Sprintf(
		"Waiting for block devices to be ready to use: %d/%d",
		diskState.counts.ready, summaryCount))

	if diskState.counts.creatingImage > 0 {
		messages = append(messages, h.getDiskUsageMessage(
			diskState.counts.creatingImage,
			summaryCount,
			diskState.diskNames.imageCreation,
			UsageTypeImageCreation))
	}

	if diskState.counts.onOtherVMs > 0 {
		messages = append(messages, h.getDiskUsageMessage(
			diskState.counts.onOtherVMs,
			summaryCount,
			diskState.diskNames.usedByOtherVM,
			UsageTypeAnotherVM))
	}

	return strings.Join(messages, "; ") + "."
}

func (h *BlockDeviceHandler) getDiskUsageMessage(count, total int, diskName string, usageType DiskUsageType) string {
	if count == 1 {
		return fmt.Sprintf("Virtual disk %q is in use %s", diskName, usageType)
	}
	return fmt.Sprintf("Virtual disks %d/%d are in use %s", count, total, usageType)
}

func (h *BlockDeviceHandler) setConditionReady(vm *virtv2.VirtualMachine) {
	cb := conditions.NewConditionBuilder(vmcondition.TypeBlockDevicesReady).
		Generation(vm.Generation)
	cb.Status(metav1.ConditionTrue).
		Reason(vmcondition.ReasonBlockDevicesReady).
		Message("")
	conditions.SetCondition(cb, &vm.Status.Conditions)
}

func (h *BlockDeviceHandler) setConditionNotReady(cb *conditions.ConditionBuilder, vm *virtv2.VirtualMachine, message string) {
	cb.Status(metav1.ConditionFalse).
		Reason(vmcondition.ReasonBlockDevicesNotReady).
		Message(message)
	conditions.SetCondition(cb, &vm.Status.Conditions)
}

func (h *BlockDeviceHandler) getVirtualDisksState(vm *virtv2.VirtualMachine, vds map[string]*virtv2.VirtualDisk) virtualDisksState {
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
	vd *virtv2.VirtualDisk,
	condition metav1.Condition,
	state *virtualDisksState,
) {
	if condition.Status == metav1.ConditionTrue && condition.Reason == vdcondition.UsedForImageCreation.String() {
		state.counts.creatingImage++
		state.diskNames.imageCreation = vd.Name
	}
}

func (h *BlockDeviceHandler) handleAttachedDisk(
	vd *virtv2.VirtualDisk,
	vm *virtv2.VirtualMachine,
	condition metav1.Condition,
	state *virtualDisksState,
) {
	if condition.Status == metav1.ConditionTrue && condition.Reason == vdcondition.AttachedToVirtualMachine.String() {
		if !h.checkVDToUseCurrentVM(vd, vm) {
			state.counts.onOtherVMs++
			state.diskNames.usedByOtherVM = vd.Name
		} else {
			state.counts.ready++
		}
	}
}

func (h *BlockDeviceHandler) handleReadyForUseDisk(
	vd *virtv2.VirtualDisk,
	vm *virtv2.VirtualMachine,
	condition metav1.Condition,
	state *virtualDisksState,
) {
	if condition.Status != metav1.ConditionTrue &&
		vm.Status.Phase == virtv2.MachineStopped &&
		h.checkVDToUseCurrentVM(vd, vm) &&
		len(vd.Status.AttachedToVirtualMachines) == 1 {
		state.counts.ready++
	}
}

func (h *BlockDeviceHandler) checkVDToUseCurrentVM(vd *virtv2.VirtualDisk, vm *virtv2.VirtualMachine) bool {
	attachedVMs := vd.Status.AttachedToVirtualMachines

	for _, attachedVM := range attachedVMs {
		if attachedVM.Name == vm.Name && attachedVM.Mounted {
			return true
		}
	}

	return false
}

func (h *BlockDeviceHandler) validateBlockDevicesToBeReady(ctx context.Context, s state.VirtualMachineState, bdState BlockDevicesState) error {
	log := logger.FromContext(ctx).With(logger.SlogHandler(nameBlockDeviceHandler))

	current := s.VirtualMachine().Current()
	changed := s.VirtualMachine().Changed()

	cb := conditions.NewConditionBuilder(vmcondition.TypeBlockDevicesReady).
		Generation(changed.Generation)

	isWFFC, err := h.checkVirtualDisksToBeWFFC(ctx, s)
	if err != nil {
		return err
	}

	if readyCount, canStartVM, warnings := h.countReadyBlockDevices(current, bdState, isWFFC); len(current.Spec.BlockDeviceRefs) != readyCount {
		var reason vmcondition.Reason
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
		h.recorder.Event(changed, corev1.EventTypeNormal, reason.String(), msg)

		cb.Status(metav1.ConditionFalse).
			Message(msg)

		if canStartVM && isWFFC {
			cb.Reason(vmcondition.ReasonWaitingForProvisioningToPVC)
			conditions.SetCondition(cb, &changed.Status.Conditions)
			return ErrBlockDeviceWaitForProvisioning
		} else {
			cb.Reason(vmcondition.ReasonBlockDevicesNotReady)
			conditions.SetCondition(cb, &changed.Status.Conditions)
			return ErrBlockDeviceNotReady
		}
	}

	return nil
}

// countReadyBlockDevices check if all attached images and disks are ready to use by the VM.
func (h *BlockDeviceHandler) countReadyBlockDevices(vm *virtv2.VirtualMachine, s BlockDevicesState, wffc bool) (int, bool, []string) {
	if vm == nil {
		return 0, false, nil
	}

	var warnings []string
	ready := 0
	canStartKVVM := true
	for _, bd := range vm.Spec.BlockDeviceRefs {
		switch bd.Kind {
		case virtv2.ImageDevice:
			if vi, hasKey := s.VIByName[bd.Name]; hasKey && vi.Status.Phase == virtv2.ImageReady {
				ready++
				continue
			}
			canStartKVVM = false
		case virtv2.ClusterImageDevice:
			if cvi, hasKey := s.CVIByName[bd.Name]; hasKey && cvi.Status.Phase == virtv2.ImageReady {
				ready++
				continue
			}
			canStartKVVM = false
		case virtv2.DiskDevice:
			vd, hasKey := s.VDByName[bd.Name]
			if !hasKey {
				canStartKVVM = false
				continue
			}

			if vd.Status.Target.PersistentVolumeClaim == "" {
				canStartKVVM = false
				continue
			}
			readyCondition, _ := conditions.GetCondition(vdcondition.ReadyType, vd.Status.Conditions)
			if readyCondition.Status == metav1.ConditionTrue {
				ready++
			} else {
				var msg string
				if wffc && vm.Status.Phase == virtv2.MachineStopped {
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
