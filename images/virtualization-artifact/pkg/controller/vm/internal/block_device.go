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
	"log/slog"
	"strings"
	"time"

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

func NewBlockDeviceHandler(cl client.Client, recorder eventrecord.EventRecorderLogger, blockDeviceService IBlockDeviceService) *BlockDeviceHandler {
	return &BlockDeviceHandler{
		client:   cl,
		recorder: recorder,
		service:  blockDeviceService,

		viProtection:  service.NewProtectionService(cl, virtv2.FinalizerVIProtection),
		cviProtection: service.NewProtectionService(cl, virtv2.FinalizerCVIProtection),
		vdProtection:  service.NewProtectionService(cl, virtv2.FinalizerVDProtection),
	}
}

type BlockDeviceHandler struct {
	client   client.Client
	recorder eventrecord.EventRecorderLogger
	service  IBlockDeviceService

	viProtection  *service.ProtectionService
	cviProtection *service.ProtectionService
	vdProtection  *service.ProtectionService
}

func (h *BlockDeviceHandler) Handle(ctx context.Context, s state.VirtualMachineState) (reconcile.Result, error) {
	log := logger.FromContext(ctx).With(logger.SlogHandler(nameBlockDeviceHandler))

	if s.VirtualMachine().IsEmpty() {
		return reconcile.Result{}, nil
	}
	current := s.VirtualMachine().Current()
	changed := s.VirtualMachine().Changed()

	if update := addAllUnknown(changed, vmcondition.TypeBlockDevicesReady); update {
		return reconcile.Result{Requeue: true}, nil
	}

	//nolint:staticcheck
	mgr := conditions.NewManager(changed.Status.Conditions)
	cb := conditions.NewConditionBuilder(vmcondition.TypeBlockDevicesReady).
		Generation(current.GetGeneration())

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

	// Get number of connected block devices
	// If it greater limit then set condition to false
	blockDeviceAttachedCount, err := h.service.CountBlockDevicesAttachedToVm(ctx, changed)
	if err != nil {
		return reconcile.Result{}, err
	}

	if blockDeviceAttachedCount > common.VmBlockDeviceAttachedLimit {
		cb.
			Status(metav1.ConditionFalse).
			Reason(vmcondition.ReasonBlockDeviceCapacityReached).
			Message(fmt.Sprintf("Can not attach %d block devices (%d is maximum) to `VirtualMachine` %q", blockDeviceAttachedCount, common.VmBlockDeviceAttachedLimit, changed.Name))
	}

	// Get hot plugged BlockDeviceRefs from vmbdas.
	vmbdaStatusRefs, err := h.getBlockDeviceStatusRefsFromVMBDA(ctx, s)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("failed to get hotplugged block devices: %w", err)
	}

	// Get BlockDeviceRefs from spec.
	bdStatusRefs, conflictWarning := h.getBlockDeviceStatusRefsFromSpec(current, bdState, vmbdaStatusRefs)

	// Fill BlockDeviceRefs every time without knowledge of previously kept BlockDeviceRefs.
	changed.Status.BlockDeviceRefs = bdStatusRefs
	changed.Status.BlockDeviceRefs = append(changed.Status.BlockDeviceRefs, vmbdaStatusRefs...)

	kvvmi, err := s.KVVMI(ctx)
	if err != nil {
		return reconcile.Result{}, err
	}

	// Sync BlockDeviceRefs in the status with KVVMI volumes.
	if kvvmi != nil {
		for i, bdStatusRef := range changed.Status.BlockDeviceRefs {
			vs := h.findVolumeStatus(GenerateDiskName(bdStatusRef.Kind, bdStatusRef.Name), kvvmi)
			if vs == nil || (vs.Phase != "" && vs.Phase != virtv1.VolumeReady) {
				continue
			}

			changed.Status.BlockDeviceRefs[i].Target = vs.Target
			changed.Status.BlockDeviceRefs[i].Attached = true
		}
	}

	// Update the BlockDevicesReady condition if there are conflicted virtual disks.
	if conflictWarning != "" {
		log.Info(fmt.Sprintf("Conflicted virtual disks: %s", conflictWarning))

		mgr.Update(cb.Status(metav1.ConditionFalse).
			Reason(vmcondition.ReasonBlockDevicesNotReady).
			Message(conflictWarning).Condition())
		changed.Status.Conditions = mgr.Generate()
		return reconcile.Result{Requeue: true}, nil
	}

	// Update the BlockDevicesReady condition.
	if readyCount, canStartKVVM, warnings := h.countReadyBlockDevices(current, bdState, log); len(current.Spec.BlockDeviceRefs) != readyCount {
		var reason vmcondition.Reason

		msg := fmt.Sprintf("Waiting for block devices to become ready: %d/%d", readyCount, len(current.Spec.BlockDeviceRefs))
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
		mgr.Update(cb.Status(metav1.ConditionFalse).
			Reason(reason).
			Message(msg).
			Condition())
		changed.Status.Conditions = mgr.Generate()
		return reconcile.Result{RequeueAfter: 60 * time.Second}, nil
	}

	vds, err := s.VirtualDisksByName(ctx)
	if err != nil {
		return reconcile.Result{}, err
	}

	if !h.areVirtualDisksAllowedToUse(vds) {
		mgr.Update(cb.Status(metav1.ConditionFalse).
			Reason(vmcondition.ReasonBlockDevicesNotReady).
			Message("Virtual disks cannot be used because they are being used for creating an image.").Condition())
		changed.Status.Conditions = mgr.Generate()
		return reconcile.Result{}, nil
	}

	mgr.Update(cb.Status(metav1.ConditionTrue).
		Reason(vmcondition.ReasonBlockDevicesReady).
		Condition())
	changed.Status.Conditions = mgr.Generate()
	return reconcile.Result{}, nil
}

