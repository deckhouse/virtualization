package ipam

import (
	"context"
	"errors"
	"fmt"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

func NewClaimValidator(log logr.Logger, client client.Client) *ClaimValidator {
	return &ClaimValidator{log: log.WithName(claimControllerName).WithValues("webhook", "validation"), client: client}
}

type ClaimValidator struct {
	log    logr.Logger
	client client.Client
}

func (v *ClaimValidator) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	claim, ok := obj.(*v1alpha2.VirtualMachineIPAddressClaim)
	if !ok {
		return nil, fmt.Errorf("expected a new VirtualMachineIPAddressClaim but got a %T", obj)
	}

	v.log.Info("Validate Claim creating", "name", claim.Name, "address", claim.Spec.Address, "leaseName", claim.Spec.LeaseName)

	err := v.validateSpecFields(claim.Spec)
	if err != nil {
		return nil, fmt.Errorf("error validating claim creation: %w", err)
	}

	ip := claim.Spec.Address
	if ip == "" {
		ip = leaseNameToIP(claim.Spec.LeaseName)
	}

	if ip != "" {
		allocatedIPs, err := getAllocatedIPs(ctx, v.client)
		if err != nil {
			return nil, fmt.Errorf("error getting allocated ips: %w", err)
		}

		allocatedLease, ok := allocatedIPs[ip]
		if ok && allocatedLease.Spec.ClaimRef != nil && (allocatedLease.Spec.ClaimRef.Namespace != claim.Namespace || allocatedLease.Spec.ClaimRef.Name != claim.Name) {
			return nil, fmt.Errorf("claim cannot be created: the address %s has already been allocated for another claim", ip)
		}
	}

	return nil, nil
}

func (v *ClaimValidator) ValidateUpdate(_ context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	oldClaim, ok := oldObj.(*v1alpha2.VirtualMachineIPAddressClaim)
	if !ok {
		return nil, fmt.Errorf("expected a old VirtualMachineIPAddressClaim but got a %T", oldObj)
	}

	newClaim, ok := newObj.(*v1alpha2.VirtualMachineIPAddressClaim)
	if !ok {
		return nil, fmt.Errorf("expected a new VirtualMachineIPAddressClaim but got a %T", newObj)
	}

	v.log.Info("Validate Claim updating", "name", newClaim.Name,
		"old.address", oldClaim.Spec.Address, "new.address", newClaim.Spec.Address,
		"old.leaseName", oldClaim.Spec.LeaseName, "new.leaseName", newClaim.Spec.LeaseName,
	)

	if oldClaim.Spec.Address != "" && oldClaim.Spec.Address != newClaim.Spec.Address {
		return nil, errors.New("the claim address cannot be changed if allocated")
	}

	if oldClaim.Spec.LeaseName != "" && oldClaim.Spec.LeaseName != newClaim.Spec.LeaseName {
		return nil, errors.New("the lease name cannot be changed if allocated")
	}

	err := v.validateSpecFields(newClaim.Spec)
	if err != nil {
		return nil, fmt.Errorf("error validating claim updating: %w", err)
	}

	return nil, nil
}

func (v *ClaimValidator) ValidateDelete(_ context.Context, _ runtime.Object) (admission.Warnings, error) {
	err := fmt.Errorf("misconfigured webhook rules: delete operation not implemented")
	v.log.Error(err, "Ensure the correctness of ValidatingWebhookConfiguration")
	return nil, nil
}

func (v *ClaimValidator) validateSpecFields(spec v1alpha2.VirtualMachineIPAddressClaimSpec) error {
	if spec.LeaseName != "" && !isValidAddressFormat(leaseNameToIP(spec.LeaseName)) {
		return errors.New("the lease name is not created from a valid IP address or ip prefix is missing")
	}

	if spec.Address != "" && !isValidAddressFormat(spec.Address) {
		return errors.New("the claim address is not a valid textual representation of an IP address")
	}

	if spec.Address != "" && spec.LeaseName != "" && spec.Address != leaseNameToIP(spec.LeaseName) {
		return errors.New("lease name doesn't match the address")
	}

	return nil
}
