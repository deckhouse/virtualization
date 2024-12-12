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

	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/deckhouse/virtualization-controller/pkg/common/ip"
	mcapi "github.com/deckhouse/virtualization-controller/pkg/controller/moduleconfig/api"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

const virtualMachineCIDRs = "virtualMachineCIDRs"
const moduleConfigName = "virtualization"

func SetupWebhookWithManager(mgr manager.Manager) error {
	if err := builder.WebhookManagedBy(mgr).
		For(&mcapi.ModuleConfig{}).
		WithValidator(NewValidator(mgr.GetClient())).
		Complete(); err != nil {
		return err
	}

	return nil
}

type Validator struct {
	client client.Client
}

func NewValidator(client client.Client) *Validator {
	return &Validator{
		client: client,
	}
}

func (v *Validator) ValidateCreate(ctx context.Context, _ runtime.Object) (admission.Warnings, error) {
	err := fmt.Errorf("misconfigured webhook rules: create operation not implemented")
	logger.FromContext(ctx).Error("Ensure the correctness of ValidatingWebhookConfiguration", "err", err)
	return nil, nil
}

func (v *Validator) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	oldMC, ok := oldObj.(*mcapi.ModuleConfig)
	if !ok {
		return nil, fmt.Errorf("expected an old ModuleConfig but got a %T", newObj)
	}

	newMC, ok := newObj.(*mcapi.ModuleConfig)
	if !ok {
		return nil, fmt.Errorf("expected a new ModuleConfig but got a %T", newObj)
	}

	if newMC.GetName() != moduleConfigName {
		return nil, nil
	}

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

func (v *Validator) ValidateDelete(ctx context.Context, _ runtime.Object) (admission.Warnings, error) {
	err := fmt.Errorf("misconfigured webhook rules: delete operation not implemented")
	logger.FromContext(ctx).Error("Ensure the correctness of ValidatingWebhookConfiguration", "err", err)
	return nil, nil
}
