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

	vsv1 "github.com/kubernetes-csi/external-snapshotter/client/v6/apis/volumesnapshot/v1"
	corev1 "k8s.io/api/core/v1"
	virtv1 "kubevirt.io/api/core/v1"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

//go:generate go tool moq -rm -out mock.go . VirtualDiskReadySnapshotter LifeCycleSnapshotter

type VirtualDiskReadySnapshotter interface {
	GetVirtualDisk(ctx context.Context, name, namespace string) (*v1alpha2.VirtualDisk, error)
}

type LifeCycleSnapshotter interface {
	Freeze(ctx context.Context, kvvmi *virtv1.VirtualMachineInstance) error
	IsFrozen(ctx context.Context, kvvmi *virtv1.VirtualMachineInstance) (bool, error)
	CanFreeze(ctx context.Context, kvvmi *virtv1.VirtualMachineInstance) (bool, error)
	CanUnfreeze(ctx context.Context, vdSnapshotName string, vm *v1alpha2.VirtualMachine, kvvmi *virtv1.VirtualMachineInstance) (bool, error)
	Unfreeze(ctx context.Context, kvvmi *virtv1.VirtualMachineInstance) error
	CreateVolumeSnapshot(ctx context.Context, vs *vsv1.VolumeSnapshot) (*vsv1.VolumeSnapshot, error)
	GetPersistentVolumeClaim(ctx context.Context, name, namespace string) (*corev1.PersistentVolumeClaim, error)
	GetVirtualDisk(ctx context.Context, name, namespace string) (*v1alpha2.VirtualDisk, error)
	GetVirtualMachine(ctx context.Context, name, namespace string) (*v1alpha2.VirtualMachine, error)
	GetVolumeSnapshot(ctx context.Context, name, namespace string) (*vsv1.VolumeSnapshot, error)
	SyncFSFreezeRequest(ctx context.Context, kvvmi *virtv1.VirtualMachineInstance) error
	GetKubeVirtVirtualMachineInstance(ctx context.Context, vm *v1alpha2.VirtualMachine) (*virtv1.VirtualMachineInstance, error)
}
