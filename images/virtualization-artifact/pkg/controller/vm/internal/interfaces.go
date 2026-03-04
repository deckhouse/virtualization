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

	"k8s.io/client-go/tools/record"
	virtv1 "kubevirt.io/api/core/v1"

	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

//go:generate go tool moq -rm -out mock.go . EventRecorder BlockDeviceService HotplugService

type EventRecorder = record.EventRecorder

type BlockDeviceService interface {
	CountBlockDevicesAttachedToVM(ctx context.Context, vm *v1alpha2.VirtualMachine) (int, error)
}

type HotplugService interface {
	HotPlugDisk(ctx context.Context, ad *service.AttachmentDisk, vm *v1alpha2.VirtualMachine, kvvm *virtv1.VirtualMachine) error
	UnplugDisk(ctx context.Context, kvvm *virtv1.VirtualMachine, diskName string) error
}
