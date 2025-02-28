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

package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	"github.com/deckhouse/virtualization-controller/pkg/controller/kvapi"
	"github.com/deckhouse/virtualization-controller/pkg/controller/kvbuilder"
	"github.com/deckhouse/virtualization/api/client/kubeclient"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/subresources/v1alpha2"
)

type AttachmentService struct {
	client              Client
	virtClient          kubeclient.Client
	controllerNamespace string
}

func NewAttachmentService(client Client, virtClient kubeclient.Client, controllerNamespace string) *AttachmentService {
	return &AttachmentService{
		client:              client,
		virtClient:          virtClient,
		controllerNamespace: controllerNamespace,
	}
}

var (
	ErrVolumeStatusNotReady      = errors.New("hotplug is not ready")
	ErrBlockDeviceIsSpecAttached = errors.New("block device is already attached to the virtual machine spec")
	ErrHotPlugRequestAlreadySent = errors.New("attachment request is already sent")
)

func (s AttachmentService) IsHotPlugged(ad *AttachmentDisk, vm *virtv2.VirtualMachine, kvvmi *virtv1.VirtualMachineInstance) (bool, error) {
	if ad == nil {
		return false, errors.New("cannot check if a empty AttachmentDisk is hot plugged")
	}

	if vm == nil {
		return false, errors.New("cannot check if a disk is hot plugged into a nil VirtualMachine")
	}

	if kvvmi == nil {
		return false, errors.New("cannot check if a disk is hot plugged into a nil KVVMI")
	}

	for _, vs := range kvvmi.Status.VolumeStatus {
		if vs.HotplugVolume != nil && vs.Name == ad.GenerateName {
			if vs.Phase == virtv1.VolumeReady {
				return true, nil
			}

			return false, fmt.Errorf("%w: %s", ErrVolumeStatusNotReady, vs.Message)
		}
	}

	return false, nil
}

func (s AttachmentService) CanHotPlug(ad *AttachmentDisk, vm *virtv2.VirtualMachine, kvvm *virtv1.VirtualMachine) (bool, error) {
	if ad == nil {
		return false, errors.New("cannot hot plug a nil AttachmentDisk")
	}

	if vm == nil {
		return false, errors.New("cannot hot plug a disk into a nil VirtualMachine")
	}

	if kvvm == nil {
		return false, errors.New("cannot hot plug a disk into a nil KVVM")
	}

	for _, bdr := range vm.Spec.BlockDeviceRefs {
		if bdr.Kind == ad.Kind && bdr.Name == ad.Name {
			return false, fmt.Errorf("%w: virtual machine has a block device reference, but it is not a hot-plugged volume", ErrBlockDeviceIsSpecAttached)
		}
	}

	name := ad.GenerateName

	if kvvm.Spec.Template != nil {
		for _, vs := range kvvm.Spec.Template.Spec.Volumes {
			if vs.Name == name {
				if vs.PersistentVolumeClaim == nil {
					return false, fmt.Errorf("kvvm %s/%s spec volume %s does not have a pvc reference", kvvm.Namespace, kvvm.Name, vs.Name)
				}

				if !vs.PersistentVolumeClaim.Hotpluggable {
					return false, fmt.Errorf("%w: virtual machine has a block device reference, but it is not a hot-plugged volume", ErrBlockDeviceIsSpecAttached)
				}

				return false, ErrHotPlugRequestAlreadySent
			}
		}
	}

	for _, vr := range kvvm.Status.VolumeRequests {
		if vr.AddVolumeOptions.Name == name {
			return false, ErrHotPlugRequestAlreadySent
		}
	}

	return true, nil
}

func (s AttachmentService) HotPlugDisk(ctx context.Context, ad *AttachmentDisk, vm *virtv2.VirtualMachine, kvvm *virtv1.VirtualMachine) error {
	if ad == nil {
		return errors.New("cannot hot plug a nil AttachmentDisk")
	}

	if vm == nil {
		return errors.New("cannot hot plug a disk into a nil VirtualMachine")
	}

	if kvvm == nil {
		return errors.New("cannot hot plug a disk into a nil KVVM")
	}

	return s.virtClient.VirtualMachines(vm.GetNamespace()).AddVolume(ctx, vm.GetName(), v1alpha2.VirtualMachineAddVolume{
		VolumeKind: string(ad.Kind),
		Name:       ad.GenerateName,
		Image:      ad.Image,
		PVCName:    ad.PVCName,
		Serial:     ad.Serial,
		IsCdrom:    ad.IsCdrom,
	})
}