func (h *BlockDeviceHandler) areVirtualDisksAllowedToUse(vds map[string]*virtv2.VirtualDisk) bool {
	for _, vd := range vds {
		inUseCondition, _ := conditions.GetCondition(vdcondition.InUseType, vd.Status.Conditions)
		if inUseCondition.Status != metav1.ConditionTrue ||
			inUseCondition.Reason != vdcondition.AttachedToVirtualMachine.String() ||
			inUseCondition.ObservedGeneration != vd.Generation {
			return false
		}
	}

	return true
}

func (h *BlockDeviceHandler) Name() string {
	return nameBlockDeviceHandler
}

func (h *BlockDeviceHandler) getBlockDeviceStatusRefsFromSpec(vm *virtv2.VirtualMachine, bdState BlockDevicesState, hotplugs []virtv2.BlockDeviceStatusRef) ([]virtv2.BlockDeviceStatusRef, string) {
	hotplugsByName := make(map[string]struct{}, len(hotplugs))
	for _, hotplug := range hotplugs {
		hotplugsByName[hotplug.Name] = struct{}{}
	}

	var conflictedRefs []string
	var refs []virtv2.BlockDeviceStatusRef

	for _, bdSpecRef := range vm.Spec.BlockDeviceRefs {
		// It is a precaution to not apply changes in spec.blockDeviceRefs if disk is already
		// hotplugged using the VMBDA resource or plugged in Spec of another VM.
		// spec check is done by VirtualDisk status
		// the reverse check is done by the vmbda-controller.
		if bdSpecRef.Kind == virtv2.DiskDevice {
			vd, hasKey := bdState.VDByName[bdSpecRef.Name]

			switch {
			case !hasKey:
				continue // can't attach not existing disk, waiting
			case len(vd.Status.AttachedToVirtualMachines) == 0: // Not connected to another VM, don't skip
			case len(vd.Status.AttachedToVirtualMachines) == 1:
				if vd.Status.AttachedToVirtualMachines[0].Name != vm.Name {
					conflictedRefs = append(conflictedRefs, bdSpecRef.Name)
					continue
				}
			default:
				conflictedRefs = append(conflictedRefs, bdSpecRef.Name)
				continue
			}

			if _, conflict := hotplugsByName[bdSpecRef.Name]; conflict {
				conflictedRefs = append(conflictedRefs, bdSpecRef.Name)
				continue
			}
		}

		bdStatusRef := h.getDiskStatusRef(bdSpecRef.Kind, bdSpecRef.Name)
		bdStatusRef.Size = h.getBlockDeviceSize(&bdStatusRef, bdState)

		refs = append(refs, bdStatusRef)
	}

	var warning string
	if len(conflictedRefs) > 0 {
		warning = fmt.Sprintf("spec.blockDeviceRefs field contains hotplugged disks (%s): unplug or remove them from spec to continue.", strings.Join(conflictedRefs, ", "))
	}

	return refs, warning
}

