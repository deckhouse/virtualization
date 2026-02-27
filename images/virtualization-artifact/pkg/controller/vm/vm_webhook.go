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

package vm

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/component-base/featuregate"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/deckhouse/deckhouse/pkg/log"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vm/internal/defaulter"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vm/internal/validators"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type VirtualMachineValidator interface {
	ValidateCreate(ctx context.Context, vm *v1alpha2.VirtualMachine) (admission.Warnings, error)
	ValidateUpdate(ctx context.Context, oldVM, newVM *v1alpha2.VirtualMachine) (admission.Warnings, error)
}

type Validator struct {
	validators []VirtualMachineValidator
	log        *log.Logger
}

func NewValidator(client client.Client, service *service.BlockDeviceService, featureGate featuregate.FeatureGate, log *log.Logger) *Validator {
	return &Validator{
		validators: []VirtualMachineValidator{
			validators.NewMetaValidator(client),
			validators.NewIPAMValidator(client),
			validators.NewBlockDeviceSpecRefsValidator(),
			validators.NewSizingPolicyValidator(client),
			validators.NewBlockDeviceLimiterValidator(service, log),
			validators.NewAffinityValidator(),
			validators.NewTopologySpreadConstraintValidator(),
			validators.NewCPUCountValidator(),
			validators.NewNetworksValidator(featureGate),
			validators.NewFirstDiskValidator(client),
			validators.NewUSBDevicesValidator(client),
		},
		log: log.With("webhook", "validation"),
	}
}

func (v *Validator) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	vm, ok := obj.(*v1alpha2.VirtualMachine)
	if !ok {
		return nil, fmt.Errorf("expected a new VirtualMachine but got a %T", obj)
	}

	v.log.Info("Validating VM", "spec.virtualMachineIPAddress", vm.Spec.VirtualMachineIPAddress)

	var warnings admission.Warnings

	for _, validator := range v.validators {
		warn, err := validator.ValidateCreate(ctx, vm)
		if err != nil {
			return nil, err
		}
		warnings = append(warnings, warn...)
	}

	return warnings, nil
}

func (v *Validator) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	oldVM, ok := oldObj.(*v1alpha2.VirtualMachine)
	if !ok {
		return nil, fmt.Errorf("expected an old VirtualMachine but got a %T", oldObj)
	}

	newVM, ok := newObj.(*v1alpha2.VirtualMachine)
	if !ok {
		return nil, fmt.Errorf("expected a new VirtualMachine but got a %T", newObj)
	}

	v.log.Info("Validating VM",
		"old.spec.virtualMachineIPAddress", oldVM.Spec.VirtualMachineIPAddress,
		"new.spec.virtualMachineIPAddress", newVM.Spec.VirtualMachineIPAddress,
	)

	var warnings admission.Warnings

	if !newVM.GetDeletionTimestamp().IsZero() {
		return warnings, nil
	}

	for _, validator := range v.validators {
		warn, err := validator.ValidateUpdate(ctx, oldVM, newVM)
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

type VirtualMachineDefaulter interface {
	Default(ctx context.Context, vm *v1alpha2.VirtualMachine) error
}

type Defaulter struct {
	defaulters []VirtualMachineDefaulter
	log        *log.Logger
}

var _ admission.CustomDefaulter = &Defaulter{}

func NewDefaulter(client client.Client, vmClassService *service.VirtualMachineClassService, log *log.Logger) *Defaulter {
	return &Defaulter{
		defaulters: []VirtualMachineDefaulter{
			defaulter.NewVirtualMachineClassNameDefaulter(client, vmClassService),
			defaulter.NewCoreFractionDefaulter(client),
			defaulter.NewNetworksDefaulter(),
		},
		log: log.With("webhook", "mutating"),
	}
}

func (d *Defaulter) Default(ctx context.Context, obj runtime.Object) error {
	vm, ok := obj.(*v1alpha2.VirtualMachine)
	if !ok {
		return fmt.Errorf("expected a VirtualMachine but got a %T", obj)
	}

	d.log.Debug("Mutating VM")

	for _, defaulter := range d.defaulters {
		err := defaulter.Default(ctx, vm)
		if err != nil {
			return err
		}
	}

	return nil
}
