package moduleconfig

import (
	"context"
	"fmt"
	"net/netip"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/deckhouse/virtualization-controller/pkg/common/ip"
	mcapi "github.com/deckhouse/virtualization-controller/pkg/controller/moduleconfig/api"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

const virtualMachineCIDRs = "virtualMachineCIDRs"

type reduceCIDRsValidator struct {
	client client.Client
}

func newReduceCIDRsValidator(client client.Client) *reduceCIDRsValidator {
	return &reduceCIDRsValidator{
		client: client,
	}
}

func (v *reduceCIDRsValidator) ValidateUpdate(ctx context.Context, oldMC, newMC *mcapi.ModuleConfig) (admission.Warnings, error) {

	oldCIDRs := oldMC.Spec.Settings[virtualMachineCIDRs].([]string)
	newCIDRs := newMC.Spec.Settings[virtualMachineCIDRs].([]string)

	var validateCIDRs []string

loop:
	for _, oldCIDR := range oldCIDRs {
		for _, newCIDR := range newCIDRs {
			if oldCIDR == newCIDR {
				continue loop
			}
		}
		validateCIDRs = append(validateCIDRs, oldCIDR)
	}

	if len(validateCIDRs) != 0 {
		return nil, nil
	}

	leases := &virtv2.VirtualMachineIPAddressLeaseList{}
	if err := v.client.List(ctx, leases); err != nil {
		return nil, fmt.Errorf("failed to list VirtualMachineIPAddressLeases: %w", err)
	}

	parseCIDRs := make([]netip.Prefix, len(validateCIDRs))
	for i, validateCIDR := range validateCIDRs {
		parsedCIDR, err := netip.ParsePrefix(validateCIDR)
		if err != nil {
			return nil, fmt.Errorf("failed to parse CIDR %s: %w", validateCIDR, err)
		}
		parseCIDRs[i] = parsedCIDR
	}

	for _, lease := range leases.Items {
		leaseIP, err := netip.ParseAddr(ip.LeaseNameToIP(lease.Name))
		if err != nil {
			return nil, fmt.Errorf("failed to parse lease ip: %w", err)
		}
		for _, CIDR := range parseCIDRs {
			if CIDR.Contains(leaseIP) {
				return nil, fmt.Errorf("CIDR %q is in use by one or more IP addresses", CIDR)
			}
		}
	}

	return nil, nil
}
