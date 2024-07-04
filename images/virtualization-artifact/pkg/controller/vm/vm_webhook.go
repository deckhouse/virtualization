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
	"log/slog"
	"strings"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"kubevirt.io/api/core"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/deckhouse/virtualization-controller/pkg/controller/vm/internal"
	"github.com/deckhouse/virtualization-controller/pkg/sdk/framework/helper"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type Validator struct {
	validators []vmValidator
	log        *slog.Logger
}

func NewValidator(ipam internal.IPAM, client client.Client, log *slog.Logger) *Validator {
	if log == nil {
		log = slog.Default().With("controller", controllerName)
	}
	return &Validator{
		validators: []vmValidator{
			newMetaVMValidator(client),
			newIPAMVMValidator(ipam, client),
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
		warn, err := validator.validateCreate(ctx, vm)
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

	for _, validator := range v.validators {
		warn, err := validator.validateUpdate(ctx, oldVM, newVM)
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

type vmValidator interface {
	validateCreate(ctx context.Context, vm *v1alpha2.VirtualMachine) (admission.Warnings, error)
	validateUpdate(ctx context.Context, oldVM, newVM *v1alpha2.VirtualMachine) (admission.Warnings, error)
}

type metaVMValidator struct {
	client client.Client
}

func newMetaVMValidator(client client.Client) *metaVMValidator {
	return &metaVMValidator{client: client}
}

func (v *metaVMValidator) validateCreate(_ context.Context, vm *v1alpha2.VirtualMachine) (admission.Warnings, error) {
	for key := range vm.Annotations {
		if strings.Contains(key, core.GroupName) {
			return nil, fmt.Errorf("using the %s group's name in the annotation is prohibited", core.GroupName)
		}
	}

	for key := range vm.Labels {
		if strings.Contains(key, core.GroupName) {
			return nil, fmt.Errorf("using the %s group's name in the label is prohibited", core.GroupName)
		}
	}

	return nil, nil
}

func (v *metaVMValidator) validateUpdate(_ context.Context, _, newVM *v1alpha2.VirtualMachine) (admission.Warnings, error) {
	for key := range newVM.Annotations {
		if strings.Contains(key, core.GroupName) {
			return nil, fmt.Errorf("using the %s group's name in the annotation is prohibited", core.GroupName)
		}
	}

	for key := range newVM.Labels {
		if strings.Contains(key, core.GroupName) {
			return nil, fmt.Errorf("using the %s group's name in the label is prohibited", core.GroupName)
		}
	}

	return nil, nil
}

type ipamVMValidator struct {
	ipam   internal.IPAM
	client client.Client
}

func newIPAMVMValidator(ipam internal.IPAM, client client.Client) *ipamVMValidator {
	return &ipamVMValidator{ipam: ipam, client: client}
}

func (v *ipamVMValidator) validateCreate(ctx context.Context, vm *v1alpha2.VirtualMachine) (admission.Warnings, error) {
	vmipName := vm.Spec.VirtualMachineIPAddress
	if vmipName == "" {
		vmipName = vm.Name
	}

	vmipKey := types.NamespacedName{Name: vmipName, Namespace: vm.Namespace}
	vmip, err := helper.FetchObject(ctx, vmipKey, v.client, &v1alpha2.VirtualMachineIPAddress{})
	if err != nil {
		return nil, fmt.Errorf("unable to get referenced VirtualMachineIPAddress %s: %w", vmipKey, err)
	}

	if vmip == nil {
		return nil, nil
	}

	// VM is created without ip address, but ip address resource is already exists.
	if vm.Spec.VirtualMachineIPAddress == "" {
		return nil, fmt.Errorf("VirtualMachineIPAddress with the name of the virtual machine"+
			" already exists: set spec.virtualMachineIPAddress field to %s to use IP %s", vmip.Name, vmip.Status.Address)
	}

	return nil, v.ipam.CheckIpAddressAvailableForBinding(vm.Name, vmip)
}

func (v *ipamVMValidator) validateUpdate(ctx context.Context, oldVM, newVM *v1alpha2.VirtualMachine) (admission.Warnings, error) {
	if oldVM.Spec.VirtualMachineIPAddress == newVM.Spec.VirtualMachineIPAddress {
		return nil, nil
	}

	if newVM.Spec.VirtualMachineIPAddress == "" {
		return nil, fmt.Errorf("spec.virtualMachineIPAddress cannot be changed to an empty value once set")
	}

	vmipKey := types.NamespacedName{Name: newVM.Spec.VirtualMachineIPAddress, Namespace: newVM.Namespace}
	vmip, err := helper.FetchObject(ctx, vmipKey, v.client, &v1alpha2.VirtualMachineIPAddress{})
	if err != nil {
		return nil, fmt.Errorf("unable to get VirtualMachineIPAddress %s: %w", vmipKey, err)
	}

	if vmip == nil {
		return nil, nil
	}

	return nil, v.ipam.CheckIpAddressAvailableForBinding(newVM.Name, vmip)
}
