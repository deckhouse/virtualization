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

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/controller/kubevirt"
	"github.com/deckhouse/virtualization-controller/pkg/controller/kvapi"
	"github.com/deckhouse/virtualization-controller/pkg/controller/kvbuilder"
	"github.com/deckhouse/virtualization-controller/pkg/sdk/framework/helper"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type AttachmentService struct {
	client              client.Client
	controllerNamespace string
}

func NewAttachmentService(client client.Client, controllerNamespace string) *AttachmentService {
	return &AttachmentService{
		client:              client,
		controllerNamespace: controllerNamespace,
	}
}

var ErrVolumeStatusNotReady = errors.New("hotplug is not ready")

func (s AttachmentService) IsHotPlugged(vd *virtv2.VirtualDisk, vm *virtv2.VirtualMachine, kvvmi *virtv1.VirtualMachineInstance) (bool, error) {
	if vd == nil {
		return false, errors.New("cannot check if a nil VirtualDisk is hot plugged")
	}

	if vm == nil {
		return false, errors.New("cannot check if a disk is hot plugged into a nil VirtualMachine")
	}

	if kvvmi == nil {
		return false, errors.New("cannot check if a disk is hot plugged into a nil KVVMI")
	}

	for _, vs := range kvvmi.Status.VolumeStatus {
		if vs.HotplugVolume != nil && vs.Name == kvbuilder.GenerateVMDDiskName(vd.Name) {
			if vs.Phase == virtv1.VolumeReady {
				return true, nil
			}

			return false, fmt.Errorf("%w: %s", ErrVolumeStatusNotReady, vs.Message)
		}
	}

	return false, nil
}

func (s AttachmentService) IsHotPlugRequestSent(vd *virtv2.VirtualDisk, kvvm *virtv1.VirtualMachine) (bool, error) {
	name := kvbuilder.GenerateVMDDiskName(vd.Name)

	for _, vr := range kvvm.Status.VolumeRequests {
		if vr.AddVolumeOptions.Name == name {
			return true, nil
		}
	}

	if kvvm.Spec.Template != nil {
		for _, vs := range kvvm.Spec.Template.Spec.Volumes {
			if vs.Name == name {
				if vs.PersistentVolumeClaim == nil {
					return false, fmt.Errorf("kvvm %s/%s spec volume %s does not have a pvc reference", kvvm.Namespace, kvvm.Name, vs.Name)
				}

				if !vs.PersistentVolumeClaim.Hotpluggable {
					return false, fmt.Errorf("kvvm %s/%s spec volume %s has a pvc reference, but it is not a hot-plugged volume", kvvm.Namespace, kvvm.Name, vs.Name)
				}

				return true, nil
			}
		}
	}

	return false, nil
}

var (
	ErrVirtualDiskIsAlreadyAttached          = errors.New("virtual disk is already attached to virtual machine")
	ErrVirtualMachineWaitsForRestartApproval = errors.New("virtual machine waits for restart approval")
)

func (s AttachmentService) HotPlugDisk(ctx context.Context, vd *virtv2.VirtualDisk, vm *virtv2.VirtualMachine) error {
	if vd == nil {
		return errors.New("cannot hot plug a nil VirtualDisk")
	}

	if vm == nil {
		return errors.New("cannot hot plug a disk into a nil VirtualMachine")
	}

	for _, bdr := range vm.Spec.BlockDeviceRefs {
		if bdr.Kind == virtv2.DiskDevice && bdr.Name == vd.Name {
			return ErrVirtualDiskIsAlreadyAttached
		}
	}

	if len(vm.Status.RestartAwaitingChanges) > 0 {
		return ErrVirtualMachineWaitsForRestartApproval
	}

	name := kvbuilder.GenerateVMDDiskName(vd.Name)

	hotplugRequest := virtv1.AddVolumeOptions{
		Name: name,
		Disk: &virtv1.Disk{
			Name: name,
			DiskDevice: virtv1.DiskDevice{
				Disk: &virtv1.DiskTarget{
					Bus: "scsi",
				},
			},
			Serial: vd.Name,
		},
		VolumeSource: &virtv1.HotplugVolumeSource{
			PersistentVolumeClaim: &virtv1.PersistentVolumeClaimVolumeSource{
				PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
					ClaimName: vd.Status.Target.PersistentVolumeClaim,
				},
				Hotpluggable: true,
			},
		},
	}

	kv, err := kubevirt.New(ctx, s.client, s.controllerNamespace)
	if err != nil {
		return err
	}

	err = kvapi.New(s.client, kv).AddVolume(ctx, vm.Namespace, vm.Name, &hotplugRequest)
	if err != nil {
		return fmt.Errorf("error adding volume, %w", err)
	}

	return nil
}

func (s AttachmentService) UnplugDisk(ctx context.Context, vd *virtv2.VirtualDisk, vm *virtv2.VirtualMachine) error {
	if vd == nil || vm == nil {
		return nil
	}

	unplugRequest := virtv1.RemoveVolumeOptions{
		Name: kvbuilder.GenerateVMDDiskName(vd.Name),
	}

	kv, err := kubevirt.New(ctx, s.client, s.controllerNamespace)
	if err != nil {
		return err
	}

	err = kvapi.New(s.client, kv).RemoveVolume(ctx, vm.Namespace, vm.Name, &unplugRequest)
	if err != nil {
		return fmt.Errorf("error removing volume, %w", err)
	}

	return nil
}

func (s AttachmentService) GetVirtualDisk(ctx context.Context, name, namespace string) (*virtv2.VirtualDisk, error) {
	return helper.FetchObject(ctx, types.NamespacedName{Namespace: namespace, Name: name}, s.client, &virtv2.VirtualDisk{})
}

func (s AttachmentService) GetPersistentVolumeClaim(ctx context.Context, vd *virtv2.VirtualDisk) (*corev1.PersistentVolumeClaim, error) {
	return helper.FetchObject(ctx, types.NamespacedName{Namespace: vd.Namespace, Name: vd.Status.Target.PersistentVolumeClaim}, s.client, &corev1.PersistentVolumeClaim{})
}

func (s AttachmentService) GetVirtualMachine(ctx context.Context, name, namespace string) (*virtv2.VirtualMachine, error) {
	return helper.FetchObject(ctx, types.NamespacedName{Namespace: namespace, Name: name}, s.client, &virtv2.VirtualMachine{})
}

func (s AttachmentService) GetKVVM(ctx context.Context, vm *virtv2.VirtualMachine) (*virtv1.VirtualMachine, error) {
	return helper.FetchObject(ctx, types.NamespacedName{Namespace: vm.Namespace, Name: vm.Name}, s.client, &virtv1.VirtualMachine{})
}

func (s AttachmentService) GetKVVMI(ctx context.Context, vm *virtv2.VirtualMachine) (*virtv1.VirtualMachineInstance, error) {
	return helper.FetchObject(ctx, types.NamespacedName{Namespace: vm.Namespace, Name: vm.Name}, s.client, &virtv1.VirtualMachineInstance{})
}
