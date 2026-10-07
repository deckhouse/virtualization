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

package moduleconfig

import (
	"context"
	"fmt"
	"net/netip"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	mcapi "github.com/deckhouse/virtualization-controller/pkg/controller/moduleconfig/api"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type removeCIDRsValidator struct {
	client client.Client
}

func newRemoveCIDRsValidator(client client.Client) *removeCIDRsValidator {
	return &removeCIDRsValidator{
		client: client,
	}
}

func (v removeCIDRsValidator) ValidateUpdate(ctx context.Context, oldMC, newMC *mcapi.ModuleConfig) (admission.Warnings, error) {
	oldCIDRs, err := ParseCIDRs(oldMC.Spec.Settings)
	if err != nil {
		return admission.Warnings{}, err
	}
	newCIDRs, err := ParseCIDRs(newMC.Spec.Settings)
	if err != nil {
		return admission.Warnings{}, err
	}

	var removedCIDRs []netip.Prefix

loop:
	for _, oldCIDR := range oldCIDRs {
		for _, newCIDR := range newCIDRs {
			if isEqualCIDRs(oldCIDR, newCIDR) {
				continue loop
			}
		}
		removedCIDRs = append(removedCIDRs, oldCIDR)
	}

	if len(removedCIDRs) == 0 {
		return nil, nil
	}

	vmips := &v1alpha2.VirtualMachineIPAddressList{}
	if err := v.client.List(ctx, vmips); err != nil {
		return nil, fmt.Errorf("failed to list VirtualMachineIPAddresses: %w", err)
	}

	if len(newCIDRs) == 0 {
		if len(vmips.Items) > 0 {
			return nil, fmt.Errorf("virtualMachineCIDRs cannot be cleared: %d VirtualMachineIPAddress resource(s) still exist", len(vmips.Items))
		}

		return nil, nil
	}

	for _, vmip := range vmips.Items {
		address := requestedIPAddress(vmip)
		if address == "" {
			continue
		}

		parsedAddress, err := netip.ParseAddr(address)
		if err != nil {
			continue
		}

		for _, CIDR := range removedCIDRs {
			if CIDR.Contains(parsedAddress) {
				return nil, fmt.Errorf("virtualMachineCIDRs item %q can't be removed: VirtualMachineIPAddress %s/%s holds the IP address %s from this network", CIDR, vmip.GetNamespace(), vmip.GetName(), address)
			}
		}
	}

	return nil, nil
}

func requestedIPAddress(vmip v1alpha2.VirtualMachineIPAddress) string {
	if vmip.Status.Address != "" {
		return vmip.Status.Address
	}

	return vmip.Spec.StaticIP
}
