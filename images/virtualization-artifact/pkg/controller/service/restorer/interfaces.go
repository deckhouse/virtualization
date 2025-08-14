/*
Copyright 2025 Flant JSC

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

package restorer

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

// //go:generate moq -rm -out mock.go . Restorer ObjectHandler
// type Restorer interface {
// 	RestoreVirtualMachine(ctx context.Context, secret *corev1.Secret) (*virtv2.VirtualMachine, error)
// 	RestoreProvisioner(ctx context.Context, secret *corev1.Secret) (*corev1.Secret, error)
// 	RestoreVirtualMachineIPAddress(ctx context.Context, secret *corev1.Secret) (*virtv2.VirtualMachineIPAddress, error)
// 	RestoreVirtualMachineBlockDeviceAttachments(ctx context.Context, secret *corev1.Secret) ([]*virtv2.VirtualMachineBlockDeviceAttachment, error)
// }

type ObjectHandler interface {
	Object() client.Object
	Validate(ctx context.Context) error
	ValidateWithForce(ctx context.Context) error
	Process(ctx context.Context) error
	ProcessWithForce(ctx context.Context) error
}
