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
	"errors"
	"fmt"
	"log/slog"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/common"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/kvbuilder"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vm/internal/state"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdcondition"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
)

const nameBlockDeviceHandler = "BlockDeviceHandler"

func NewBlockDeviceHandler(cl client.Client, recorder eventrecord.EventRecorderLogger, blockDeviceService BlockDeviceService) *BlockDeviceHandler {
	return &BlockDeviceHandler{
		client:             cl,
		recorder:           recorder,
		blockDeviceService: blockDeviceService,

		viProtection:  service.NewProtectionService(cl, virtv2.FinalizerVIProtection),
		cviProtection: service.NewProtectionService(cl, virtv2.FinalizerCVIProtection),
		vdProtection:  service.NewProtectionService(cl, virtv2.FinalizerVDProtection),
	}
}

type BlockDeviceHandler struct {
	client             client.Client
	recorder           eventrecord.EventRecorderLogger
	blockDeviceService BlockDeviceService

	viProtection  *service.ProtectionService
	cviProtection *service.ProtectionService
	vdProtection  *service.ProtectionService
}

var (
	ErrBlockDeviceLimitExceeded       = errors.New("block device limit exceeded")
	ErrConflictedVirtualDisksDetected = errors.New("conflicted virtual disks detected")
	ErrBlockDeviceNotReady            = errors.New("block device not ready")
	ErrBlockDeviceNotReadyForUse      = errors.New("block device not ready for use")
)

