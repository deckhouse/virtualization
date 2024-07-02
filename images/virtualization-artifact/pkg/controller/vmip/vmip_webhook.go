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

package vmip

import (
	"context"
	"errors"
	"fmt"
	"net"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/deckhouse/virtualization-controller/pkg/controller/vmip/internal/util"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

func NewVMIPValidator(log logr.Logger, client client.Client) *VMIPValidator {
	return &VMIPValidator{log: log.WithName(controllerName).WithValues("webhook", "validation"), client: client}
}

type VMIPValidator struct {
	log    logr.Logger
	client client.Client
}

func (v *VMIPValidator) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	vmip, ok := obj.(*v1alpha2.VirtualMachineIPAddress)
	if !ok {
		return nil, fmt.Errorf("expected a new VirtualMachineIPAddress but got a %T", obj)
	}

	v.log.Info("Validate VirtualMachineIP creating", "name", vmip.Name, "address", vmip.Spec.Address, "leaseName", vmip.Spec.VirtualMachineIPAddressLease)

	err := v.validateSpecFields(vmip.Spec)
	if err != nil {
		return nil, fmt.Errorf("error validating VirtualMachineIP creation: %w", err)
	}

	ip := vmip.Spec.Address
	if ip == "" {
		ip = util.LeaseNameToIP(vmip.Spec.VirtualMachineIPAddressLease)
	}

	if ip != "" {
		allocatedIPs, err := util.GetAllocatedIPs(ctx, v.client)
		if err != nil {
			return nil, fmt.Errorf("error getting allocated ips: %w", err)
		}

		allocatedLease, ok := allocatedIPs[ip]
		if ok && allocatedLease.Spec.IpAddressRef != nil && (allocatedLease.Spec.IpAddressRef.Namespace != vmip.Namespace || allocatedLease.Spec.IpAddressRef.Name != vmip.Name) {
			return nil, fmt.Errorf("VirtualMachineIP cannot be created: the address %s has already been allocated for another VMIP", ip)
		}
	}

	return nil, nil
}

func (v *VMIPValidator) ValidateUpdate(_ context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	oldVmip, ok := oldObj.(*v1alpha2.VirtualMachineIPAddress)
	if !ok {
		return nil, fmt.Errorf("expected a old VirtualMachineIP but got a %T", oldObj)
	}

	newVmip, ok := newObj.(*v1alpha2.VirtualMachineIPAddress)
	if !ok {
		return nil, fmt.Errorf("expected a new VirtualMachineIP but got a %T", newObj)
	}

	v.log.Info("Validate VirtualMachineIP updating", "name", newVmip.Name,
		"old.address", oldVmip.Spec.Address, "new.address", newVmip.Spec.Address,
		"old.leaseName", oldVmip.Spec.VirtualMachineIPAddressLease, "new.leaseName", newVmip.Spec.VirtualMachineIPAddressLease,
	)

	if oldVmip.Spec.Address != "" && oldVmip.Spec.Address != newVmip.Spec.Address {
		return nil, errors.New("the VirtualMachineIP address cannot be changed if allocated")
	}

	if oldVmip.Spec.VirtualMachineIPAddressLease != "" && oldVmip.Spec.VirtualMachineIPAddressLease != newVmip.Spec.VirtualMachineIPAddressLease {
		return nil, errors.New("the VirtualMachineIPLease name cannot be changed if allocated")
	}

	err := v.validateSpecFields(newVmip.Spec)
	if err != nil {
		return nil, fmt.Errorf("error validating VirtualMachineIP updating: %w", err)
	}

	return nil, nil
}

func (v *VMIPValidator) ValidateDelete(_ context.Context, _ runtime.Object) (admission.Warnings, error) {
	err := fmt.Errorf("misconfigured webhook rules: delete operation not implemented")
	v.log.Error(err, "Ensure the correctness of ValidatingWebhookConfiguration")
	return nil, nil
}

func (v *VMIPValidator) validateSpecFields(spec v1alpha2.VirtualMachineIPAddressSpec) error {
	if spec.VirtualMachineIPAddressLease != "" && !isValidAddressFormat(util.LeaseNameToIP(spec.VirtualMachineIPAddressLease)) {
		return errors.New("the VirtualMachineIP name is not created from a valid IP address or ip prefix is missing")
	}

	if spec.Address != "" && !isValidAddressFormat(spec.Address) {
		return errors.New("the VirtualMachineIP address is not a valid textual representation of an IP address")
	}

	if spec.Address != "" && spec.VirtualMachineIPAddressLease != "" && spec.Address != util.LeaseNameToIP(spec.VirtualMachineIPAddressLease) {
		return errors.New("VirtualMachineIPLease name doesn't match the address")
	}

	return nil
}

func isValidAddressFormat(address string) bool {
	return net.ParseIP(address) != nil
}