func (h *BlockDeviceHandler) getBlockDeviceStatusRefsFromVMBDA(ctx context.Context, s state.VirtualMachineState) ([]virtv2.BlockDeviceStatusRef, error) {
	vmbdas, err := s.VMBDAList(ctx)
	if err != nil {
		return nil, err
	}

	var refs []virtv2.BlockDeviceStatusRef

	for _, vmbda := range vmbdas {
		switch vmbda.Status.Phase {
		case virtv2.BlockDeviceAttachmentPhaseInProgress,
			virtv2.BlockDeviceAttachmentPhaseAttached:
		default:
			continue
		}

		var vd *virtv2.VirtualDisk
		vd, err = s.VirtualDisk(ctx, vmbda.Spec.BlockDeviceRef.Name)
		if err != nil {
			return nil, err
		}

		if vd == nil {
			continue
		}

		bdStatusRef := h.getDiskStatusRef(virtv2.DiskDevice, vmbda.Spec.BlockDeviceRef.Name)
		bdStatusRef.Size = vd.Status.Capacity
		bdStatusRef.Hotplugged = true
		bdStatusRef.VirtualMachineBlockDeviceAttachmentName = vmbda.Name

		refs = append(refs, bdStatusRef)
	}

	return refs, nil
}

// countReadyBlockDevices check if all attached images and disks are ready to use by the VM.
func (h *BlockDeviceHandler) countReadyBlockDevices(vm *virtv2.VirtualMachine, s BlockDevicesState, log *slog.Logger) (int, bool, []string) {
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

			var canAttach bool

			switch {
			case len(vd.Status.AttachedToVirtualMachines) == 0:
				canAttach = true
			case len(vd.Status.AttachedToVirtualMachines) == 1:
				if vd.Status.AttachedToVirtualMachines[0].Name != vm.GetName() {
					canAttach = false
					msg := fmt.Sprintf("unable to attach virtual disk %s because it is already attached to another virtual machine %s", vd.Name, vd.Status.AttachedToVirtualMachines[0].Name)
					warnings = append(warnings, msg)
					h.recorder.Event(vm, corev1.EventTypeWarning, virtv2.ReasonVDAlreadyInUse, msg)
				} else {
					canAttach = true
				}
			default:
				canAttach = false
				msg := fmt.Sprintf("unable to attach virtual disk %s because it is currently attached to multiple virtual machines", vd.Name)
				warnings = append(warnings, msg)
				log.Error(msg)
			}

			if !canAttach || vd.Status.Target.PersistentVolumeClaim == "" {
				canStartKVVM = false
				continue
			}
			readyCondition, _ := conditions.GetCondition(vdcondition.ReadyType, vd.Status.Conditions)
			if readyCondition.Status == metav1.ConditionTrue {
				ready++
			} else {
				msg := fmt.Sprintf("virtual disk %s is waiting for the it's pvc to be bound", vd.Name)
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

func (h *BlockDeviceHandler) getDiskStatusRef(kind virtv2.BlockDeviceKind, name string) virtv2.BlockDeviceStatusRef {
	return virtv2.BlockDeviceStatusRef{
		Kind: kind,
		Name: name,
	}
}

func (h *BlockDeviceHandler) getBlockDeviceSize(ref *virtv2.BlockDeviceStatusRef, state BlockDevicesState) string {
	switch ref.Kind {
	case virtv2.ImageDevice:
		vi, hasVI := state.VIByName[ref.Name]
		if !hasVI {
			return ""
		}

		return vi.Status.Size.Unpacked
	case virtv2.DiskDevice:
		vd, hasVI := state.VDByName[ref.Name]
		if !hasVI {
			return ""
		}

		return vd.Status.Capacity
	case virtv2.ClusterImageDevice:
		cvi, hasCvi := state.CVIByName[ref.Name]
		if !hasCvi {
			return ""
		}

		return cvi.Status.Size.Unpacked
	}

	return ""
}

func (h *BlockDeviceHandler) findVolumeStatus(name string, kvvmi *virtv1.VirtualMachineInstance) *virtv1.VolumeStatus {
	if kvvmi == nil {
		return nil
	}

	for i := range kvvmi.Status.VolumeStatus {
		vs := kvvmi.Status.VolumeStatus[i]
		if vs.Name == name {
			return &vs
		}
	}

	return nil
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

func GenerateDiskName(kind virtv2.BlockDeviceKind, name string) string {
	switch kind {
	case virtv2.ImageDevice:
		return kvbuilder.GenerateVMIDiskName(name)
	case virtv2.ClusterImageDevice:
		return kvbuilder.GenerateCVMIDiskName(name)
	case virtv2.DiskDevice:
		return kvbuilder.GenerateVMDDiskName(name)
	default:
		return ""
	}
}
