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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/common"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vm/internal/state"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
)

const nameBlockDeviceHandler = "BlockDeviceHandler"

func NewBlockDeviceHandler(cl client.Client, blockDeviceService BlockDeviceService) *BlockDeviceHandler {
	return &BlockDeviceHandler{
		client:             cl,
		blockDeviceService: blockDeviceService,

		viProtection:  service.NewProtectionService(cl, v1alpha2.FinalizerVIProtection),
		cviProtection: service.NewProtectionService(cl, v1alpha2.FinalizerCVIProtection),
		vdProtection:  service.NewProtectionService(cl, v1alpha2.FinalizerVDProtection),
	}
}

type BlockDeviceHandler struct {
	client             client.Client
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

func (h *BlockDeviceHandler) handleBlockDeviceLimit(ctx context.Context, vm *v1alpha2.VirtualMachine) (bool, error) {
	// Get number of connected block devices.
	// If it's greater than the limit, then set the condition to false.
	blockDeviceAttachedCount, err := h.blockDeviceService.CountBlockDevicesAttachedToVM(ctx, vm)
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

func (h *BlockDeviceHandler) getBlockDeviceWarnings(_ context.Context, _ state.VirtualMachineState) (string, error) {
	return "", nil
}

// setFinalizersOnBlockDevices sets protection finalizers on CVMI and VMD attached to the VM.
func (h *BlockDeviceHandler) setFinalizersOnBlockDevices(ctx context.Context, vm *v1alpha2.VirtualMachine, s BlockDevicesState) error {
	return h.updateFinalizers(ctx, vm, s, func(p *service.ProtectionService) func(ctx context.Context, objs ...client.Object) error {
		return p.AddProtection
	})
}

// removeFinalizersOnBlockDevices remove protection finalizers on CVI,VI and VMD attached to the VM.
func (h *BlockDeviceHandler) removeFinalizersOnBlockDevices(ctx context.Context, vm *v1alpha2.VirtualMachine, s BlockDevicesState) error {
	return h.updateFinalizers(ctx, vm, s, func(p *service.ProtectionService) func(ctx context.Context, objs ...client.Object) error {
		return p.RemoveProtection
	})
}

// updateFinalizers remove protection finalizers on CVI,VI and VD attached to the VM.
func (h *BlockDeviceHandler) updateFinalizers(ctx context.Context, vm *v1alpha2.VirtualMachine, s BlockDevicesState, update updaterProtection) error {
	if vm == nil {
		return fmt.Errorf("VM is empty")
	}
	for _, bd := range vm.Spec.BlockDeviceRefs {
		switch bd.Kind {
		case v1alpha2.ImageDevice:
			if vi, hasKey := s.VIByName[bd.Name]; hasKey {
				if err := update(h.viProtection)(ctx, vi); err != nil {
					return err
				}
			}
		case v1alpha2.ClusterImageDevice:
			if cvi, hasKey := s.CVIByName[bd.Name]; hasKey {
				if err := update(h.cviProtection)(ctx, cvi); err != nil {
					return err
				}
			}
		case v1alpha2.DiskDevice:
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
		VIByName:  make(map[string]*v1alpha2.VirtualImage),
		CVIByName: make(map[string]*v1alpha2.ClusterVirtualImage),
		VDByName:  make(map[string]*v1alpha2.VirtualDisk),
	}
}

type BlockDevicesState struct {
	s         state.VirtualMachineState
	VIByName  map[string]*v1alpha2.VirtualImage
	CVIByName map[string]*v1alpha2.ClusterVirtualImage
	VDByName  map[string]*v1alpha2.VirtualDisk
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
