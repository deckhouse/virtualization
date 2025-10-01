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

	corev1 "k8s.io/api/core/v1"

	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

//go:generate go tool moq -rm -out mock.go . Storer Snapshotter

type Storer interface {
	Store(ctx context.Context, vm *virtv2.VirtualMachine, vmSnapshot *virtv2.VirtualMachineSnapshot) (*corev1.Secret, error)
}

type Snapshotter interface {
	GetSecret(ctx context.Context, name, namespace string) (*corev1.Secret, error)
	GetVirtualMachine(ctx context.Context, name, namespace string) (*virtv2.VirtualMachine, error)
	GetVirtualDisk(ctx context.Context, name, namespace string) (*virtv2.VirtualDisk, error)
	GetPersistentVolumeClaim(ctx context.Context, name, namespace string) (*corev1.PersistentVolumeClaim, error)
	GetVirtualDiskSnapshot(ctx context.Context, name, namespace string) (*virtv2.VirtualDiskSnapshot, error)
	CreateVirtualDiskSnapshot(ctx context.Context, vdSnapshot *virtv2.VirtualDiskSnapshot) (*virtv2.VirtualDiskSnapshot, error)
	Freeze(ctx context.Context, name, namespace string) error
	Unfreeze(ctx context.Context, name, namespace string) error
	IsFrozen(vm *virtv2.VirtualMachine) bool
	CanFreeze(vm *virtv2.VirtualMachine) bool
	CanUnfreezeWithVirtualMachineSnapshot(ctx context.Context, vmSnapshotName string, vm *virtv2.VirtualMachine) (bool, error)
}
