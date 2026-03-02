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

package vmbda

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/deckhouse/deckhouse/pkg/log"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	intsvc "github.com/deckhouse/virtualization-controller/pkg/controller/vmbda/internal/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmbda/internal/validators"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type VirtualMachineBlockDeviceAttachmentValidator interface {
	ValidateCreate(ctx context.Context, vm *v1alpha2.VirtualMachineBlockDeviceAttachment) (admission.Warnings, error)
	ValidateUpdate(ctx context.Context, oldVM, newVM *v1alpha2.VirtualMachineBlockDeviceAttachment) (admission.Warnings, error)
}

type Validator struct {
	validators []VirtualMachineBlockDeviceAttachmentValidator
	log        *log.Logger
}

func NewValidator(attachmentService *intsvc.AttachmentService, service *service.BlockDeviceService, log *log.Logger) *Validator {
	return &Validator{
		log: log.With("webhook", "validation"),
		validators: []VirtualMachineBlockDeviceAttachmentValidator{
			validators.NewSpecMutateValidator(),
			validators.NewAttachmentConflictValidator(attachmentService, log),
			validators.NewVMConnectLimiterValidator(service, log),
		},
	}
}

func (v *Validator) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	vmbda, ok := obj.(*v1alpha2.VirtualMachineBlockDeviceAttachment)
	if !ok {
		return nil, fmt.Errorf("expected a new VirtualMachineBlockDeviceAttachment but got a %T", obj)
	}

	var warnings admission.Warnings

	for _, validator := range v.validators {
		warn, err := validator.ValidateCreate(ctx, vmbda)
		if err != nil {
			return nil, err
		}
		warnings = append(warnings, warn...)
	}

	return warnings, nil
}

func (v *Validator) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	oldVMBDA, ok := oldObj.(*v1alpha2.VirtualMachineBlockDeviceAttachment)
	if !ok {
		return nil, fmt.Errorf("expected an old VirtualMachineBlockDeviceAttachment but got a %T", oldObj)
	}

	newVMBDA, ok := newObj.(*v1alpha2.VirtualMachineBlockDeviceAttachment)
	if !ok {
		return nil, fmt.Errorf("expected a new VirtualMachineBlockDeviceAttachment but got a %T", newObj)
	}

	var warnings admission.Warnings

	for _, validator := range v.validators {
		warn, err := validator.ValidateUpdate(ctx, oldVMBDA, newVMBDA)
		if err != nil {
			return nil, err
		}
		warnings = append(warnings, warn...)
	}

	return warnings, nil
}

func (v *Validator) ValidateDelete(_ context.Context, _ runtime.Object) (admission.Warnings, error) {
	err := fmt.Errorf("misconfigured webhook rules: delete operation not implemented")
	v.log.Error("Ensure the correctness of ValidatingWebhookConfiguration", "err", err.Error())
	return nil, nil
}
