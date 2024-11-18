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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/deckhouse/deckhouse/pkg/log"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmip/internal/util"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmipcondition"
)

func NewValidator(log *log.Logger, client client.Client, ipAddressService *service.IpAddressService) *Validator {
	return &Validator{
		log:       log.With("webhook", "validation"),
		client:    client,
		ipService: ipAddressService,
	}
}

type Validator struct {
	log       *log.Logger
	client    client.Client
	ipService *service.IpAddressService
}

func (v *Validator) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	vmip, ok := obj.(*v1alpha2.VirtualMachineIPAddress)
	if !ok {
		return nil, fmt.Errorf("expected a new VirtualMachineIPAddress but got a %T", obj)
	}

	v.log.Info("Validate VirtualMachineIP creating", "name", vmip.Name, "type", vmip.Spec.Type, "address", vmip.Spec.StaticIP)

	err := v.validateSpecFields(vmip.Spec)
	if err != nil {
		return nil, fmt.Errorf("error validating VirtualMachineIP creation: %w", err)
	}

	var warnings admission.Warnings

	if vmip.Spec.Type == v1alpha2.VirtualMachineIPAddressTypeStatic {
		ip := vmip.Spec.StaticIP

		if ip != "" {
			allocatedIPs, err := util.GetAllocatedIPs(ctx, v.client, vmip.Spec.Type)
			if err != nil {
				return nil, fmt.Errorf("error getting allocated ips: %w", err)
			}

			allocatedLease, ok := allocatedIPs[ip]
			if ok && allocatedLease.Spec.VirtualMachineIPAddressRef != nil &&
				(allocatedLease.Spec.VirtualMachineIPAddressRef.Namespace != vmip.Namespace ||
					allocatedLease.Spec.VirtualMachineIPAddressRef.Name != vmip.Name) {
				return nil, fmt.Errorf("VirtualMachineIPAddress cannot be created: the IP address %s has already been allocated by VirtualMachineIPAddress/%s in ns/%s", ip, allocatedLease.Spec.VirtualMachineIPAddressRef.Name, allocatedLease.Spec.VirtualMachineIPAddressRef.Namespace)
			}

			err = v.ipService.IsAvailableAddress(vmip.Spec.StaticIP, allocatedIPs)
			if err != nil {
				if errors.Is(err, service.ErrIPAddressOutOfRange) {
					warnings = append(warnings, fmt.Sprintf("The requested address %s is out of the valid range",
						vmip.Spec.StaticIP))
				}
			}
		}
	}

	if vmip.Spec.Type == v1alpha2.VirtualMachineIPAddressTypeAuto && vmip.Spec.StaticIP != "" {
		return nil, fmt.Errorf("VirtualMachineIPAddress cannot be created: The 'Static IP' field is set for the %s IP address with the 'Auto' type", vmip.Name)
	}

	return warnings, nil
}

func (v *Validator) ValidateUpdate(_ context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	oldVmip, ok := oldObj.(*v1alpha2.VirtualMachineIPAddress)
	if !ok {
		return nil, fmt.Errorf("expected an old VirtualMachineIP but got a %T", oldObj)
	}

	newVmip, ok := newObj.(*v1alpha2.VirtualMachineIPAddress)
	if !ok {
		return nil, fmt.Errorf("expected a new VirtualMachineIP but got a %T", newObj)
	}

	v.log.Info("Validate VirtualMachineIP updating", "name", newVmip.Name,
		"old.type", oldVmip.Spec.Type, "new.type", newVmip.Spec.Type,
		"old.address", oldVmip.Spec.StaticIP, "new.address", newVmip.Spec.StaticIP,
	)

	boundCondition, exist := service.GetCondition(vmipcondition.BoundType.String(), oldVmip.Status.Conditions)
	if exist && boundCondition.Status == metav1.ConditionTrue {
		if oldVmip.Spec.Type == v1alpha2.VirtualMachineIPAddressTypeAuto && newVmip.Spec.Type == v1alpha2.VirtualMachineIPAddressTypeStatic {
			v.log.Info("Change the VirtualMachineIP address type to 'Auto' from 'Static' for ip: ", "address", newVmip.Spec.StaticIP)
			if newVmip.Spec.StaticIP != newVmip.Status.Address {
				return nil, fmt.Errorf("only type change Auto->Static is allowed: can't change current IP %s to the specified %s", newVmip.Status.Address, newVmip.Spec.StaticIP)
			}

			err := v.validateSpecFields(newVmip.Spec)
			if err != nil {
				return nil, fmt.Errorf("error validating VirtualMachineIP update: %w", err)
			}

			return nil, nil
		}

		if oldVmip.Spec.Type != newVmip.Spec.Type {
			return nil, errors.New("the VirtualMachineIPAddress is in 'Bound' state -> type cannot be changed")
		}

		if newVmip.Spec.Type == v1alpha2.VirtualMachineIPAddressTypeStatic && oldVmip.Spec.StaticIP != newVmip.Spec.StaticIP {
			return nil, errors.New("the VirtualMachineIPAddress is in 'Bound' state -> static IP cannot be changed")
		}
	}

	err := v.validateSpecFields(newVmip.Spec)
	if err != nil {
		return nil, fmt.Errorf("error validating VirtualMachineIP update: %w", err)
	}

	return nil, nil
}

func (v *Validator) ValidateDelete(_ context.Context, _ runtime.Object) (admission.Warnings, error) {
	err := fmt.Errorf("misconfigured webhook rules: delete operation not implemented")
	v.log.Error("Ensure the correctness of ValidatingWebhookConfiguration", "err", err)
	return nil, nil
}

func (v *Validator) validateSpecFields(spec v1alpha2.VirtualMachineIPAddressSpec) error {
	switch spec.Type {
	case v1alpha2.VirtualMachineIPAddressTypeStatic:
		if !isValidAddressFormat(spec.StaticIP) {
			return errors.New("the VirtualMachineIP address is not a valid textual representation of an IP address")
		}
	case v1alpha2.VirtualMachineIPAddressTypeAuto:
		if spec.StaticIP != "" {
			v.log.Error("Invalid combination: StaticIP is specified with Type 'Auto'")
		}
	default:
		return fmt.Errorf("invalid type for VirtualMachineIP: %s. Type must be either 'Static' or 'Auto'", spec.Type)
	}

	return nil
}

func isValidAddressFormat(address string) bool {
	return net.ParseIP(address) != nil
}