func (s AttachmentService) CanUnplug(kvvm *virtv1.VirtualMachine, vm *virtv2.VirtualMachine, blockDeviceName, originalName string, blockDeviceKind virtv2.VMBDAObjectRefKind) bool {
	if blockDeviceName == "" || kvvm == nil || kvvm.Spec.Template == nil {
		return false
	}

	for _, specDisk := range vm.Spec.BlockDeviceRefs {
		if specDisk.Name == originalName && specDisk.Kind.String() == blockDeviceKind.String() {
			return false
		}
	}

	for _, volume := range kvvm.Spec.Template.Spec.Volumes {
		if kvapi.VolumeExists(volume, blockDeviceName) {
			return true
		}
	}

	return false
}

func (s AttachmentService) UnplugDisk(ctx context.Context, kvvm *virtv1.VirtualMachine, diskName string) error {
	if kvvm == nil {
		return errors.New("cannot unplug a disk from a nil KVVM")
	}
	if diskName == "" {
		return errors.New("cannot unplug a disk with a empty DiskName")
	}
	return s.virtClient.VirtualMachines(kvvm.GetNamespace()).RemoveVolume(ctx, kvvm.GetName(), v1alpha2.VirtualMachineRemoveVolume{
		Name: diskName,
	})
}

// IsConflictedAttachment returns true if the provided VMBDA conflicts with another
// previously created or started VMBDA with the same disk.
// There should be no other Non-Conflicted VMBDAs.
//
// Examples: Check if VMBDA A is conflicted:
//
// T1: -->VMBDA A Should be Conflicted
// T1:    VMBDA B Phase: "Attached"
//
// T1: -->VMBDA A Should be Non-Conflicted
// T1:    VMBDA B Phase: "Failed"
//
// T1:    VMBDA B Phase: ""
// T2: -->VMBDA A Should be Conflicted
//
// T1: -->VMBDA A Should be Non-Conflicted
// T2:    VMBDA B Phase: ""
//
// T1: -->VMBDA A Should be Non-Conflicted lexicographically
// T1:    VMBDA B Phase: ""
func (s AttachmentService) IsConflictedAttachment(ctx context.Context, vmbda *virtv2.VirtualMachineBlockDeviceAttachment) (bool, string, error) {
	// CVI and VI always has no conflicts. Skip
	if vmbda.Spec.BlockDeviceRef.Kind == virtv2.ClusterVirtualImageKind || vmbda.Spec.BlockDeviceRef.Kind == virtv2.VirtualImageKind {
		return false, "", nil
	}

	var vmbdas virtv2.VirtualMachineBlockDeviceAttachmentList
	err := s.client.List(ctx, &vmbdas, &client.ListOptions{Namespace: vmbda.Namespace})
	if err != nil {
		return false, "", err
	}

	for i := range vmbdas.Items {
		// If the virtual machine and disk do not match, there is no conflict with this VMBDA.
		if vmbdas.Items[i].Name == vmbda.Name || !isSameBlockDeviceRefs(vmbdas.Items[i].Spec.BlockDeviceRef, vmbda.Spec.BlockDeviceRef) {
			continue
		}

		// There is already a Non-Conflicted VMBDA.
		if vmbdas.Items[i].Status.Phase != "" && vmbdas.Items[i].Status.Phase != virtv2.BlockDeviceAttachmentPhaseFailed {
			return true, vmbdas.Items[i].Name, nil
		}

		switch vmbdas.Items[i].CreationTimestamp.Time.Compare(vmbda.CreationTimestamp.Time) {
		case -1:
			// The current VMBDA undergoing reconciliation conflicts with another previously created VMBDA.
			return true, vmbdas.Items[i].Name, nil
		case 0:
			// Same creation time, the earliest lexicographically should be processed, the others are considered conflicting.
			if strings.Compare(vmbdas.Items[i].Name, vmbda.Name) == -1 {
				return true, vmbdas.Items[i].Name, nil
			}
		case 1:
		}
	}

	return false, "", nil
}

func (s AttachmentService) GetVirtualDisk(ctx context.Context, name, namespace string) (*virtv2.VirtualDisk, error) {
	return object.FetchObject(ctx, types.NamespacedName{Namespace: namespace, Name: name}, s.client, &virtv2.VirtualDisk{})
}

