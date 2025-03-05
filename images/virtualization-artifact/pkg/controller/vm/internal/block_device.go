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

	// Get number of connected block devices.
	// If it's greater than the limit, then set the condition to false.
	blockDeviceAttachedCount, err := h.blockDeviceService.CountBlockDevicesAttachedToVm(ctx, changed)
	if err != nil {
		return reconcile.Result{}, err
	}

	if blockDeviceAttachedCount > common.VmBlockDeviceAttachedLimit {
		cb.
			Status(metav1.ConditionFalse).
			Reason(vmcondition.ReasonBlockDeviceLimitExceeded).
			Message(fmt.Sprintf("Cannot attach %d block devices (%d is maximum) to VirtualMachine %q", blockDeviceAttachedCount, common.VmBlockDeviceAttachedLimit, changed.Name))
		mgr.Update(cb.Condition())
		changed.Status.Conditions = mgr.Generate()
		return reconcile.Result{}, nil
	}

	// Fill BlockDeviceRefs every time without knowledge of previously kept BlockDeviceRefs.
	changed.Status.BlockDeviceRefs, err = h.getBlockDeviceStatusRefs(ctx, s)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("failed to get block device status refs: %w", err)
	}

	// There is no need to set block device refs acquired here to the status now,
	// as they will be set to the status by the new method `getBlockDeviceStatusRefs` above.
	// It hasn't been refactored now because a new PR, which will completely refactor this handler, will be merged soon.
	vmbdaRefs, err := h.getBlockDeviceStatusRefsFromVMBDA(ctx, s)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("failed to get hotplugged block devices: %w", err)
	}

	conflictWarning := h.getBlockDeviceWarnings(current, bdState, vmbdaRefs)

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

	allowedVdCount := h.areVirtualDisksAllowedToUse(vds)
	if len(vds) != allowedVdCount {
		mgr.Update(cb.Status(metav1.ConditionFalse).
			Reason(vmcondition.ReasonBlockDevicesNotReady).
			Message(fmt.Sprintf("Waiting for virtual disks to become allowed for use: %d/%d", allowedVdCount, len(vds))).Condition())
		changed.Status.Conditions = mgr.Generate()
		return reconcile.Result{}, nil
	}

	mgr.Update(cb.Status(metav1.ConditionTrue).
		Reason(vmcondition.ReasonBlockDevicesReady).
		Condition())
	changed.Status.Conditions = mgr.Generate()
	return reconcile.Result{}, nil
}

func (h *BlockDeviceHandler) areVirtualDisksAllowedToUse(vds map[string]*virtv2.VirtualDisk) int {
	var allowedCount int
	for _, vd := range vds {
		inUseCondition, _ := conditions.GetCondition(vdcondition.InUseType, vd.Status.Conditions)
		if inUseCondition.Status == metav1.ConditionTrue &&
			inUseCondition.Reason == vdcondition.AttachedToVirtualMachine.String() &&
			inUseCondition.ObservedGeneration == vd.Generation {
			allowedCount++
		}
	}

	return allowedCount
}

func (h *BlockDeviceHandler) Name() string {
	return nameBlockDeviceHandler
}

func (h *BlockDeviceHandler) getBlockDeviceWarnings(vm *virtv2.VirtualMachine, bdState BlockDevicesState, hotplugs []virtv2.BlockDeviceStatusRef) string {
	hotplugsByName := make(map[string]struct{}, len(hotplugs))
	for _, hotplug := range hotplugs {
		hotplugsByName[hotplug.Name] = struct{}{}
	}

	var conflictedRefs []string

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
	}

	var warning string
	if len(conflictedRefs) > 0 {
		warning = fmt.Sprintf("spec.blockDeviceRefs field contains hotplugged disks (%s): unplug or remove them from spec to continue.", strings.Join(conflictedRefs, ", "))
	}

	return warning
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

// Deprecated. It will be removed soon.
func (h *BlockDeviceHandler) getBlockDeviceStatusRefsFromVMBDA(ctx context.Context, s state.VirtualMachineState) ([]virtv2.BlockDeviceStatusRef, error) {
	vmbdasByBlockDevice, err := s.VirtualMachineBlockDeviceAttachments(ctx)
	if err != nil {
		return nil, err
	}

	var refs []virtv2.BlockDeviceStatusRef

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
					return nil, err
				}

				if vd == nil {
					continue
				}

				bdStatusRef = h.getBlockDeviceStatusRef(virtv2.DiskDevice, vmbda.Spec.BlockDeviceRef.Name)
				bdStatusRef.Size = vd.Status.Capacity
			case virtv2.VMBDAObjectRefKindVirtualImage:
				vi, err = s.VirtualImage(ctx, vmbda.Spec.BlockDeviceRef.Name)
				if err != nil {
					return nil, err
				}

				if vi == nil {
					continue
				}

				bdStatusRef = h.getBlockDeviceStatusRef(virtv2.ImageDevice, vmbda.Spec.BlockDeviceRef.Name)
				bdStatusRef.Size = vi.Status.Size.Unpacked

			case virtv2.VMBDAObjectRefKindClusterVirtualImage:
				cvi, err = s.ClusterVirtualImage(ctx, vmbda.Spec.BlockDeviceRef.Name)
				if err != nil {
					return nil, err
				}

				if cvi == nil {
					continue
				}

				bdStatusRef = h.getBlockDeviceStatusRef(virtv2.ClusterImageDevice, vmbda.Spec.BlockDeviceRef.Name)
				bdStatusRef.Size = cvi.Status.Size.Unpacked
			default:
				return nil, fmt.Errorf("unacceptable `Kind` of `BlockDeviceRef`: %s", vmbda.Spec.BlockDeviceRef.Kind)
			}

			bdStatusRef.Hotplugged = true
			bdStatusRef.VirtualMachineBlockDeviceAttachmentName = vmbda.Name

			refs = append(refs, bdStatusRef)
		}
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