func (h *BlockDeviceHandler) Handle(ctx context.Context, s state.VirtualMachineState) (reconcile.Result, error) {
	log := logger.FromContext(ctx).With(logger.SlogHandler(nameBlockDeviceHandler))

	if s.VirtualMachine().IsEmpty() {
		return reconcile.Result{}, nil
	}
	current := s.VirtualMachine().Current()
	changed := s.VirtualMachine().Changed()

	_, ok := conditions.GetCondition(vdcondition.InUseType, changed.Status.Conditions)
	if !ok {
		cb := conditions.NewConditionBuilder(vdcondition.InUseType).
			Status(metav1.ConditionUnknown).
			Reason(conditions.ReasonUnknown).
			Generation(changed.Generation)
		conditions.SetCondition(cb, &changed.Status.Conditions)
	}

	bdState := NewBlockDeviceState(s)
	err := bdState.Reload(ctx)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("failed to reload blockdevice state: %w", err)
	}

	if isDeletion(current) {
		return reconcile.Result{}, h.removeFinalizersOnBlockDevices(ctx, changed, bdState)
	}

	if err = h.setFinalizersOnBlockDevices(ctx, changed, bdState); err != nil {
		return reconcile.Result{}, fmt.Errorf("unable to add block devices finalizers: %w", err)
	}

	if err = h.checkBlockDeviceLimit(ctx, changed); err != nil {
		if errors.Is(err, ErrBlockDeviceLimitExceeded) {
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}

	if err = h.updateStatusBlockDeviceRefs(ctx, s, log); err != nil {
		if errors.Is(err, ErrConflictedVirtualDisksDetected) {
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}

	if err = h.checkBlockDevicesToBeReady(s, bdState, log); err != nil {
		if errors.Is(err, ErrBlockDeviceNotReady) {
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}

	if err = h.checkBlockDevicesToBeReadyForUse(ctx, s); err != nil {
		if errors.Is(err, ErrBlockDeviceNotReadyForUse) {
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}

	h.syncBlockDeviceReadyCondition(changed)
	return reconcile.Result{}, nil
}

func (h *BlockDeviceHandler) syncBlockDeviceReadyCondition(vm *virtv2.VirtualMachine) {
	cb := conditions.NewConditionBuilder(vmcondition.TypeBlockDevicesReady).
		Generation(vm.Generation)
	cb.Status(metav1.ConditionTrue).
		Reason(vmcondition.ReasonBlockDevicesReady).
		Message("")
	conditions.SetCondition(cb, &vm.Status.Conditions)
}

func (h *BlockDeviceHandler) checkBlockDevicesToBeReadyForUse(ctx context.Context, s state.VirtualMachineState) error {
	vm := s.VirtualMachine().Changed()
	cb := conditions.NewConditionBuilder(vmcondition.TypeBlockDevicesReady).
		Generation(vm.Generation)
	vds, err := s.VirtualDisksByName(ctx)
	if err != nil {
		return err
	}

	countReadyForUseVD, countUsingForCreateImageVD, countUsingInOtherVMVD := 0, 0, 0

	var imageDiskName string
	var otherVMDiskName string

	msg := ""
	for _, vd := range vds {
		inUseCondition, _ := conditions.GetCondition(vdcondition.InUseType, vd.Status.Conditions)
		if inUseCondition.ObservedGeneration != vd.Generation {
			continue
		}

		if inUseCondition.Status == metav1.ConditionTrue {
			switch inUseCondition.Reason {
			case vdcondition.UsedForImageCreation.String():
				countUsingForCreateImageVD++
				if len(vds) == 1 {
					msg = fmt.Sprintf("Virtual disk %q is in use for image creation.", vd.Name)
				}
				imageDiskName = vd.Name
			case vdcondition.AttachedToVirtualMachine.String():
				if !h.checkVDToUseCurrentVM(vd, vm) {
					countUsingInOtherVMVD++
					if len(vds) == 1 {
						msg = fmt.Sprintf("Virtual disk %q is in use by another VM.", vd.Name)
					}
					otherVMDiskName = vd.Name
				} else {
					countReadyForUseVD++
				}
			}
		} else {
			if vm.Status.Phase == virtv2.MachineStopped && h.checkVDToUseCurrentVM(vd, vm) && len(vd.Status.AttachedToVirtualMachines) == 1 {
				countReadyForUseVD++
			} else if len(vds) == 1 {
				msg = fmt.Sprintf("Waiting for block device %q to be ready to use.", vd.Name)
			}
		}
	}

	if len(vds) == countReadyForUseVD {
		return nil
	}

	if msg != "" {
		cb.Status(metav1.ConditionFalse).
			Reason(vmcondition.ReasonBlockDevicesNotReady).
			Message(msg)
		conditions.SetCondition(cb, &vm.Status.Conditions)
		return ErrBlockDeviceNotReadyForUse
	}

	if countReadyForUseVD == 0 && countUsingInOtherVMVD == 0 && countUsingForCreateImageVD == 0 {
		var msgBuilder strings.Builder
		msgBuilder.WriteString(fmt.Sprintf("Waiting for block devices to be ready to use: %d/%d.", 0, len(vds)))
		cb.Status(metav1.ConditionFalse).
			Reason(vmcondition.ReasonBlockDevicesNotReady).
			Message(msgBuilder.String())
		conditions.SetCondition(cb, &vm.Status.Conditions)
		return ErrBlockDeviceNotReadyForUse
	}

	if len(vds) > 1 {
		var msgBuilder strings.Builder
		msgBuilder.WriteString(fmt.Sprintf("Waiting for block devices to be ready to use: %d/%d", countReadyForUseVD, len(vds)))

		if countUsingForCreateImageVD > 0 {
			if countUsingForCreateImageVD == 1 {
				msgBuilder.WriteString(fmt.Sprintf("; Disk %q is in use for image creation", imageDiskName))
			} else {
				msgBuilder.WriteString(fmt.Sprintf("; Disks %d/%d are in use for image creation", countUsingForCreateImageVD, len(vds)))
			}
		}

		if countUsingInOtherVMVD > 0 {
			if countUsingInOtherVMVD == 1 {
				msgBuilder.WriteString(fmt.Sprintf("; Disk %q is in use by another VM", otherVMDiskName))
			} else {
				msgBuilder.WriteString(fmt.Sprintf("; Disks %d/%d are in use by another VM", countUsingInOtherVMVD, len(vds)))
			}
		}

		msgBuilder.WriteString(".")
		if msgBuilder.Len() > 0 {
			cb.Status(metav1.ConditionFalse).
				Reason(vmcondition.ReasonBlockDevicesNotReady).
				Message(msgBuilder.String())
			conditions.SetCondition(cb, &vm.Status.Conditions)
			return ErrBlockDeviceNotReadyForUse
		}
	}

	return nil
}

func (h *BlockDeviceHandler) checkVDToUseCurrentVM(vd *virtv2.VirtualDisk, vm *virtv2.VirtualMachine) bool {
	attachedVMs := vd.Status.AttachedToVirtualMachines
	for _, attachedVM := range attachedVMs {
		if attachedVM.Name == vm.Name {
			return true
		}
	}

	return false
}

func (h *BlockDeviceHandler) checkBlockDevicesToBeReady(s state.VirtualMachineState, bdState BlockDevicesState, log *slog.Logger) error {
	current := s.VirtualMachine().Current()
	changed := s.VirtualMachine().Changed()

	cb := conditions.NewConditionBuilder(vmcondition.TypeBlockDevicesReady).
		Generation(changed.Generation)

	if readyCount, canStartKVVM, warnings := h.countReadyBlockDevices(current, bdState); len(current.Spec.BlockDeviceRefs) != readyCount {
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

		if !canStartKVVM {
			reason = vmcondition.ReasonBlockDevicesNotReady
		} else {
			reason = vmcondition.ReasonWaitingForProvisioningToPVC
		}

		log.Info(msg, "actualReady", readyCount, "expectedReady", len(current.Spec.BlockDeviceRefs))

		h.recorder.Event(changed, corev1.EventTypeNormal, reason.String(), msg)
		cb.Status(metav1.ConditionFalse).
			Reason(reason).
			Message(msg)
		conditions.SetCondition(cb, &changed.Status.Conditions)
		return ErrBlockDeviceNotReady
	}

	return nil
}

func (h *BlockDeviceHandler) updateStatusBlockDeviceRefs(
	ctx context.Context,
	s state.VirtualMachineState,
	log *slog.Logger,
) error {
	changed := s.VirtualMachine().Changed()

	var err error
	// Fill BlockDeviceRefs every time without knowledge of previously kept BlockDeviceRefs.
	changed.Status.BlockDeviceRefs, err = h.getBlockDeviceStatusRefs(ctx, s)
	if err != nil {
		return fmt.Errorf("failed to get block device status refs: %w", err)
	}

	conflictWarning, err := h.getBlockDeviceWarnings(ctx, s)
	if err != nil {
		return fmt.Errorf("failed to get hotplugged block devices: %w", err)
	}

	// Update the BlockDevicesReady condition if there are conflicted virtual disks.
	if conflictWarning != "" {
		log.Info(fmt.Sprintf("Conflicted virtual disks: %s", conflictWarning))
		cd := conditions.NewConditionBuilder(vmcondition.TypeBlockDevicesReady).Status(metav1.ConditionFalse).
			Reason(vmcondition.ReasonBlockDevicesNotReady).
			Message(conflictWarning).
			Generation(changed.Generation)
		conditions.SetCondition(cd, &changed.Status.Conditions)
		return ErrConflictedVirtualDisksDetected
	}

	return nil
}

func (h *BlockDeviceHandler) checkBlockDeviceLimit(ctx context.Context, vm *virtv2.VirtualMachine) error {
	// Get number of connected block devices.
	// If it's greater than the limit, then set the condition to false.
	blockDeviceAttachedCount, err := h.blockDeviceService.CountBlockDevicesAttachedToVm(ctx, vm)
	if err != nil {
		return err
	}

	if blockDeviceAttachedCount > common.VmBlockDeviceAttachedLimit {
		cb := conditions.NewConditionBuilder(vmcondition.TypeBlockDevicesReady).
			Status(metav1.ConditionFalse).
			Reason(vmcondition.ReasonBlockDeviceLimitExceeded).
			Message(fmt.Sprintf("Cannot attach %d block devices (%d is maximum) to VirtualMachine %q", blockDeviceAttachedCount, common.VmBlockDeviceAttachedLimit, vm.Name)).
			Generation(vm.Generation)

		conditions.SetCondition(cb, &vm.Status.Conditions)
		return ErrBlockDeviceLimitExceeded
	}

	return nil
}

func (h *BlockDeviceHandler) Name() string {
	return nameBlockDeviceHandler
}

func (h *BlockDeviceHandler) getBlockDeviceWarnings(ctx context.Context, s state.VirtualMachineState) (string, error) {
	vmbdasByBlockDevice, err := s.VirtualMachineBlockDeviceAttachments(ctx)
	if err != nil {
		return "", err
	}

	hotplugsByName := make(map[string]struct{})

	for _, vmbdas := range vmbdasByBlockDevice {
		for _, vmbda := range vmbdas {
			switch vmbda.Status.Phase {
			case virtv2.BlockDeviceAttachmentPhaseInProgress,
				virtv2.BlockDeviceAttachmentPhaseAttached:
			default:
				continue
			}

			var (
				cvi         *virtv2.ClusterVirtualImage
				vi          *virtv2.VirtualImage
				vd          *virtv2.VirtualDisk
				bdStatusRef virtv2.BlockDeviceStatusRef
			)

			switch vmbda.Spec.BlockDeviceRef.Kind {
			case virtv2.VMBDAObjectRefKindVirtualDisk:
				vd, err = s.VirtualDisk(ctx, vmbda.Spec.BlockDeviceRef.Name)
				if err != nil {
					return "", err
				}

				if vd == nil {
					continue
				}

				bdStatusRef = h.getBlockDeviceStatusRef(virtv2.DiskDevice, vmbda.Spec.BlockDeviceRef.Name)
				bdStatusRef.Size = vd.Status.Capacity
			case virtv2.VMBDAObjectRefKindVirtualImage:
				vi, err = s.VirtualImage(ctx, vmbda.Spec.BlockDeviceRef.Name)
				if err != nil {
					return "", err
				}

				if vi == nil {
					continue
				}

				bdStatusRef = h.getBlockDeviceStatusRef(virtv2.ImageDevice, vmbda.Spec.BlockDeviceRef.Name)
				bdStatusRef.Size = vi.Status.Size.Unpacked

			case virtv2.VMBDAObjectRefKindClusterVirtualImage:
				cvi, err = s.ClusterVirtualImage(ctx, vmbda.Spec.BlockDeviceRef.Name)
				if err != nil {
					return "", err
				}

				if cvi == nil {
					continue
				}

				bdStatusRef = h.getBlockDeviceStatusRef(virtv2.ClusterImageDevice, vmbda.Spec.BlockDeviceRef.Name)
				bdStatusRef.Size = cvi.Status.Size.Unpacked
			default:
				return "", fmt.Errorf("unacceptable `Kind` of `BlockDeviceRef`: %s", vmbda.Spec.BlockDeviceRef.Kind)
			}
			// todo dlopatin remove this
			bdStatusRef.Hotplugged = true
			bdStatusRef.VirtualMachineBlockDeviceAttachmentName = vmbda.Name

			hotplugsByName[bdStatusRef.Name] = struct{}{}
		}
	}

	var conflictedRefs []string
	vm := s.VirtualMachine().Current()

	for _, bdSpecRef := range vm.Spec.BlockDeviceRefs {
		// It is a precaution to not apply changes in spec.blockDeviceRefs if disk is already
		// hotplugged using the VMBDA resource.
		// spec check is done by VirtualDisk status
		// the reverse check is done by the vmbda-controller.
		if bdSpecRef.Kind == virtv2.DiskDevice {
			if _, conflict := hotplugsByName[bdSpecRef.Name]; conflict {
				conflictedRefs = append(conflictedRefs, bdSpecRef.Name)
				continue
			}
		}

		if _, conflict := hotplugsByName[bdSpecRef.Name]; conflict {
			conflictedRefs = append(conflictedRefs, bdSpecRef.Name)
			continue
		}
	}

	var warning string
	if len(conflictedRefs) > 0 {
		warning = fmt.Sprintf("spec.blockDeviceRefs field contains hotplugged disks (%s): unplug or remove them from spec to continue.", strings.Join(conflictedRefs, ", "))
	}

	return warning, nil
}

type nameKindKey struct {
	kind virtv2.BlockDeviceKind
	name string
}

// getBlockDeviceStatusRefs returns block device refs to populate .status.blockDeviceRefs of the virtual machine.
// If kvvm is present, this method will reflect all volumes with prefixes (vi,vd, or cvi) into the slice of `BlockDeviceStatusRef`.
// Block devices from the virtual machine specification will be added to the resulting slice if they have not been included in the previous step.
func (h *BlockDeviceHandler) getBlockDeviceStatusRefs(ctx context.Context, s state.VirtualMachineState) ([]virtv2.BlockDeviceStatusRef, error) {
	kvvm, err := s.KVVM(ctx)
	if err != nil {
		return nil, err
	}

	var refs []virtv2.BlockDeviceStatusRef

	// 1. There is no kvvm yet: populate block device refs with the spec.
	if kvvm == nil {
		for _, specBlockDeviceRef := range s.VirtualMachine().Current().Spec.BlockDeviceRefs {
			ref := h.getBlockDeviceStatusRef(specBlockDeviceRef.Kind, specBlockDeviceRef.Name)
			ref.Size, err = h.getBlockDeviceRefSize(ctx, ref, s)
			if err != nil {
				return nil, err
			}
			refs = append(refs, ref)
		}

		return refs, nil
	}

	if kvvm.Spec.Template == nil {
		return nil, errors.New("there is no spec template")
	}

	kvvmi, err := s.KVVMI(ctx)
	if err != nil {
		return nil, err
	}

	var kvvmiVolumeStatusByName map[string]virtv1.VolumeStatus
	if kvvmi != nil {
		kvvmiVolumeStatusByName = make(map[string]virtv1.VolumeStatus)
		for _, vs := range kvvmi.Status.VolumeStatus {
			kvvmiVolumeStatusByName[vs.Name] = vs
		}
	}

	attachedBlockDeviceRefs := make(map[nameKindKey]struct{})

	// 2. The kvvm already exists: populate block device refs with the kvvm volumes.
	for _, volume := range kvvm.Spec.Template.Spec.Volumes {
		bdName, kind := kvbuilder.GetOriginalDiskName(volume.Name)
		if kind == "" {
			// Reflect only vi, vd, or cvi block devices in status.
			// This is neither of them, so skip.
			continue
		}

		ref := h.getBlockDeviceStatusRef(kind, bdName)
		ref.Target, ref.Attached = h.getBlockDeviceTarget(volume, kvvmiVolumeStatusByName)
		ref.Size, err = h.getBlockDeviceRefSize(ctx, ref, s)
		if err != nil {
			return nil, err
		}
		ref.Hotplugged, err = h.isHotplugged(ctx, volume, kvvmiVolumeStatusByName, s)
		if err != nil {
			return nil, err
		}
		if ref.Hotplugged {
			ref.VirtualMachineBlockDeviceAttachmentName, err = h.getBlockDeviceAttachmentName(ctx, kind, bdName, s)
			if err != nil {
				return nil, err
			}
		}

		refs = append(refs, ref)
		attachedBlockDeviceRefs[nameKindKey{
			kind: ref.Kind,
			name: ref.Name,
		}] = struct{}{}
	}

	// 3. The kvvm may be missing some block devices from the spec; they need to be added as well.
	for _, specBlockDeviceRef := range s.VirtualMachine().Current().Spec.BlockDeviceRefs {
		_, ok := attachedBlockDeviceRefs[nameKindKey{
			kind: specBlockDeviceRef.Kind,
			name: specBlockDeviceRef.Name,
		}]
		if ok {
			continue
		}

		ref := h.getBlockDeviceStatusRef(specBlockDeviceRef.Kind, specBlockDeviceRef.Name)
		ref.Size, err = h.getBlockDeviceRefSize(ctx, ref, s)
		if err != nil {
			return nil, err
		}
		refs = append(refs, ref)
	}

	return refs, nil
}

// countReadyBlockDevices check if all attached images and disks are ready to use by the VM.
func (h *BlockDeviceHandler) countReadyBlockDevices(vm *virtv2.VirtualMachine, s BlockDevicesState) (int, bool, []string) {
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
				msg := fmt.Sprintf("Virtual disk %s is waiting for the underlying PVC to be bound", vd.Name)
				warnings = append(warnings, msg)
			}
		}
	}

	return ready, canStartKVVM, warnings
}

