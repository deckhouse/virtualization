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

func (s AttachmentService) IsHotPlugged(ctx context.Context, vd *virtv2.VirtualDisk, vm *virtv2.VirtualMachine) (bool, error) {
	if vd == nil {
		return false, errors.New("cannot check if a nil VirtualDisk is hot plugged")
	}

	if vm == nil {
		return false, errors.New("cannot check if a disk is hot plugged into a nil VirtualMachine")
	}

	kvvmiKey := types.NamespacedName{Namespace: vm.Namespace, Name: vm.Name}
	kvvmi, err := helper.FetchObject(ctx, kvvmiKey, s.client, &virtv1.VirtualMachineInstance{})
	if err != nil {
		return false, err
	}

	if kvvmi == nil {
		return false, nil
	}

	for _, vs := range kvvmi.Status.VolumeStatus {
		if vs.HotplugVolume != nil && vs.Name == kvbuilder.GenerateVMDDiskName(vd.Name) {
			return true, nil
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

func (s AttachmentService) GetVirtualMachine(ctx context.Context, name, namespace string) (*virtv2.VirtualMachine, error) {
	return helper.FetchObject(ctx, types.NamespacedName{Namespace: namespace, Name: name}, s.client, &virtv2.VirtualMachine{})
}

func (s AttachmentService) GetVirtualMachineInstance(ctx context.Context, name, namespace string) (*virtv1.VirtualMachineInstance, error) {
	return helper.FetchObject(ctx, types.NamespacedName{Namespace: namespace, Name: name}, s.client, &virtv1.VirtualMachineInstance{})
}
