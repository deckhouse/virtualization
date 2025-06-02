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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/common"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vm/internal/state"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
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
	errBlockDeviceNotReady            = errors.New("block device not ready")
	errBlockDeviceWaitForProvisioning = errors.New("block device wait for provisioning")
)

func (h *BlockDeviceHandler) Handle(ctx context.Context, s state.VirtualMachineState) (reconcile.Result, error) {
	log := logger.FromContext(ctx).With(logger.SlogHandler(nameBlockDeviceHandler))

	if s.VirtualMachine().IsEmpty() {
		return reconcile.Result{}, nil
	}
	current := s.VirtualMachine().Current()
	changed := s.VirtualMachine().Changed()

	_, ok := conditions.GetCondition(vmcondition.TypeBlockDevicesReady, changed.Status.Conditions)
	if !ok {
		conditions.SetCondition(
			conditions.NewConditionBuilder(vmcondition.TypeBlockDevicesReady).
				Status(metav1.ConditionUnknown).
				Reason(conditions.ReasonUnknown).
				Generation(changed.Generation),
			&changed.Status.Conditions,
		)
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

	var shouldStop bool
	shouldStop, err = h.handleBlockDeviceLimit(ctx, changed)
	if err != nil {
		return reconcile.Result{}, err
	}

	if shouldStop {
		return reconcile.Result{}, nil
	}

	changed.Status.BlockDeviceRefs, err = h.getBlockDeviceStatusRefs(ctx, s)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("failed to get block device status refs: %w", err)
	}

	shouldStop, err = h.handleBlockDeviceConflicts(ctx, s, log)
	if err != nil {
		return reconcile.Result{}, err
	}

	if shouldStop {
		return reconcile.Result{}, nil
	}

	readyErr := h.handleBlockDevicesReady(ctx, s, bdState)
	switch {
	case errors.Is(readyErr, errBlockDeviceWaitForProvisioning):
		// No action needed for ErrBlockDeviceWaitForProvisioning
	case errors.Is(readyErr, errBlockDeviceNotReady):
		return reconcile.Result{}, nil
	case readyErr != nil:
		return reconcile.Result{}, readyErr
	}

	shouldStop, err = h.handleVirtualDisksReadyForUse(ctx, s)
	if err != nil {
		return reconcile.Result{}, err
	}

	if shouldStop {
		return reconcile.Result{}, nil
	}

	if !errors.Is(readyErr, errBlockDeviceWaitForProvisioning) {
		h.setConditionReady(s.VirtualMachine().Changed())
	}

	return reconcile.Result{}, nil
}

func (h *BlockDeviceHandler) handleBlockDeviceConflicts(ctx context.Context, s state.VirtualMachineState, log *slog.Logger) (bool, error) {
	changed := s.VirtualMachine().Changed()

	conflictWarning, err := h.getBlockDeviceWarnings(ctx, s)
	if err != nil {
		return false, fmt.Errorf("failed to get hotplugged block devices: %w", err)
	}

	// Update the BlockDevicesReady condition if there are conflicted virtual disks.
	if conflictWarning != "" {
		log.Info(fmt.Sprintf("Conflicted virtual disks: %s", conflictWarning))
		cd := conditions.NewConditionBuilder(vmcondition.TypeBlockDevicesReady).Status(metav1.ConditionFalse).
			Reason(vmcondition.ReasonBlockDevicesNotReady).
			Message(conflictWarning).
			Generation(changed.Generation)
		conditions.SetCondition(cd, &changed.Status.Conditions)
		return true, nil
	}

	return false, nil
}

func (h *BlockDeviceHandler) handleBlockDeviceLimit(ctx context.Context, vm *virtv2.VirtualMachine) (bool, error) {
	// Get number of connected block devices.
	// If it's greater than the limit, then set the condition to false.
	blockDeviceAttachedCount, err := h.blockDeviceService.CountBlockDevicesAttachedToVm(ctx, vm)
	if err != nil {
		return false, err
	}

	if blockDeviceAttachedCount > common.VMBlockDeviceAttachedLimit {
		conditions.SetCondition(
			conditions.NewConditionBuilder(vmcondition.TypeBlockDevicesReady).
				Status(metav1.ConditionFalse).
				Reason(vmcondition.ReasonBlockDeviceLimitExceeded).
				Message(fmt.Sprintf("Cannot attach %d block devices (%d is maximum) to VirtualMachine %q", blockDeviceAttachedCount, common.VMBlockDeviceAttachedLimit, vm.Name)).
				Generation(vm.Generation),
			&vm.Status.Conditions,
		)
		return true, nil
	}

	return false, nil
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
