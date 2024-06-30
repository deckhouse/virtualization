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

package vmiplease

import (
	"context"
	"fmt"
	"net"
	"strings"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

func NewVMIPLeaseValidator(log logr.Logger) *VMIPLease {
	return &VMIPLease{log: log.WithName(controllerName).WithValues("webhook", "validation")}
}

type VMIPLease struct {
	log logr.Logger
}

func (v *VMIPLease) ValidateCreate(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	lease, ok := obj.(*v1alpha2.VirtualMachineIPAddressLease)
	if !ok {
		return nil, fmt.Errorf("expected a new VirtualMachineIPLease but got a %T", obj)
	}

	v.log.Info("Validate VMIPLease creating", "name", lease.Name)

	if !isValidAddressFormat(leaseNameToIP(lease.Name)) {
		return nil, fmt.Errorf("the lease address is not a valid textual representation of an IP address")
	}

	return nil, nil
}

func (v *VMIPLease) ValidateUpdate(_ context.Context, _, _ runtime.Object) (admission.Warnings, error) {
	err := fmt.Errorf("misconfigured webhook rules: update operation not implemented")
	v.log.Error(err, "Ensure the correctness of ValidatingWebhookConfiguration")
	return nil, nil
}

func (v *VMIPLease) ValidateDelete(_ context.Context, _ runtime.Object) (admission.Warnings, error) {
	err := fmt.Errorf("misconfigured webhook rules: delete operation not implemented")
	v.log.Error(err, "Ensure the correctness of ValidatingWebhookConfiguration")
	return nil, nil
}

func isValidAddressFormat(address string) bool {
	return net.ParseIP(address) != nil
}

func leaseNameToIP(leaseName string) string {
	const ipPrefix = "ip-"
	if strings.HasPrefix(leaseName, ipPrefix) && len(leaseName) > len(ipPrefix) {
		return strings.ReplaceAll(leaseName[len(ipPrefix):], "-", ".")
	}

	return ""
}