func (s AttachmentService) GetVirtualImage(ctx context.Context, name, namespace string) (*virtv2.VirtualImage, error) {
	return object.FetchObject(ctx, types.NamespacedName{Namespace: namespace, Name: name}, s.client, &virtv2.VirtualImage{})
}

func (s AttachmentService) GetClusterVirtualImage(ctx context.Context, name string) (*virtv2.ClusterVirtualImage, error) {
	return object.FetchObject(ctx, types.NamespacedName{Name: name}, s.client, &virtv2.ClusterVirtualImage{})
}

func (s AttachmentService) GetPersistentVolumeClaim(ctx context.Context, ad *AttachmentDisk) (*corev1.PersistentVolumeClaim, error) {
	return object.FetchObject(ctx, types.NamespacedName{Namespace: ad.Namespace, Name: ad.PVCName}, s.client, &corev1.PersistentVolumeClaim{})
}

func (s AttachmentService) GetVirtualMachine(ctx context.Context, name, namespace string) (*virtv2.VirtualMachine, error) {
	return object.FetchObject(ctx, types.NamespacedName{Namespace: namespace, Name: name}, s.client, &virtv2.VirtualMachine{})
}

func (s AttachmentService) GetKVVM(ctx context.Context, vm *virtv2.VirtualMachine) (*virtv1.VirtualMachine, error) {
	return object.FetchObject(ctx, types.NamespacedName{Namespace: vm.Namespace, Name: vm.Name}, s.client, &virtv1.VirtualMachine{})
}

func (s AttachmentService) GetKVVMI(ctx context.Context, vm *virtv2.VirtualMachine) (*virtv1.VirtualMachineInstance, error) {
	return object.FetchObject(ctx, types.NamespacedName{Namespace: vm.Namespace, Name: vm.Name}, s.client, &virtv1.VirtualMachineInstance{})
}

func isSameBlockDeviceRefs(a, b virtv2.VMBDAObjectRef) bool {
	return a.Kind == b.Kind && a.Name == b.Name
}

type AttachmentDisk struct {
	Kind         virtv2.BlockDeviceKind
	Name         string
	Namespace    string
	GenerateName string
	PVCName      string
	Image        string
	Serial       string
	IsCdrom      bool
}

func NewAttachmentDiskFromVirtualDisk(vd *virtv2.VirtualDisk) *AttachmentDisk {
	return &AttachmentDisk{
		Kind:         virtv2.DiskDevice,
		Name:         vd.GetName(),
		Namespace:    vd.GetNamespace(),
		GenerateName: kvbuilder.GenerateVMDDiskName(vd.GetName()),
		Serial:       kvbuilder.GenerateSerialFromObject(vd),
		PVCName:      vd.Status.Target.PersistentVolumeClaim,
	}
}

func NewAttachmentDiskFromVirtualImage(vi *virtv2.VirtualImage) *AttachmentDisk {
	serial := ""
	if !vi.Status.CDROM {
		serial = kvbuilder.GenerateSerialFromObject(vi)
	}
	ad := AttachmentDisk{
		Kind:         virtv2.ImageDevice,
		Name:         vi.GetName(),
		Namespace:    vi.GetNamespace(),
		GenerateName: kvbuilder.GenerateVMIDiskName(vi.GetName()),
		Serial:       serial,
		IsCdrom:      vi.Status.CDROM,
	}

	if vi.Spec.Storage == virtv2.StorageContainerRegistry {
		ad.Image = vi.Status.Target.RegistryURL
	} else {
		ad.PVCName = vi.Status.Target.PersistentVolumeClaim
	}

	return &ad
}

func NewAttachmentDiskFromClusterVirtualImage(cvi *virtv2.ClusterVirtualImage) *AttachmentDisk {
	serial := ""
	if !cvi.Status.CDROM {
		serial = kvbuilder.GenerateSerialFromObject(cvi)
	}
	return &AttachmentDisk{
		Kind:         virtv2.ClusterImageDevice,
		Name:         cvi.GetName(),
		GenerateName: kvbuilder.GenerateCVMIDiskName(cvi.GetName()),
		Image:        cvi.Status.Target.RegistryURL,
		Serial:       serial,
		IsCdrom:      cvi.Status.CDROM,
	}
}
