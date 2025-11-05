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

	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
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
	return &ProvisioningHandler{client: client, validator: newProvisioningValidator(client)}
}

type ProvisioningHandler struct {
	client    client.Client
	validator *provisioningValidator
}

func (h *ProvisioningHandler) Handle(ctx context.Context, s state.VirtualMachineState) (reconcile.Result, error) {
	if s.VirtualMachine().IsEmpty() {
		return reconcile.Result{}, nil
	}

	current := s.VirtualMachine().Current()
	changed := s.VirtualMachine().Changed()

	if update := addAllUnknown(changed, vmcondition.TypeProvisioningReady); update {
		return reconcile.Result{}, nil
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
		if p.UserData != "" {
			cb.Status(metav1.ConditionTrue).Reason(vmcondition.ReasonProvisioningReady)
		} else {
			cb.Status(metav1.ConditionFalse).
				Reason(vmcondition.ReasonProvisioningNotReady).
				Message("Provisioning is defined but it is empty.")
		}
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
	err := h.validator.Validate(ctx, secretKey)

	switch {
	case err == nil:
		builder.Reason(vmcondition.ReasonProvisioningReady).Status(metav1.ConditionTrue)
		return nil
	case errors.As(err, new(secretNotFoundError)):
		builder.Status(metav1.ConditionFalse).
			Reason(vmcondition.ReasonProvisioningNotReady).
			Message(service.CapitalizeFirstLetter(err.Error()))
		return nil

	case errors.Is(err, errSecretIsNotValid):
		builder.Status(metav1.ConditionFalse).
			Reason(vmcondition.ReasonProvisioningNotReady).
			Message(fmt.Sprintf("Invalid secret %q: %s", secretKey.String(), err.Error()))
		return nil

	case errors.As(err, new(unexpectedSecretTypeError)):
		builder.Status(metav1.ConditionFalse).
			Reason(vmcondition.ReasonProvisioningNotReady).
			Message(service.CapitalizeFirstLetter(err.Error()))
		return nil

	default:
		return err
	}
}

var errSecretIsNotValid = errors.New("secret is not valid")

type secretNotFoundError string

func (e secretNotFoundError) Error() string {
	return fmt.Sprintf("secret %s not found", string(e))
}

type unexpectedSecretTypeError string

func (e unexpectedSecretTypeError) Error() string {
	return fmt.Sprintf("unexpected secret type: %s", string(e))
}

var cloudInitCheckKeys = []string{
	"userdata",
	"userData",
}

func newProvisioningValidator(reader client.Reader) *provisioningValidator {
	return &provisioningValidator{
		reader: reader,
	}
}

type provisioningValidator struct {
	reader client.Reader
}

func (v provisioningValidator) Validate(ctx context.Context, key types.NamespacedName) error {
	secret := &corev1.Secret{}
	err := v.reader.Get(ctx, key, secret)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return secretNotFoundError(key.String())
		}
		return err
	}
	switch secret.Type {
	case v1alpha2.SecretTypeCloudInit:
		return v.validateCloudInitSecret(secret)
	case v1alpha2.SecretTypeSysprep:
		return v.validateSysprepSecret(secret)
	default:
		return unexpectedSecretTypeError(secret.Type)
	}
}

func (v provisioningValidator) validateCloudInitSecret(secret *corev1.Secret) error {
	if !v.hasOneOfKeys(secret, cloudInitCheckKeys...) {
		return fmt.Errorf("the secret should have one of data fields %v: %w", cloudInitCheckKeys, errSecretIsNotValid)
	}
	return nil
}

func (v provisioningValidator) validateSysprepSecret(_ *corev1.Secret) error {
	return nil
}

func (v provisioningValidator) hasOneOfKeys(secret *corev1.Secret, checkKeys ...string) bool {
	validate := len(checkKeys) == 0
	for _, key := range checkKeys {
		if _, ok := secret.Data[key]; ok {
			validate = true
			break
		}
	}
	return validate
}
