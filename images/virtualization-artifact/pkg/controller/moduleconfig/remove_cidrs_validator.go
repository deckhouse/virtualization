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

	"github.com/deckhouse/virtualization-controller/pkg/common/ip"
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

	var validateCIDRs []netip.Prefix

loop:
	for _, oldCIDR := range oldCIDRs {
		for _, newCIDR := range newCIDRs {
			if isEqualCIDRs(oldCIDR, newCIDR) {
				continue loop
			}
		}
		validateCIDRs = append(validateCIDRs, oldCIDR)
	}

	if len(validateCIDRs) == 0 {
		return nil, nil
	}

	leases := &v1alpha2.VirtualMachineIPAddressLeaseList{}
	if err := v.client.List(ctx, leases); err != nil {
		return nil, fmt.Errorf("failed to list VirtualMachineIPAddressLeases: %w", err)
	}

	for _, lease := range leases.Items {
		leaseIP, err := netip.ParseAddr(ip.LeaseNameToIP(lease.Name))
		if err != nil {
			continue
		}
		for _, CIDR := range validateCIDRs {
			if CIDR.Contains(leaseIP) {
				return nil, fmt.Errorf("virtualMachineCIDRs item %q can't be removed: VirtualMachineIPAddressLease/%s holds IP address from this network", CIDR, lease.GetName())
			}
		}
	}

	return nil, nil
}