// setFinalizersOnBlockDevices sets protection finalizers on CVMI and VMD attached to the VM.
func (h *BlockDeviceHandler) setFinalizersOnBlockDevices(ctx context.Context, vm *virtv2.VirtualMachine, s BlockDevicesState) error {
	return h.updateFinalizers(ctx, vm, s, func(p *service.ProtectionService) func(ctx context.Context, objs ...client.Object) error {
		return p.AddProtection
	})
}

// removeFinalizersOnBlockDevices remove protection finalizers on CVI,VI and VMD attached to the VM.
func (h *BlockDeviceHandler) removeFinalizersOnBlockDevices(ctx context.Context, vm *virtv2.VirtualMachine, s BlockDevicesState) error {
	return h.updateFinalizers(ctx, vm, s, func(p *service.ProtectionService) func(ctx context.Context, objs ...client.Object) error {
		return p.RemoveProtection
	})
}

// updateFinalizers remove protection finalizers on CVI,VI and VD attached to the VM.
func (h *BlockDeviceHandler) updateFinalizers(ctx context.Context, vm *virtv2.VirtualMachine, s BlockDevicesState, update updaterProtection) error {
	if vm == nil {
		return fmt.Errorf("VM is empty")
	}
	for _, bd := range vm.Spec.BlockDeviceRefs {
		switch bd.Kind {
		case virtv2.ImageDevice:
			if vi, hasKey := s.VIByName[bd.Name]; hasKey {
				if err := update(h.viProtection)(ctx, vi); err != nil {
					return err
				}
			}
		case virtv2.ClusterImageDevice:
			if cvi, hasKey := s.CVIByName[bd.Name]; hasKey {
				if err := update(h.cviProtection)(ctx, cvi); err != nil {
					return err
				}
			}
		case virtv2.DiskDevice:
			if vd, hasKey := s.VDByName[bd.Name]; hasKey {
				if err := update(h.vdProtection)(ctx, vd); err != nil {
					return err
				}
			}
		default:
			return fmt.Errorf("unexpected block device kind %q. %w", bd.Kind, common.ErrUnknownType)
		}
	}
	return nil
}

