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
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/kvbuilder"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vm/internal/state"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
)

const nameHotplugHandler = "HotplugHandler"

type hotplugService interface {
	HotPlugDisk(ctx context.Context, ad *service.AttachmentDisk, vm *v1alpha2.VirtualMachine, kvvm *virtv1.VirtualMachine) error
	UnplugDisk(ctx context.Context, kvvm *virtv1.VirtualMachine, diskName string) error
}

func NewHotplugHandler(svc hotplugService) *HotplugHandler {
	return &HotplugHandler{svc: svc}
}

type HotplugHandler struct {
	svc hotplugService
}

func (h *HotplugHandler) Handle(ctx context.Context, s state.VirtualMachineState) (reconcile.Result, error) {
	log := logger.FromContext(ctx).With(logger.SlogHandler(nameHotplugHandler))

	if s.VirtualMachine().IsEmpty() {
		return reconcile.Result{}, nil
	}

	current := s.VirtualMachine().Current()

	if isDeletion(current) {
		return reconcile.Result{}, nil
	}

	kvvmi, err := s.KVVMI(ctx)
	if err != nil {
		return reconcile.Result{}, err
	}
	if kvvmi == nil {
		return reconcile.Result{}, nil
	}

	if current.Status.Phase == v1alpha2.MachineMigrating {
		log.Info("VM is migrating, skip hotplug")
		return reconcile.Result{RequeueAfter: 30 * time.Second}, nil
	}

	bdReady, ok := conditions.GetCondition(vmcondition.TypeBlockDevicesReady, current.Status.Conditions)
	if ok && bdReady.Status != metav1.ConditionTrue {
		return reconcile.Result{}, nil
	}

	kvvm, err := s.KVVM(ctx)
	if err != nil || kvvm == nil {
		return reconcile.Result{}, err
	}

	specDevices := make(map[nameKindKey]v1alpha2.BlockDeviceSpecRef)
	for _, bd := range current.Spec.BlockDeviceRefs {
		specDevices[nameKindKey{kind: bd.Kind, name: bd.Name}] = bd
	}

	type kvvmVolume struct {
		name         string
		hotpluggable bool
	}
	kvvmDevices := make(map[nameKindKey]kvvmVolume)
	if kvvm.Spec.Template != nil {
		for _, vol := range kvvm.Spec.Template.Spec.Volumes {
			name, kind := kvbuilder.GetOriginalDiskName(vol.Name)
			if kind == "" {
				continue
			}
			hp := (vol.PersistentVolumeClaim != nil && vol.PersistentVolumeClaim.Hotpluggable) ||
				(vol.ContainerDisk != nil && vol.ContainerDisk.Hotpluggable)
			kvvmDevices[nameKindKey{kind: kind, name: name}] = kvvmVolume{name: vol.Name, hotpluggable: hp}
		}
	}

	pendingRequests := make(map[string]struct{})
	for _, vr := range kvvm.Status.VolumeRequests {
		if vr.AddVolumeOptions != nil {
			pendingRequests[vr.AddVolumeOptions.Name] = struct{}{}
		}
		if vr.RemoveVolumeOptions != nil {
			pendingRequests[vr.RemoveVolumeOptions.Name] = struct{}{}
		}
	}

	bdState := NewBlockDeviceState(s)
	if err = bdState.Reload(ctx); err != nil {
		return reconcile.Result{}, fmt.Errorf("failed to reload block device state: %w", err)
	}

	var errs []error

	for key := range specDevices {
		if _, attached := kvvmDevices[key]; attached {
			continue
		}

		var volumeName string
		switch key.kind {
		case v1alpha2.DiskDevice:
			volumeName = kvbuilder.GenerateVDDiskName(key.name)
		case v1alpha2.ImageDevice:
			volumeName = kvbuilder.GenerateVIDiskName(key.name)
		case v1alpha2.ClusterImageDevice:
			volumeName = kvbuilder.GenerateCVIDiskName(key.name)
		}

		if _, pending := pendingRequests[volumeName]; pending {
			continue
		}

		ad := h.buildAttachmentDisk(key, bdState)
		if ad == nil {
			log.Info("Block device not ready for hotplug", "kind", key.kind, "name", key.name)
			continue
		}

		if err = h.svc.HotPlugDisk(ctx, ad, current, kvvm); err != nil {
			errs = append(errs, fmt.Errorf("hotplug %s/%s: %w", key.kind, key.name, err))
		}
	}

	for key, vol := range kvvmDevices {
		if _, inSpec := specDevices[key]; inSpec {
			continue
		}

		if !vol.hotpluggable {
			continue
		}

		if _, pending := pendingRequests[vol.name]; pending {
			continue
		}

		if err = h.svc.UnplugDisk(ctx, kvvm, vol.name); err != nil {
			errs = append(errs, fmt.Errorf("unplug %s/%s: %w", key.kind, key.name, err))
		}
	}

	if len(errs) > 0 {
		return reconcile.Result{}, fmt.Errorf("hotplug errors: %v", errs)
	}

	return reconcile.Result{}, nil
}

func (h *HotplugHandler) Name() string {
	return nameHotplugHandler
}

func (h *HotplugHandler) buildAttachmentDisk(key nameKindKey, bdState BlockDevicesState) *service.AttachmentDisk {
	switch key.kind {
	case v1alpha2.DiskDevice:
		vd, ok := bdState.VDByName[key.name]
		if !ok || vd == nil || vd.Status.Target.PersistentVolumeClaim == "" {
			return nil
		}
		return service.NewAttachmentDiskFromVirtualDisk(vd)
	case v1alpha2.ImageDevice:
		vi, ok := bdState.VIByName[key.name]
		if !ok || vi == nil {
			return nil
		}
		return service.NewAttachmentDiskFromVirtualImage(vi)
	case v1alpha2.ClusterImageDevice:
		cvi, ok := bdState.CVIByName[key.name]
		if !ok || cvi == nil {
			return nil
		}
		return service.NewAttachmentDiskFromClusterVirtualImage(cvi)
	}
	return nil
}
