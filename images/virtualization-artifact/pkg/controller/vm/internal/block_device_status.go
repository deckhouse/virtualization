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

	virtv1 "kubevirt.io/api/core/v1"

	"github.com/deckhouse/virtualization-controller/pkg/controller/kvbuilder"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vm/internal/state"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

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
