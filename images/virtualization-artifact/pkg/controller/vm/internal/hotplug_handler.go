/*
Copyright 2026 Flant JSC

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

	if s.VirtualMachine().IsEmpty() || isDeletion(s.VirtualMachine().Current()) {
		return reconcile.Result{}, nil
	}

	current := s.VirtualMachine().Current()

	kvvmi, err := s.KVVMI(ctx)
	if err != nil || kvvmi == nil {
		return reconcile.Result{}, err
	}

	if current.Status.Phase == v1alpha2.MachineMigrating {
		log.Info("VM is migrating, skip hotplug")
		return reconcile.Result{}, nil
	}

	if bdReady, ok := conditions.GetCondition(vmcondition.TypeBlockDevicesReady, current.Status.Conditions); ok && bdReady.Status != metav1.ConditionTrue {
		return reconcile.Result{}, nil
	}

	kvvm, err := s.KVVM(ctx)
	if err != nil || kvvm == nil {
		return reconcile.Result{}, err
	}

	specDevices := make(map[nameKindKey]struct{})
	for _, bd := range current.Spec.BlockDeviceRefs {
		specDevices[nameKindKey{kind: bd.Kind, name: bd.Name}] = struct{}{}
	}

	kvvmDevices, pending := parseKVVMVolumes(kvvm)

	var errs []error

	// 1. Hotplugging
	for key := range specDevices {
		volName := generateVolumeName(key)
		if _, onKVVM := kvvmDevices[key]; onKVVM {
			continue
		}
		if _, ok := pending[volName]; ok {
			continue
		}

		ad, adErr := h.buildAttachmentDisk(ctx, key, s)
		if adErr != nil {
			errs = append(errs, fmt.Errorf("build attachment disk %s/%s: %w", key.kind, key.name, adErr))
			continue
		}
		if ad == nil {
			log.Info("Block device not ready for hotplug", "kind", key.kind, "name", key.name)
			continue
		}

		if err = h.svc.HotPlugDisk(ctx, ad, current, kvvm); err != nil {
			errs = append(errs, fmt.Errorf("hotplug %s/%s: %w", key.kind, key.name, err))
		}
	}

	// 2. Unplugging
	for key, vol := range kvvmDevices {
		if _, wanted := specDevices[key]; wanted || !vol.hotpluggable {
			continue
		}
		if _, ok := pending[vol.name]; ok {
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

type kvvmVolume struct {
	name         string
	hotpluggable bool
}

func parseKVVMVolumes(kvvm *virtv1.VirtualMachine) (map[nameKindKey]kvvmVolume, map[string]struct{}) {
	devices := make(map[nameKindKey]kvvmVolume)
	pending := make(map[string]struct{})

	if kvvm.Spec.Template != nil {
		for _, vol := range kvvm.Spec.Template.Spec.Volumes {
			name, kind := kvbuilder.GetOriginalDiskName(vol.Name)
			if kind == "" {
				continue
			}
			hp := (vol.PersistentVolumeClaim != nil && vol.PersistentVolumeClaim.Hotpluggable) ||
				(vol.ContainerDisk != nil && vol.ContainerDisk.Hotpluggable)
			devices[nameKindKey{kind: kind, name: name}] = kvvmVolume{name: vol.Name, hotpluggable: hp}
		}
	}

	for _, vr := range kvvm.Status.VolumeRequests {
		if vr.AddVolumeOptions != nil {
			pending[vr.AddVolumeOptions.Name] = struct{}{}
		}
		if vr.RemoveVolumeOptions != nil {
			pending[vr.RemoveVolumeOptions.Name] = struct{}{}
		}
	}

	return devices, pending
}

func generateVolumeName(key nameKindKey) string {
	return kvbuilder.GenerateDiskName(key.kind, key.name)
}

func (h *HotplugHandler) buildAttachmentDisk(ctx context.Context, key nameKindKey, s state.VirtualMachineState) (*service.AttachmentDisk, error) {
	switch key.kind {
	case v1alpha2.DiskDevice:
		vd, err := s.VirtualDisk(ctx, key.name)
		if err != nil {
			return nil, err
		}
		if vd == nil || vd.Status.Target.PersistentVolumeClaim == "" {
			return nil, nil
		}
		return service.NewAttachmentDiskFromVirtualDisk(vd), nil
	case v1alpha2.ImageDevice:
		vi, err := s.VirtualImage(ctx, key.name)
		if err != nil {
			return nil, err
		}
		if vi == nil {
			return nil, nil
		}
		return service.NewAttachmentDiskFromVirtualImage(vi), nil
	case v1alpha2.ClusterImageDevice:
		cvi, err := s.ClusterVirtualImage(ctx, key.name)
		if err != nil {
			return nil, err
		}
		if cvi == nil {
			return nil, nil
		}
		return service.NewAttachmentDiskFromClusterVirtualImage(cvi), nil
	}
	return nil, nil
}
