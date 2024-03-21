package ipam

import (
	"context"
	"fmt"
	"net"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

func NewLeaseValidator(log logr.Logger) *LeaseValidator {
	return &LeaseValidator{log: log.WithName(leaseControllerName).WithValues("webhook", "validation")}
}

type LeaseValidator struct {
	log logr.Logger
}

func (v *LeaseValidator) ValidateCreate(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	lease, ok := obj.(*v1alpha2.VirtualMachineIPAddressLease)
	if !ok {
		return nil, fmt.Errorf("expected a new VirtualMachineIPAddressLease but got a %T", obj)
	}

	v.log.Info("Validate Lease creating", "name", lease.Name)

	if !isValidAddressFormat(leaseNameToIP(lease.Name)) {
		return nil, fmt.Errorf("the lease address is not a valid textual representation of an IP address")
	}

	return nil, nil
}

func (v *LeaseValidator) ValidateUpdate(_ context.Context, _, _ runtime.Object) (admission.Warnings, error) {
	err := fmt.Errorf("misconfigured webhook rools: update operation not implemented")
	v.log.Error(err, "Ensure the correctness of ValidatingWebhookConfiguration")
	return nil, nil
}

func (v *LeaseValidator) ValidateDelete(_ context.Context, _ runtime.Object) (admission.Warnings, error) {
	err := fmt.Errorf("misconfigured webhook rools: delete operation not implemented")
	v.log.Error(err, "Ensure the correctness of ValidatingWebhookConfiguration")
	return nil, nil
}

func isValidAddressFormat(address string) bool {
	return net.ParseIP(address) != nil
}