func (h *BlockDeviceHandler) getBlockDeviceStatusRef(kind virtv2.BlockDeviceKind, name string) virtv2.BlockDeviceStatusRef {
	return virtv2.BlockDeviceStatusRef{
		Kind: kind,
		Name: name,
	}
}

type BlockDeviceGetter interface {
	VirtualDisk(ctx context.Context, name string) (*virtv2.VirtualDisk, error)
	VirtualImage(ctx context.Context, name string) (*virtv2.VirtualImage, error)
	ClusterVirtualImage(ctx context.Context, name string) (*virtv2.ClusterVirtualImage, error)
}

func (h *BlockDeviceHandler) getBlockDeviceRefSize(ctx context.Context, ref virtv2.BlockDeviceStatusRef, getter BlockDeviceGetter) (string, error) {
	switch ref.Kind {
	case virtv2.ImageDevice:
		vi, err := getter.VirtualImage(ctx, ref.Name)
		if err != nil {
			return "", err
		}

		if vi == nil {
			return "", nil
		}

		return vi.Status.Size.Unpacked, nil
	case virtv2.DiskDevice:
		vd, err := getter.VirtualDisk(ctx, ref.Name)
		if err != nil {
			return "", err
		}

		if vd == nil {
			return "", nil
		}

		return vd.Status.Capacity, nil
	case virtv2.ClusterImageDevice:
		cvi, err := getter.ClusterVirtualImage(ctx, ref.Name)
		if err != nil {
			return "", err
		}

		if cvi == nil {
			return "", nil
		}

		return cvi.Status.Size.Unpacked, nil
	}

	return "", nil
}

