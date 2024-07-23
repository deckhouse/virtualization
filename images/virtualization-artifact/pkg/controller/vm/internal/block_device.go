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
	"k8s.io/client-go/tools/record"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/common"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/kvbuilder"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vm/internal/state"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
)

const nameBlockDeviceHandler = "BlockDeviceHandler"

func NewBlockDeviceHandler(cl client.Client, recorder record.EventRecorder, logger *slog.Logger) *BlockDeviceHandler {
	return &BlockDeviceHandler{
		client:   cl,
		recorder: recorder,
		logger:   logger.With("handler", nameBlockDeviceHandler),

		viProtection:  service.NewProtectionService(cl, virtv2.FinalizerVIProtection),
		cviProtection: service.NewProtectionService(cl, virtv2.FinalizerCVIProtection),
		vdProtection:  service.NewProtectionService(cl, virtv2.FinalizerVDProtection),
	}
}

type BlockDeviceHandler struct {
	client   client.Client
	recorder record.EventRecorder
	logger   *slog.Logger

	viProtection  *service.ProtectionService
	cviProtection *service.ProtectionService
	vdProtection  *service.ProtectionService
}

func (h *BlockDeviceHandler) Handle(ctx context.Context, s state.VirtualMachineState) (reconcile.Result, error) {
	if s.VirtualMachine().IsEmpty() {
		return reconcile.Result{}, nil
	}
	current := s.VirtualMachine().Current()
	changed := s.VirtualMachine().Changed()

	if update := addAllUnknown(changed, vmcondition.TypeBlockDevicesReady.String()); update {
		return reconcile.Result{Requeue: true}, nil
	}

	mgr := conditions.NewManager(changed.Status.Conditions)
	cb := conditions.NewConditionBuilder2(vmcondition.TypeBlockDevicesReady).
		Generation(current.GetGeneration())

	disksMessage := h.checkBlockDevicesSanity(current)
	if !isDeletion(current) && disksMessage != "" {
		h.logger.Error(fmt.Sprintf("invalid disks: %s", disksMessage))
		mgr.Update(cb.
			Status(metav1.ConditionFalse).
			Reason2(vmcondition.ReasonBlockDevicesNotReady).
			Message(disksMessage).Condition())
		changed.Status.Conditions = mgr.Generate()
		return reconcile.Result{Requeue: true}, nil
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

	// Fill BlockDeviceRefs every time without knowledge of previously kept BlockDeviceRefs.
	changed.Status.BlockDeviceRefs = nil

	// Set BlockDeviceRef from spec to the status.
	for _, bdSpecRef := range changed.Spec.BlockDeviceRefs {
		bdStatusRef := h.getDiskStatusRef(bdSpecRef.Kind, bdSpecRef.Name)
		bdStatusRef.Size = h.getBlockDeviceSize(&bdStatusRef, bdState)

		changed.Status.BlockDeviceRefs = append(
			changed.Status.BlockDeviceRefs,
			bdStatusRef,
		)
	}

	vmbdas, err := s.VMBDAList(ctx)
	if err != nil {
		return reconcile.Result{}, err
	}

	// Set hot plugged BlockDeviceRef to the status.
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
			return reconcile.Result{}, err
		}

		if vd == nil {
			continue
		}

		bdStatusRef := h.getDiskStatusRef(virtv2.DiskDevice, vmbda.Spec.BlockDeviceRef.Name)
		bdStatusRef.Size = vd.Status.Capacity
		bdStatusRef.Hotplugged = true
		bdStatusRef.VirtualMachineBlockDeviceAttachmentName = vmbda.Name

		changed.Status.BlockDeviceRefs = append(
			changed.Status.BlockDeviceRefs,
			bdStatusRef,
		)
	}

	kvvmi, err := s.KVVMI(ctx)
	if err != nil {
		return reconcile.Result{}, err
	}

	// Sync BlockDeviceRef in the status with KVVMI volumes.
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

	// Update the BlockDevicesReady condition.
	countBD := len(current.Spec.BlockDeviceRefs)
	if ready, count := h.countReadyBlockDevices(current, bdState); !ready {
		// Wait until block devices are ready.
		h.logger.Info("Waiting for block devices to become available")
		reason := vmcondition.ReasonBlockDevicesNotReady.String()
		h.recorder.Event(changed, corev1.EventTypeNormal, reason, "Waiting for block devices to become available")
		msg := fmt.Sprintf("Waiting for block devices to become available: %d/%d", count, countBD)
		mgr.Update(cb.
			Status(metav1.ConditionFalse).
			Reason(reason).
			Message(msg).
			Condition())
		changed.Status.Conditions = mgr.Generate()
		return reconcile.Result{RequeueAfter: 60 * time.Second}, nil
	}

	mgr.Update(cb.Status(metav1.ConditionTrue).
		Reason2(vmcondition.ReasonBlockDevicesReady).
		Condition())
	changed.Status.Conditions = mgr.Generate()
	return reconcile.Result{}, nil
}

