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

	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/deckhouse/deckhouse/pkg/log"
	"github.com/deckhouse/virtualization-controller/pkg/common"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type BlockDeviceLimiterValidator struct {
	service *service.BlockDeviceService
	log     *log.Logger
}

func NewBlockDeviceLimiterValidator(service *service.BlockDeviceService, log *log.Logger) *BlockDeviceLimiterValidator {
	return &BlockDeviceLimiterValidator{
		service: service,
		log:     log,
	}
}

func (v *BlockDeviceLimiterValidator) ValidateCreate(ctx context.Context, vm *v1alpha2.VirtualMachine) (admission.Warnings, error) {
	return v.validate(ctx, vm)
}

func (v *BlockDeviceLimiterValidator) ValidateUpdate(ctx context.Context, _, newVM *v1alpha2.VirtualMachine) (admission.Warnings, error) {
	if newVM == nil || !newVM.GetDeletionTimestamp().IsZero() {
		return nil, nil
	}
	return v.validate(ctx, newVM)
}

func (v *BlockDeviceLimiterValidator) validate(ctx context.Context, vm *v1alpha2.VirtualMachine) (admission.Warnings, error) {
	count, err := v.service.CountBlockDevicesAttachedToVM(ctx, vm)
	if err != nil {
		v.log.Error(err.Error())
		return nil, err
	}

	if count > common.VMBlockDeviceAttachedLimit {
		err = fmt.Errorf("block device attached to VirtualMachine %q limit reached: %d devices found, %d is maximum", vm.Name, count, common.VMBlockDeviceAttachedLimit)
		return nil, err
	}

	return nil, nil
}