func (h *BlockDeviceHandler) getBlockDeviceTarget(volume virtv1.Volume, kvvmiVolumeStatusByName map[string]virtv1.VolumeStatus) (string, bool) {
	vs, ok := kvvmiVolumeStatusByName[volume.Name]
	if !ok {
		return "", false
	}

	return vs.Target, true
}

func (h *BlockDeviceHandler) isHotplugged(ctx context.Context, volume virtv1.Volume, kvvmiVolumeStatusByName map[string]virtv1.VolumeStatus, s state.VirtualMachineState) (bool, error) {
	switch {
	// 1. If kvvmi has volume status with hotplugVolume reference then it's 100% hot-plugged volume.
	case kvvmiVolumeStatusByName[volume.Name].HotplugVolume != nil:
		return true, nil

	// 2. If kvvm has volume with hot-pluggable pvc reference then it's 100% hot-plugged volume.
	case volume.PersistentVolumeClaim != nil && volume.PersistentVolumeClaim.Hotpluggable:
		return true, nil

	// 3. We cannot check volume.ContainerDisk.Hotpluggable, as this field was added in our patches and is not reflected in the api version of virtv1 used by us.
	// Until we have a 3rd-party repository to import the modified virtv1, we have to make decisions based on indirect signs.
	// If there was a previously hot-plugged block device and the VMBDA is still alive, then it's a hot-plugged block device.
	// TODO: Use volume.ContainerDisk.Hotpluggable for decision-making when the 3rd-party repository is available.
	case volume.ContainerDisk != nil:
		bdName, kind := kvbuilder.GetOriginalDiskName(volume.Name)
		if h.canBeHotPlugged(s.VirtualMachine().Current(), kind, bdName) {
			vmbdaName, err := h.getBlockDeviceAttachmentName(ctx, kind, bdName, s)
			if err != nil {
				return false, err
			}
			return vmbdaName != "", nil
		}
	}

	// 4. Is not hot-plugged.
	return false, nil
}

