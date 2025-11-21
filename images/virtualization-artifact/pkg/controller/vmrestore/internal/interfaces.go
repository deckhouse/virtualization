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

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

//go:generate go tool moq -rm -out mock.go . Restorer

type Restorer interface {
	Get(ctx context.Context, vmSnapshot *v1alpha2.VirtualMachineSnapshot) (*corev1.Secret, error)
	RestoreVirtualMachine(ctx context.Context, secret *corev1.Secret) (*v1alpha2.VirtualMachine, error)
	RestoreProvisioner(ctx context.Context, secret *corev1.Secret) (*corev1.Secret, error)
	RestoreVirtualMachineIPAddress(ctx context.Context, secret *corev1.Secret) (*v1alpha2.VirtualMachineIPAddress, error)
	RestoreVirtualMachineBlockDeviceAttachments(ctx context.Context, secret *corev1.Secret) ([]*v1alpha2.VirtualMachineBlockDeviceAttachment, error)
	RestoreVirtualMachineMACAddresses(ctx context.Context, secret *corev1.Secret) ([]*v1alpha2.VirtualMachineMACAddress, error)
	RestoreMACAddressOrder(ctx context.Context, secret *corev1.Secret) ([]string, error)
}
