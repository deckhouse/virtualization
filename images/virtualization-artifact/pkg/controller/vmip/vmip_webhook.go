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
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/deckhouse/deckhouse/pkg/log"

	"github.com/deckhouse/virtualization-controller/pkg/common/ip"
	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmip/internal/service"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmipcondition"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmiplcondition"
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

	v.log.Info("Validate VirtualMachineIPAddress creating", "name", vmip.Name, "type", vmip.Spec.Type, "address", vmip.Spec.StaticIP)

	err := v.validateSpecFields(vmip)
	if err != nil {
		return nil, fmt.Errorf("the VirtualMachineIPAddress validation is failed: %w", err)
	}

	var warnings admission.Warnings

	if vmip.Spec.StaticIP != "" {
		err = v.validateAllocatedIPAddresses(ctx, vmip.Spec.StaticIP)
		switch {
		case err == nil:
			// OK.
		case errors.Is(err, service.ErrIPAddressOutOfRange):
			warnings = append(warnings, fmt.Sprintf("The requested address %s is out of the valid range", vmip.Spec.StaticIP))
		default:
			return nil, err
		}
	}

	return nil, nil
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

	err := v.validateSpecFields(newVmip)
	if err != nil {
		return nil, fmt.Errorf("error validating VirtualMachineIP update: %w", err)
	}

	boundCondition, _ := conditions.GetCondition(vmipcondition.BoundType, oldVmip.Status.Conditions)
	if boundCondition.Status == metav1.ConditionTrue {
		if oldVmip.Spec.Type == v1alpha2.VirtualMachineIPAddressTypeAuto && newVmip.Spec.Type == v1alpha2.VirtualMachineIPAddressTypeStatic {
			v.log.Info("Change the VirtualMachineIP address type to 'Auto' from 'Static'", "ipAddress", newVmip.Spec.StaticIP)
			if newVmip.Spec.StaticIP != newVmip.Status.Address {
				return nil, fmt.Errorf("only type change Auto->Static is allowed: can't change current IP %s to the specified %s", newVmip.Status.Address, newVmip.Spec.StaticIP)
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

	return nil, nil
}

func (v *Validator) ValidateDelete(_ context.Context, _ runtime.Object) (admission.Warnings, error) {
	err := fmt.Errorf("misconfigured webhook rules: delete operation not implemented")
	v.log.Error("Ensure the correctness of ValidatingWebhookConfiguration", "err", err)
	return nil, nil
}

func (v *Validator) validateSpecFields(vmip *v1alpha2.VirtualMachineIPAddress) error {
	switch vmip.Spec.Type {
	case v1alpha2.VirtualMachineIPAddressTypeStatic:
		if vmip.Spec.StaticIP == "" {
			return errors.New("the 'Static IP' field should be set for the IP address with the 'Static' type")
		}

		if !isValidAddressFormat(vmip.Spec.StaticIP) {
			return fmt.Errorf("the specified static IP address %q is not a valid textual representation of an IP address", vmip.Spec.StaticIP)
		}
	case v1alpha2.VirtualMachineIPAddressTypeAuto:
		if vmip.Spec.StaticIP != "" {
			return fmt.Errorf("the VirtualMachineIPAddress cannot be created: The 'Static IP' field is set for the %s IP address with the 'Auto' type", vmip.Name)
		}
	default:
		return fmt.Errorf("invalid type for VirtualMachineIP: %s. Type must be either 'Static' or 'Auto'", vmip.Spec.Type)
	}

	return nil
}

func (v *Validator) validateAllocatedIPAddresses(ctx context.Context, ipAddress string) error {
	err := v.ipService.IsInsideOfRange(ipAddress)
	if err != nil {
		return fmt.Errorf("failed to check availability of address: %w", err)
	}

	allocatedIPs, err := v.ipService.GetAllocatedIPs(ctx)
	if err != nil {
		return fmt.Errorf("failed to get allocated IP addresses: %w", err)
	}

	_, ok := allocatedIPs[ipAddress]
	if ok {
		var lease *v1alpha2.VirtualMachineIPAddressLease
		lease, err = object.FetchObject(ctx, types.NamespacedName{Name: ip.IpToLeaseName(ipAddress)}, v.client, &v1alpha2.VirtualMachineIPAddressLease{})
		if err != nil {
			return fmt.Errorf("failed to fetch allocated IP address: %w", err)
		}

		if lease != nil {
			boundCondition, _ := conditions.GetCondition(vmiplcondition.BoundType, lease.Status.Conditions)
			if boundCondition.Reason != vmiplcondition.Released.String() {
				return fmt.Errorf("the VirtualMachineIPAddress cannot be created: the specified IP address %s has already been allocated and has not been released", ipAddress)
			}
		}
	}

	return nil
}

func isValidAddressFormat(address string) bool {
	return net.ParseIP(address) != nil
}
