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

package validators

import (
	"context"
	"fmt"
	"log/slog"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/deckhouse/virtualization-controller/pkg/common"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type VMConnectLimiterValidator struct {
	client  client.Client
	service *service.BlockDeviceService
	log     *slog.Logger
}

func NewVMConnectLimiterValidator(client client.Client, log *slog.Logger) *VMConnectLimiterValidator {
	return &VMConnectLimiterValidator{
		client:  client,
		service: service.NewBlockDeviceService(client),
		log:     log,
	}
}

func (v *VMConnectLimiterValidator) ValidateCreate(ctx context.Context, vmbda *virtv2.VirtualMachineBlockDeviceAttachment) (admission.Warnings, error) {
	count, err := v.service.CountBlockDevicesAttachedToVmName(ctx, vmbda.Spec.VirtualMachineName, vmbda.Namespace)
	if err != nil {
		return nil, err
	}

	// created entity counted too
	if count+1 > common.VmBlockDeviceAttachedLimit {
		return nil, fmt.Errorf("block device limit reached: %d devices found, %d is maximum", count, common.VmBlockDeviceAttachedLimit)
	}

	return nil, nil
}

func (v *VMConnectLimiterValidator) ValidateUpdate(ctx context.Context, _, newVMBDA *virtv2.VirtualMachineBlockDeviceAttachment) (admission.Warnings, error) {
	count, err := v.service.CountBlockDevicesAttachedToVmName(ctx, newVMBDA.Spec.VirtualMachineName, newVMBDA.Namespace)
	if err != nil {
		v.log.Error(err.Error())
		return nil, err
	}

	if count > common.VmBlockDeviceAttachedLimit {
		return nil, fmt.Errorf("block device limit reached: %d devices found, %d is maximum", count, common.VmBlockDeviceAttachedLimit)
	}

	return nil, nil
}