func (h *BlockDeviceHandler) getBlockDeviceAttachmentName(ctx context.Context, kind virtv2.BlockDeviceKind, bdName string, s state.VirtualMachineState) (string, error) {
	log := logger.FromContext(ctx).With(logger.SlogHandler(nameBlockDeviceHandler))

	vmbdasByRef, err := s.VirtualMachineBlockDeviceAttachments(ctx)
	if err != nil {
		return "", err
	}

	vmbdas := vmbdasByRef[virtv2.VMBDAObjectRef{
		Kind: virtv2.VMBDAObjectRefKind(kind),
		Name: bdName,
	}]

	switch len(vmbdas) {
	case 0:
		log.Error("No one vmbda was found for hot-plugged block device")
		return "", nil
	case 1:
		// OK.
	default:
		log.Error("Only one vmbda should be found for hot-plugged block device")
	}

	return vmbdas[0].Name, nil
}

func (h *BlockDeviceHandler) canBeHotPlugged(vm *virtv2.VirtualMachine, kind virtv2.BlockDeviceKind, bdName string) bool {
	for _, bdRef := range vm.Status.BlockDeviceRefs {
		if bdRef.Kind == kind && bdRef.Name == bdName {
			return bdRef.Hotplugged
		}
	}

	for _, bdRef := range vm.Spec.BlockDeviceRefs {
		if bdRef.Kind == kind && bdRef.Name == bdName {
			return false
		}
	}

	return true
}

func NewBlockDeviceState(s state.VirtualMachineState) BlockDevicesState {
	return BlockDevicesState{
		s:         s,
		VIByName:  make(map[string]*virtv2.VirtualImage),
		CVIByName: make(map[string]*virtv2.ClusterVirtualImage),
		VDByName:  make(map[string]*virtv2.VirtualDisk),
	}
}

type BlockDevicesState struct {
	s         state.VirtualMachineState
	VIByName  map[string]*virtv2.VirtualImage
	CVIByName map[string]*virtv2.ClusterVirtualImage
	VDByName  map[string]*virtv2.VirtualDisk
}

func (s *BlockDevicesState) Reload(ctx context.Context) error {
	viByName, err := s.s.VirtualImagesByName(ctx)
	if err != nil {
		return err
	}
	ciByName, err := s.s.ClusterVirtualImagesByName(ctx)
	if err != nil {
		return err
	}
	vdByName, err := s.s.VirtualDisksByName(ctx)
	if err != nil {
		return err
	}
	s.VIByName = viByName
	s.CVIByName = ciByName
	s.VDByName = vdByName
	return nil
}
