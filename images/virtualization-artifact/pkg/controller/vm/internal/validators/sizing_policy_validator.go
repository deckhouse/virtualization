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

package validators

import (
	"context"
	"errors"
	"fmt"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type SizingPolicyValidator struct {
	client  client.Client
	service *service.SizePolicyService
}

func NewSizingPolicyValidator(client client.Client) *SizingPolicyValidator {
	return &SizingPolicyValidator{
		client:  client,
		service: service.NewSizePolicyService(),
	}
}

func (v *SizingPolicyValidator) ValidateCreate(ctx context.Context, vm *v1alpha2.VirtualMachine) (admission.Warnings, error) {
	return v.validate(ctx, vm)
}

func (v *SizingPolicyValidator) ValidateUpdate(ctx context.Context, _, newVM *v1alpha2.VirtualMachine) (admission.Warnings, error) {
	if newVM == nil || !newVM.GetDeletionTimestamp().IsZero() {
		return nil, nil
	}
	return v.validate(ctx, newVM)
}

func (v *SizingPolicyValidator) validate(ctx context.Context, vm *v1alpha2.VirtualMachine) (admission.Warnings, error) {
	var warnings admission.Warnings
	vmClass := &v1alpha2.VirtualMachineClass{}
	err := v.client.Get(ctx, types.NamespacedName{
		Name: vm.Spec.VirtualMachineClassName,
	}, vmClass)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			warnings = append(warnings, fmt.Sprintf(
				"The VM class %q does not exist; it may not have been created yet. Until it is created, the virtual machine will remain in a pending status.",
				vm.Spec.VirtualMachineClassName,
			))
			return warnings, nil
		} else {
			log := logger.FromContext(ctx)
			log.Error(
				"An error occurred while retrieving the VM class.",
				logger.SlogErr(err),
			)
			return nil, err
		}
	}

	err = v.service.CheckVMMatchedSizePolicy(vm, vmClass)
	// If the VM is being deleted (DeletionTimestamp is set), we allow the deletion to proceed
	// even if no sizing policy matches, to prevent the VM from being stuck in Terminating
	// due to an outdated or invalid VMClass.
	var noSizingErr *service.NoSizingPolicyMatchError
	if errors.As(err, &noSizingErr) && vm.DeletionTimestamp != nil {
		return nil, nil
	}

	return nil, err
}
