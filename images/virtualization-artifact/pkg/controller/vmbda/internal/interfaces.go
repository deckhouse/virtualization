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

	corev1 "k8s.io/api/core/v1"
	virtv1 "kubevirt.io/api/core/v1"

	intsvc "github.com/deckhouse/virtualization-controller/pkg/controller/vmbda/internal/service"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

//go:generate go tool moq -rm -out mock.go . AttachmentService

type AttachmentService interface {
	GetVirtualMachine(ctx context.Context, name, namespace string) (*v1alpha2.VirtualMachine, error)
	GetKVVMI(ctx context.Context, vm *v1alpha2.VirtualMachine) (*virtv1.VirtualMachineInstance, error)
	GetKVVM(ctx context.Context, vm *v1alpha2.VirtualMachine) (*virtv1.VirtualMachine, error)
	GetVirtualDisk(ctx context.Context, name, namespace string) (*v1alpha2.VirtualDisk, error)
	GetVirtualImage(ctx context.Context, name, namespace string) (*v1alpha2.VirtualImage, error)
	GetClusterVirtualImage(ctx context.Context, name string) (*v1alpha2.ClusterVirtualImage, error)
	GetPersistentVolumeClaim(ctx context.Context, ad *intsvc.AttachmentDisk) (*corev1.PersistentVolumeClaim, error)
}