func (h *BlockDeviceHandler) Name() string {
	return nameBlockDeviceHandler
}

// checkBlockDevicesSanity compares spec.blockDevices and status.blockDevicesAttached.
// It returns false if the same disk contains in both arrays.
// It is a precaution to not apply changes in spec.blockDevices if disk is already
// hotplugged using the VMBDA resource. The reverse check is done by the vmbda-controller.
func (h *BlockDeviceHandler) checkBlockDevicesSanity(vm *virtv2.VirtualMachine) string {
	if vm == nil {
		return ""
	}
	disks := make([]string, 0)
	hotplugged := make(map[string]struct{})

	for _, bda := range vm.Status.BlockDeviceRefs {
		if bda.Hotplugged {
			hotplugged[bda.Name] = struct{}{}
		}
	}

	for _, bd := range vm.Spec.BlockDeviceRefs {
		if bd.Kind == virtv2.DiskDevice {
			if _, ok := hotplugged[bd.Name]; ok {
				disks = append(disks, bd.Name)
			}
		}
	}

	if len(disks) == 0 {
		return ""
	}
	return fmt.Sprintf("spec.blockDeviceRefs contain hotplugged disks: %s. Unplug or remove them from spec to continue.", strings.Join(disks, ", "))
}

// countReadyBlockDevices check if all attached images and disks are ready to use by the VM.
func (h *BlockDeviceHandler) countReadyBlockDevices(vm *virtv2.VirtualMachine, s BlockDevicesState) (bool, int) {
	if vm == nil {
		return false, 0
	}
	ready := 0
	for _, bd := range vm.Spec.BlockDeviceRefs {
		switch bd.Kind {
		case virtv2.ImageDevice:
			if vi, hasKey := s.VIByName[bd.Name]; hasKey {
				if vi.Status.Phase == virtv2.ImageReady {
					ready++
				}
			}
		case virtv2.ClusterImageDevice:
			if cvi, hasKey := s.CVIByName[bd.Name]; hasKey {
				if cvi.Status.Phase == virtv2.ImageReady {
					ready++
				}
			}
		case virtv2.DiskDevice:
			if vd, hasKey := s.VDByName[bd.Name]; hasKey {
				// we can attach vd to only one vm
				attachReady := len(vd.Status.AttachedToVirtualMachines) == 0
				for _, attached := range vd.Status.AttachedToVirtualMachines {
					if attached.Name != vm.GetName() {
						attachReady = false
						msg := fmt.Sprintf("VirtualDisk was attached to another virtual machine %q", attached.Name)
						h.logger.Warn(msg)
						h.recorder.Event(vm, corev1.EventTypeWarning, virtv2.ReasonVDAlreadyInUse, msg)
						break
					}
					attachReady = true
				}
				if vd.Status.Phase == virtv2.DiskReady && attachReady {
					ready++
				}
			}
		}
	}

	return len(vm.Spec.BlockDeviceRefs) == ready, ready
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
