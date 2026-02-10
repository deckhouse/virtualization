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

package internal

import (
	"context"
	"errors"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vm/internal/state"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
)

const nameProvisioningHandler = "ProvisioningHandler"

func NewProvisioningHandler(client client.Client) *ProvisioningHandler {
	return &ProvisioningHandler{
		provisioningService: service.NewProvisioningService(client),
	}
}

type ProvisioningHandler struct {
	provisioningService *service.ProvisioningService
}

func (h *ProvisioningHandler) Handle(ctx context.Context, s state.VirtualMachineState) (reconcile.Result, error) {
	if s.VirtualMachine().IsEmpty() {
		return reconcile.Result{}, nil
	}

	current := s.VirtualMachine().Current()
	changed := s.VirtualMachine().Changed()

	if update := addAllUnknown(changed, vmcondition.TypeProvisioningReady); update {
		return reconcile.Result{Requeue: true}, nil
	}

	if isDeletion(current) {
		return reconcile.Result{}, nil
	}

	cb := conditions.NewConditionBuilder(vmcondition.TypeProvisioningReady).
		Generation(current.GetGeneration())

	if current.Spec.Provisioning == nil {
		conditions.SetCondition(cb.Status(metav1.ConditionTrue).
			Reason(vmcondition.ReasonProvisioningReady), &changed.Status.Conditions)
		return reconcile.Result{}, nil
	}
	p := current.Spec.Provisioning
	switch p.Type {
	case v1alpha2.ProvisioningTypeUserData:
		err := h.provisioningService.ValidateUserDataLen(p.UserData)
		if err != nil {
			errMsg := fmt.Errorf("failed to validate userdata length: %w", err)
			cb.Status(metav1.ConditionFalse).
				Reason(vmcondition.ReasonProvisioningNotReady).
				Message(service.CapitalizeFirstLetter(errMsg.Error() + "."))
			return reconcile.Result{}, errMsg
		}
		cb.Status(metav1.ConditionTrue).Reason(vmcondition.ReasonProvisioningReady)
	case v1alpha2.ProvisioningTypeUserDataRef:
		if p.UserDataRef == nil || p.UserDataRef.Kind != v1alpha2.UserDataRefKindSecret {
			cb.Status(metav1.ConditionFalse).
				Reason(vmcondition.ReasonProvisioningNotReady).
				Message(fmt.Sprintf("userdataRef must be %q", v1alpha2.UserDataRefKindSecret))
		}
		key := types.NamespacedName{Name: p.UserDataRef.Name, Namespace: current.GetNamespace()}
		err := h.genConditionFromSecret(ctx, cb, key)
		if err != nil {
			return reconcile.Result{}, err
		}

	case v1alpha2.ProvisioningTypeSysprepRef:
		if p.SysprepRef == nil || p.SysprepRef.Kind != v1alpha2.SysprepRefKindSecret {
			cb.Status(metav1.ConditionFalse).
				Reason(vmcondition.ReasonProvisioningNotReady).
				Message(fmt.Sprintf("sysprepRef must be %q", v1alpha2.SysprepRefKindSecret))
		}
		key := types.NamespacedName{Name: p.SysprepRef.Name, Namespace: current.GetNamespace()}
		err := h.genConditionFromSecret(ctx, cb, key)
		if err != nil {
			return reconcile.Result{}, err
		}
	default:
		cb.Status(metav1.ConditionFalse).
			Reason(vmcondition.ReasonProvisioningNotReady).
			Message("Unexpected provisioning type.")
	}

	conditions.SetCondition(cb, &changed.Status.Conditions)

	return reconcile.Result{}, nil
}

func (h *ProvisioningHandler) Name() string {
	return nameProvisioningHandler
}

func (h *ProvisioningHandler) genConditionFromSecret(ctx context.Context, builder *conditions.ConditionBuilder, secretKey types.NamespacedName) error {
	err := h.provisioningService.Validate(ctx, secretKey)

	switch {
	case err == nil:
		builder.Reason(vmcondition.ReasonProvisioningReady).Status(metav1.ConditionTrue)
		return nil
	case errors.As(err, new(service.SecretNotFoundError)):
		builder.Status(metav1.ConditionFalse).
			Reason(vmcondition.ReasonProvisioningNotReady).
			Message(service.CapitalizeFirstLetter(err.Error()))
		return nil

	case errors.Is(err, service.ErrSecretIsNotValid):
		builder.Status(metav1.ConditionFalse).
			Reason(vmcondition.ReasonProvisioningNotReady).
			Message(fmt.Sprintf("Invalid secret %q: %s", secretKey.String(), err.Error()))
		return nil

	case errors.As(err, new(service.UnexpectedSecretTypeError)):
		builder.Status(metav1.ConditionFalse).
			Reason(vmcondition.ReasonProvisioningNotReady).
			Message(service.CapitalizeFirstLetter(err.Error()))
		return nil

	default:
		return err
	}
}
